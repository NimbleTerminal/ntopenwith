package ntopenwith

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestCommand(t *testing.T) {
	// p makes an absolute path on this OS: on Windows it needs a drive.
	p := func(s string) string {
		if runtime.GOOS == "windows" {
			return `C:` + filepath.FromSlash(s)
		}
		return s
	}
	cases := []struct {
		name string
		argv []string
		path string
		want []string
	}{
		{"appended", []string{"open", "-a", "/Applications/X.app"}, p("/p/a b.png"),
			[]string{"open", "-a", "/Applications/X.app", p("/p/a b.png")}},
		{"replaced in place", []string{"code", "--wait", "{}", "--new"}, p("/p/a.txt"),
			[]string{"code", "--wait", p("/p/a.txt"), "--new"}},
		{"replaced twice", []string{"diff", "{}", "{}"}, p("/p/a.txt"),
			[]string{"diff", p("/p/a.txt"), p("/p/a.txt")}},
		{"program alone", []string{"viewer"}, p("/p/a.txt"),
			[]string{"viewer", p("/p/a.txt")}},
	}
	for _, c := range cases {
		orig := append([]string(nil), c.argv...)
		got, err := Command(c.argv, c.path)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
		if !reflect.DeepEqual(c.argv, orig) {
			t.Errorf("%s: argv modified: %q, was %q", c.name, c.argv, orig)
		}
	}

	bad := []struct {
		name string
		argv []string
		path string
	}{
		{"placeholder inside an argument", []string{"x", "a{}b"}, "/p/a"},
		{"empty program", []string{"", "x"}, "/p/a"},
		{"placeholder as program", []string{"{}"}, "/p/a"},
		{"placeholder inside program", []string{"a{}b"}, "/p/a"},
		{"no argv", nil, "/p/a"},
		{"relative path", []string{"x"}, "a.txt"},
		{"empty path", []string{"x"}, ""},
		{"NUL in path", []string{"x"}, "/p/a\x00b"},
	}
	for _, c := range bad {
		if got, err := Command(c.argv, c.path); err == nil {
			t.Errorf("%s: got %q, want an error", c.name, got)
		}
	}
}

func TestStartReportsFailure(t *testing.T) {
	if err := Start(nil); err == nil {
		t.Error("Start(nil) succeeded")
	}
	if err := Start([]string{filepath.Join(t.TempDir(), "no-such-program")}); err == nil {
		t.Error("Start of a missing program succeeded")
	}
}

func TestStartUsesHook(t *testing.T) {
	var got []string
	want := errors.New("refused")
	old := startCommand
	t.Cleanup(func() { startCommand = old })
	startCommand = func(c *exec.Cmd) error {
		got = c.Args
		return want
	}
	argv := []string{"app", "--flag", "/p/a.txt"}
	if err := Start(argv); err != want {
		t.Errorf("Start = %v, want %v", err, want)
	}
	if !reflect.DeepEqual(got, argv) {
		t.Errorf("hook saw %q, want %q", got, argv)
	}
}

// TestHelperRecordsArgs is not a test: when GO_NTOPENWITH_HELPER=1 this test
// binary, started by TestLaunchPassesPathAsOneArgument, records its arguments.
func TestHelperRecordsArgs(t *testing.T) {
	if os.Getenv("GO_NTOPENWITH_HELPER") != "1" {
		return
	}
	data, err := json.Marshal(os.Args[1:])
	if err != nil {
		os.Exit(2)
	}
	out := os.Getenv("GO_NTOPENWITH_OUT")
	tmp := out + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		os.Exit(2)
	}
	if err := os.Rename(tmp, out); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestLaunchPassesPathAsOneArgument(t *testing.T) {
	names := []string{"a b.png", `q"uote's.txt`, "$HOME.txt", "-dash.txt", "naïve 文件.md"}
	dir := t.TempDir()
	t.Setenv("GO_NTOPENWITH_HELPER", "1")
	for i, name := range names {
		if runtime.GOOS == "windows" && name == `q"uote's.txt` {
			continue // a Windows file name cannot hold a double quote
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(dir, "out"+string(rune('0'+i))+".json")
		t.Setenv("GO_NTOPENWITH_OUT", out)

		argv, err := Command([]string{os.Args[0], "-test.run=^TestHelperRecordsArgs$", "--", "{}"}, path)
		if err != nil {
			t.Fatalf("%q: %v", name, err)
		}
		if err := Start(argv); err != nil {
			t.Fatalf("%q: Start: %v", name, err)
		}
		var data []byte
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if data, err = os.ReadFile(out); err == nil {
				break
			}
		}
		if err != nil {
			t.Fatalf("%q: no output from the helper: %v", name, err)
		}
		var args []string
		if err := json.Unmarshal(data, &args); err != nil {
			t.Fatalf("%q: %v", name, err)
		}
		want := []string{"-test.run=^TestHelperRecordsArgs$", "--", path}
		if !reflect.DeepEqual(args, want) {
			t.Errorf("%q: helper got %q, want %q", name, args, want)
		}
	}
}

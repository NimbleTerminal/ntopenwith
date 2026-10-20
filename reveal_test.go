package ntopenwith

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// wantReveal is the argv Reveal must start on this OS, or nil when the OS has
// no file manager to ask.
func wantReveal(file, dir string, isDir bool) []string {
	switch runtime.GOOS {
	case "darwin":
		if isDir {
			return []string{"open", dir}
		}
		return []string{"open", "-R", file}
	case "windows":
		if isDir {
			return []string{"explorer.exe", dir}
		}
		return []string{"explorer.exe", "/select," + file}
	case "linux", "freebsd", "openbsd", "netbsd":
		return []string{"xdg-open", dir}
	}
	return nil
}

func TestRevealArgs(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var got []string
	old := startCommand
	t.Cleanup(func() { startCommand = old })
	startCommand = func(c *exec.Cmd) error {
		got = c.Args
		return nil
	}
	for _, tc := range []struct {
		name  string
		path  string
		isDir bool
	}{{"file", file, false}, {"directory", dir, true}} {
		t.Run(tc.name, func(t *testing.T) {
			got = nil
			err := Reveal(tc.path)
			want := wantReveal(file, dir, tc.isDir)
			if want == nil {
				if !errors.Is(err, ErrUnsupported) {
					t.Fatalf("Reveal = %v, want ErrUnsupported", err)
				}
				if got != nil {
					t.Fatalf("started %q on an unsupported OS", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("argv = %q, want %q", got, want)
			}
		})
	}
}

func TestRevealRejectsBadPath(t *testing.T) {
	if wantReveal("", "", false) == nil {
		t.Skip("no file manager on this OS")
	}
	old := startCommand
	t.Cleanup(func() { startCommand = old })
	startCommand = func(c *exec.Cmd) error {
		t.Errorf("started %q for a bad path", c.Args)
		return nil
	}
	for _, p := range []string{"", filepath.Join(t.TempDir(), "missing"), "a\x00b"} {
		if err := Reveal(p); err == nil {
			t.Errorf("Reveal(%q) = nil, want an error", p)
		}
	}
}

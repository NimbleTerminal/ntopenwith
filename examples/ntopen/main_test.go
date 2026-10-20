package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"nimbleterminal.dev/ntopenwith"
)

// fake offers fixed apps and records what it was asked to launch.
type fake struct {
	apps     map[ntopenwith.Verb][]ntopenwith.App
	targets  []ntopenwith.Target
	launched []string
}

func (f *fake) Apps(_ context.Context, t ntopenwith.Target, v ntopenwith.Verb) ([]ntopenwith.App, error) {
	f.targets = append(f.targets, t)
	return append([]ntopenwith.App(nil), f.apps[v]...), nil
}

func (f *fake) Launch(_ context.Context, t ntopenwith.Target, v ntopenwith.Verb, app ntopenwith.App) (*exec.Cmd, error) {
	f.launched = append(f.launched, string(v)+" "+app.ID+" "+t.Path)
	return nil, nil
}

func use(t *testing.T, f *fake) {
	t.Helper()
	old := source
	source = func() ntopenwith.Source { return f }
	t.Cleanup(func() { source = old })
}

func file(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func twoVerbs() *fake {
	return &fake{apps: map[ntopenwith.Verb][]ntopenwith.App{
		ntopenwith.Open: {
			{ID: "mac:one", Name: "One", Default: true},
			{ID: "system", Name: "System default"},
		},
		ntopenwith.Edit: {
			{ID: "mac:two", Name: "Two"},
			{ID: "nano", Name: "nano", Terminal: true},
		},
	}}
}

func TestListJSON(t *testing.T) {
	use(t, twoVerbs())
	var out, errs bytes.Buffer
	if code := run([]string{"list", "--json", file(t, "a.txt", "hello\n")}, &out, &errs); code != 0 {
		t.Fatalf("exit %d: %s", code, errs.String())
	}
	var got []entry
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("%v in %s", err, out.String())
	}
	want := []entry{
		{ntopenwith.Open, "mac:one", "One", true, false},
		{ntopenwith.Open, "system", "System default", false, false},
		{ntopenwith.Edit, "mac:two", "Two", false, false},
		{ntopenwith.Edit, "nano", "nano", false, true},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
	// The JSON uses the documented field names.
	for _, field := range []string{`"verb"`, `"id"`, `"name"`, `"default"`, `"terminal"`} {
		if !strings.Contains(out.String(), field) {
			t.Errorf("no %s in %s", field, out.String())
		}
	}
}

func TestListTextMarksTheDefault(t *testing.T) {
	use(t, twoVerbs())
	var out bytes.Buffer
	// Flags may follow the file.
	if code := run([]string{file(t, "a.txt", "hello\n")}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := "open:\n  * mac:one  One\n    system  System default\n\nedit:\n    mac:two  Two\n    nano  nano  (terminal)\n"
	if out.String() != want {
		t.Errorf("got\n%s\nwant\n%s", out.String(), want)
	}
}

func TestListEmptyVerbs(t *testing.T) {
	use(t, &fake{})
	file := file(t, "a.txt", "x")
	var out bytes.Buffer
	if code := run([]string{"list", file, "--json"}, &out, &bytes.Buffer{}); code != 0 || strings.TrimSpace(out.String()) != "[]" {
		t.Errorf("exit %d, got %q, want []", code, out.String())
	}
	out.Reset()
	run([]string{"list", file}, &out, &bytes.Buffer{})
	if out.String() != "open:\n  (none)\n\nedit:\n  (none)\n" {
		t.Errorf("got %q", out.String())
	}
}

func TestTextDetection(t *testing.T) {
	f := twoVerbs()
	use(t, f)
	for _, tc := range []struct {
		name, content string
		flag          []string
		text          bool
	}{
		{"a.txt", "plain\n", nil, true},
		{"b.bin", "ab\x00cd", nil, false},
		{"c.bin", "ab\x00cd", []string{"--text"}, true},
		{"d.txt", "\xff\xfe", nil, false},
	} {
		f.targets = nil
		args := append([]string{"list", file(t, tc.name, tc.content)}, tc.flag...)
		if code := run(args, &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
			t.Fatalf("%s: exit %d", tc.name, code)
		}
		if len(f.targets) == 0 || f.targets[0].Text != tc.text {
			t.Errorf("%s: Text = %v, want %v", tc.name, f.targets, tc.text)
		}
	}
}

func TestOpenUsesTheDefault(t *testing.T) {
	f := twoVerbs()
	use(t, f)
	path := file(t, "a.txt", "x")
	var out bytes.Buffer
	if code := run([]string{"open", path}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := "open mac:one " + mustAbs(t, path)
	if len(f.launched) != 1 || f.launched[0] != want {
		t.Errorf("launched %q, want %q", f.launched, want)
	}
	if !strings.Contains(out.String(), "opened") || !strings.Contains(out.String(), "mac:one") {
		t.Errorf("output %q", out.String())
	}
}

func TestEditWithAppAndJSON(t *testing.T) {
	f := twoVerbs()
	use(t, f)
	path := file(t, "a.txt", "x")
	var out bytes.Buffer
	if code := run([]string{"edit", "--app", "mac:two", "--json", path}, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if len(f.launched) != 1 || f.launched[0] != "edit mac:two "+mustAbs(t, path) {
		t.Errorf("launched %q", f.launched)
	}
	var got entry
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || got.ID != "mac:two" || got.Verb != ntopenwith.Edit {
		t.Errorf("output %q: %v %+v", out.String(), err, got)
	}
}

func TestAppMustBeOfferedForTheVerb(t *testing.T) {
	f := twoVerbs()
	use(t, f)
	var errs bytes.Buffer
	// mac:one is offered for open, not for edit.
	if code := run([]string{"edit", "--app", "mac:one", file(t, "a.txt", "x")}, &bytes.Buffer{}, &errs); code != 1 {
		t.Errorf("exit %d", code)
	}
	if len(f.launched) != 0 || !strings.Contains(errs.String(), "not offered") {
		t.Errorf("launched %q, stderr %q", f.launched, errs.String())
	}
}

func TestEditWithoutDefaultAsksToChoose(t *testing.T) {
	f := twoVerbs()
	use(t, f)
	var out, errs bytes.Buffer
	if code := run([]string{"edit", file(t, "a.png", "x")}, &out, &errs); code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	if len(f.launched) != 0 {
		t.Errorf("launched %q", f.launched)
	}
	if !strings.Contains(out.String(), "mac:two") || strings.Contains(out.String(), "mac:one") {
		t.Errorf("choices %q", out.String())
	}
	if !strings.Contains(errs.String(), "choose") {
		t.Errorf("stderr %q", errs.String())
	}
}

func TestErrors(t *testing.T) {
	use(t, twoVerbs())
	dir := t.TempDir()
	for name, args := range map[string][]string{
		"no file":      {"list"},
		"two files":    {"list", "a", "b"},
		"missing file": {"open", filepath.Join(dir, "none.txt")},
		"a folder":     {"open", dir},
		"bad flag":     {"list", "--nope", file(t, "a.txt", "x")},
	} {
		var errs bytes.Buffer
		if code := run(args, &bytes.Buffer{}, &errs); code != 1 || errs.Len() == 0 {
			t.Errorf("%s: exit %d, stderr %q", name, code, errs.String())
		}
	}
}

func TestNamesAreCleaned(t *testing.T) {
	f := &fake{apps: map[ntopenwith.Verb][]ntopenwith.App{
		ntopenwith.Open: {{ID: "xdg:\x1b]0;x\x07.desktop", Name: "Evil\x1b[31m"}},
	}}
	use(t, f)
	var out bytes.Buffer
	run([]string{"list", file(t, "a.txt", "x")}, &out, &bytes.Buffer{})
	if strings.ContainsAny(out.String(), "\x1b\x07") {
		t.Errorf("escape in %q", out.String())
	}
}

func mustAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

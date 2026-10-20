package ntopenwith

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTargetKey(t *testing.T) {
	cases := []struct {
		t        Target
		key, typ string
	}{
		{Target{Path: "/a/PHOTO.PNG"}, ".png", ".png"},
		{Target{Ext: ".Png"}, ".png", ".png"},
		{Target{Path: "/a/README"}, "", ""},
		{Target{Path: "/a/x.pdf", Kind: "pdf"}, "pdf", ".pdf"},
		{Target{Path: "/a/x.tar.gz"}, ".gz", ".gz"},
	}
	for _, c := range cases {
		if got := c.t.Type(); got != c.typ {
			t.Errorf("%+v Type() = %q, want %q", c.t, got, c.typ)
		}
		if got := c.t.Key(); got != c.key {
			t.Errorf("%+v Key() = %q, want %q", c.t, got, c.key)
		}
	}
}

func TestCheckPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	got, err := CheckPath("b.txt", false)
	if err != nil || !filepath.IsAbs(got) {
		t.Fatalf("relative: %q, %v", got, err)
	}
	want, _ := filepath.EvalSymlinks(file)
	if real, _ := filepath.EvalSymlinks(got); real != want {
		t.Errorf("relative resolved to %q, want %q", got, file)
	}

	got, err = CheckPath(filepath.Join(dir, "a", "..", "b.txt"), false)
	if err != nil || got != file {
		t.Errorf("clean: %q, %v; want %q", got, err, file)
	}

	if got, err := CheckPath(dir, true); err != nil || got != dir {
		t.Errorf("directory: %q, %v", got, err)
	}

	for name, c := range map[string]struct {
		path string
		dir  bool
	}{
		"empty":         {"", false},
		"nul":           {"x\x00y", false},
		"dir as file":   {dir, false},
		"file as dir":   {file, true},
		"missing":       {filepath.Join(dir, "missing"), false},
		"missing a dir": {filepath.Join(dir, "missing"), true},
	} {
		if got, err := CheckPath(c.path, c.dir); err == nil {
			t.Errorf("%s: CheckPath(%q, %v) = %q, want error", name, c.path, c.dir, got)
		}
	}
}

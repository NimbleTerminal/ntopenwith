package launchservices

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func checkBundles(t *testing.T, what string, bs []Bundle) {
	t.Helper()
	if len(bs) == 0 {
		t.Fatalf("%s: no apps", what)
	}
	for _, b := range bs {
		checkBundle(t, what, b)
	}
}

func checkBundle(t *testing.T, what string, b Bundle) {
	t.Helper()
	if b.ID == "" || b.Name == "" || !filepath.IsAbs(b.Path) {
		t.Errorf("%s: incomplete bundle %+v", what, b)
	}
	if info, err := os.Stat(b.Path); err != nil || !info.IsDir() {
		t.Errorf("%s: %s: %v", what, b.Path, err)
	}
}

func TestServicesRealFile(t *testing.T) {
	ls, err := New()
	if err != nil {
		t.Fatal(err)
	}
	txt := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(txt, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	apps, err := ls.ForFile(txt, Viewer|Editor)
	if err != nil {
		t.Fatal(err)
	}
	checkBundles(t, "ForFile", apps)
	if !slices.ContainsFunc(apps, func(b Bundle) bool { return b.ID == "com.apple.TextEdit" }) {
		t.Errorf("ForFile: TextEdit missing from %+v", apps)
	}

	def, ok, err := ls.DefaultForFile(txt, Viewer|Editor)
	if err != nil || !ok {
		t.Fatalf("DefaultForFile = %+v, %v, %v", def, ok, err)
	}
	checkBundle(t, "DefaultForFile", def)

	editors, err := ls.ForType(".txt", Editor)
	if err != nil {
		t.Fatal(err)
	}
	checkBundles(t, "ForType", editors)
	if !slices.ContainsFunc(editors, func(b Bundle) bool { return b.ID == "com.apple.TextEdit" }) {
		t.Errorf("ForType: TextEdit missing from %+v", editors)
	}
	if def, ok, err := ls.DefaultForType("txt", Viewer|Editor); err != nil || !ok {
		t.Errorf("DefaultForType = %+v, %v, %v", def, ok, err)
	} else {
		checkBundle(t, "DefaultForType", def)
	}
}

func TestServicesRefuseBadInput(t *testing.T) {
	ls, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ls.ForFile("/tmp/a\x00b.txt", Viewer); err == nil {
		t.Error("ForFile accepted a NUL byte")
	}
	if _, err := ls.ForType("", Viewer); err == nil {
		t.Error("ForType accepted an empty extension")
	}
	if _, _, err := ls.DefaultForType("t\x00xt", Viewer); err == nil {
		t.Error("DefaultForType accepted a NUL byte")
	}
	// An extension nothing claims lists nothing, without an error.
	if apps, err := ls.ForType(".ntopenwith-none", Editor); err != nil || len(apps) != 0 {
		t.Errorf("ForType(unknown) = %+v, %v", apps, err)
	}
	if _, ok, err := ls.DefaultForType(".ntopenwith-none", Editor); err != nil || ok {
		t.Errorf("DefaultForType(unknown) = %v, %v", ok, err)
	}
}

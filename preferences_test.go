package ntopenwith

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "prefs.json")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPreferencesRoundTrip(t *testing.T) {
	var p Preferences
	p.SetDefault(Open, "pdf", "mac:com.apple.Preview")
	p.SetDefault(Open, ".png", "system")
	p.SetDefault(Edit, "md", "terminal-editor")
	p.SetHidden("zed", true)
	p.SetHidden("vim", true)
	p.SetHidden("zed", true)
	if len(p.Hidden) != 2 {
		t.Fatalf("SetHidden duplicated: %v", p.Hidden)
	}

	dir := filepath.Join(t.TempDir(), "new", "ntopenwith")
	path := filepath.Join(dir, "prefs.json")
	if err := p.Save(path); err != nil {
		t.Fatal(err)
	}
	if p.Version != 0 {
		t.Errorf("Save changed the receiver's Version to %d", p.Version)
	}
	got, err := LoadPreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	p.Version = 1
	if !reflect.DeepEqual(got, p) {
		t.Errorf("round trip\n got %+v\nwant %+v", got, p)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if mode := info.Mode().Perm(); mode != 0700 {
			t.Errorf("new folder mode = %o, want 700", mode)
		}
	}
	if got.Default(Open, "pdf") != "mac:com.apple.Preview" || got.Default(Edit, "pdf") != "" {
		t.Errorf("Default lookups wrong: %+v", got)
	}
	if !got.IsHidden("vim") || got.IsHidden("emacs") {
		t.Errorf("IsHidden wrong: %v", got.Hidden)
	}
	got.SetDefault(Open, "pdf", "")
	if got.Default(Open, "pdf") != "" {
		t.Error("an empty id should delete the default")
	}
	got.SetHidden("vim", false)
	if got.IsHidden("vim") {
		t.Error("SetHidden(false) should unhide")
	}
}

func TestLoadMissingFile(t *testing.T) {
	p, err := LoadPreferences(filepath.Join(t.TempDir(), "none.json"))
	if err != nil || !reflect.DeepEqual(p, Preferences{}) {
		t.Errorf("missing file = %+v, %v", p, err)
	}
	p, err = LoadLegacyPreferences(filepath.Join(t.TempDir(), "none.json"))
	if err != nil || !reflect.DeepEqual(p, Preferences{}) {
		t.Errorf("missing legacy file = %+v, %v", p, err)
	}
}

func TestLoadRejects(t *testing.T) {
	big := `{"version":1,"defaults":{},"hidden":["` + strings.Repeat("a", 64<<10) + `"]}`
	cases := map[string]string{
		"too large":      big,
		"array":          `[]`,
		"empty":          ``,
		"wrong version":  `{"version":2}`,
		"no version":     `{"defaults":{}}`,
		"unknown field":  `{"version":1,"extra":true}`,
		"trailing data":  `{"version":1}{}`,
		"trailing junk":  `{"version":1} x`,
		"unknown verb":   `{"version":1,"defaults":{"print":{"pdf":"x"}}}`,
		"bad json":       `{"version":1,`,
		"wrong type":     `{"version":1,"hidden":"zed"}`,
		"nested default": `{"version":1,"defaults":{"open":{"pdf":["x"]}}}`,
	}
	for name, content := range cases {
		if p, err := LoadPreferences(writeFile(t, content)); err == nil {
			t.Errorf("%s: loaded %+v, want an error", name, p)
		}
	}
	// Exactly 64 KiB is allowed.
	ok := `{"version":1}` + strings.Repeat(" ", 64<<10-len(`{"version":1}`))
	if _, err := LoadPreferences(writeFile(t, ok)); err != nil {
		t.Errorf("64 KiB exactly: %v", err)
	}
}

func TestLoadDropsBadEntries(t *testing.T) {
	long := strings.Repeat("a", 257)
	just := strings.Repeat("b", 256)
	content := `{"version":1,"defaults":{` +
		`"open":{"pdf":"good","":"x","png":"","` + long + `":"x","txt":"` + long + `","a\u0000b":"x","gif":"a\nb","` + just + `":"ok"},` +
		`"edit":{"":"x"}},` +
		`"hidden":["zed","","` + long + `","tab\tid","zed","vim","del\u007f"]}`
	p, err := LoadPreferences(writeFile(t, content))
	if err != nil {
		t.Fatal(err)
	}
	want := map[Verb]map[string]string{Open: {"pdf": "good", just: "ok"}}
	if !reflect.DeepEqual(p.Defaults, want) {
		t.Errorf("Defaults = %v, want %v", p.Defaults, want)
	}
	if !reflect.DeepEqual(p.Hidden, []string{"zed", "vim"}) {
		t.Errorf("Hidden = %q, want [zed vim]", p.Hidden)
	}
}

func TestLegacy(t *testing.T) {
	p, err := LoadLegacyPreferences(writeFile(t, `{"defaults":{"pdf":"skim","":"x","png":"a\u0001"},"hidden":["zed","zed",""]}`))
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != 1 || p.Defaults[Open]["pdf"] != "skim" || len(p.Defaults[Open]) != 1 || len(p.Defaults) != 1 {
		t.Errorf("legacy defaults = %+v", p)
	}
	if !reflect.DeepEqual(p.Hidden, []string{"zed"}) {
		t.Errorf("legacy hidden = %q", p.Hidden)
	}
	for name, content := range map[string]string{
		"array":    `[]`,
		"unknown":  `{"version":1}`,
		"trailing": `{"defaults":{}}{}`,
		"large":    `{"hidden":["` + strings.Repeat("a", 64<<10) + `"]}`,
	} {
		if _, err := LoadLegacyPreferences(writeFile(t, content)); err == nil {
			t.Errorf("legacy %s: want an error", name)
		}
	}
}

func TestSaveIsAtomic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions do not stop writes on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	var p Preferences
	p.SetDefault(Open, "pdf", "first")
	if err := p.Save(path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0700) })
	if f, err := os.CreateTemp(dir, "probe"); err == nil {
		f.Close()
		t.Skip("directory permissions are not enforced (running as root?)")
	}
	p.SetDefault(Open, "pdf", "second")
	if err := p.Save(path); err == nil {
		t.Fatal("Save into a read-only folder succeeded")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Errorf("old file changed: %q, %v (was %q)", after, err, before)
	}
	os.Chmod(dir, 0700)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temp files left behind: %v", entries)
	}
}

func TestSaveRejectsOversizeAndBadVerb(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.json")
	var p Preferences
	p.SetHidden(strings.Repeat("a", 64<<10), true)
	if err := p.Save(path); err == nil {
		t.Error("an oversize file was saved")
	}
	q := Preferences{Defaults: map[Verb]map[string]string{"print": {"pdf": "x"}}}
	if err := q.Save(path); err == nil {
		t.Error("an unknown verb was saved")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("a rejected save created the file")
	}
}

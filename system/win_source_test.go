package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"nimbleterminal.dev/ntopenwith"
)

// fakeReg is a registry in memory that ignores case, as the real one does,
// and counts its reads.
type fakeReg struct {
	keys  map[string][][2]string // `ROOT\path` → name/data pairs in order
	reads int
}

var errNoKey = errors.New("no such key")

func (f *fakeReg) find(root, path string) ([][2]string, bool) {
	f.reads++
	for k, v := range f.keys {
		if strings.EqualFold(k, root+`\`+path) {
			return v, true
		}
	}
	return nil, false
}

func (f *fakeReg) String(root, path, name string) (string, error) {
	vals, _ := f.find(root, path)
	for _, v := range vals {
		if strings.EqualFold(v[0], name) {
			return v[1], nil
		}
	}
	return "", errNoKey
}

func (f *fakeReg) ValueNames(root, path string) ([]string, error) {
	vals, ok := f.find(root, path)
	if !ok {
		return nil, errNoKey
	}
	var names []string
	for _, v := range vals {
		names = append(names, v[0])
	}
	return names, nil
}

func (f *fakeReg) SubKeys(root, path string) ([]string, error) {
	f.reads++
	prefix := strings.ToLower(root + `\` + path + `\`)
	var subs []string
	for k := range f.keys {
		if strings.HasPrefix(strings.ToLower(k), prefix) {
			sub, _, _ := strings.Cut(k[len(prefix):], `\`)
			subs = append(subs, sub)
		}
	}
	if subs == nil {
		return nil, errNoKey
	}
	slices.Sort(subs)
	return slices.Compact(subs), nil
}

// winFixture is a .png that Viewer (the person's choice), Paint and gimp.exe
// (which also edit) open, with a fake for the shell.
type winFixture struct {
	src     *winSource
	reg     *fakeReg
	png     string
	shelled [][]string // path, verb, class
}

const userExts = `HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\`

func newWinFixture(t *testing.T) *winFixture {
	t.Helper()
	f := &winFixture{
		reg: &fakeReg{keys: map[string][][2]string{
			`HKCU\Software\Classes\.png\OpenWithProgids`: {{"Paint.Picture", ""}, {"Viewer.PNG", ""}},
			`HKCR\.png`:                             {{"", "pngfile"}},
			userExts + `.png\UserChoice`:            {{"ProgId", "Viewer.PNG"}},
			userExts + `.png\OpenWithList`:          {{"a", "gimp.exe"}, {"b", `C:\evil\x.exe`}, {"c", "foo.cmd"}, {"d", "foo"}, {"e", "tool.exe"}, {"MRUList", "abcde"}},
			`HKCR\Paint.Picture\shell\edit\command`: {{"", "paint.exe %1"}},
			`HKCR\Viewer.PNG\shell\open\command`:    {{"", "viewer.exe %1"}},
			// Programs are offered by their Applications class; tool.exe
			// has none, so Windows could not run it and it is not offered.
			`HKCR\Applications\gimp.exe\shell\open\command`: {{"", `"C:\GIMP\gimp.exe" "%1"`}},
			`HKCR\Applications\gimp.exe\shell\edit\command`: {{"", `"C:\GIMP\gimp.exe" "%1"`}},
			`HKCR\Applications\foo.cmd\shell\open\command`:  {{"", "foo.cmd %1"}},
		}},
	}
	f.png = filepath.Join(t.TempDir(), "x.png")
	if err := os.WriteFile(f.png, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	friendly := func(progID, verb string) string {
		if strings.HasPrefix(progID, `Applications\`) {
			return progID // Windows has no name for it
		}
		return progID + " (" + verb + ")"
	}
	shell := func(path, verb, progID string) error {
		f.shelled = append(f.shelled, []string{path, verb, progID})
		return nil
	}
	f.src = newWinSource(f.reg, friendly, shell, new(cache))
	return f
}

func TestWindowsSourceWithFake(t *testing.T) {
	f := newWinFixture(t)
	ctx := context.Background()
	tgt := ntopenwith.Target{Path: f.png}

	// Open: the person's choice first and marked, then the system default,
	// then the rest; a program named by a path is not offered.
	apps, err := f.src.Apps(ctx, tgt, ntopenwith.Open)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(apps), []string{"win:Viewer.PNG", "system", "win:Paint.Picture", "win:app:gimp.exe"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Open ids = %v, want %v", got, want)
	}
	names := []string{"Viewer.PNG (open)", "System default", "Paint.Picture (open)", "gimp.exe"}
	for i, a := range apps {
		if a.Name != names[i] || a.Default != (i == 0) || a.Terminal {
			t.Errorf("%s: Name %q, Default %v, Terminal %v", a.ID, a.Name, a.Default, a.Terminal)
		}
	}

	// Edit: only ProgIDs and programs that edit; Viewer cannot, so there is
	// no default.
	apps, err = f.src.Apps(ctx, tgt, ntopenwith.Edit)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(apps), []string{"win:Paint.Picture", "win:app:gimp.exe"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Edit ids = %v, want %v", got, want)
	}
	if apps[0].Default || apps[0].Name != "Paint.Picture (edit)" || apps[1].Default || apps[1].Name != "gimp.exe" {
		t.Errorf("Edit apps = %+v", apps)
	}

	// Launch: everything through the shell, a program with its Applications
	// class, so Windows runs the command it registered; nothing is started
	// directly.
	got := capture(t)
	for _, c := range []struct {
		v  ntopenwith.Verb
		id string
	}{
		{ntopenwith.Open, "system"},
		{ntopenwith.Open, "win:Viewer.PNG"},
		{ntopenwith.Edit, "win:Paint.Picture"},
		{ntopenwith.Open, "win:app:gimp.exe"},
		{ntopenwith.Edit, "win:app:gimp.exe"},
	} {
		if cmd, err := f.src.Launch(ctx, tgt, c.v, ntopenwith.App{ID: c.id}); err != nil || cmd != nil {
			t.Fatalf("Launch %s %s = %v, %v", c.v, c.id, cmd, err)
		}
	}
	if want := [][]string{
		{f.png, "open", ""}, {f.png, "open", "Viewer.PNG"}, {f.png, "edit", "Paint.Picture"},
		{f.png, "open", `Applications\gimp.exe`}, {f.png, "edit", `Applications\gimp.exe`},
	}; !reflect.DeepEqual(f.shelled, want) {
		t.Errorf("shell ran %v, want %v", f.shelled, want)
	}
	if len(*got) != 0 {
		t.Errorf("started %v, want nothing started directly", *got)
	}

	// Not offered: the system default or a viewer for Edit, a program named
	// by a path, a batch file, a program with no Applications class, an ID
	// never listed.
	f.shelled, *got = nil, nil
	for _, c := range []struct {
		v  ntopenwith.Verb
		id string
	}{
		{ntopenwith.Edit, "system"},
		{ntopenwith.Edit, "win:Viewer.PNG"},
		{ntopenwith.Open, `win:app:C:\evil\x.exe`},
		{ntopenwith.Open, "win:app:foo.cmd"},
		{ntopenwith.Open, "win:app:foo"},
		{ntopenwith.Open, "win:app:tool.exe"},
		{ntopenwith.Open, "win:Gone.File"},
	} {
		if _, err := f.src.Launch(ctx, tgt, c.v, ntopenwith.App{ID: c.id}); !errors.Is(err, ntopenwith.ErrNotOffered) {
			t.Errorf("Launch %s %s: err = %v, want ErrNotOffered", c.v, c.id, err)
		}
	}

	// Launch reads the program's class again: once it no longer registers
	// an open command, the program is refused although the listing
	// offered it.
	delete(f.reg.keys, `HKCR\Applications\gimp.exe\shell\open\command`)
	if _, err := f.src.Launch(ctx, tgt, ntopenwith.Open, ntopenwith.App{ID: "win:app:gimp.exe"}); !errors.Is(err, ntopenwith.ErrNotOffered) {
		t.Errorf("Launch of a program whose class is gone: err = %v, want ErrNotOffered", err)
	}

	// Launch reads the registry again: a handler removed since the listing
	// is refused although the listing is still cached.
	delete(f.reg.keys, `HKCU\Software\Classes\.png\OpenWithProgids`)
	if apps, _ := f.src.Apps(ctx, tgt, ntopenwith.Open); !slices.Contains(ids(apps), "win:Paint.Picture") {
		t.Fatalf("the cached listing changed: %v", ids(apps))
	}
	if _, err := f.src.Launch(ctx, tgt, ntopenwith.Open, ntopenwith.App{ID: "win:Paint.Picture"}); !errors.Is(err, ntopenwith.ErrNotOffered) {
		t.Errorf("Launch of a removed handler: err = %v, want ErrNotOffered", err)
	}
	if len(f.shelled) != 0 || len(*got) != 0 {
		t.Fatalf("ran %v and started %v", f.shelled, *got)
	}
}

func TestWindowsSourceDefaults(t *testing.T) {
	f := newWinFixture(t)
	ctx := context.Background()
	tgt := ntopenwith.Target{Path: f.png}

	// Without a UserChoice the class's default, pngfile, is first; it edits
	// too, so it is the Edit default.
	delete(f.reg.keys, userExts+`.png\UserChoice`)
	f.reg.keys[`HKCR\pngfile\shell\edit\command`] = [][2]string{{"", "paint.exe %1"}}
	apps, err := f.src.Apps(ctx, tgt, ntopenwith.Open)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(apps), []string{"win:pngfile", "system", "win:Paint.Picture", "win:Viewer.PNG", "win:app:gimp.exe"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Open ids = %v, want %v", got, want)
	}
	apps, err = f.src.Apps(ctx, tgt, ntopenwith.Edit)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(apps), []string{"win:pngfile", "win:Paint.Picture", "win:app:gimp.exe"}; !reflect.DeepEqual(got, want) || !apps[0].Default || apps[1].Default || apps[2].Default {
		t.Fatalf("Edit = %+v, want %v with the first marked", apps, want)
	}

	// With no default at all, the system default is marked.
	delete(f.reg.keys, `HKCR\.png`)
	f.src.listings.clear()
	apps, err = f.src.Apps(ctx, tgt, ntopenwith.Open)
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) == 0 || apps[0].ID != "system" || !apps[0].Default {
		t.Fatalf("apps = %+v, want the system default first and marked", apps)
	}
	for _, a := range apps[1:] {
		if a.Default {
			t.Errorf("%s is marked Default too", a.ID)
		}
	}
}

func TestWindowsSourceCachesByExtension(t *testing.T) {
	f := newWinFixture(t)
	ctx := context.Background()
	other := filepath.Join(filepath.Dir(f.png), "Y.PNG")
	for _, tgt := range []ntopenwith.Target{{Path: f.png}, {Path: other}, {Path: f.png, Ext: ".Png"}} {
		if _, err := f.src.Apps(ctx, tgt, ntopenwith.Open); err != nil {
			t.Fatal(err)
		}
	}
	first := f.reg.reads
	if first == 0 {
		t.Fatal("the registry was never read")
	}
	f.src.listings.clear()
	if _, err := f.src.Apps(ctx, ntopenwith.Target{Path: f.png}, ntopenwith.Open); err != nil {
		t.Fatal(err)
	}
	if f.reg.reads != 2*first {
		t.Errorf("reads = %d after clear, want %d: one listing for .png, then one more", f.reg.reads, 2*first)
	}

	// Ext names the type instead of the path's.
	apps, err := f.src.Apps(ctx, ntopenwith.Target{Path: f.png, Ext: ".txt"}, ntopenwith.Open)
	if err != nil || !reflect.DeepEqual(ids(apps), []string{"system"}) {
		t.Errorf(".txt: %v, %v; want only the system default", ids(apps), err)
	}
}

func TestWindowsSourceContextAndErrors(t *testing.T) {
	f := newWinFixture(t)
	got := capture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tgt := ntopenwith.Target{Path: f.png}
	if apps, err := f.src.Apps(ctx, tgt, ntopenwith.Open); !errors.Is(err, context.Canceled) || apps != nil {
		t.Errorf("Apps = %v, %v; want context.Canceled", apps, err)
	}
	if _, err := f.src.Launch(ctx, tgt, ntopenwith.Open, ntopenwith.App{ID: "system"}); !errors.Is(err, context.Canceled) {
		t.Errorf("Launch err = %v, want context.Canceled", err)
	}
	missing := filepath.Join(filepath.Dir(f.png), "missing.png")
	if _, err := f.src.Launch(context.Background(), ntopenwith.Target{Path: missing}, ntopenwith.Open, ntopenwith.App{ID: "system"}); err == nil {
		t.Error("a missing file launched")
	}
	if len(f.shelled) != 0 || len(*got) != 0 {
		t.Fatalf("ran %v and started %v", f.shelled, *got)
	}

	boom := errors.New("boom")
	f.src.shell = func(string, string, string) error { return boom }
	if _, err := f.src.Launch(context.Background(), tgt, ntopenwith.Open, ntopenwith.App{ID: "system"}); !errors.Is(err, boom) {
		t.Errorf("Launch err = %v, want the shell's", err)
	}
}

func TestWindowsSourceDirectoryAndNoExtension(t *testing.T) {
	f := newWinFixture(t)
	ctx := context.Background()
	dir := filepath.Dir(f.png)
	bare := filepath.Join(dir, "Makefile")
	if err := os.WriteFile(bare, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tgt := range []ntopenwith.Target{{Path: dir, Directory: true}, {Path: bare}} {
		apps, err := f.src.Apps(ctx, tgt, ntopenwith.Open)
		if err != nil || !reflect.DeepEqual(ids(apps), []string{"system"}) || !apps[0].Default {
			t.Fatalf("Open %s: %+v, %v", tgt.Path, apps, err)
		}
		if apps, err := f.src.Apps(ctx, tgt, ntopenwith.Edit); err != nil || len(apps) != 0 {
			t.Fatalf("Edit %s: %v, %v", tgt.Path, apps, err)
		}
		if _, err := f.src.Launch(ctx, tgt, ntopenwith.Open, ntopenwith.App{ID: "system"}); err != nil {
			t.Fatal(err)
		}
	}
	if f.reg.reads != 0 {
		t.Errorf("read the registry %d times for a folder or a file without a type", f.reg.reads)
	}
	if want := [][]string{{dir, "open", ""}, {bare, "open", ""}}; !reflect.DeepEqual(f.shelled, want) {
		t.Errorf("shell ran %v, want %v", f.shelled, want)
	}
}

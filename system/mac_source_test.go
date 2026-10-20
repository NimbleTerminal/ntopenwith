package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"nimbleterminal.dev/ntopenwith"
	"nimbleterminal.dev/ntopenwith/internal/launchservices"
)

// fakeLS answers from fixed tables and records what it was asked.
type fakeLS struct {
	apps  map[launchservices.Role][]launchservices.Bundle
	def   map[launchservices.Role]launchservices.Bundle
	err   error
	calls []string // "file:<path>:<role>" or "type:<ext>:<role>"
}

func (f *fakeLS) list(call string, r launchservices.Role) ([]launchservices.Bundle, error) {
	f.calls = append(f.calls, call)
	return f.apps[r], f.err
}

func (f *fakeLS) dflt(call string, r launchservices.Role) (launchservices.Bundle, bool, error) {
	f.calls = append(f.calls, call)
	b, ok := f.def[r]
	return b, ok, f.err
}

func (f *fakeLS) ForFile(p string, r launchservices.Role) ([]launchservices.Bundle, error) {
	return f.list("file:"+p, r)
}

func (f *fakeLS) DefaultForFile(p string, r launchservices.Role) (launchservices.Bundle, bool, error) {
	return f.dflt("default-file:"+p, r)
}

func (f *fakeLS) ForType(ext string, r launchservices.Role) ([]launchservices.Bundle, error) {
	return f.list("type:"+ext, r)
}

func (f *fakeLS) DefaultForType(ext string, r launchservices.Role) (launchservices.Bundle, bool, error) {
	return f.dflt("default-type:"+ext, r)
}

var (
	preview  = launchservices.Bundle{ID: "com.apple.Preview", Name: "Preview", Path: "/System/Applications/Preview.app"}
	pixel    = launchservices.Bundle{ID: "com.pixelmatorteam.pixelmator.x", Name: "Pixelmator Pro", Path: "/Applications/Pixelmator Pro.app"}
	quick    = launchservices.Bundle{ID: "com.apple.QuickTimePlayerX", Name: "QuickTime Player", Path: "/System/Applications/QuickTime Player.app"}
	anon     = launchservices.Bundle{Name: "No ID", Path: "/Applications/NoID.app"}
	viewEdit = launchservices.Viewer | launchservices.Editor
)

func macFixture(t *testing.T) (*macSource, *fakeLS, string) {
	t.Helper()
	ls := &fakeLS{
		apps: map[launchservices.Role][]launchservices.Bundle{
			viewEdit:              {preview, anon, quick, pixel, preview},
			launchservices.Editor: {pixel, preview},
		},
		def: map[launchservices.Role]launchservices.Bundle{
			viewEdit:              preview,
			launchservices.Editor: pixel,
		},
	}
	png := filepath.Join(t.TempDir(), "x.png")
	if err := os.WriteFile(png, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	return newMacSource(ls, new(cache)), ls, png
}

func TestDarwinSourceWithFake(t *testing.T) {
	src, ls, png := macFixture(t)
	ctx := context.Background()
	tgt := ntopenwith.Target{Path: png}

	// Open: the default first and marked, then the system default, then the
	// rest once each; a bundle without an ID is skipped.
	apps, err := src.Apps(ctx, tgt, ntopenwith.Open)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(apps), []string{"mac:com.apple.Preview", "system", "mac:com.apple.QuickTimePlayerX", "mac:com.pixelmatorteam.pixelmator.x"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Open ids = %v, want %v", got, want)
	}
	for _, a := range apps {
		if a.Default != (a.ID == "mac:com.apple.Preview") || a.Terminal {
			t.Errorf("%s: Default = %v, Terminal = %v", a.ID, a.Default, a.Terminal)
		}
	}
	if apps[0].Name != "Preview" || apps[1].Name != "System default" {
		t.Errorf("names = %q, %q", apps[0].Name, apps[1].Name)
	}
	if want := []string{"default-file:" + png, "file:" + png}; !reflect.DeepEqual(ls.calls, want) {
		t.Errorf("asked %v, want %v", ls.calls, want)
	}

	// Edit asks for editors only and never offers the system default.
	apps, err = src.Apps(ctx, tgt, ntopenwith.Edit)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(apps), []string{"mac:com.pixelmatorteam.pixelmator.x", "mac:com.apple.Preview"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Edit ids = %v, want %v", got, want)
	}
	if !apps[0].Default || apps[1].Default {
		t.Errorf("Edit Default = %v, %v", apps[0].Default, apps[1].Default)
	}

	// Launch: the system default through open, an app through open -a.
	got := capture(t)
	if cmd, err := src.Launch(ctx, tgt, ntopenwith.Open, ntopenwith.App{ID: "system"}); err != nil || cmd != nil {
		t.Fatalf("Launch system = %v, %v", cmd, err)
	}
	if _, err := src.Launch(ctx, tgt, ntopenwith.Edit, ntopenwith.App{ID: "mac:com.pixelmatorteam.pixelmator.x"}); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"open", png}, {"open", "-a", "/Applications/Pixelmator Pro.app", png}}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("started %v, want %v", *got, want)
	}

	// Not offered: the system default for Edit, a viewer for Edit, an app
	// Launch Services no longer lists, an ID never listed.
	*got = nil
	for _, c := range []struct {
		v  ntopenwith.Verb
		id string
	}{
		{ntopenwith.Edit, "system"},
		{ntopenwith.Edit, "mac:com.apple.QuickTimePlayerX"},
		{ntopenwith.Open, "mac:"},
		{ntopenwith.Open, "mac:com.example.Gone"},
	} {
		if _, err := src.Launch(ctx, tgt, c.v, ntopenwith.App{ID: c.id}); !errors.Is(err, ntopenwith.ErrNotOffered) {
			t.Errorf("Launch %s %s: err = %v, want ErrNotOffered", c.v, c.id, err)
		}
	}
	ls.apps[viewEdit] = []launchservices.Bundle{preview}
	if _, err := src.Launch(ctx, tgt, ntopenwith.Open, ntopenwith.App{ID: "mac:com.apple.QuickTimePlayerX"}); !errors.Is(err, ntopenwith.ErrNotOffered) {
		t.Errorf("Launch of an app no longer listed: err = %v, want ErrNotOffered", err)
	}
	if len(*got) != 0 {
		t.Fatalf("started %v", *got)
	}
}

func TestDarwinSourceWithoutADefault(t *testing.T) {
	src, ls, png := macFixture(t)
	ls.def = nil
	apps, err := src.Apps(context.Background(), ntopenwith.Target{Path: png}, ntopenwith.Open)
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
	apps, err = src.Apps(context.Background(), ntopenwith.Target{Path: png}, ntopenwith.Edit)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range apps {
		if a.Default {
			t.Errorf("Edit: %s is marked Default", a.ID)
		}
	}
}

func TestDarwinSourceExtAsksForTheType(t *testing.T) {
	src, ls, png := macFixture(t)
	if _, err := src.Apps(context.Background(), ntopenwith.Target{Path: png, Ext: ".TXT"}, ntopenwith.Open); err != nil {
		t.Fatal(err)
	}
	if want := []string{"default-type:.txt", "type:.txt"}; !reflect.DeepEqual(ls.calls, want) {
		t.Errorf("asked %v, want %v", ls.calls, want)
	}
}

func TestDarwinSourceCachesByType(t *testing.T) {
	src, ls, png := macFixture(t)
	ctx := context.Background()
	other := filepath.Join(filepath.Dir(png), "y.png")
	for _, p := range []string{png, other, png} {
		if _, err := src.Apps(ctx, ntopenwith.Target{Path: p}, ntopenwith.Open); err != nil {
			t.Fatal(err)
		}
	}
	if len(ls.calls) != 2 {
		t.Fatalf("asked %d times for one type, want 2 calls: %v", len(ls.calls), ls.calls)
	}
	src.listings.clear()
	if _, err := src.Apps(ctx, ntopenwith.Target{Path: png}, ntopenwith.Open); err != nil {
		t.Fatal(err)
	}
	if len(ls.calls) != 4 {
		t.Fatalf("after clear: %d calls, want 4", len(ls.calls))
	}

	// Files without an extension are kept apart by path.
	ls.calls = nil
	a := filepath.Join(filepath.Dir(png), "Makefile")
	b := filepath.Join(filepath.Dir(png), "README")
	for _, p := range []string{a, b} {
		if _, err := src.Apps(ctx, ntopenwith.Target{Path: p}, ntopenwith.Open); err != nil {
			t.Fatal(err)
		}
	}
	if want := []string{"default-file:" + a, "file:" + a, "default-file:" + b, "file:" + b}; !reflect.DeepEqual(ls.calls, want) {
		t.Errorf("asked %v, want %v", ls.calls, want)
	}
}

func TestDarwinSourceErrorsAndContext(t *testing.T) {
	src, ls, png := macFixture(t)
	boom := errors.New("boom")
	ls.err = boom
	if apps, err := src.Apps(context.Background(), ntopenwith.Target{Path: png}, ntopenwith.Open); !errors.Is(err, boom) || apps != nil {
		t.Errorf("Apps = %v, %v; want boom", apps, err)
	}
	got := capture(t)
	if _, err := src.Launch(context.Background(), ntopenwith.Target{Path: png}, ntopenwith.Open, ntopenwith.App{ID: "system"}); !errors.Is(err, boom) {
		t.Errorf("Launch err = %v, want boom", err)
	}
	ls.err = nil

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if apps, err := src.Apps(ctx, ntopenwith.Target{Path: png}, ntopenwith.Open); !errors.Is(err, context.Canceled) || apps != nil {
		t.Errorf("Apps = %v, %v; want context.Canceled", apps, err)
	}
	if _, err := src.Launch(ctx, ntopenwith.Target{Path: png}, ntopenwith.Open, ntopenwith.App{ID: "system"}); !errors.Is(err, context.Canceled) {
		t.Errorf("Launch err = %v, want context.Canceled", err)
	}
	missing := filepath.Join(filepath.Dir(png), "missing.png")
	if _, err := src.Launch(context.Background(), ntopenwith.Target{Path: missing}, ntopenwith.Open, ntopenwith.App{ID: "system"}); err == nil {
		t.Error("a missing file launched")
	}
	if len(*got) != 0 {
		t.Fatalf("started %v", *got)
	}
}

func TestDarwinSourceDirectory(t *testing.T) {
	src, ls, png := macFixture(t)
	dir := filepath.Dir(png)
	tgt := ntopenwith.Target{Path: dir, Directory: true}
	apps, err := src.Apps(context.Background(), tgt, ntopenwith.Open)
	if err != nil || !reflect.DeepEqual(ids(apps), []string{"system"}) || !apps[0].Default {
		t.Fatalf("Open: %+v, %v", apps, err)
	}
	if apps, err := src.Apps(context.Background(), tgt, ntopenwith.Edit); err != nil || len(apps) != 0 {
		t.Fatalf("Edit: %v, %v", apps, err)
	}
	if len(ls.calls) != 0 {
		t.Errorf("asked Launch Services about a folder: %v", ls.calls)
	}
	got := capture(t)
	if _, err := src.Launch(context.Background(), tgt, ntopenwith.Open, ntopenwith.App{ID: "system"}); err != nil {
		t.Fatal(err)
	}
	if want := [][]string{{"open", dir}}; !reflect.DeepEqual(*got, want) {
		t.Fatalf("started %v, want %v", *got, want)
	}
}

package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"nimbleterminal.dev/ntopenwith"
	"nimbleterminal.dev/ntopenwith/internal/xdg"
)

// fixture writes a small XDG tree and returns a source over it, the folder of
// its applications, and a regular file x.png to act on.
func fixture(t *testing.T, mimeapps string) (src *xdgSource, apps, png string) {
	t.Helper()
	root := t.TempDir()
	write := func(rel, text string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("data/mime/globs2", "50:image/png:*.png\n50:text/plain:*.txt\n")
	write("data/applications/viewer.desktop", "[Desktop Entry]\nType=Application\nName=Viewer\nExec=viewer %f\nMimeType=image/png;\nCategories=Graphics;Viewer;\n")
	write("data/applications/paint.desktop", "[Desktop Entry]\nType=Application\nName=Paint\nExec=paint --open=%f\nMimeType=image/png;\nCategories=Graphics;\n")
	write("data/applications/tool.desktop", "[Desktop Entry]\nType=Application\nName=Tool\nExec=tool %f\nMimeType=image/png;\nCategories=Utility;\n")
	write("data/applications/nano.desktop", "[Desktop Entry]\nType=Application\nName=Nano\nExec=nano %f\nTerminal=true\nMimeType=text/plain;\nCategories=TextEditor;\n")
	write("data/applications/broken.desktop", "[Desktop Entry]\nType=Application\nName=Broken\nExec=broken %q\nMimeType=image/png;\nCategories=Graphics;\n")
	write("config/mimeapps.list", mimeapps)
	write("files/x.png", "png")
	write("files/notes.txt", "text")

	env := xdg.Env{DataHome: filepath.Join(root, "data"), ConfigHome: filepath.Join(root, "config")}
	load := func() (*xdg.Index, error) {
		return xdg.Load(env, func(string) (string, error) { return "", errors.New("not found") })
	}
	return newXDGSource(load, new(cache)), filepath.Join(root, "data", "applications"), filepath.Join(root, "files", "x.png")
}

func ids(apps []ntopenwith.App) []string {
	var out []string
	for _, a := range apps {
		out = append(out, a.ID)
	}
	return out
}

func capture(t *testing.T) *[][]string {
	t.Helper()
	var got [][]string
	old := start
	start = func(argv []string) error {
		got = append(got, slices.Clone(argv))
		return nil
	}
	t.Cleanup(func() { start = old })
	return &got
}

func TestOpenListsTheDefaultFirst(t *testing.T) {
	src, _, png := fixture(t, "[Default Applications]\nimage/png=paint.desktop;\n")
	apps, err := src.Apps(context.Background(), ntopenwith.Target{Path: png}, ntopenwith.Open)
	if err != nil {
		t.Fatal(err)
	}
	// The default, then the system default, then the other handlers; the
	// broken entry is still listed (it fails when launched, not here).
	want := []string{"xdg:paint.desktop", "system", "xdg:broken.desktop", "xdg:tool.desktop", "xdg:viewer.desktop"}
	got := ids(apps)
	slices.Sort(got[2:])
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	for _, a := range apps {
		if a.Default != (a.ID == "xdg:paint.desktop") {
			t.Errorf("%s: Default = %v", a.ID, a.Default)
		}
	}
	if apps[0].Name != "Paint" || apps[1].Name != "System default" {
		t.Errorf("names = %q, %q", apps[0].Name, apps[1].Name)
	}
}

func TestOpenWithoutADefaultMakesTheSystemDefault(t *testing.T) {
	src, _, png := fixture(t, "")
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
}

func TestTypeComesFromExtOverPath(t *testing.T) {
	src, _, png := fixture(t, "")
	// Path says .png, Ext says .txt: the text handlers are offered.
	apps, err := src.Apps(context.Background(), ntopenwith.Target{Path: png, Ext: ".TXT"}, ntopenwith.Open)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(apps), []string{"system", "xdg:nano.desktop"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	if !apps[1].Terminal || apps[0].Terminal {
		t.Errorf("Terminal = %v, %v; want only nano", apps[0].Terminal, apps[1].Terminal)
	}
}

func TestEditListsOnlyCategoryMatchesAndNoDefault(t *testing.T) {
	// tool.desktop is the mimeapps default but is not an editor.
	src, _, png := fixture(t, "[Default Applications]\nimage/png=tool.desktop;\n")
	apps, err := src.Apps(context.Background(), ntopenwith.Target{Path: png}, ntopenwith.Edit)
	if err != nil {
		t.Fatal(err)
	}
	got := ids(apps)
	slices.Sort(got)
	if want := []string{"xdg:broken.desktop", "xdg:paint.desktop", "xdg:viewer.desktop"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	for _, a := range apps {
		if a.Default {
			t.Errorf("%s is Default for Edit", a.ID)
		}
	}
	// Edit never offers the system default.
	if _, err := src.Launch(context.Background(), ntopenwith.Target{Path: png}, ntopenwith.Edit, ntopenwith.App{ID: "system"}); !errors.Is(err, ntopenwith.ErrNotOffered) {
		t.Errorf("system for Edit: err = %v, want ErrNotOffered", err)
	}
}

func TestLaunchSystemRunsXdgOpen(t *testing.T) {
	src, _, png := fixture(t, "")
	got := capture(t)
	cmd, err := src.Launch(context.Background(), ntopenwith.Target{Path: png}, ntopenwith.Open, ntopenwith.App{ID: "system"})
	if err != nil || cmd != nil {
		t.Fatalf("Launch = %v, %v", cmd, err)
	}
	if want := [][]string{{"xdg-open", png}}; !reflect.DeepEqual(*got, want) {
		t.Fatalf("started %v, want %v", *got, want)
	}
}

func TestLaunchExpandsTheExecLine(t *testing.T) {
	src, _, png := fixture(t, "")
	got := capture(t)
	_, err := src.Launch(context.Background(), ntopenwith.Target{Path: png}, ntopenwith.Open, ntopenwith.App{ID: "xdg:paint.desktop"})
	if err != nil {
		t.Fatal(err)
	}
	if want := [][]string{{"paint", "--open=" + png}}; !reflect.DeepEqual(*got, want) {
		t.Fatalf("started %v, want %v", *got, want)
	}
}

func TestLaunchTerminalEntryReturnsAnUnstartedCommand(t *testing.T) {
	src, _, png := fixture(t, "")
	got := capture(t)
	txt := filepath.Join(filepath.Dir(png), "notes.txt")
	cmd, err := src.Launch(context.Background(), ntopenwith.Target{Path: txt}, ntopenwith.Edit, ntopenwith.App{ID: "xdg:nano.desktop"})
	if err != nil {
		t.Fatal(err)
	}
	if len(*got) != 0 {
		t.Fatalf("a terminal app was started: %v", *got)
	}
	if cmd == nil || cmd.Process != nil {
		t.Fatalf("cmd = %+v, want an unstarted command", cmd)
	}
	if want := []string{"nano", txt}; !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("Args = %v, want %v", cmd.Args, want)
	}
}

func TestLaunchRefusesWhatIsNoLongerOffered(t *testing.T) {
	src, apps, png := fixture(t, "")
	got := capture(t)
	tgt := ntopenwith.Target{Path: png}

	listed, err := src.Apps(context.Background(), tgt, ntopenwith.Open)
	if err != nil || !slices.Contains(ids(listed), "xdg:tool.desktop") {
		t.Fatalf("Apps = %v, %v", ids(listed), err)
	}
	if err := os.Remove(filepath.Join(apps, "tool.desktop")); err != nil {
		t.Fatal(err)
	}
	// The cached listing still shows it, but Launch looks again.
	_, err = src.Launch(context.Background(), tgt, ntopenwith.Open, ntopenwith.App{ID: "xdg:tool.desktop"})
	if !errors.Is(err, ntopenwith.ErrNotOffered) {
		t.Fatalf("err = %v, want ErrNotOffered", err)
	}
	// Nor is an app offered for another type: nano handles only text.
	_, err = src.Launch(context.Background(), tgt, ntopenwith.Open, ntopenwith.App{ID: "xdg:nano.desktop"})
	if !errors.Is(err, ntopenwith.ErrNotOffered) {
		t.Fatalf("err = %v, want ErrNotOffered for the wrong type", err)
	}
	if len(*got) != 0 {
		t.Fatalf("started %v", *got)
	}
}

func TestLaunchRefusesABadExecLineAndABadPath(t *testing.T) {
	src, _, png := fixture(t, "")
	got := capture(t)
	if _, err := src.Launch(context.Background(), ntopenwith.Target{Path: png}, ntopenwith.Open, ntopenwith.App{ID: "xdg:broken.desktop"}); err == nil {
		t.Error("an Exec line with an unknown field code launched")
	}
	missing := filepath.Join(filepath.Dir(png), "missing.png")
	if _, err := src.Launch(context.Background(), ntopenwith.Target{Path: missing}, ntopenwith.Open, ntopenwith.App{ID: "system"}); err == nil {
		t.Error("a missing file launched")
	}
	if len(*got) != 0 {
		t.Fatalf("started %v", *got)
	}
}

func TestAppsStopsWithTheContext(t *testing.T) {
	src, _, png := fixture(t, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	apps, err := src.Apps(ctx, ntopenwith.Target{Path: png}, ntopenwith.Open)
	if !errors.Is(err, context.Canceled) || apps != nil {
		t.Fatalf("Apps = %v, %v; want nil and context.Canceled", apps, err)
	}
	if _, err := src.Launch(ctx, ntopenwith.Target{Path: png}, ntopenwith.Open, ntopenwith.App{ID: "system"}); !errors.Is(err, context.Canceled) {
		t.Errorf("Launch err = %v, want context.Canceled", err)
	}
}

func TestAppsIsCachedUntilCleared(t *testing.T) {
	src, apps, png := fixture(t, "")
	loads := 0
	inner := src.load
	src.load = func() (*xdg.Index, error) { loads++; return inner() }
	tgt := ntopenwith.Target{Path: png}
	list := func() []string {
		t.Helper()
		a, err := src.Apps(context.Background(), tgt, ntopenwith.Open)
		if err != nil {
			t.Fatal(err)
		}
		return ids(a)
	}
	first := list()
	// A different file of the same type reads nothing again.
	if _, err := src.Apps(context.Background(), ntopenwith.Target{Path: filepath.Join(filepath.Dir(png), "y.png")}, ntopenwith.Open); err != nil {
		t.Fatal(err)
	}
	if loads != 1 {
		t.Fatalf("loaded %d times, want 1", loads)
	}
	if err := os.Remove(filepath.Join(apps, "tool.desktop")); err != nil {
		t.Fatal(err)
	}
	if got := list(); !reflect.DeepEqual(got, first) {
		t.Fatalf("listing changed before the cache was cleared: %v", got)
	}
	src.listings.clear()
	if got := list(); slices.Contains(got, "xdg:tool.desktop") || loads != 2 {
		t.Fatalf("after clear: %v, %d loads", got, loads)
	}
}

func TestADirectoryIsOfferedTheSystemDefault(t *testing.T) {
	src, _, png := fixture(t, "")
	dir := filepath.Dir(png)
	apps, err := src.Apps(context.Background(), ntopenwith.Target{Path: dir, Directory: true}, ntopenwith.Open)
	if err != nil || len(apps) != 1 || apps[0].ID != "system" {
		t.Fatalf("Open: %v, %v", apps, err)
	}
	apps, err = src.Apps(context.Background(), ntopenwith.Target{Path: dir, Directory: true}, ntopenwith.Edit)
	if err != nil || len(apps) != 0 {
		t.Fatalf("Edit: %v, %v", apps, err)
	}
}

package ntopenwith

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// stubLookPath makes lookPath find only the given names.
func stubLookPath(t *testing.T, found ...string) {
	t.Helper()
	old := lookPath
	t.Cleanup(func() { lookPath = old })
	lookPath = func(name string) (string, error) {
		for _, f := range found {
			if f == name {
				return "/bin/" + name, nil
			}
		}
		return "", exec.ErrNotFound
	}
}

// stubBundleDirs makes bundleDirs return dirs.
func stubBundleDirs(t *testing.T, dirs ...string) {
	t.Helper()
	old := bundleDirs
	t.Cleanup(func() { bundleDirs = old })
	bundleDirs = func() []string { return dirs }
}

// captureStart records the argv of a started command instead of running it.
func captureStart(t *testing.T) *[][]string {
	t.Helper()
	var started [][]string
	old := startCommand
	t.Cleanup(func() { startCommand = old })
	startCommand = func(c *exec.Cmd) error {
		started = append(started, c.Args)
		return nil
	}
	return &started
}

func tempFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func ids(apps []App) []string {
	var out []string
	for _, a := range apps {
		out = append(out, a.ID)
	}
	return out
}

func TestListOffers(t *testing.T) {
	stubLookPath(t, "viewer", "editor", "pdfview", "anyview")
	bundles := t.TempDir()
	if err := os.Mkdir(filepath.Join(bundles, "Preview.app"), 0o755); err != nil {
		t.Fatal(err)
	}
	stubBundleDirs(t, t.TempDir(), bundles)

	src := List(
		ListApp{ID: "png", Name: "PNG viewer", Verbs: []Verb{Open}, Kinds: []string{".png"}, Command: []string{"viewer"}},
		ListApp{ID: "pdf", Name: "PDF viewer", Verbs: []Verb{Open}, Kinds: []string{"pdf"}, Command: []string{"pdfview"}},
		ListApp{ID: "txt", Name: "Text editor", Verbs: []Verb{Edit}, Text: true, Command: []string{"editor"}},
		ListApp{ID: "gone", Name: "Missing", Verbs: []Verb{Open}, Command: []string{"nothere"}},
		ListApp{ID: "needs-two", Name: "Two", Verbs: []Verb{Open}, Command: []string{"viewer"}, Find: []string{"viewer", "nothere"}},
		ListApp{ID: "preview", Name: "Preview", Verbs: []Verb{Open}, Bundle: "Preview.app"},
		ListApp{ID: "nobundle", Name: "No bundle", Verbs: []Verb{Open}, Bundle: "Nope.app"},
		ListApp{ID: "any", Name: "Any", Verbs: []Verb{Open, Edit}, Command: []string{"anyview"}, Default: []Verb{Open}},
	)
	ctx := context.Background()
	png := filepath.Join(t.TempDir(), "A.PNG")

	apps, err := src.Apps(ctx, Target{Path: png}, Open)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(apps), []string{"any", "png", "preview"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("png open = %q, want %q", got, want)
	}
	if !apps[0].Default || apps[1].Default || apps[2].Default {
		t.Errorf("defaults = %+v, want only the first", apps)
	}
	if apps[1].Name != "PNG viewer" {
		t.Errorf("Name = %q", apps[1].Name)
	}

	// Kind matches too, and wins over the extension for nothing else.
	apps, _ = src.Apps(ctx, Target{Path: "/x/a.bin", Kind: "pdf"}, Open)
	if got, want := ids(apps), []string{"any", "pdf", "preview"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pdf kind = %q, want %q", got, want)
	}

	// Edit: a text-only app is hidden for binary targets.
	apps, _ = src.Apps(ctx, Target{Path: "/x/a.png"}, Edit)
	if got, want := ids(apps), []string{"any"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("binary edit = %q, want %q", got, want)
	}
	if apps[0].Default {
		t.Error("Default set for a verb not in Default")
	}
	apps, _ = src.Apps(ctx, Target{Path: "/x/a.txt", Text: true}, Edit)
	if got, want := ids(apps), []string{"txt", "any"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("text edit = %q, want %q", got, want)
	}
}

func TestListKindsIgnoreCase(t *testing.T) {
	stubLookPath(t, "viewer")
	for _, tt := range []struct {
		kind   string
		target Target
	}{
		{".PNG", Target{Path: "/x/a.png"}},
		{".png", Target{Ext: ".PNG"}},
		{"IMAGE/*", Target{Kind: "image/*"}},
		{"image/*", Target{Kind: "IMAGE/*"}},
	} {
		src := List(ListApp{ID: "v", Verbs: []Verb{Open}, Kinds: []string{tt.kind}, Command: []string{"viewer"}})
		apps, err := src.Apps(context.Background(), tt.target, Open)
		if err != nil || len(apps) != 1 || apps[0].ID != "v" {
			t.Errorf("kind %q, target %+v: apps = %v, err = %v", tt.kind, tt.target, apps, err)
		}
	}
}

func TestListLaunch(t *testing.T) {
	stubLookPath(t, "viewer")
	bundles := t.TempDir()
	if err := os.Mkdir(filepath.Join(bundles, "Preview.app"), 0o755); err != nil {
		t.Fatal(err)
	}
	stubBundleDirs(t, t.TempDir(), bundles)
	started := captureStart(t)
	path := tempFile(t, "a b.png")
	tgt := Target{Path: path}
	ctx := context.Background()

	src := List(
		ListApp{ID: "v", Name: "V", Verbs: []Verb{Open}, Command: []string{"viewer", "--new", "{}", "--"}},
		ListApp{ID: "p", Name: "P", Verbs: []Verb{Open}, Bundle: "Preview.app", Command: []string{"viewer"}},
		ListApp{ID: "gone", Name: "G", Verbs: []Verb{Open}, Command: []string{"nothere"}},
		ListApp{ID: "txt", Name: "T", Verbs: []Verb{Edit}, Text: true, Command: []string{"viewer"}},
	)

	cmd, err := src.Launch(ctx, tgt, Open, App{ID: "v"})
	if err != nil || cmd != nil {
		t.Fatalf("Launch = %v, %v", cmd, err)
	}
	if want := []string{"viewer", "--new", path, "--"}; !reflect.DeepEqual((*started)[0], want) {
		t.Errorf("argv = %q, want %q", (*started)[0], want)
	}

	// Bundle launches use open -a on darwin, the command elsewhere.
	oldOS := hostOS
	t.Cleanup(func() { hostOS = oldOS })
	hostOS = "darwin"
	*started = nil
	if _, err := src.Launch(ctx, tgt, Open, App{ID: "p"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"open", "-a", filepath.Join(bundles, "Preview.app"), path}; !reflect.DeepEqual((*started)[0], want) {
		t.Errorf("bundle argv = %q, want %q", (*started)[0], want)
	}
	hostOS = "linux"
	*started = nil
	if _, err := src.Launch(ctx, tgt, Open, App{ID: "p"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"viewer", path}; !reflect.DeepEqual((*started)[0], want) {
		t.Errorf("non-darwin argv = %q, want %q", (*started)[0], want)
	}

	// Not offered: unknown ID, missing program, wrong verb, binary target.
	*started = nil
	for _, c := range []struct {
		name string
		tgt  Target
		v    Verb
		id   string
	}{
		{"unknown", tgt, Open, "nope"},
		{"program gone", tgt, Open, "gone"},
		{"wrong verb", tgt, Edit, "v"},
		{"binary target", tgt, Edit, "txt"},
	} {
		if _, err := src.Launch(ctx, c.tgt, c.v, App{ID: c.id}); !errors.Is(err, ErrNotOffered) {
			t.Errorf("%s: err = %v, want ErrNotOffered", c.name, err)
		}
	}
	if len(*started) != 0 {
		t.Errorf("started %q for apps not offered", *started)
	}

	// A path that is not a file is refused before anything starts.
	if _, err := src.Launch(ctx, Target{Path: filepath.Join(t.TempDir(), "missing.png")}, Open, App{ID: "v"}); err == nil || errors.Is(err, ErrNotOffered) {
		t.Errorf("missing file: err = %v", err)
	}
	if len(*started) != 0 {
		t.Errorf("started %q for a missing file", *started)
	}
}

func TestListLaunchTerminalReturnsCmd(t *testing.T) {
	stubLookPath(t, "ed")
	started := captureStart(t)
	path := tempFile(t, "a.txt")
	src := List(ListApp{ID: "ed", Name: "Ed", Verbs: []Verb{Edit}, Text: true, Terminal: true, Command: []string{"ed", "-x"}})

	apps, err := src.Apps(context.Background(), Target{Path: path, Text: true}, Edit)
	if err != nil || len(apps) != 1 || !apps[0].Terminal {
		t.Fatalf("Apps = %+v, %v", apps, err)
	}
	cmd, err := src.Launch(context.Background(), Target{Path: path, Text: true}, Edit, apps[0])
	if err != nil {
		t.Fatal(err)
	}
	if cmd == nil || cmd.Process != nil {
		t.Fatalf("cmd = %+v, want an unstarted command", cmd)
	}
	// It runs in the caller's terminal, so it is not detached from it.
	if cmd.SysProcAttr != nil {
		t.Errorf("SysProcAttr = %+v, want none for a terminal app", cmd.SysProcAttr)
	}
	if want := []string{"ed", "-x", path}; !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}
	if len(*started) != 0 {
		t.Errorf("a terminal app was started: %q", *started)
	}
}

func TestTerminalEditorReadsEnvAtLaunch(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	editor, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := tempFile(t, "a.txt")
	tgt := Target{Path: path, Text: true}
	ctx := context.Background()
	src := List(TerminalEditor())

	apps, err := src.Apps(ctx, tgt, Edit)
	if err != nil || len(apps) != 0 {
		t.Fatalf("unset: Apps = %+v, %v; want none", apps, err)
	}
	if _, err := src.Launch(ctx, tgt, Edit, App{ID: TerminalEditorID}); !errors.Is(err, ErrNotOffered) {
		t.Fatalf("unset: Launch err = %v, want ErrNotOffered", err)
	}

	t.Setenv("EDITOR", editor)
	apps, err = src.Apps(ctx, tgt, Edit)
	if err != nil || len(apps) != 1 {
		t.Fatalf("set: Apps = %+v, %v", apps, err)
	}
	if a := apps[0]; a.ID != TerminalEditorID || a.Name != "Edit in terminal" || !a.Terminal {
		t.Errorf("app = %+v", a)
	}
	cmd, err := src.Launch(ctx, tgt, Edit, apps[0])
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{editor, path}; !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("Args = %q, want %q", cmd.Args, want)
	}

	// $VISUAL wins, and a binary target never gets the editor.
	t.Setenv("VISUAL", editor)
	if apps, _ := src.Apps(ctx, Target{Path: path}, Edit); len(apps) != 0 {
		t.Errorf("binary target offered %+v", apps)
	}
	if apps, _ := src.Apps(ctx, tgt, Open); len(apps) != 0 {
		t.Errorf("Open offered %+v", apps)
	}
}

func TestTerminalEditors(t *testing.T) {
	stubLookPath(t, "nvim", "emacs", "nano")
	list := TerminalEditors()
	var got []string
	for _, a := range list {
		got = append(got, a.ID)
		if !a.Terminal || !a.Text || !reflect.DeepEqual(a.Verbs, []Verb{Edit}) {
			t.Errorf("%s: %+v", a.ID, a)
		}
	}
	if want := []string{"nvim", "vim", "hx", "nano", "micro", "emacs"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("IDs = %q, want %q", got, want)
	}
	names := map[string]string{"nvim": "Neovim", "vim": "Vim", "hx": "Helix", "nano": "nano", "micro": "micro", "emacs": "Emacs"}
	for _, a := range list {
		if a.Name != names[a.ID] {
			t.Errorf("%s Name = %q, want %q", a.ID, a.Name, names[a.ID])
		}
		if a.ID == "emacs" && !reflect.DeepEqual(a.Command, []string{"emacs", "-nw"}) {
			t.Errorf("emacs Command = %q", a.Command)
		}
	}

	apps, err := List(list...).Apps(context.Background(), Target{Path: "/x/a.txt", Text: true}, Edit)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(apps), []string{"nvim", "nano", "emacs"}; !reflect.DeepEqual(got, want) {
		t.Errorf("installed = %q, want %q", got, want)
	}
	if !strings.HasPrefix(apps[0].Name, "Neo") {
		t.Errorf("Name = %q", apps[0].Name)
	}
}

func TestListBundleAndCommandOnDarwin(t *testing.T) {
	oldOS := hostOS
	t.Cleanup(func() { hostOS = oldOS })
	started := captureStart(t)
	path := tempFile(t, "a.png")
	tgt := Target{Path: path}
	ctx := context.Background()
	src := List(ListApp{ID: "p", Name: "P", Verbs: []Verb{Open}, Bundle: "Preview.app", Command: []string{"viewer"}})

	bundles := t.TempDir()
	stubBundleDirs(t, bundles)

	// On darwin the bundle is what must exist, not the program.
	hostOS = "darwin"
	stubLookPath(t) // no program
	if err := os.Mkdir(filepath.Join(bundles, "Preview.app"), 0o755); err != nil {
		t.Fatal(err)
	}
	apps, _ := src.Apps(ctx, tgt, Open)
	if got, want := ids(apps), []string{"p"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("bundle present, program missing: %q, want %q", got, want)
	}
	if _, err := src.Launch(ctx, tgt, Open, apps[0]); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if want := []string{"open", "-a", filepath.Join(bundles, "Preview.app"), path}; len(*started) != 1 || !reflect.DeepEqual((*started)[0], want) {
		t.Errorf("started %q, want %q", *started, want)
	}

	// Program present, bundle missing: not offered on darwin.
	stubLookPath(t, "viewer")
	stubBundleDirs(t, t.TempDir())
	if apps, _ := src.Apps(ctx, tgt, Open); len(apps) != 0 {
		t.Errorf("bundle missing: offered %+v", apps)
	}
	if _, err := src.Launch(ctx, tgt, Open, App{ID: "p"}); !errors.Is(err, ErrNotOffered) {
		t.Errorf("bundle missing: Launch err = %v", err)
	}

	// Elsewhere the program is what must exist.
	hostOS = "linux"
	if apps, _ := src.Apps(ctx, tgt, Open); len(apps) != 1 {
		t.Errorf("linux, program present: %+v", apps)
	}
}

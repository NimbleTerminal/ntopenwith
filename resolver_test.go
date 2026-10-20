package ntopenwith

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

// askSource answers Apps by calling a function, and records its launches.
type askSource struct {
	ask      func(Target, Verb) []App
	err      error
	launched []string
	cmd      *exec.Cmd
}

func (s *askSource) Apps(_ context.Context, t Target, v Verb) ([]App, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.ask(t, v), nil
}

func (s *askSource) Launch(_ context.Context, _ Target, _ Verb, app App) (*exec.Cmd, error) {
	s.launched = append(s.launched, app.ID)
	return s.cmd, nil
}

// fixed returns a source that offers apps for every target and verb.
func fixed(apps ...App) *askSource {
	return &askSource{ask: func(Target, Verb) []App { return slices.Clone(apps) }}
}

// canonSource is a Source that also canonicalizes IDs.
type canonSource struct {
	*askSource
	aliases map[string]string
}

func (c canonSource) Canonical(id string) string {
	if to, ok := c.aliases[id]; ok {
		return to
	}
	return id
}

var errBoom = errors.New("boom")

func prefsWith(v Verb, key, id string, hidden ...string) Preferences {
	p := Preferences{Version: 1}
	p.SetDefault(v, key, id)
	for _, h := range hidden {
		p.SetHidden(h, true)
	}
	return p
}

func TestResolve(t *testing.T) {
	a := App{ID: "a", Name: "A"}
	b := App{ID: "b", Name: "B"}
	def := App{ID: "d", Name: "D", Default: true}
	term := App{ID: TerminalEditorID, Name: "Edit in terminal", Terminal: true}
	pdf := Target{Path: "/x/a.pdf", Kind: "pdf"}
	txt := Target{Path: "/x/a.txt", Text: true}
	bin := Target{Path: "/x/a.png"}

	tests := []struct {
		name     string
		src      Source
		prefs    Preferences
		target   Target
		verb     Verb
		want     string
		fallback bool
		wantErr  error
	}{
		{name: "open saved beats default", src: fixed(a, def), prefs: prefsWith(Open, "pdf", "a"), target: pdf, verb: Open, want: "a"},
		{name: "open saved is keyed by Kind", src: fixed(a, b, def), prefs: prefsWith(Open, ".pdf", "a"), target: pdf, verb: Open, want: "d"},
		{name: "open saved by extension", src: fixed(a, b, def), prefs: prefsWith(Open, ".txt", "b"), target: Target{Path: "/x/A.TXT"}, verb: Open, want: "b"},
		{name: "open source default", src: fixed(a, def), target: pdf, verb: Open, want: "d"},
		{name: "open first app", src: fixed(a, b), target: pdf, verb: Open, want: "a"},
		{name: "open first app skips terminal apps", src: fixed(term, b), target: pdf, verb: Open, want: "b"},
		{name: "open nothing offered", src: fixed(), target: pdf, verb: Open, wantErr: ErrChoose},
		{name: "open saved not offered falls back", src: fixed(a, def), prefs: prefsWith(Open, "pdf", "x"), target: pdf, verb: Open, want: "d", fallback: true},
		{name: "open saved hidden falls back", src: fixed(a, def), prefs: prefsWith(Open, "pdf", "a", "a"), target: pdf, verb: Open, want: "d", fallback: true},
		{name: "open saved other verb ignored", src: fixed(a, def), prefs: prefsWith(Edit, "pdf", "a"), target: pdf, verb: Open, want: "d"},
		{name: "open hidden default is skipped", src: fixed(a, def), prefs: prefsWith(Open, "z", "z", "d"), target: pdf, verb: Open, want: "a"},
		{name: "open no preferences", src: fixed(def), prefs: Preferences{}, target: pdf, verb: Open, want: "d"},
		{name: "open nil verb maps", src: fixed(def), prefs: Preferences{Defaults: map[Verb]map[string]string{Open: nil, Edit: nil}}, target: pdf, verb: Open, want: "d"},
		{name: "open nil maps, nothing offered", src: fixed(), prefs: Preferences{Defaults: map[Verb]map[string]string{Open: nil}}, target: pdf, verb: Open, wantErr: ErrChoose},
		{name: "open saved id canonicalized", src: canonSource{fixed(a, def), map[string]string{"old": "a"}}, prefs: prefsWith(Open, "pdf", "old"), target: pdf, verb: Open, want: "a"},
		{name: "open hidden id canonicalized", src: canonSource{fixed(a, def), map[string]string{"old": "d"}}, prefs: prefsWith(Open, "z", "z", "old"), target: pdf, verb: Open, want: "a"},
		{name: "open source error", src: &askSource{err: errBoom}, target: pdf, verb: Open, wantErr: errBoom},

		{name: "edit saved", src: fixed(a, term, def), prefs: prefsWith(Edit, ".txt", "a"), target: txt, verb: Edit, want: "a"},
		{name: "edit text uses editor over default", src: fixed(a, def, term), target: txt, verb: Edit, want: TerminalEditorID},
		{name: "edit binary skips editor, uses default", src: fixed(a, def, term), target: bin, verb: Edit, want: "d"},
		{name: "edit binary with only the editor", src: fixed(term), target: bin, verb: Edit, wantErr: ErrChoose},
		{name: "edit default", src: fixed(a, def), target: txt, verb: Edit, want: "d"},
		{name: "edit apps but no default", src: fixed(a, b), target: txt, verb: Edit, wantErr: ErrChoose},
		{name: "edit nothing offered", src: fixed(), target: txt, verb: Edit, wantErr: ErrNoEditor},
		{name: "edit saved gone, editor", src: fixed(a, term), prefs: prefsWith(Edit, ".txt", "x"), target: txt, verb: Edit, want: TerminalEditorID, fallback: true},
		{name: "edit empty preferences", src: fixed(term), prefs: Preferences{}, target: txt, verb: Edit, want: TerminalEditorID},
		{name: "edit nil verb maps", src: fixed(term), prefs: Preferences{Defaults: map[Verb]map[string]string{Edit: nil}}, target: txt, verb: Edit, want: TerminalEditorID},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewResolver(tc.src, tc.prefs)
			got, err := r.Resolve(context.Background(), tc.target, tc.verb)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.App.ID != tc.want || got.Fallback != tc.fallback {
				t.Fatalf("Resolve = %q fallback %v, want %q fallback %v", got.App.ID, got.Fallback, tc.want, tc.fallback)
			}
		})
	}
}

func TestResolveErrorsNameTheFile(t *testing.T) {
	r := NewResolver(fixed(), Preferences{})
	_, err := r.Resolve(context.Background(), Target{Path: "/x/report.pdf"}, Open)
	if !errors.Is(err, ErrChoose) || !strings.Contains(err.Error(), "report.pdf") {
		t.Fatalf("err = %v, want ErrChoose naming report.pdf", err)
	}
	_, err = r.Resolve(context.Background(), Target{Path: "/x/notes.txt", Text: true}, Edit)
	if !errors.Is(err, ErrNoEditor) || !strings.Contains(err.Error(), "notes.txt") {
		t.Fatalf("err = %v, want ErrNoEditor naming notes.txt", err)
	}
}

func TestResolveSavedAppGone(t *testing.T) {
	src := fixed(App{ID: "d", Name: "D", Default: true}, App{ID: "a", Name: "A"})
	r := NewResolver(src, prefsWith(Open, "pdf", "x"))
	got, err := r.Resolve(context.Background(), Target{Path: "/x/a.pdf", Kind: "pdf"}, Open)
	if err != nil {
		t.Fatal(err)
	}
	if got.App.ID != "d" || !got.Fallback {
		t.Fatalf("got %q fallback %v, want d fallback true", got.App.ID, got.Fallback)
	}
}

func editorSource(t *testing.T) Source {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", exe)
	return Layered(List(TerminalEditor()))
}

func TestEditBinaryNeverUsesEditor(t *testing.T) {
	r := NewResolver(editorSource(t), Preferences{})
	_, err := r.Resolve(context.Background(), Target{Path: tempFile(t, "a.png"), Text: false}, Edit)
	if !errors.Is(err, ErrNoEditor) {
		t.Fatalf("err = %v, want ErrNoEditor", err)
	}
}

func TestEditTextUsesEditor(t *testing.T) {
	r := NewResolver(editorSource(t), Preferences{})
	got, err := r.Resolve(context.Background(), Target{Path: tempFile(t, "a.txt"), Text: true}, Edit)
	if err != nil {
		t.Fatal(err)
	}
	if got.App.ID != TerminalEditorID || got.Fallback || !got.App.Terminal {
		t.Fatalf("got %+v", got)
	}
}

func TestEditSavedBeatsEditor(t *testing.T) {
	src := Layered(editorSource(t), List(ListApp{ID: "gui", Name: "GUI", Verbs: []Verb{Edit}, Command: []string{"gui"}}))
	stubLookPath(t, "gui")
	r := NewResolver(src, prefsWith(Edit, ".txt", "gui"))
	got, err := r.Resolve(context.Background(), Target{Path: tempFile(t, "a.txt"), Text: true}, Edit)
	if err != nil {
		t.Fatal(err)
	}
	if got.App.ID != "gui" || got.Fallback {
		t.Fatalf("got %+v, want gui", got)
	}
}

func TestChoicesHiddenAndOrder(t *testing.T) {
	src := fixed(
		App{ID: "t", Name: "T", Terminal: true},
		App{ID: "a", Name: "A"},
		App{ID: "h", Name: "H"},
		App{ID: "d", Name: "D", Default: true},
		App{ID: "b", Name: "B"},
	)
	tgt := Target{Path: "/x/a.pdf"}
	r := NewResolver(src, prefsWith(Open, ".z", "z", "h"))
	got, err := r.Choices(context.Background(), tgt, Open)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "d", "b", "t"}; !slices.Equal(ids(got), want) {
		t.Fatalf("order = %v, want %v", ids(got), want)
	}
	for _, app := range got {
		if app.Default != (app.ID == "d") {
			t.Fatalf("Default on %q = %v", app.ID, app.Default)
		}
	}

	// Default follows what Resolve picks: the saved app, not the source's.
	r = NewResolver(src, prefsWith(Open, ".pdf", "b", "h"))
	got, err = r.Choices(context.Background(), tgt, Open)
	if err != nil {
		t.Fatal(err)
	}
	for _, app := range got {
		if app.Default != (app.ID == "b") {
			t.Fatalf("saved: Default on %q = %v", app.ID, app.Default)
		}
	}
	chosen, _ := r.Resolve(context.Background(), tgt, Open)
	if chosen.App.ID != "b" {
		t.Fatalf("Resolve = %q", chosen.App.ID)
	}

	// No default anywhere (Edit with several apps and none chosen): none marked.
	r = NewResolver(fixed(App{ID: "a", Name: "A", Default: true}, App{ID: "b", Name: "B"}), Preferences{})
	r.prefs.SetHidden("a", true)
	got, _ = r.Choices(context.Background(), Target{Path: "/x/a.txt", Text: true}, Edit)
	if want := []string{"b"}; !slices.Equal(ids(got), want) || got[0].Default {
		t.Fatalf("got %+v", got)
	}
}

func TestChoicesError(t *testing.T) {
	r := NewResolver(&askSource{err: errBoom}, Preferences{})
	if _, err := r.Choices(context.Background(), Target{Path: "/x/a.pdf"}, Open); err == nil {
		t.Fatal("want error")
	}
}

func TestSetDefaultRefusesUnoffered(t *testing.T) {
	r := NewResolver(fixed(App{ID: "a", Name: "A"}, App{ID: "h", Name: "H"}), Preferences{})
	r.prefs.SetHidden("h", true)
	tgt := Target{Path: "/x/a.pdf"}
	ctx := context.Background()
	for _, id := range []string{"nope", "h", TerminalEditorID} {
		if err := r.SetDefault(ctx, tgt, Open, id); !errors.Is(err, ErrNotOffered) {
			t.Fatalf("SetDefault(%q) = %v, want ErrNotOffered", id, err)
		}
	}
	if got := r.Preferences().Default(Open, ".pdf"); got != "" {
		t.Fatalf("saved %q after refusals", got)
	}
	if err := r.SetDefault(ctx, tgt, Open, "a"); err != nil {
		t.Fatal(err)
	}
	if got := r.Preferences().Default(Open, ".pdf"); got != "a" {
		t.Fatalf("saved %q, want a", got)
	}
	// A target with no key has nothing to save under.
	if err := r.SetDefault(ctx, Target{Path: "/x/Makefile"}, Open, "a"); err == nil {
		t.Fatal("want error for a target with no type")
	}
}

func TestSetDefaultUsesKeyAndCanonicalID(t *testing.T) {
	src := canonSource{fixed(App{ID: "a", Name: "A"}), map[string]string{"old": "a"}}
	r := NewResolver(src, Preferences{})
	if err := r.SetDefault(context.Background(), Target{Path: "/x/a.pdf", Kind: "pdf"}, Open, "old"); err != nil {
		t.Fatal(err)
	}
	if got := r.Preferences().Default(Open, "pdf"); got != "a" {
		t.Fatalf("saved %q under pdf, want a", got)
	}
}

func TestSetDefaultSystemClears(t *testing.T) {
	r := NewResolver(fixed(App{ID: "a", Name: "A"}), prefsWith(Open, ".pdf", "a"))
	tgt := Target{Path: "/x/a.pdf"}
	// system need not be offered to clear.
	if err := r.SetDefault(context.Background(), tgt, Open, SystemID); err != nil {
		t.Fatal(err)
	}
	if got := r.Preferences().Default(Open, ".pdf"); got != "" {
		t.Fatalf("saved %q, want cleared", got)
	}
}

func TestSetHidden(t *testing.T) {
	r := NewResolver(fixed(), Preferences{})
	for _, id := range []string{SystemID, TerminalEditorID} {
		if err := r.SetHidden(id, true); err == nil {
			t.Fatalf("SetHidden(%q) succeeded", id)
		}
	}
	if err := r.SetHidden("a", true); err != nil {
		t.Fatal(err)
	}
	if !r.Preferences().IsHidden("a") {
		t.Fatal("a not hidden")
	}
	if err := r.SetHidden("a", false); err != nil {
		t.Fatal(err)
	}
	if r.Preferences().IsHidden("a") {
		t.Fatal("a still hidden")
	}
	// Showing is allowed for any ID, so a file that hid them can be repaired.
	if err := r.SetHidden(SystemID, false); err != nil {
		t.Fatal(err)
	}
}

func TestHiddenSystemAndEditorStayOffered(t *testing.T) {
	// A hand-edited file may hide them; they are still offered.
	term := App{ID: TerminalEditorID, Name: "Edit", Terminal: true}
	sys := App{ID: SystemID, Name: "System", Default: true}
	p := Preferences{Version: 1, Hidden: []string{SystemID, TerminalEditorID}}
	r := NewResolver(fixed(sys, term), p)
	got, err := r.Choices(context.Background(), Target{Path: "/x/a.txt", Text: true}, Edit)
	if err != nil || len(got) != 2 {
		t.Fatalf("Choices = %+v, %v", got, err)
	}
}

func TestPreferencesAreCopied(t *testing.T) {
	in := prefsWith(Open, ".pdf", "a", "h")
	r := NewResolver(fixed(), in)

	// Mutating what was passed in does not reach the resolver.
	in.SetDefault(Open, ".pdf", "changed")
	in.SetDefault(Open, ".png", "new")
	in.Hidden[0] = "zzz"
	in.SetHidden("extra", true)
	got := r.Preferences()
	if got.Default(Open, ".pdf") != "a" || got.Default(Open, ".png") != "" || !got.IsHidden("h") || got.IsHidden("extra") || got.IsHidden("zzz") {
		t.Fatalf("resolver saw caller's edits: %+v", got)
	}

	// Mutating what was returned does not reach the resolver.
	got.SetDefault(Open, ".pdf", "changed")
	got.SetDefault(Open, ".gif", "new")
	got.Hidden[0] = "zzz"
	got.SetHidden("extra", true)
	again := r.Preferences()
	want := prefsWith(Open, ".pdf", "a", "h")
	if !reflect.DeepEqual(again, want) {
		t.Fatalf("resolver changed through a copy: %+v, want %+v", again, want)
	}
}

func TestPreferencesEmptyCopy(t *testing.T) {
	r := NewResolver(fixed(), Preferences{})
	p := r.Preferences()
	p.SetDefault(Open, ".pdf", "a")
	if r.Preferences().Default(Open, ".pdf") != "" {
		t.Fatal("copy of empty preferences shares maps")
	}
}

func TestAllApps(t *testing.T) {
	src := &askSource{ask: func(tgt Target, v Verb) []App {
		switch {
		case tgt.Kind == "pdf" && tgt.Ext == ".pdf" && v == Open:
			return []App{{ID: SystemID, Name: "System", Default: true}, {ID: "zed", Name: "Zed"}, {ID: "b", Name: "bee"}}
		case tgt.Kind == "text" && tgt.Ext == ".txt" && v == Open:
			return []App{{ID: "zed", Name: "Zed"}, {ID: "c", Name: "Cee"}}
		case tgt.Kind == "text" && v == Edit:
			return []App{{ID: TerminalEditorID, Name: "Edit in terminal", Terminal: true}, {ID: "a", Name: "Aye", Terminal: true}}
		}
		return nil
	}}
	r := NewResolver(src, Preferences{})
	r.prefs.SetHidden("c", true)
	got, err := r.AllApps(context.Background(), map[string]string{"pdf": ".pdf", "text": ".txt"})
	if err != nil {
		t.Fatal(err)
	}
	// Sorted by Name without regard to case; hidden "c" included; system and terminal-editor excluded;
	// "zed" once.
	if want := []string{"a", "b", "c", "zed"}; !slices.Equal(ids(got), want) {
		t.Fatalf("AllApps = %v, want %v", ids(got), want)
	}
	for _, app := range got {
		if app.Default {
			t.Fatalf("%q marked Default", app.ID)
		}
	}
	if got, err := r.AllApps(context.Background(), nil); err != nil || len(got) != 0 {
		t.Fatalf("no types: %v, %v", got, err)
	}
	r = NewResolver(&askSource{err: errBoom}, Preferences{})
	if _, err := r.AllApps(context.Background(), map[string]string{"pdf": ".pdf"}); err == nil {
		t.Fatal("want error")
	}
}

func TestResolverLaunch(t *testing.T) {
	src := fixed(App{ID: "a", Name: "A"})
	src.cmd = exec.Command("true")
	r := NewResolver(src, Preferences{})
	path := tempFile(t, "a.txt")
	cmd, err := r.Launch(context.Background(), Target{Path: path}, Open, App{ID: "a"})
	if err != nil || cmd != src.cmd || !slices.Equal(src.launched, []string{"a"}) {
		t.Fatalf("Launch = %v, %v, launched %v", cmd, err, src.launched)
	}
	// A bad path never reaches the source.
	for _, tgt := range []Target{
		{Path: ""},
		{Path: path + "\x00"},
		{Path: path + ".missing"},
		{Path: os.TempDir()},
		{Path: path, Directory: true},
	} {
		if _, err := r.Launch(context.Background(), tgt, Open, App{ID: "a"}); err == nil {
			t.Fatalf("Launch(%q, dir %v) succeeded", tgt.Path, tgt.Directory)
		}
	}
	if len(src.launched) != 1 {
		t.Fatalf("source launched %v", src.launched)
	}
	// A directory is accepted when asked for.
	if _, err := r.Launch(context.Background(), Target{Path: os.TempDir(), Directory: true}, Open, App{ID: "a"}); err != nil {
		t.Fatal(err)
	}
}

func TestResolverConcurrent(t *testing.T) {
	r := NewResolver(fixed(App{ID: "a", Name: "A"}, App{ID: "b", Name: "B"}), Preferences{})
	tgt := Target{Path: "/x/a.pdf"}
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			for range 50 {
				switch i % 4 {
				case 0:
					_ = r.SetDefault(ctx, tgt, Open, "a")
				case 1:
					_ = r.SetHidden("b", i%8 == 1)
				case 2:
					_, _ = r.Resolve(ctx, tgt, Open)
				default:
					_ = r.Preferences()
					_, _ = r.Choices(ctx, tgt, Open)
				}
			}
		}()
	}
	wg.Wait()
}

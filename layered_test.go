package ntopenwith

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"testing"
)

// fakeSource is a Source with fixed apps that records its launches.
type fakeSource struct {
	apps     []App
	err      error
	launched []string
	launchOf *exec.Cmd
}

func (f *fakeSource) Apps(context.Context, Target, Verb) ([]App, error) {
	return append([]App(nil), f.apps...), f.err
}

func (f *fakeSource) Launch(_ context.Context, _ Target, _ Verb, app App) (*exec.Cmd, error) {
	f.launched = append(f.launched, app.ID)
	return f.launchOf, nil
}

var layerTarget = Target{Path: "/x/a.txt", Text: true}

func TestLayeredMergesAndCorrects(t *testing.T) {
	a := &fakeSource{apps: []App{{ID: "a1", Name: "A1", Default: true}, {ID: "q", Name: "Q"}}}
	stubLookPath(t, "bee", "q")
	b := List(
		ListApp{ID: "q", Name: "Q!", Verbs: []Verb{Open}, Command: []string{"q"}},
		ListApp{ID: "b1", Name: "B1", Verbs: []Verb{Open}, Command: []string{"bee"}},
	)

	// Without corrections: a repeated ID only takes the later Name.
	got, err := Layered(a, b).Apps(context.Background(), layerTarget, Open)
	if err != nil {
		t.Fatal(err)
	}
	want := []App{{ID: "a1", Name: "A1", Default: true}, {ID: "q", Name: "Q!"}, {ID: "b1", Name: "B1"}}
	if !equalApps(got, want) {
		t.Fatalf("merged = %+v, want %+v", got, want)
	}

	got, err = Layered(a, b, Hide("q"), Rename("b1", "Bee"), Rank("b1")).Apps(context.Background(), layerTarget, Open)
	if err != nil {
		t.Fatal(err)
	}
	want = []App{{ID: "a1", Name: "A1", Default: true}, {ID: "b1", Name: "Bee"}}
	if !equalApps(got, want) {
		t.Fatalf("corrected = %+v, want %+v", got, want)
	}
}

// equalApps compares the public fields of two lists.
func equalApps(a, b []App) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Name != b[i].Name || a[i].Default != b[i].Default || a[i].Terminal != b[i].Terminal {
			return false
		}
	}
	return true
}

func TestLayeredOrder(t *testing.T) {
	src := &fakeSource{apps: []App{
		{ID: "t1", Name: "T1", Terminal: true},
		{ID: "x", Name: "X"},
		{ID: "d", Name: "D", Default: true},
		{ID: "y", Name: "Y"},
		{ID: "z", Name: "Z"},
		{ID: "t2", Name: "T2", Terminal: true},
	}}
	got, _ := Layered(src).Apps(context.Background(), layerTarget, Edit)
	if want := []string{"d", "x", "y", "z", "t1", "t2"}; !reflect.DeepEqual(ids(got), want) {
		t.Errorf("default and terminal last = %q, want %q", ids(got), want)
	}
	got, _ = Layered(src, Rank("z", "gone", "x"), Rank("d")).Apps(context.Background(), layerTarget, Edit)
	if want := []string{"d", "z", "x", "y", "t1", "t2"}; !reflect.DeepEqual(ids(got), want) {
		t.Errorf("ranked = %q, want %q", ids(got), want)
	}
	// Hiding the default leaves no default.
	got, _ = Layered(src, Hide("d")).Apps(context.Background(), layerTarget, Edit)
	for _, a := range got {
		if a.Default {
			t.Errorf("%s still default", a.ID)
		}
	}
}

func TestLayeredOverrideDefault(t *testing.T) {
	a := &fakeSource{apps: []App{{ID: "a1", Name: "A1", Default: true}, {ID: "a2", Name: "A2"}}}
	b := &fakeSource{apps: []App{{ID: "b1", Name: "B1", Default: true}}}
	got, _ := Layered(a, b).Apps(context.Background(), layerTarget, Open)
	if want := []string{"a1", "a2", "b1"}; !reflect.DeepEqual(ids(got), want) || !got[0].Default || got[2].Default {
		t.Errorf("first default = %+v", got)
	}
	got, _ = Layered(a, Override(b)).Apps(context.Background(), layerTarget, Open)
	if want := []string{"b1", "a1", "a2"}; !reflect.DeepEqual(ids(got), want) {
		t.Fatalf("override = %q, want %q", ids(got), want)
	}
	if !got[0].Default || got[1].Default {
		t.Errorf("override defaults = %+v", got)
	}
	// An override with no default of its own changes nothing.
	c := &fakeSource{apps: []App{{ID: "c1", Name: "C1"}}}
	got, _ = Layered(a, Override(c)).Apps(context.Background(), layerTarget, Open)
	if got[0].ID != "a1" || !got[0].Default {
		t.Errorf("no override default = %+v", got)
	}
	// A later source's default is used when the first has none.
	got, _ = Layered(c, b).Apps(context.Background(), layerTarget, Open)
	if got[0].ID != "b1" || !got[0].Default {
		t.Errorf("later default = %+v", got)
	}
}

func TestLayeredLaunchRoutesToSource(t *testing.T) {
	cmd := exec.Command("ed")
	a := &fakeSource{apps: []App{{ID: "a1", Name: "A1"}}}
	b := &fakeSource{apps: []App{{ID: "a1", Name: "again"}, {ID: "b1", Name: "B1"}}, launchOf: cmd}
	l := Layered(a, b, Hide("hidden"))
	ctx := context.Background()

	apps, _ := l.Apps(ctx, layerTarget, Open)
	byID := map[string]App{}
	for _, app := range apps {
		byID[app.ID] = app
	}
	if _, err := l.Launch(ctx, layerTarget, Open, byID["b1"]); err != nil {
		t.Fatal(err)
	}
	if got, err := l.Launch(ctx, layerTarget, Open, byID["a1"]); err != nil || got != nil {
		t.Fatalf("a1: %v, %v", got, err)
	}
	if !reflect.DeepEqual(a.launched, []string{"a1"}) || !reflect.DeepEqual(b.launched, []string{"b1"}) {
		t.Errorf("a got %q, b got %q", a.launched, b.launched)
	}

	// An App built by hand, with no source recorded, goes to the first source offering it.
	got, err := l.Launch(ctx, layerTarget, Open, App{ID: "b1"})
	if err != nil || got != cmd {
		t.Errorf("by ID: %v, %v", got, err)
	}
	if len(b.launched) != 2 {
		t.Errorf("b got %q", b.launched)
	}
	if _, err := l.Launch(ctx, layerTarget, Open, App{ID: "nobody"}); !errors.Is(err, ErrNotOffered) {
		t.Errorf("unknown: %v", err)
	}
	if _, err := Layered(a, Hide("a1")).Launch(ctx, layerTarget, Open, App{ID: "a1"}); !errors.Is(err, ErrNotOffered) {
		t.Errorf("hidden: %v", err)
	}
}

func TestLayeredNested(t *testing.T) {
	a := &fakeSource{apps: []App{{ID: "a1", Name: "A1"}}}
	outer := Layered(Layered(a), &fakeSource{})
	ctx := context.Background()
	apps, _ := outer.Apps(ctx, layerTarget, Open)
	if len(apps) != 1 {
		t.Fatalf("apps = %+v", apps)
	}
	if _, err := outer.Launch(ctx, layerTarget, Open, apps[0]); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.launched, []string{"a1"}) {
		t.Errorf("launched %q", a.launched)
	}
}

func TestLayeredErrors(t *testing.T) {
	boom := errors.New("boom")
	bad := &fakeSource{err: boom}
	good := &fakeSource{apps: []App{{ID: "g", Name: "G"}}}
	ctx := context.Background()

	got, err := Layered(bad, good).Apps(ctx, layerTarget, Open)
	if err != nil || len(got) != 1 {
		t.Errorf("one answered: %+v, %v", got, err)
	}
	if _, err := Layered(bad, &fakeSource{err: boom}).Apps(ctx, layerTarget, Open); !errors.Is(err, boom) {
		t.Errorf("none answered: %v", err)
	}
	if got, err := Layered().Apps(ctx, layerTarget, Open); err != nil || len(got) != 0 {
		t.Errorf("empty: %+v, %v", got, err)
	}
}

func TestAliasCanonical(t *testing.T) {
	target := &fakeSource{apps: []App{{ID: "mac:com.microsoft.VSCode", Name: "Visual Studio Code"}}}
	l := Layered(target, Alias("vscode", "mac:com.microsoft.VSCode"))
	c, ok := l.(Canonicalizer)
	if !ok {
		t.Fatal("Layered value is not a Canonicalizer")
	}
	if got := c.Canonical("vscode"); got != "mac:com.microsoft.VSCode" {
		t.Errorf("Canonical(vscode) = %q", got)
	}
	if got := c.Canonical("other"); got != "other" {
		t.Errorf("Canonical(other) = %q", got)
	}

	apps, _ := l.Apps(context.Background(), layerTarget, Open)
	if want := []string{"mac:com.microsoft.VSCode"}; !reflect.DeepEqual(ids(apps), want) {
		t.Errorf("alias added an app: %q", ids(apps))
	}
	if _, err := l.Launch(context.Background(), layerTarget, Open, App{ID: "vscode"}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"mac:com.microsoft.VSCode"}; !reflect.DeepEqual(target.launched, want) {
		t.Errorf("launched %q, want %q", target.launched, want)
	}
}

// wrapper is a user's Source around another, forwarding both calls.
type wrapper struct{ Source }

func TestLayeredWrappedLaunches(t *testing.T) {
	ctx := context.Background()
	for name, build := range map[string]func(a Source) Source{
		"Override":       func(a Source) Source { return Layered(Override(Layered(a))) },
		"user wrapper":   func(a Source) Source { return Layered(wrapper{Layered(a)}) },
		"double nesting": func(a Source) Source { return Layered(Override(Layered(wrapper{Layered(a)}))) },
	} {
		a := &fakeSource{apps: []App{{ID: "a1", Name: "A1", Default: true}}}
		l := build(a)
		apps, err := l.Apps(ctx, layerTarget, Open)
		if err != nil || len(apps) != 1 {
			t.Fatalf("%s: Apps = %+v, %v", name, apps, err)
		}
		if _, err := l.Launch(ctx, layerTarget, Open, apps[0]); err != nil {
			t.Fatalf("%s: Launch: %v", name, err)
		}
		if !reflect.DeepEqual(a.launched, []string{"a1"}) {
			t.Errorf("%s: launched %q", name, a.launched)
		}
	}
}

func TestLayeredIgnoresStaleApp(t *testing.T) {
	ctx := context.Background()
	a := &fakeSource{apps: []App{{ID: "a1", Name: "A1"}}}
	apps, _ := Layered(a).Apps(ctx, layerTarget, Open)
	if len(apps) != 1 {
		t.Fatalf("apps = %+v", apps)
	}
	hidden := Layered(a, Hide("a1"))
	if _, err := hidden.Launch(ctx, layerTarget, Open, apps[0]); !errors.Is(err, ErrNotOffered) {
		t.Errorf("stale app through Hide: %v, want ErrNotOffered", err)
	}
	// An App from another Layered whose sources this one lacks.
	b := &fakeSource{apps: []App{{ID: "b1", Name: "B1"}}}
	other, _ := Layered(b).Apps(ctx, layerTarget, Open)
	if _, err := Layered(a).Launch(ctx, layerTarget, Open, other[0]); !errors.Is(err, ErrNotOffered) {
		t.Errorf("foreign app: %v, want ErrNotOffered", err)
	}
	if len(a.launched) != 0 || len(b.launched) != 0 {
		t.Errorf("launched a %q, b %q", a.launched, b.launched)
	}
}

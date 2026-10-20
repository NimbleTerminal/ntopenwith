package main

import (
	"context"
	"errors"
	"os/exec"
	"path"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	tea "charm.land/bubbletea/v2"
	"nimbleterminal.dev/ntfilepicker"
	"nimbleterminal.dev/ntopenwith"
)

// driver runs a model as Bubble Tea would, but one message at a time: each
// command is run and its message fed back until none is left.
type driver struct {
	t *testing.T
	m model
}

func newDriver(t *testing.T, m model) *driver {
	t.Helper()
	d := &driver{t: t, m: m}
	d.run(m.Init())
	d.send(tea.WindowSizeMsg{Width: 100, Height: 30})
	return d
}

func pretendDriver(t *testing.T, prefs ntopenwith.Preferences) *driver {
	t.Helper()
	c := pretendConfig(ntfilepicker.LayoutList)
	c.prefs = prefs
	return newDriver(t, newModel(c))
}

func (d *driver) send(msg tea.Msg) {
	d.t.Helper()
	next, cmd := d.m.Update(msg)
	d.m = next.(model)
	d.run(cmd)
}

// run runs a command and feeds back what it returns. A command that takes
// longer than a second (a timer) is dropped.
func (d *driver) run(cmd tea.Cmd) {
	d.t.Helper()
	if cmd == nil {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(time.Second):
		return
	}
	switch msg := msg.(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			d.run(c)
		}
	default:
		d.send(msg)
	}
}

// press sends keys by name, as tea.KeyPressMsg.String spells them.
func (d *driver) press(keys ...string) {
	d.t.Helper()
	for _, k := range keys {
		d.send(keyPress(k))
	}
}

func keyPress(name string) tea.KeyPressMsg {
	var k tea.Key
	parts := strings.Split(name, "+")
	for _, mod := range parts[:len(parts)-1] {
		switch mod {
		case "ctrl":
			k.Mod |= tea.ModCtrl
		case "alt":
			k.Mod |= tea.ModAlt
		}
	}
	switch last := parts[len(parts)-1]; last {
	case "enter":
		k.Code = tea.KeyEnter
	case "esc":
		k.Code = tea.KeyEscape
	case "tab":
		k.Code = tea.KeyTab
	case "up":
		k.Code = tea.KeyUp
	case "down":
		k.Code = tea.KeyDown
	default:
		k.Code = []rune(last)[0]
		if k.Mod == 0 {
			k.Text = last
		}
	}
	return tea.KeyPressMsg(k)
}

// highlight puts the chooser's cursor on a file of the pretend home.
func (d *driver) highlight(rel string) {
	d.t.Helper()
	p := path.Join(pretendHome, rel)
	d.run(d.m.picker.GoTo(path.Dir(p)))
	d.run(d.m.picker.SetFilter(path.Base(p)))
	if got := d.m.picker.CurrentPath(); got != p {
		d.t.Fatalf("highlighted %q, want %q", got, p)
	}
}

func (d *driver) status(want string) {
	d.t.Helper()
	if got := d.m.status; !strings.Contains(got, want) {
		d.t.Fatalf("status %q, want it to contain %q", got, want)
	}
}

func (d *driver) chooser() *chooser {
	d.t.Helper()
	if d.m.chooser == nil {
		d.t.Fatalf("no chooser open; status %q", d.m.status)
	}
	return d.m.chooser
}

func names(apps []ntopenwith.App) []string {
	var out []string
	for _, a := range apps {
		out = append(out, a.Name)
	}
	return out
}

// moveTo moves the open chooser's cursor down to the app with this name.
func (d *driver) moveTo(name string) {
	d.t.Helper()
	c := d.chooser()
	i := slices.IndexFunc(c.apps, func(a ntopenwith.App) bool { return a.Name == name })
	if i < 0 {
		d.t.Fatalf("%s not in %v", name, names(c.apps))
	}
	for d.chooser().cursor < i {
		d.press("down")
	}
	for d.chooser().cursor > i {
		d.press("up")
	}
}

func TestEnterOpensWithTheDefault(t *testing.T) {
	d := pretendDriver(t, ntopenwith.Preferences{})
	d.highlight("Documents/report.pdf")
	d.press("enter")
	d.status("Would open report.pdf with Preview (pretend)")
	if d.m.chooser != nil {
		t.Fatal("chooser opened")
	}
}

func TestOpenWithMarksTheDefault(t *testing.T) {
	d := pretendDriver(t, ntopenwith.Preferences{})
	d.highlight("Documents/report.pdf")
	d.press("alt+enter")
	c := d.chooser()
	if c.verb != ntopenwith.Open || path.Base(c.target.Path) != "report.pdf" {
		t.Fatalf("chooser for %s %s", c.verb, c.target.Path)
	}
	if got := c.apps[c.cursor]; got.Name != "Preview" || !got.Default {
		t.Fatalf("cursor on %+v, want the default Preview", got)
	}
	if view := d.m.View().Content; !strings.Contains(view, "Open report.pdf with") {
		t.Fatalf("header missing from:\n%s", view)
	}
	d.press("esc")
	if d.m.chooser != nil {
		t.Fatal("Esc left the chooser open")
	}
}

func TestChooserMovesAndLaunches(t *testing.T) {
	d := pretendDriver(t, ntopenwith.Preferences{})
	d.highlight("Documents/report.pdf")
	d.press("alt+enter", "j")
	if got := d.chooser().cursor; got != 1 {
		t.Fatalf("j moved to %d", got)
	}
	d.press("k", "down", "enter")
	if d.m.chooser != nil {
		t.Fatal("Enter left the chooser open")
	}
	d.status("Would open report.pdf with " + d.lastApp(t))
}

// lastApp is the name of the app the pretend source last launched.
func (d *driver) lastApp(t *testing.T) string {
	launched := d.m.pretend.Launched()
	if len(launched) == 0 {
		t.Fatal("nothing launched")
	}
	id := strings.Fields(launched[len(launched)-1])[1]
	for _, apps := range pretendApps {
		for _, a := range apps {
			if a.id == id {
				return a.name
			}
		}
	}
	if id == ntopenwith.SystemID {
		return "System default"
	}
	return id
}

func TestSavedDefaultIsUsed(t *testing.T) {
	d := pretendDriver(t, ntopenwith.Preferences{})
	d.highlight("Pictures/landscape.png")
	d.press("alt+enter")
	d.moveTo("Pixelmator Pro")
	d.press("d")
	d.status("Pixelmator Pro")
	c := d.chooser()
	if got := c.apps[slices.IndexFunc(c.apps, func(a ntopenwith.App) bool { return a.Default })]; got.Name != "Pixelmator Pro" {
		t.Fatalf("default now %s", got.Name)
	}
	if c.apps[c.cursor].Name != "Pixelmator Pro" {
		t.Fatalf("cursor moved to %s", c.apps[c.cursor].Name)
	}
	d.press("esc", "enter")
	d.status("Would open landscape.png with Pixelmator Pro (pretend)")

	// System default clears the choice, and the OS's default is back.
	d.press("alt+enter")
	d.moveTo("System default")
	d.press("d", "esc", "enter")
	d.status("Would open landscape.png with Preview (pretend)")
}

func TestEditTextUsesTheTerminalEditor(t *testing.T) {
	d := pretendDriver(t, ntopenwith.Preferences{})
	d.highlight("Documents/notes.md")
	d.press("ctrl+e")
	d.status("Would edit notes.md with Edit in terminal ($EDITOR)")
	d.status("terminal")
}

func TestEditWithLeavesQuickTimeOutForImages(t *testing.T) {
	d := pretendDriver(t, ntopenwith.Preferences{})
	d.highlight("Pictures/landscape.png")
	d.press("alt+e")
	c := d.chooser()
	if c.verb != ntopenwith.Edit {
		t.Fatalf("verb %s", c.verb)
	}
	if slices.Contains(names(c.apps), "QuickTime Player") {
		t.Fatalf("QuickTime offered to edit: %v", names(c.apps))
	}
	if !strings.Contains(d.m.View().Content, "Edit landscape.png with") {
		t.Fatal("header missing")
	}
	d.press("esc", "alt+enter")
	if !slices.Contains(names(d.chooser().apps), "QuickTime Player") {
		t.Fatalf("QuickTime not offered to open: %v", names(d.chooser().apps))
	}
}

func TestNoDefaultEditorOpensTheChooser(t *testing.T) {
	d := pretendDriver(t, ntopenwith.Preferences{})
	d.highlight("Models/bracket.stl")
	d.press("ctrl+e")
	c := d.chooser()
	if c.verb != ntopenwith.Edit || !slices.Contains(names(c.apps), "Bambu Studio") {
		t.Fatalf("chooser %s %v", c.verb, names(c.apps))
	}
	d.press("enter")
	d.status("Would edit bracket.stl with Bambu Studio (pretend)")
}

func TestNothingCanEdit(t *testing.T) {
	d := pretendDriver(t, ntopenwith.Preferences{})
	d.highlight("Music/Ferrite Hearts - Magnetic.mp3")
	d.press("ctrl+e")
	if d.m.chooser != nil {
		t.Fatal("chooser opened")
	}
	d.status("no app can edit")
}

func TestFallbackIsSaid(t *testing.T) {
	prefs := ntopenwith.Preferences{}
	prefs.SetDefault(ntopenwith.Open, ".pdf", "mac:com.example.Gone")
	d := pretendDriver(t, prefs)
	d.highlight("Documents/report.pdf")
	d.press("enter")
	d.status("Would open report.pdf with Preview (pretend)")
	d.status("saved app unavailable")
}

func TestFoldersAreNotEdited(t *testing.T) {
	d := pretendDriver(t, ntopenwith.Preferences{})
	d.highlight("Documents")
	d.press("ctrl+e")
	if d.m.chooser != nil {
		t.Fatal("chooser opened")
	}
	d.status("Folders cannot be edited")
	d.press("alt+enter")
	if c := d.chooser(); !c.target.Directory || c.apps[c.cursor].Name != "Finder" {
		t.Fatalf("folder chooser %+v", c)
	}
}

func TestTextTargets(t *testing.T) {
	for name, want := range map[string]bool{"a.md": true, "go.mod": true, "x.PY": true, "a.png": false, "README": false} {
		if got := isText(name); got != want {
			t.Errorf("isText(%q) = %v", name, got)
		}
	}
}

func TestPretendTree(t *testing.T) {
	var want []string
	for name := range pretendFiles {
		if !strings.HasSuffix(name, "/") {
			want = append(want, path.Join(pretendHome[1:], name))
		}
	}
	if err := fstest.TestFS(pretendTree(), want...); err != nil {
		t.Fatal(err)
	}
}

// terminalSource offers one terminal editor whose command is "true".
type terminalSource struct{}

func (terminalSource) Apps(context.Context, ntopenwith.Target, ntopenwith.Verb) ([]ntopenwith.App, error) {
	return []ntopenwith.App{{ID: "ed", Name: "Ed", Terminal: true}}, nil
}

func (terminalSource) Launch(context.Context, ntopenwith.Target, ntopenwith.Verb, ntopenwith.App) (*exec.Cmd, error) {
	return exec.Command("true"), nil
}

func TestNativeTerminalAppTakesTheTerminal(t *testing.T) {
	m := newModel(config{fsys: pretendTree(), home: pretendHome, source: terminalSource{}, layout: ntfilepicker.LayoutList})
	target := ntopenwith.Target{Path: "/home/ada/todo.txt", Text: true}
	app := ntopenwith.App{ID: "ed", Name: "Ed", Terminal: true}
	next, cmd := m.Update(launchedMsg{target: target, verb: ntopenwith.Edit, app: app, cmd: exec.Command("true")})
	m = next.(model)
	if cmd == nil || !strings.Contains(m.status, "Ed") {
		t.Fatalf("no exec command; status %q", m.status)
	}
	next, _ = m.Update(execDoneMsg{target: target, verb: ntopenwith.Edit, app: app})
	if got := next.(model).status; !strings.Contains(got, "Back from Ed") {
		t.Fatalf("status %q", got)
	}
	next, _ = m.Update(execDoneMsg{target: target, verb: ntopenwith.Edit, app: app, err: errors.New("exit status 1")})
	if got := next.(model).status; !strings.Contains(got, "exit status 1") {
		t.Fatalf("status %q", got)
	}
}

func TestNamesAreMadeSafe(t *testing.T) {
	if got := safe("evil\x1b[2Jname\n"); strings.ContainsAny(got, "\x1b\n") {
		t.Fatalf("safe left control characters: %q", got)
	}
}

func TestTabSwitchesVerb(t *testing.T) {
	d := pretendDriver(t, ntopenwith.Preferences{})
	d.highlight("Pictures/landscape.png")
	d.press("alt+enter", "tab")
	c := d.chooser()
	if c.verb != ntopenwith.Edit || path.Base(c.target.Path) != "landscape.png" {
		t.Fatalf("chooser for %s %s", c.verb, c.target.Path)
	}
	if slices.Contains(names(c.apps), "QuickTime Player") {
		t.Fatalf("QuickTime offered to edit: %v", names(c.apps))
	}
	view := d.m.View().Content
	if !strings.Contains(view, "Edit landscape.png with") || !strings.Contains(view, "Tab open with") {
		t.Fatalf("header or hint missing from:\n%s", view)
	}
	d.press("tab")
	c = d.chooser()
	if c.verb != ntopenwith.Open || !slices.Contains(names(c.apps), "QuickTime Player") {
		t.Fatalf("chooser for %s %v", c.verb, names(c.apps))
	}
	if view := d.m.View().Content; !strings.Contains(view, "Open landscape.png with") || !strings.Contains(view, "Tab edit with") {
		t.Fatalf("header or hint missing from:\n%s", view)
	}
}

func TestTabKeepsTheChooserWhenNothingCanEdit(t *testing.T) {
	d := pretendDriver(t, ntopenwith.Preferences{})
	d.highlight("Music/Ferrite Hearts - Magnetic.mp3")
	d.press("alt+enter", "tab")
	if c := d.chooser(); c.verb != ntopenwith.Open {
		t.Fatalf("verb %s", c.verb)
	}
	d.status("No app can edit")
	d.highlight("Documents")
	d.press("esc", "alt+enter", "tab")
	if c := d.chooser(); c.verb != ntopenwith.Open {
		t.Fatalf("verb %s", c.verb)
	}
	d.status("Folders cannot be edited")
}

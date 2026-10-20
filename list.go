package ntopenwith

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// ListApp is one app a program compiles in, with the files it is offered for.
type ListApp struct {
	// ID is the app's ID in preferences; it is the developer's own.
	ID string
	// Name is what to show a person.
	Name string
	// Verbs are the verbs the app is offered for.
	Verbs []Verb
	// Kinds are the Target.Kind or Target.Type values the app is offered for;
	// empty means every file.
	Kinds []string
	// Text offers the app only for text targets.
	Text bool
	// Command is the program and its arguments. An argument that is exactly
	// "{}" stands for the file's path; with none, the path is appended.
	Command []string
	// Terminal means Launch returns the command unstarted, for the caller to
	// run in its terminal.
	Terminal bool
	// Bundle is a macOS application bundle, such as "Preview.app". On darwin
	// the app is launched with "open -a" on the bundle found.
	Bundle string
	// Find names what must exist for the app to be offered: programs found on
	// the PATH, or bundles ending ".app" found in the Applications folders.
	// By default it is Bundle on darwin, else Command[0], else Bundle.
	Find []string
	// Default lists the verbs for which the app is the default.
	Default []Verb

	// command, when set, supplies the argv at the time of asking and launch,
	// and the app is available when it succeeds.
	command func() ([]string, error)
}

// Test hooks.
var (
	lookPath = exec.LookPath
	// bundleDirs lists the folders searched for application bundles.
	bundleDirs = func() []string {
		var dirs []string
		if home, err := os.UserHomeDir(); err == nil {
			dirs = append(dirs, filepath.Join(home, "Applications"))
		}
		return append(dirs, "/Applications", "/System/Applications")
	}
	hostOS = runtime.GOOS
)

// List returns a Source offering the given apps, each when its program is
// installed. Apps appear in the order given, with defaults first.
func List(apps ...ListApp) Source {
	return &list{apps: slices.Clone(apps)}
}

// list is a pointer type so that Sources stay comparable.
type list struct{ apps []ListApp }

// offered reports whether the app is offered for t and v, installed or not.
func (a *ListApp) offered(t Target, v Verb) bool {
	if !slices.Contains(a.Verbs, v) {
		return false
	}
	if a.Text && !t.Text {
		return false
	}
	if len(a.Kinds) == 0 {
		return true
	}
	return (t.Kind != "" && slices.Contains(a.Kinds, t.Kind)) ||
		(t.Type() != "" && slices.Contains(a.Kinds, t.Type()))
}

// available reports whether the app can be launched now.
func (a *ListApp) available() bool {
	if a.command != nil {
		_, err := a.command()
		return err == nil
	}
	find := a.Find
	if len(find) == 0 {
		switch {
		case a.Bundle != "" && hostOS == "darwin":
			find = []string{a.Bundle}
		case len(a.Command) > 0:
			find = []string{a.Command[0]}
		case a.Bundle != "":
			find = []string{a.Bundle}
		default:
			return false
		}
	}
	for _, name := range find {
		if !found(name) {
			return false
		}
	}
	return true
}

// found reports whether a program is on the PATH, or a ".app" bundle is in
// one of the Applications folders.
func found(name string) bool {
	if strings.HasSuffix(name, ".app") {
		return findBundle(name) != ""
	}
	_, err := lookPath(name)
	return err == nil
}

// findBundle returns the path of the named bundle, or "".
func findBundle(name string) string {
	if name == "" || name != filepath.Base(name) {
		return ""
	}
	for _, dir := range bundleDirs() {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}
	return ""
}

// argv returns the command to run, before the path is added.
func (a *ListApp) argv() ([]string, error) {
	if a.command != nil {
		return a.command()
	}
	return slices.Clone(a.Command), nil
}

func (l *list) Apps(ctx context.Context, t Target, v Verb) ([]App, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var first, rest []App
	for i := range l.apps {
		a := &l.apps[i]
		if !a.offered(t, v) || !a.available() {
			continue
		}
		app := App{ID: a.ID, Name: a.Name, Terminal: a.Terminal}
		if slices.Contains(a.Default, v) {
			app.Default = true
			first = append(first, app)
		} else {
			rest = append(rest, app)
		}
	}
	return append(first, rest...), nil
}

func (l *list) Launch(ctx context.Context, t Target, v Verb, app App) (*exec.Cmd, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Under js/wasm nothing runs: not even a terminal app is handed back.
	if runtime.GOOS == "js" {
		return nil, ErrUnsupported
	}
	var entry *ListApp
	for i := range l.apps {
		if l.apps[i].ID == app.ID {
			entry = &l.apps[i]
			break
		}
	}
	// Launch re-checks everything: an ID launches only if still offered.
	if entry == nil || !entry.offered(t, v) || !entry.available() {
		return nil, ErrNotOffered
	}
	path, err := CheckPath(t.Path, t.Directory)
	if err != nil {
		return nil, err
	}
	var argv []string
	if bundle := entry.Bundle; bundle != "" && hostOS == "darwin" {
		dir := findBundle(bundle)
		if dir == "" {
			return nil, ErrNotOffered
		}
		argv = []string{"open", "-a", dir}
	} else if argv, err = entry.argv(); err != nil {
		return nil, err
	}
	argv, err = Command(argv, path)
	if err != nil {
		return nil, err
	}
	if entry.Terminal {
		return exec.Command(argv[0], argv[1:]...), nil
	}
	return nil, Start(argv)
}

// TerminalEditor returns the app that edits text in the editor named by
// $VISUAL or else $EDITOR. The environment is read each time it is asked
// about or launched, and the app is offered only when the setting names an
// installed program.
func TerminalEditor() ListApp {
	return ListApp{
		ID:       TerminalEditorID,
		Name:     "Edit in terminal",
		Verbs:    []Verb{Edit},
		Text:     true,
		Terminal: true,
		command:  func() ([]string, error) { return editorArgs(editorSetting()) },
	}
}

// TerminalEditors returns apps for the common terminal editors, each offered
// to edit text when installed.
func TerminalEditors() []ListApp {
	editor := func(id, name string, command ...string) ListApp {
		if command == nil {
			command = []string{id}
		}
		return ListApp{ID: id, Name: name, Verbs: []Verb{Edit}, Text: true, Terminal: true, Command: command}
	}
	return []ListApp{
		editor("nvim", "Neovim"),
		editor("vim", "Vim"),
		editor("hx", "Helix"),
		editor("nano", "nano"),
		editor("micro", "micro"),
		editor("emacs", "Emacs", "emacs", "-nw"),
	}
}

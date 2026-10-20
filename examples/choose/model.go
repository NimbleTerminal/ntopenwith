package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"nimbleterminal.dev/ntfilepicker"
	"nimbleterminal.dev/ntopenwith"
)

// config is what a model is made from: the files to choose among and the
// apps to offer.
type config struct {
	fsys   fs.ReadDirFS // rooted at "/" of the chooser's slash paths
	home   string       // slash path to start in
	volume string       // prefixed to a slash path to make a real one ("C:" on Windows)
	source ntopenwith.Source
	prefs  ntopenwith.Preferences
	// save, when set, keeps the preferences after each change.
	save   func(ntopenwith.Preferences) error
	layout ntfilepicker.Layout
	// pretend, when set, is the pretend source under source: nothing is
	// launched and the status line says what would have been.
	pretend *pretend
}

// below is the rows under the file chooser: a rule, the status, the keys.
const below = 3

var (
	ruleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	hintStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
)

type model struct {
	resolver *ntopenwith.Resolver
	source   ntopenwith.Source
	save     func(ntopenwith.Preferences) error
	volume   string
	pretend  *pretend

	picker        ntfilepicker.Model
	width, height int
	status        string
	failed        bool     // the status is an error
	chooser       *chooser // the "Open with" or "Edit with" overlay, when open
}

func newModel(c config) model {
	picker := ntfilepicker.New(c.fsys, c.home,
		ntfilepicker.WithHome(c.home),
		ntfilepicker.WithLayout(c.layout),
		ntfilepicker.WithPrompt("  Choose a file: "),
	)
	m := model{
		resolver: ntopenwith.NewResolver(c.source, c.prefs),
		source:   c.source,
		save:     c.save,
		volume:   c.volume,
		pretend:  c.pretend,
		picker:   picker,
		width:    80,
		height:   24,
	}
	if m.pretend != nil {
		m.status = "A pretend home folder: nothing is launched."
	}
	return m
}

// Messages from the work done outside Update.
type (
	resolvedMsg struct {
		target ntopenwith.Target
		verb   ntopenwith.Verb
		choice ntopenwith.Choice
		err    error
	}
	choicesMsg struct {
		target ntopenwith.Target
		verb   ntopenwith.Verb
		apps   []ntopenwith.App
		keep   string // the app ID to put the cursor on, else the default
		// switching is true when Tab asked for the other verb's list: the
		// chooser stays as it was if that list is empty.
		switching bool
		err       error
	}
	launchedMsg struct {
		target   ntopenwith.Target
		verb     ntopenwith.Verb
		app      ntopenwith.App
		fallback bool
		cmd      *exec.Cmd // a terminal app, not yet started
		err      error
	}
	execDoneMsg struct {
		target ntopenwith.Target
		verb   ntopenwith.Verb
		app    ntopenwith.App
		err    error
	}
	defaultMsg struct {
		target ntopenwith.Target
		verb   ntopenwith.Verb
		app    ntopenwith.App
		err    error
	}
)

func (m model) Init() tea.Cmd { return m.picker.Init() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		msg.Height = max(1, msg.Height-below)
		var cmd tea.Cmd
		m.picker, cmd = m.picker.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.chooser != nil {
			return m.chooserKey(msg)
		}
		switch msg.String() {
		case "esc":
			// Esc clears the filter; with none, it leaves (not in pretend
			// mode, which in a browser has nowhere to leave to).
			if m.picker.FilterValue() == "" && m.pretend == nil {
				return m, tea.Quit
			}
			return m, m.picker.ClearFilter()
		case "ctrl+e":
			return m.onCurrent(ntopenwith.Edit, m.resolve)
		case "alt+e":
			return m.onCurrent(ntopenwith.Edit, m.choices)
		}
	case ntfilepicker.OpenWithMsg:
		return m, m.choices(m.target(msg.Path, msg.Directory), ntopenwith.Open, "")
	case resolvedMsg:
		return m.resolved(msg)
	case choicesMsg:
		return m.gotChoices(msg)
	case launchedMsg:
		return m.launched(msg)
	case execDoneMsg:
		name := safe(msg.app.Name)
		if msg.err != nil {
			m.say(true, "%s on %s: %v", name, m.name(msg.target), msg.err)
		} else {
			m.say(false, "Back from %s (%s)", name, m.name(msg.target))
		}
		return m, nil
	case defaultMsg:
		return m.defaultSet(msg)
	}
	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	if chosen, ok := m.picker.Chosen(); ok {
		m.picker.ClearChosen()
		return m, tea.Batch(cmd, m.resolve(m.target(chosen, false), ntopenwith.Open, ""))
	}
	return m, cmd
}

// onCurrent acts on the highlighted entry: a file, or for Open a folder.
func (m model) onCurrent(v ntopenwith.Verb, act func(ntopenwith.Target, ntopenwith.Verb, string) tea.Cmd) (tea.Model, tea.Cmd) {
	e := m.picker.Current()
	if e == nil || e.Name() == ".." {
		return m, nil
	}
	if e.IsDir() && v == ntopenwith.Edit {
		m.say(true, "Folders cannot be edited")
		return m, nil
	}
	return m, act(m.target(m.picker.CurrentPath(), e.IsDir()), v, "")
}

// target is what ntopenwith is told about the file at a chooser path.
func (m model) target(p string, dir bool) ntopenwith.Target {
	t := ntopenwith.Target{Path: p, Directory: dir, Text: !dir && isText(p)}
	if m.pretend == nil {
		t.Path = m.volume + filepath.FromSlash(p)
	}
	return t
}

// textExts are the extensions this program treats as text, so that a
// terminal editor may edit them. A host that reads files could look instead.
var textExts = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".go": true, ".mod": true, ".sum": true,
	".json": true, ".csv": true, ".tsv": true, ".html": true, ".css": true, ".xml": true,
	".py": true, ".js": true, ".ts": true, ".rb": true, ".rs": true, ".c": true, ".h": true,
	".sh": true, ".toml": true, ".yaml": true, ".yml": true, ".ini": true, ".log": true, ".sql": true,
}

func isText(name string) bool { return textExts[strings.ToLower(path.Ext(name))] }

func (m model) resolve(t ntopenwith.Target, v ntopenwith.Verb, _ string) tea.Cmd {
	r := m.resolver
	return func() tea.Msg {
		choice, err := r.Resolve(context.Background(), t, v)
		return resolvedMsg{target: t, verb: v, choice: choice, err: err}
	}
}

func (m model) choices(t ntopenwith.Target, v ntopenwith.Verb, keep string) tea.Cmd {
	r := m.resolver
	return func() tea.Msg {
		apps, err := r.Choices(context.Background(), t, v)
		return choicesMsg{target: t, verb: v, apps: apps, keep: keep, err: err}
	}
}

// launch starts app. Natively that is the Resolver's Launch, which checks the
// path. The pretend files are not on disk, so in pretend mode the source
// launches (Layered still refuses an app it does not offer now).
func (m model) launch(t ntopenwith.Target, v ntopenwith.Verb, app ntopenwith.App, fallback bool) tea.Cmd {
	r, src, pretend := m.resolver, m.source, m.pretend != nil
	return func() tea.Msg {
		var cmd *exec.Cmd
		var err error
		if pretend {
			cmd, err = src.Launch(context.Background(), t, v, app)
		} else {
			cmd, err = r.Launch(context.Background(), t, v, app)
		}
		return launchedMsg{target: t, verb: v, app: app, fallback: fallback, cmd: cmd, err: err}
	}
}

func (m model) resolved(msg resolvedMsg) (tea.Model, tea.Cmd) {
	switch {
	case errors.Is(msg.err, ntopenwith.ErrChoose):
		return m, m.choices(msg.target, msg.verb, "")
	case msg.err != nil:
		m.say(true, "%s", safe(msg.err.Error()))
		return m, nil
	}
	return m, m.launch(msg.target, msg.verb, msg.choice.App, msg.choice.Fallback)
}

func (m model) launched(msg launchedMsg) (tea.Model, tea.Cmd) {
	name, file := safe(msg.app.Name), m.name(msg.target)
	var note string
	if msg.fallback {
		note = " · saved app unavailable"
	}
	switch {
	case msg.err != nil:
		m.say(true, "Could not %s %s with %s: %s", msg.verb, file, name, safe(msg.err.Error()))
		return m, nil
	case m.pretend != nil && msg.app.Terminal:
		m.say(false, "Would %s %s with %s, which would take the terminal (pretend)%s", msg.verb, file, name, note)
	case m.pretend != nil:
		m.say(false, "Would %s %s with %s (pretend)%s", msg.verb, file, name, note)
	case msg.cmd != nil:
		// A terminal app: Bubble Tea hands it the terminal until it exits.
		m.say(false, "In %s (%s)%s", name, file, note)
		t, v, app := msg.target, msg.verb, msg.app
		return m, tea.ExecProcess(msg.cmd, func(err error) tea.Msg {
			return execDoneMsg{target: t, verb: v, app: app, err: err}
		})
	case msg.verb == ntopenwith.Edit:
		m.say(false, "Opened %s to edit in %s%s", file, name, note)
	default:
		m.say(false, "Opened %s with %s%s", file, name, note)
	}
	return m, nil
}

// name is a file's name, safe to draw.
func (m model) name(t ntopenwith.Target) string { return safe(filepath.Base(t.Path)) }

func (m *model) say(failed bool, format string, args ...any) {
	m.status, m.failed = fmt.Sprintf(format, args...), failed
}

// safe makes a name from the system or the file system safe to draw: control
// characters, escapes among them, become U+FFFD.
func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '�'
		}
		return r
	}, s)
}

func (m model) View() tea.View {
	w := max(m.width, 1)
	list := m.picker.View()
	if m.chooser != nil {
		list = m.chooser.over(list, w, max(1, m.height-below))
	}
	style := statusStyle
	if m.failed {
		style = errorStyle
	}
	hint := "Enter open · Ctrl+E edit · Alt+Enter open with (Tab: edit with) · Alt+E edit with · Ctrl+C quit"
	if m.chooser != nil {
		hint = "↑/↓ move · Enter " + string(m.chooser.verb) + " · Tab " + string(other(m.chooser.verb)) + " with · d make default · Esc close"
	}
	rule := ruleStyle.Render(strings.Repeat("─", w))
	v := tea.NewView(list + "\n" + rule + "\n" + style.Render(clip(m.status, w)) + "\n" + hintStyle.Render(clip(hint, w)))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// clip cuts s to width cells, with an ellipsis when it was longer.
func clip(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

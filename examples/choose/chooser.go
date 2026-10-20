package main

import (
	"context"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"nimbleterminal.dev/ntopenwith"
)

// chooser is the "Open with" or "Edit with" list, drawn over the files.
type chooser struct {
	target ntopenwith.Target
	verb   ntopenwith.Verb
	apps   []ntopenwith.App // from Resolver.Choices: the effective default marked
	cursor int
}

var (
	boxStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).Padding(0, 1)
	titleStyle  = lipgloss.NewStyle().Bold(true)
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	tagStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

func (m model) gotChoices(msg choicesMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.say(true, "%s", safe(msg.err.Error()))
		return m, nil
	}
	if msg.keep != "" && (m.chooser == nil || m.chooser.target != msg.target || m.chooser.verb != msg.verb) {
		return m, nil // a reload for a chooser since closed
	}
	if msg.switching && (m.chooser == nil || m.chooser.target != msg.target) {
		return m, nil // the chooser closed, or moved on, before the list came
	}
	if len(msg.apps) == 0 && msg.switching {
		m.say(true, "No app can %s %s", msg.verb, m.name(msg.target))
		return m, nil
	}
	if len(msg.apps) == 0 {
		m.chooser = nil
		m.say(true, "No app can %s %s", msg.verb, m.name(msg.target))
		return m, nil
	}
	c := &chooser{target: msg.target, verb: msg.verb, apps: msg.apps}
	keep := func(a ntopenwith.App) bool { return a.Default }
	if msg.keep != "" {
		keep = func(a ntopenwith.App) bool { return a.ID == msg.keep }
	}
	c.cursor = max(0, slices.IndexFunc(c.apps, keep))
	m.chooser = c
	return m, nil
}

func (m model) chooserKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	c := m.chooser
	switch msg.String() {
	case "esc":
		m.chooser = nil
	case "up", "k":
		c.cursor = (c.cursor + len(c.apps) - 1) % len(c.apps)
	case "down", "j":
		c.cursor = (c.cursor + 1) % len(c.apps)
	case "home":
		c.cursor = 0
	case "end":
		c.cursor = len(c.apps) - 1
	case "tab":
		// The same file, with the other verb's list.
		v := other(c.verb)
		if v == ntopenwith.Edit && c.target.Directory {
			m.say(true, "Folders cannot be edited")
			return m, nil
		}
		r, t := m.resolver, c.target
		return m, func() tea.Msg {
			apps, err := r.Choices(context.Background(), t, v)
			return choicesMsg{target: t, verb: v, apps: apps, err: err, switching: true}
		}
	case "enter":
		m.chooser = nil
		return m, m.launch(c.target, c.verb, c.apps[c.cursor], false)
	case "d":
		r, save, t, v, app := m.resolver, m.save, c.target, c.verb, c.apps[c.cursor]
		return m, func() tea.Msg {
			err := r.SetDefault(context.Background(), t, v, app.ID)
			if err == nil && save != nil {
				err = save(r.Preferences())
			}
			return defaultMsg{target: t, verb: v, app: app, err: err}
		}
	}
	return m, nil
}

func (m model) defaultSet(msg defaultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.say(true, "Could not save the default: %s", safe(msg.err.Error()))
		return m, nil
	}
	kind := safe(msg.target.Key())
	if msg.target.Directory {
		kind = "folders"
	}
	if msg.app.ID == ntopenwith.SystemID && msg.verb == ntopenwith.Open {
		m.say(false, "No saved app to open %s files: the system's default is used", kind)
	} else {
		m.say(false, "%s is now the app to %s %s files", safe(msg.app.Name), msg.verb, kind)
	}
	// Ask again, so that the mark moves to the app now chosen.
	return m, m.choices(msg.target, msg.verb, msg.app.ID)
}

// over draws the chooser in the middle of the screen above base.
func (c *chooser) over(base string, width, height int) string {
	verb := "Open"
	if c.verb == ntopenwith.Edit {
		verb = "Edit"
	}
	title := verb + " " + safe(baseName(c.target.Path)) + " with"
	// As wide as the longest row, within the screen.
	inner := lipgloss.Width(title)
	for _, a := range c.apps {
		inner = max(inner, 4+lipgloss.Width(safe(a.Name)+tag(a)))
	}
	inner = max(10, min(width-4, inner))
	lines := []string{titleStyle.Render(clip(title, inner)), ""}
	// Show as many rows as fit, keeping the cursor in view.
	rows := max(1, min(len(c.apps), height-4))
	first := min(max(0, c.cursor-rows/2), len(c.apps)-rows)
	for i := first; i < first+rows; i++ {
		a := c.apps[i]
		name := clip(safe(a.Name), max(1, inner-4-lipgloss.Width(tag(a))))
		star := "  "
		if a.Default {
			star = "★ "
		}
		if i == c.cursor {
			lines = append(lines, cursorStyle.Render("› "+star+name)+tagStyle.Render(tag(a)))
		} else {
			lines = append(lines, "  "+star+name+tagStyle.Render(tag(a)))
		}
	}
	box := boxStyle.Render(strings.Join(lines, "\n"))
	x := max(0, (width-lipgloss.Width(box))/2)
	y := max(0, (height-lipgloss.Height(box))/2)
	canvas := lipgloss.NewCanvas(width, height)
	canvas.Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(box).X(x).Y(y).Z(1),
	))
	return canvas.Render()
}

// other is the verb Tab switches to.
func other(v ntopenwith.Verb) ntopenwith.Verb {
	if v == ntopenwith.Open {
		return ntopenwith.Edit
	}
	return ntopenwith.Open
}

// tag notes a terminal app, which takes the terminal while it runs.
func tag(a ntopenwith.App) string {
	if a.Terminal {
		return "  (terminal)"
	}
	return ""
}

// baseName is the last element of a slash or OS path.
func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

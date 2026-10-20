package ntopenwith

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// Choice is the app a [Resolver] chose.
type Choice struct {
	App App
	// Fallback is true when a saved default existed but was skipped because
	// the app is hidden or no longer offered for the file.
	Fallback bool
}

// Resolver chooses the app for a target and verb from a [Source] and a
// person's [Preferences]. It is safe for concurrent use.
type Resolver struct {
	src   Source
	canon Canonicalizer // nil when src has none

	mu    sync.Mutex
	prefs Preferences
}

// NewResolver returns a Resolver over src. It keeps its own copy of prefs;
// later changes to the argument do not reach it. Read the current choices,
// to save them, with [Resolver.Preferences].
func NewResolver(src Source, prefs Preferences) *Resolver {
	r := &Resolver{src: src, prefs: prefs.clone()}
	r.canon, _ = src.(Canonicalizer)
	return r
}

// clone returns a copy that shares nothing with p.
func (p Preferences) clone() Preferences {
	c := p
	c.Hidden = slices.Clone(p.Hidden)
	if p.Defaults != nil {
		c.Defaults = make(map[Verb]map[string]string, len(p.Defaults))
		for v, m := range p.Defaults {
			c.Defaults[v] = maps.Clone(m)
		}
	}
	return c
}

// Preferences returns a copy of the current preferences.
func (r *Resolver) Preferences() Preferences {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.prefs.clone()
}

func (r *Resolver) canonical(id string) string {
	if r.canon == nil {
		return id
	}
	return r.canon.Canonical(id)
}

// protected reports whether id may not be hidden: the system handler and the
// terminal editor are always offered.
func protected(id string) bool { return id == SystemID || id == TerminalEditorID }

// snapshot returns the saved default for t and v (canonical) and the hidden
// IDs (canonical), without holding the lock afterwards.
func (r *Resolver) snapshot(t Target, v Verb) (saved string, hidden []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if id := r.prefs.Default(v, t.Key()); id != "" {
		saved = r.canonical(id)
	}
	for _, h := range r.prefs.Hidden {
		hidden = append(hidden, r.canonical(h))
	}
	return saved, hidden
}

// visible asks the source and drops hidden apps, and the terminal editor for
// a target that is not text, then puts terminal apps last, keeping the order
// otherwise.
func (r *Resolver) visible(ctx context.Context, t Target, v Verb, hidden []string) ([]App, error) {
	all, err := r.src.Apps(ctx, t, v)
	if err != nil {
		return nil, err
	}
	var apps, terminal []App
	for _, a := range all {
		if !protected(a.ID) && slices.Contains(hidden, a.ID) {
			continue
		}
		// Whatever the source says, $VISUAL/$EDITOR never edits a binary file.
		if a.ID == TerminalEditorID && v == Edit && !t.Text {
			continue
		}
		if a.Terminal {
			terminal = append(terminal, a)
		} else {
			apps = append(apps, a)
		}
	}
	return append(apps, terminal...), nil
}

// pick applies the rules to the visible apps. ok is false when none applies.
func pick(apps []App, t Target, v Verb, saved string) (app App, fallback, ok bool) {
	find := func(match func(App) bool) bool {
		if i := slices.IndexFunc(apps, match); i >= 0 {
			app, ok = apps[i], true
		}
		return ok
	}
	if saved != "" {
		if find(func(a App) bool { return a.ID == saved }) {
			return app, false, true
		}
		fallback = true
	}
	if v == Edit && t.Text && find(func(a App) bool { return a.ID == TerminalEditorID }) {
		return app, fallback, true
	}
	if find(func(a App) bool { return a.Default }) {
		return app, fallback, true
	}
	if v == Open && len(apps) > 0 {
		return apps[0], fallback, true
	}
	return App{}, fallback, false
}

// Resolve chooses the app to perform v on t. For Open: the saved default,
// else the source's default, else the first app offered. For Edit: the saved
// default, else for text the terminal editor, else the source's default. A
// saved default counts only if it is offered and not hidden. When nothing
// is chosen the error is [ErrChoose] if some app is offered, and for Edit
// with none offered [ErrNoEditor]; both name the file.
func (r *Resolver) Resolve(ctx context.Context, t Target, v Verb) (Choice, error) {
	saved, hidden := r.snapshot(t, v)
	apps, err := r.visible(ctx, t, v, hidden)
	if err != nil {
		return Choice{}, err
	}
	app, fallback, ok := pick(apps, t, v, saved)
	if ok {
		return Choice{App: app, Fallback: fallback}, nil
	}
	name := filepath.Base(t.Path)
	if v == Edit && len(apps) == 0 {
		return Choice{}, fmt.Errorf("%s: %w", name, ErrNoEditor)
	}
	return Choice{}, fmt.Errorf("%s: %w", name, ErrChoose)
}

// Choices lists the apps to show for t and v: hidden apps removed, terminal
// apps last, and Default set only on the app [Resolver.Resolve] would pick.
func (r *Resolver) Choices(ctx context.Context, t Target, v Verb) ([]App, error) {
	saved, hidden := r.snapshot(t, v)
	apps, err := r.visible(ctx, t, v, hidden)
	if err != nil {
		return nil, err
	}
	chosen, _, ok := pick(apps, t, v, saved)
	for i := range apps {
		apps[i].Default = ok && apps[i].ID == chosen.ID
	}
	return apps, nil
}

// AllApps lists every app offered for any of the types, which map a kind to
// an extension, for either verb, whether or not the file is text. Each app
// appears once, sorted by name without regard to case.
// Hidden apps are included, so that they can be shown again; the system
// handler and the terminal editor are left out. Default is not set.
func (r *Resolver) AllApps(ctx context.Context, types map[string]string) ([]App, error) {
	var out []App
	seen := map[string]bool{SystemID: true, TerminalEditorID: true}
	for _, kind := range slices.Sorted(maps.Keys(types)) {
		for _, text := range []bool{false, true} {
			t := Target{Kind: kind, Ext: types[kind], Text: text}
			for _, v := range []Verb{Open, Edit} {
				apps, err := r.src.Apps(ctx, t, v)
				if err != nil {
					return nil, err
				}
				for _, a := range apps {
					if !seen[a.ID] {
						seen[a.ID] = true
						a.Default = false
						out = append(out, a)
					}
				}
			}
		}
	}
	slices.SortStableFunc(out, func(a, b App) int {
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

// SetDefault chooses the app with this ID for t's type and v. The system
// handler (with Open) clears the choice. Any other app must be among
// [Resolver.Choices], else the error is [ErrNotOffered].
func (r *Resolver) SetDefault(ctx context.Context, t Target, v Verb, id string) error {
	key := t.Key()
	if !validPreferenceString(key) {
		return errors.New("the file has no type to save a default for")
	}
	if id = r.canonical(id); id == SystemID && v == Open {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.prefs.SetDefault(v, key, "")
		return nil
	}
	apps, err := r.Choices(ctx, t, v)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(apps, func(a App) bool { return a.ID == id }) {
		return fmt.Errorf("%s: %w", id, ErrNotOffered)
	}
	if !validPreferenceString(id) {
		return fmt.Errorf("%q is not a valid app ID", id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prefs.SetDefault(v, key, id)
	return nil
}

// SetHidden hides or shows the app with this ID. The ID is canonicalized, and
// showing removes every entry saved under an ID that now means the same app.
// The system handler and the terminal editor cannot be hidden.
func (r *Resolver) SetHidden(id string, hidden bool) error {
	id = r.canonical(id)
	if hidden && protected(id) {
		return fmt.Errorf("%s cannot be hidden", id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if hidden {
		if !r.hiddenLocked(id) {
			r.prefs.SetHidden(id, true)
		}
		return nil
	}
	r.prefs.Hidden = slices.DeleteFunc(r.prefs.Hidden, func(h string) bool { return r.canonical(h) == id })
	return nil
}

// IsHidden reports whether the app with this ID is hidden, whether it was
// hidden under this ID or under one that now stands for it. The system handler
// and the terminal editor are never hidden.
func (r *Resolver) IsHidden(id string) bool {
	id = r.canonical(id)
	if protected(id) {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hiddenLocked(id)
}

// hiddenLocked reports whether a saved hidden ID is, canonically, id.
func (r *Resolver) hiddenLocked(id string) bool {
	return slices.ContainsFunc(r.prefs.Hidden, func(h string) bool { return r.canonical(h) == id })
}

// Launch checks t's path and starts app on it through the source. A terminal
// app comes back as an unstarted command. The terminal editor never edits a
// target that is not Text, whatever the source offers: that is
// [ErrNotOffered].
func (r *Resolver) Launch(ctx context.Context, t Target, v Verb, app App) (*exec.Cmd, error) {
	if v == Edit && !t.Text && r.canonical(app.ID) == TerminalEditorID {
		return nil, ErrNotOffered
	}
	path, err := CheckPath(t.Path, t.Directory)
	if err != nil {
		return nil, err
	}
	t.Path = path
	return r.src.Launch(ctx, t, v, app)
}

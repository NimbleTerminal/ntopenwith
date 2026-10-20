package ntopenwith

import (
	"context"
	"errors"
	"os/exec"
	"slices"
)

// Canonicalizer maps an app ID to the ID the app is known by now. A person's
// saved preference may hold an old or friendly ID; Canonical returns the ID to
// look up. The value returned by [Layered] implements it.
type Canonicalizer interface {
	Canonical(id string) string
}

// correction is a Source that offers nothing itself but adjusts what the
// sources of a [Layered] offer.
type correction interface {
	Source
	apply([]App) []App
}

// override wraps a Source whose default app beats the other sources'.
type override struct{ Source }

// Override marks s as the source whose default app, if it offers one, is the
// default of a [Layered] it is part of. Its apps are listed in order as usual.
func Override(s Source) Source { return override{s} }

type hide []string

// Hide removes the apps with these IDs.
func Hide(ids ...string) Source { return hide(slices.Clone(ids)) }

func (h hide) apply(apps []App) []App {
	return slices.DeleteFunc(apps, func(a App) bool { return slices.Contains(h, a.ID) })
}

type rename struct{ id, name string }

// Rename changes the Name of the app with this ID.
func Rename(id, name string) Source { return rename{id, name} }

func (r rename) apply(apps []App) []App {
	for i := range apps {
		if apps[i].ID == r.id {
			apps[i].Name = r.name
		}
	}
	return apps
}

type rank []string

// Rank puts the apps with these IDs first, after the default, in this order.
func Rank(ids ...string) Source { return rank(slices.Clone(ids)) }

func (r rank) apply(apps []App) []App { return apps } // ordering happens last

type alias struct{ id, target string }

// Alias makes id stand for the app target: a launch of id goes to target, and
// [Canonicalizer] reports target for it. It lists nothing itself.
func Alias(id, target string) Source { return alias{id, target} }

func (a alias) apply(apps []App) []App { return apps }

// The corrections offer no apps and launch nothing.
func (hide) Apps(context.Context, Target, Verb) ([]App, error)   { return nil, nil }
func (rename) Apps(context.Context, Target, Verb) ([]App, error) { return nil, nil }
func (rank) Apps(context.Context, Target, Verb) ([]App, error)   { return nil, nil }
func (alias) Apps(context.Context, Target, Verb) ([]App, error)  { return nil, nil }

func (hide) Launch(context.Context, Target, Verb, App) (*exec.Cmd, error) {
	return nil, ErrNotOffered
}
func (rename) Launch(context.Context, Target, Verb, App) (*exec.Cmd, error) {
	return nil, ErrNotOffered
}
func (rank) Launch(context.Context, Target, Verb, App) (*exec.Cmd, error) {
	return nil, ErrNotOffered
}
func (alias) Launch(context.Context, Target, Verb, App) (*exec.Cmd, error) {
	return nil, ErrNotOffered
}

// Layered combines sources into one. The sources are asked in order and their
// apps listed in that order; an app whose ID was already listed only gives
// its Name. The default is the first default any source offers, unless an
// [Override] source offers one. The corrections [Hide], [Rename], [Rank] and
// [Alias] are applied in the order given. The result lists the default,
// then the [Rank] IDs, then the rest in source order with terminal apps last.
//
// Launch goes to the source that offered the app; an App without one goes to
// the first source that offers its ID. An error from a source is returned only
// when no source answered.
func Layered(sources ...Source) Source {
	l := &layered{aliases: map[string]string{}}
	for _, s := range sources {
		switch c := s.(type) {
		case alias:
			l.aliases[c.id] = c.target
			l.corrections = append(l.corrections, c)
		case rank:
			l.ranks = append(l.ranks, c...)
			l.corrections = append(l.corrections, c)
		case correction:
			l.corrections = append(l.corrections, c)
		default:
			l.sources = append(l.sources, s)
		}
	}
	return l
}

type layered struct {
	sources     []Source
	corrections []correction
	ranks       []string
	aliases     map[string]string
}

// Canonical returns the target of the Alias for id, else id.
func (l *layered) Canonical(id string) string {
	if target, ok := l.aliases[id]; ok {
		return target
	}
	return id
}

func (l *layered) Apps(ctx context.Context, t Target, v Verb) ([]App, error) {
	var (
		apps         []App
		errs         []error
		answered     bool
		firstDefault string
		overDefault  string
	)
	for _, s := range l.sources {
		got, err := s.Apps(ctx, t, v)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		answered = true
		_, isOverride := s.(override)
		for _, app := range got {
			if app.Default {
				if isOverride && overDefault == "" {
					overDefault = app.ID
				}
				if firstDefault == "" {
					firstDefault = app.ID
				}
			}
			if i := slices.IndexFunc(apps, func(a App) bool { return a.ID == app.ID }); i >= 0 {
				apps[i].Name = app.Name
				continue
			}
			app.from = s
			apps = append(apps, app)
		}
	}
	if !answered && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	for _, c := range l.corrections {
		apps = c.apply(apps)
	}
	defaultID := firstDefault
	if overDefault != "" {
		defaultID = overDefault
	}
	return l.order(apps, defaultID), nil
}

// order puts the default first, then the ranked IDs, then the rest with
// terminal apps last, and marks only the default as Default.
func (l *layered) order(apps []App, defaultID string) []App {
	out := make([]App, 0, len(apps))
	taken := make([]bool, len(apps))
	take := func(id string) {
		for i, a := range apps {
			if !taken[i] && a.ID == id {
				taken[i] = true
				a.Default = false
				out = append(out, a)
				return
			}
		}
	}
	if defaultID != "" {
		take(defaultID)
		if len(out) == 1 {
			out[0].Default = true
		}
	}
	for _, id := range l.ranks {
		take(id)
	}
	for _, terminal := range []bool{false, true} {
		for i, a := range apps {
			if !taken[i] && a.Terminal == terminal {
				taken[i] = true
				a.Default = false
				out = append(out, a)
			}
		}
	}
	return out
}

func (l *layered) Launch(ctx context.Context, t Target, v Verb, app App) (*exec.Cmd, error) {
	if canon := l.Canonical(app.ID); canon != app.ID {
		app = App{ID: canon}
	}
	if app.from != nil && app.from != Source(l) {
		return app.from.Launch(ctx, t, v, app)
	}
	apps, err := l.Apps(ctx, t, v)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(apps, func(a App) bool { return a.ID == app.ID })
	if i < 0 || apps[i].from == nil || apps[i].from == Source(l) {
		return nil, ErrNotOffered
	}
	return apps[i].from.Launch(ctx, t, v, apps[i])
}

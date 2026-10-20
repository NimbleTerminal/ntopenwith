package system

import (
	"context"
	"os/exec"
	"slices"

	"nimbleterminal.dev/ntopenwith"
	"nimbleterminal.dev/ntopenwith/internal/launchservices"
)

// macSource is the Source for macOS, over Launch Services. It has no build
// tag so that its tests run on any OS with a fake; only the constructor that
// loads the frameworks is tagged.
type macSource struct {
	ls launchservices.Services
	// listings keeps recent answers; Refresh clears the process's one.
	listings *cache
}

func newMacSource(ls launchservices.Services, listings *cache) *macSource {
	return &macSource{ls: ls, listings: listings}
}

// macChoice is an app offered, with its bundle (zero for the system default).
type macChoice struct {
	app    ntopenwith.App
	bundle launchservices.Bundle
}

// macKey is the cache key for t: its type, or for a file without an
// extension its path, which Launch Services may judge by more than the name.
func macKey(t ntopenwith.Target) string {
	if k := t.Type(); k != "" {
		return k
	}
	return "path:" + t.Path
}

// offer asks Launch Services what it offers for t and v, in the order to
// show: its default, then for Open the system default, then the others.
func (s *macSource) offer(t ntopenwith.Target, v ntopenwith.Verb) ([]macChoice, error) {
	system := macChoice{app: ntopenwith.App{ID: ntopenwith.SystemID, Name: "System default", Default: true}}
	var role launchservices.Role
	switch v {
	case ntopenwith.Open:
		role = launchservices.Viewer | launchservices.Editor
	case ntopenwith.Edit:
		role = launchservices.Editor
	default:
		return nil, nil
	}
	if t.Directory {
		// A folder opens in the Finder; nothing edits one.
		if v == ntopenwith.Open {
			return []macChoice{system}, nil
		}
		return nil, nil
	}

	var (
		def     launchservices.Bundle
		hasDef  bool
		bundles []launchservices.Bundle
		err     error
	)
	if t.Ext != "" {
		if def, hasDef, err = s.ls.DefaultForType(t.Type(), role); err != nil {
			return nil, err
		}
		bundles, err = s.ls.ForType(t.Type(), role)
	} else {
		if def, hasDef, err = s.ls.DefaultForFile(t.Path, role); err != nil {
			return nil, err
		}
		bundles, err = s.ls.ForFile(t.Path, role)
	}
	if err != nil {
		return nil, err
	}
	hasDef = hasDef && def.ID != ""

	var out []macChoice
	seen := map[string]bool{}
	add := func(b launchservices.Bundle, isDefault bool) {
		// A bundle without an ID cannot be named in preferences, nor one
		// without a path launched.
		if b.ID == "" || b.Path == "" || seen[b.ID] {
			return
		}
		seen[b.ID] = true
		out = append(out, macChoice{
			app:    ntopenwith.App{ID: "mac:" + b.ID, Name: b.Name, Default: isDefault},
			bundle: b,
		})
	}
	if hasDef {
		add(def, true)
	}
	if v == ntopenwith.Open {
		system.app.Default = !hasDef
		out = append(out, system)
	}
	for _, b := range bundles {
		add(b, false)
	}
	return out, nil
}

func (s *macSource) Apps(ctx context.Context, t ntopenwith.Target, v ntopenwith.Verb) ([]ntopenwith.App, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := macKey(t)
	if t.Directory {
		key = "folder"
	}
	return s.listings.get(key, v, func() ([]ntopenwith.App, error) {
		// Never a partial list: a caller that gave up gets the error and
		// nothing is kept.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		offered, err := s.offer(t, v)
		if err != nil {
			return nil, err
		}
		var apps []ntopenwith.App
		for _, c := range offered {
			apps = append(apps, c.app)
		}
		return apps, ctx.Err()
	})
}

func (s *macSource) Launch(ctx context.Context, t ntopenwith.Target, v ntopenwith.Verb, app ntopenwith.App) (*exec.Cmd, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := ntopenwith.CheckPath(t.Path, t.Directory)
	if err != nil {
		return nil, err
	}
	t.Path = path

	// Ask again rather than trust a listing that may be old: the app must be
	// offered now, for this file and verb.
	offered, err := s.offer(t, v)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(offered, func(c macChoice) bool { return c.app.ID == app.ID })
	if i < 0 {
		return nil, ntopenwith.ErrNotOffered
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if app.ID == ntopenwith.SystemID {
		return nil, start([]string{"open", path})
	}
	return nil, start([]string{"open", "-a", offered[i].bundle.Path, path})
}

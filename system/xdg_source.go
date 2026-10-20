package system

import (
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"nimbleterminal.dev/ntopenwith"
	"nimbleterminal.dev/ntopenwith/internal/xdg"
)

// xdgSource is the Source for desktops that follow the freedesktop.org
// specifications. It has no build tag so that its tests run on any OS; only
// the constructor that reads the real environment is tagged.
type xdgSource struct {
	// load reads the XDG files afresh.
	load func() (*xdg.Index, error)
	// listings keeps recent answers; Refresh clears the process's one.
	listings *cache

	mu       sync.Mutex
	index    *xdg.Index
	indexGen uint64
	indexEnd time.Time
}

func newXDGSource(load func() (*xdg.Index, error), listings *cache) *xdgSource {
	return &xdgSource{load: load, listings: listings}
}

// editCategories are the desktop entry categories of apps that can change a
// file; an entry in none of them only views it.
var editCategories = []string{"TextEditor", "Graphics", "Office", "Development", "3DGraphics"}

// choice is an app offered, with the desktop entry behind it (zero for the
// system default).
type choice struct {
	app   ntopenwith.App
	entry xdg.Entry
}

// cachedIndex returns the index read within the cache's lifetime, so a
// listing for each row of a file browser reads the XDG files once.
func (s *xdgSource) cachedIndex() (*xdg.Index, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	gen := s.listings.generation()
	if s.index != nil && s.indexGen == gen && clock().Before(s.indexEnd) {
		return s.index, nil
	}
	ix, err := s.load()
	if err != nil {
		return nil, err
	}
	s.index, s.indexGen, s.indexEnd = ix, gen, clock().Add(lifetime)
	return ix, nil
}

// mimeOf is the MIME type of the target: a folder's, or that of Ext, else
// of Path's name.
func mimeOf(ix *xdg.Index, t ntopenwith.Target) string {
	switch {
	case t.Directory:
		return "inode/directory"
	case t.Ext != "":
		return ix.TypeOf(t.Ext)
	}
	return ix.TypeOf(filepath.Base(t.Path))
}

// launchable reports whether e's Exec line is one Launch would run: an entry
// whose line is refused (a quoted file code, an unknown field code) is not
// offered at all.
func launchable(e xdg.Entry) bool {
	_, err := xdg.ExpandExec(e, "/x")
	return err == nil
}

// offer lists what ix offers for t and v, in the order to show.
func offer(ix *xdg.Index, t ntopenwith.Target, v ntopenwith.Verb) []choice {
	mime := mimeOf(ix, t)
	def, hasDef := ix.Default(mime)
	hasDef = hasDef && launchable(def)
	handlers := slices.DeleteFunc(ix.Handlers(mime), func(e xdg.Entry) bool { return !launchable(e) })

	entry := func(e xdg.Entry) choice {
		return choice{
			app:   ntopenwith.App{ID: "xdg:" + e.ID, Name: e.Name, Terminal: e.Terminal},
			entry: e,
		}
	}
	var out []choice
	switch v {
	case ntopenwith.Open:
		system := choice{app: ntopenwith.App{ID: ntopenwith.SystemID, Name: "System default", Default: !hasDef}}
		if hasDef {
			c := entry(def)
			c.app.Default = true
			out = append(out, c)
		}
		out = append(out, system)
		for _, e := range handlers {
			if hasDef && e.ID == def.ID {
				continue
			}
			out = append(out, entry(e))
		}
	case ntopenwith.Edit:
		// Never a default: whichever opens a file is not necessarily an
		// editor, so the person is asked.
		if t.Directory {
			return nil
		}
		for _, e := range handlers {
			if slices.ContainsFunc(e.Categories, func(c string) bool { return slices.Contains(editCategories, c) }) {
				out = append(out, entry(e))
			}
		}
	}
	return out
}

func (s *xdgSource) Apps(ctx context.Context, t ntopenwith.Target, v ntopenwith.Verb) ([]ntopenwith.App, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ix, err := s.cachedIndex()
	if err != nil {
		return nil, err
	}
	return s.listings.get(mimeOf(ix, t), v, func() ([]ntopenwith.App, error) {
		// Never a partial list: a caller that gave up gets the error and
		// nothing is kept.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var apps []ntopenwith.App
		for _, c := range offer(ix, t, v) {
			apps = append(apps, c.app)
		}
		return apps, ctx.Err()
	})
}

func (s *xdgSource) Launch(ctx context.Context, t ntopenwith.Target, v ntopenwith.Verb, app ntopenwith.App) (*exec.Cmd, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := ntopenwith.CheckPath(t.Path, t.Directory)
	if err != nil {
		return nil, err
	}
	t.Path = path

	// Read the files again rather than trust a listing that may be old: the
	// app must be offered now, for this file and verb.
	ix, err := s.load()
	if err != nil {
		return nil, err
	}
	offered := offer(ix, t, v)
	i := slices.IndexFunc(offered, func(c choice) bool { return c.app.ID == app.ID })
	if i < 0 {
		return nil, ntopenwith.ErrNotOffered
	}
	c := offered[i]
	if c.app.ID == ntopenwith.SystemID {
		return nil, start([]string{"xdg-open", path})
	}
	argv, err := xdg.ExpandExec(c.entry, path)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.entry.Terminal {
		return exec.Command(argv[0], argv[1:]...), nil
	}
	return nil, start(argv)
}

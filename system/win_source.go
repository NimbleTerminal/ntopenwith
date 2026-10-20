package system

import (
	"context"
	"fmt"
	"os/exec"
	"slices"

	"nimbleterminal.dev/ntopenwith"
	"nimbleterminal.dev/ntopenwith/internal/winassoc"
)

// winSource is the Source for Windows, over the registry's associations and
// the shell. It has no build tag so that its tests run on any OS with fakes;
// only the constructor that wires in the real calls is tagged.
type winSource struct {
	reg winassoc.Registry
	// friendly names a ProgID's verb as Windows shows it.
	friendly func(progID, verb string) string
	// shell performs a verb on a path with a ProgID's command, or the type's
	// own when progID is "".
	shell func(path, verb, progID string) error
	// lookPath finds a program named by its file name.
	lookPath func(string) (string, error)
	// listings keeps recent answers; Refresh clears the process's one.
	listings *cache
}

func newWinSource(reg winassoc.Registry, friendly func(progID, verb string) string, shell func(path, verb, progID string) error, lookPath func(string) (string, error), listings *cache) *winSource {
	return &winSource{reg: reg, friendly: friendly, shell: shell, lookPath: lookPath, listings: listings}
}

// winChoice is an app offered, with its handler (zero for the system default).
type winChoice struct {
	app     ntopenwith.App
	handler winassoc.Handler
}

// offer reads what the registry offers for t and v, in the order to show:
// the default, then for Open the system default, then the other handlers.
// Names are left for Apps to fill.
func (s *winSource) offer(t ntopenwith.Target, v ntopenwith.Verb) []winChoice {
	if v != ntopenwith.Open && v != ntopenwith.Edit {
		return nil
	}
	system := winChoice{app: ntopenwith.App{ID: ntopenwith.SystemID, Name: "System default", Default: true}}
	ext := t.Type()
	if t.Directory || ext == "" {
		// The registry associates extensions: a folder or a file without
		// one is left to the shell, and nothing edits it.
		if v == ntopenwith.Open {
			return []winChoice{system}
		}
		return nil
	}

	verb := string(v)
	def, hasDef := winassoc.Default(s.reg, ext, verb)
	var out []winChoice
	if hasDef {
		out = append(out, winChoice{app: ntopenwith.App{ID: def.ID, Default: true}, handler: def})
	}
	if v == ntopenwith.Open {
		system.app.Default = !hasDef
		out = append(out, system)
	}
	for _, h := range winassoc.Handlers(s.reg, ext, verb) {
		if hasDef && h.ID == def.ID {
			continue
		}
		out = append(out, winChoice{app: ntopenwith.App{ID: h.ID}, handler: h})
	}
	return out
}

// name is what Windows calls h for verb, or the program's file name.
func (s *winSource) name(h winassoc.Handler, verb ntopenwith.Verb) string {
	if h.ProgID != "" {
		return s.friendly(h.ProgID, string(verb))
	}
	// A program's own class is Applications\<exe name>.
	class := `Applications\` + h.Exe
	if n := s.friendly(class, string(ntopenwith.Open)); n != class {
		return n
	}
	return h.Exe
}

func (s *winSource) Apps(ctx context.Context, t ntopenwith.Target, v ntopenwith.Verb) ([]ntopenwith.App, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := t.Type()
	if t.Directory {
		key = "folder"
	}
	return s.listings.get(key, v, func() ([]ntopenwith.App, error) {
		// Never a partial list: a caller that gave up gets the error and
		// nothing is kept.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var apps []ntopenwith.App
		for _, c := range s.offer(t, v) {
			if c.app.ID != ntopenwith.SystemID {
				c.app.Name = s.name(c.handler, v)
			}
			apps = append(apps, c.app)
		}
		return apps, ctx.Err()
	})
}

func (s *winSource) Launch(ctx context.Context, t ntopenwith.Target, v ntopenwith.Verb, app ntopenwith.App) (*exec.Cmd, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := ntopenwith.CheckPath(t.Path, t.Directory)
	if err != nil {
		return nil, err
	}
	t.Path = path

	// Read the registry again rather than trust a listing that may be old:
	// the app must be offered now, for this file and verb.
	offered := s.offer(t, v)
	i := slices.IndexFunc(offered, func(c winChoice) bool { return c.app.ID == app.ID })
	if i < 0 {
		return nil, ntopenwith.ErrNotOffered
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	h := offered[i].handler
	switch {
	case app.ID == ntopenwith.SystemID:
		return nil, s.shell(path, string(ntopenwith.Open), "")
	case h.Exe != "":
		exe, err := s.lookPath(h.Exe)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", h.Exe, err)
		}
		return nil, start([]string{exe, path})
	}
	return nil, s.shell(path, string(v), h.ProgID)
}

package main

import (
	"context"
	"io/fs"
	"os/exec"
	"path"
	"slices"
	"strings"
	"sync"

	"nimbleterminal.dev/ntopenwith"
)

// pretendHome is where the pretend mode starts: a made-up person's home.
const pretendHome = "/home/ada"

// pretendFiles is the made-up home folder, by path under it and size in
// bytes. A name ending in "/" is an empty folder.
var pretendFiles = map[string]int64{
	"Desktop/":                            0,
	"Downloads/":                          0,
	"todo.txt":                            312,
	"Pictures/landscape.png":              2_480_211,
	"Pictures/portrait.jpg":               1_120_004,
	"Pictures/IMG_2041.heic":              3_904_112,
	"Pictures/beach.mov":                  187_300_000,
	"Documents/report.pdf":                148_220,
	"Documents/letter.docx":               18_800,
	"Documents/budget.xlsx":               40_210,
	"Documents/notes.md":                  6_120,
	"Projects/demo/main.go":               2_600,
	"Projects/demo/go.mod":                412,
	"Projects/demo/README.md":             3_100,
	"Projects/demo/data.csv":              12_400,
	"Projects/demo/testdata/":             0,
	"Models/bracket.stl":                  684_084,
	"Models/benchy.3mf":                   1_890_310,
	"Music/Ferrite Hearts - Magnetic.mp3": 7_200_000,
}

// pretendTree is the pretend home folder as a filesystem rooted at "/".
func pretendTree() *memFS {
	f := &memFS{nodes: map[string]*memNode{".": {name: ".", dir: true}}}
	for name, size := range pretendFiles {
		f.add(path.Join(pretendHome[1:], name), size, strings.HasSuffix(name, "/"))
	}
	return f
}

// pretendApp is one row of the table: an app, the verbs it is offered for,
// and those it is the default for.
type pretendApp struct {
	id, name string
	verbs    []ntopenwith.Verb
	defaults []ntopenwith.Verb
}

var (
	open     = []ntopenwith.Verb{ntopenwith.Open}
	openEdit = []ntopenwith.Verb{ntopenwith.Open, ntopenwith.Edit}
)

func app(id, name string, verbs, defaults []ntopenwith.Verb) pretendApp {
	return pretendApp{"mac:" + id, name, verbs, defaults}
}

// Apps that appear for several types.
var (
	preview     = app("com.apple.Preview", "Preview", openEdit, openEdit)
	pixelmator  = app("com.pixelmatorteam.pixelmator.x", "Pixelmator Pro", openEdit, nil)
	safari      = app("com.apple.Safari", "Safari", open, nil)
	quickTime   = app("com.apple.QuickTimePlayerX", "QuickTime Player", openEdit, nil)
	vsCode      = app("com.microsoft.VSCode", "Visual Studio Code", openEdit, openEdit)
	textEdit    = app("com.apple.TextEdit", "TextEdit", openEdit, nil)
	zed         = app("dev.zed.Zed", "Zed", openEdit, nil)
	numbers     = app("com.apple.iWork.Numbers", "Numbers", openEdit, openEdit)
	bambuStudio = app("com.bambulab.bambu-studio", "Bambu Studio", openEdit, nil)
)

// pretendApps mimics, by extension, what Launch Services reported on a Mac
// on 2026-10-20, trimmed. Images list QuickTime Player as an editor, as the
// real data does: the Layered in main corrects that.
var pretendApps = map[string][]pretendApp{
	".png":  {preview, pixelmator, safari, quickTime},
	".jpg":  {preview, pixelmator, safari, quickTime},
	".heic": {preview, pixelmator, app("com.apple.Safari", "Safari", open, nil)},
	".mov": {
		app("com.apple.QuickTimePlayerX", "QuickTime Player", open, open),
		app("com.apple.TV", "TV", open, nil),
	},
	".pdf": {
		app("com.apple.Preview", "Preview", openEdit, openEdit),
		app("com.apple.Safari", "Safari", open, nil),
	},
	".docx": {
		app("com.apple.iWork.Pages", "Pages", openEdit, openEdit),
		app("com.microsoft.Word", "Microsoft Word", openEdit, nil),
		textEdit,
	},
	".xlsx": {numbers, app("com.microsoft.Excel", "Microsoft Excel", openEdit, nil)},
	".csv":  {numbers, vsCode, textEdit},
	".md":   {vsCode, textEdit, zed},
	".go":   {vsCode, textEdit, zed},
	".txt":  {app("com.apple.TextEdit", "TextEdit", openEdit, openEdit), vsCode, zed},
	".stl": {
		bambuStudio,
		app("com.apple.Preview", "Preview", open, nil),
	},
	".3mf": {app("com.bambulab.bambu-studio", "Bambu Studio", openEdit, open)},
	".mp3": {
		app("com.apple.Music", "Music", open, open),
		app("com.apple.QuickTimePlayerX", "QuickTime Player", open, nil),
	},
}

// Terminal apps, offered to edit text only.
var pretendTerminal = []ntopenwith.App{
	{ID: ntopenwith.TerminalEditorID, Name: "Edit in terminal ($EDITOR)", Terminal: true},
	{ID: "vim", Name: "Vim", Terminal: true},
}

// pretend is an ntopenwith.Source over the pretend home folder. It offers the
// apps in pretendApps, "System default" for Open, and the terminal apps for
// text. Its Launch starts nothing: it records the launch and returns nil.
type pretend struct {
	fsys fs.FS

	mu       sync.Mutex
	launched []string // "verb app-id path", in order
}

func newPretend(fsys fs.FS) *pretend { return &pretend{fsys: fsys} }

func (p *pretend) Apps(ctx context.Context, t ntopenwith.Target, v ntopenwith.Verb) ([]ntopenwith.App, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var apps []ntopenwith.App
	if t.Directory {
		if v == ntopenwith.Open {
			apps = append(apps, ntopenwith.App{ID: "mac:com.apple.finder", Name: "Finder", Default: true})
		}
	} else {
		for _, a := range pretendApps[t.Type()] {
			if slices.Contains(a.verbs, v) {
				apps = append(apps, ntopenwith.App{ID: a.id, Name: a.name, Default: slices.Contains(a.defaults, v)})
			}
		}
	}
	if v == ntopenwith.Open {
		// Launch Services lists the system handler second, after the default.
		at := min(1, len(apps))
		apps = slices.Insert(apps, at, ntopenwith.App{ID: ntopenwith.SystemID, Name: "System default", Default: len(apps) == 0})
	}
	if v == ntopenwith.Edit && t.Text {
		apps = append(apps, pretendTerminal...)
	}
	return apps, nil
}

func (p *pretend) Launch(ctx context.Context, t ntopenwith.Target, v ntopenwith.Verb, app ntopenwith.App) (*exec.Cmd, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// As Resolver.Launch checks a real path, check that this one is in the tree.
	info, err := fs.Stat(p.fsys, strings.TrimPrefix(t.Path, "/"))
	if err != nil {
		return nil, err
	}
	if info.IsDir() != t.Directory {
		return nil, &fs.PathError{Op: "launch", Path: t.Path, Err: fs.ErrInvalid}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.launched = append(p.launched, string(v)+" "+app.ID+" "+t.Path)
	return nil, nil
}

// Launched lists the launches so far, as "verb app-id path".
func (p *pretend) Launched() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.launched)
}

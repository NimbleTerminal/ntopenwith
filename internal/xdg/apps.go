package xdg

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
)

// maxDesktopFiles bounds how many .desktop files are read in all.
const maxDesktopFiles = 50000

// Index answers which MIME type a file name has and which apps handle a type.
// It is read once by [Load] and never changes, so it is safe for concurrent use.
type Index struct {
	globs   map[string]glob
	entries map[string]Entry
	byMime  map[string][]string // MimeType= handlers, in load order
	assocs  []mimeapps          // in lookup order, most important first
}

// mimeapps is one mimeapps.list file.
type mimeapps struct {
	defaults, added, removed map[string][]string
}

// Load reads the XDG files that env names. A missing file or folder is
// not an error; it is simply empty. lookPath finds TryExec programs and is
// usually exec.LookPath; it is a parameter so tests never touch the system.
func Load(env Env, lookPath func(string) (string, error)) (*Index, error) {
	if lookPath == nil {
		return nil, errors.New("xdg: Load needs a lookPath function")
	}
	ix := &Index{
		globs:   map[string]glob{},
		entries: map[string]Entry{},
		byMime:  map[string][]string{},
	}

	dataDirs := nonEmpty(append([]string{env.DataHome}, env.DataDirs...))
	for _, dir := range dataDirs {
		readGlobs(dir, ix.globs)
	}

	// The first file with an ID wins, even if it is Hidden: a hidden entry
	// in a more important folder deletes the later ones.
	seen := map[string]bool{}
	files := 0
	for _, dir := range dataDirs {
		root := filepath.Join(dir, "applications")
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".desktop") {
				return nil
			}
			if files++; files > maxDesktopFiles {
				return fs.SkipAll
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return nil
			}
			id := strings.ReplaceAll(filepath.ToSlash(rel), "/", "-")
			if seen[id] {
				return nil
			}
			data, ok := readBounded(path, maxDesktopSize)
			if !ok {
				return nil
			}
			e, ok := parseDesktop(data)
			if !ok {
				return nil
			}
			seen[id] = true
			e.ID, e.File = id, path
			if e.Name == "" {
				e.Name = strings.TrimSuffix(id, ".desktop")
			}
			if !e.shown(env.Desktops, lookPath) {
				return nil
			}
			ix.entries[id] = e
			for _, m := range e.MimeTypes {
				m = strings.ToLower(m)
				ix.byMime[m] = append(ix.byMime[m], id)
			}
			return nil
		})
	}

	for _, path := range mimeappsPaths(env, dataDirs) {
		if data, ok := readBounded(path, maxMimeappsSize); ok {
			ix.assocs = append(ix.assocs, parseMimeapps(data))
		}
	}
	return ix, nil
}

func nonEmpty(in []string) []string {
	var out []string
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// mimeappsPaths lists the mimeapps.list files in the specification's order:
// the config home and config dirs, then applications/ in the data dirs; in
// each folder the desktop-specific files come before the plain one.
func mimeappsPaths(env Env, dataDirs []string) []string {
	var paths []string
	add := func(dir string) {
		for _, d := range env.Desktops {
			paths = append(paths, filepath.Join(dir, d+"-mimeapps.list"))
		}
		paths = append(paths, filepath.Join(dir, "mimeapps.list"))
	}
	for _, dir := range nonEmpty(append([]string{env.ConfigHome}, env.ConfigDirs...)) {
		add(dir)
	}
	for _, dir := range dataDirs {
		add(filepath.Join(dir, "applications"))
	}
	return paths
}

func parseMimeapps(data []byte) mimeapps {
	m := mimeapps{
		defaults: map[string][]string{},
		added:    map[string][]string{},
		removed:  map[string][]string{},
	}
	scanKeyFile(data, func(group, key, value string) {
		var into map[string][]string
		switch group {
		case "Default Applications":
			into = m.defaults
		case "Added Associations":
			into = m.added
		case "Removed Associations":
			into = m.removed
		default:
			return
		}
		key = strings.ToLower(key)
		into[key] = append(into[key], splitList(value)...)
	})
	return m
}

// Handlers returns the apps for a MIME type, best first: those in
// [Added Associations] (in file order, most important file first), then those
// whose MimeType= lists the type, without any in [Removed Associations] and
// without duplicates. A removal in one file hides the app from the files and
// entries after it.
func (ix *Index) Handlers(mime string) []Entry {
	mime = strings.ToLower(mime)
	removed := map[string]bool{}
	seen := map[string]bool{}
	var out []Entry
	add := func(id string) {
		if e, ok := ix.entries[id]; ok && !removed[id] && !seen[id] {
			seen[id] = true
			out = append(out, e)
		}
	}
	for _, f := range ix.assocs {
		for _, id := range f.removed[mime] {
			removed[id] = true
		}
		for _, id := range f.added[mime] {
			add(id)
		}
	}
	for _, id := range ix.byMime[mime] {
		add(id)
	}
	return out
}

// Default returns the first app named in a [Default Applications] line for
// the MIME type that exists, searching the files in lookup order. A name that
// is missing or was dropped (hidden, no TryExec program, and so on) is skipped.
func (ix *Index) Default(mime string) (Entry, bool) {
	mime = strings.ToLower(mime)
	for _, f := range ix.assocs {
		for _, id := range f.defaults[mime] {
			if e, ok := ix.entries[id]; ok {
				return e, true
			}
		}
	}
	return Entry{}, false
}

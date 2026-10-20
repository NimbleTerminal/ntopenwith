package ntopenwith

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Verb is what to do with a file.
type Verb string

const (
	// Open shows or plays the file in an app that handles its type.
	Open Verb = "open"
	// Edit changes the file; only text can be edited in a terminal editor.
	Edit Verb = "edit"
)

// Target is the file (or a folder) to act on.
type Target struct {
	// Path is the absolute path of the file.
	Path string
	// Ext, when set, is the type to use instead of Path's extension, with its dot.
	Ext string
	// Kind, when set, is the program's own name for the type, such as "pdf".
	// It takes precedence over the extension in preferences and List matching.
	Kind string
	// Directory is true when Path is a folder.
	Directory bool
	// Text is true when the file is text, so a terminal editor may edit it.
	Text bool
}

// Type is the lower-case extension with its dot, taken from Ext or else from
// Path, or "" when neither has one.
func (t Target) Type() string {
	ext := t.Ext
	if ext == "" {
		ext = filepath.Ext(t.Path)
	}
	return strings.ToLower(ext)
}

// matchesKinds matches Kind or Type, with an empty list accepting every target.
func (t Target) matchesKinds(kinds []string) bool {
	if len(kinds) == 0 {
		return true
	}
	ext := t.Type()
	for _, kind := range kinds {
		if (t.Kind != "" && strings.EqualFold(kind, t.Kind)) ||
			(ext != "" && strings.EqualFold(kind, ext)) {
			return true
		}
	}
	return false
}

// Key is what preferences and List entries are matched on: Kind if set,
// else Type.
func (t Target) Key() string {
	if t.Kind != "" {
		return t.Kind
	}
	return t.Type()
}

// CheckPath returns path made absolute and clean, after checking that it
// exists and is a regular file, or a directory when directory is true.
// It rejects an empty path and one holding a NUL byte.
func CheckPath(path string, directory bool) (string, error) {
	if path == "" {
		return "", errors.New("empty path")
	}
	if strings.ContainsRune(path, 0) {
		return "", errors.New("path contains a NUL byte")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	switch {
	case directory && !info.IsDir():
		return "", fmt.Errorf("%s is not a directory", abs)
	case !directory && !info.Mode().IsRegular():
		return "", fmt.Errorf("%s is not a regular file", abs)
	}
	return abs, nil
}

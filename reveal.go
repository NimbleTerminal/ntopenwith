package ntopenwith

import (
	"os"
	"path/filepath"
	"runtime"
)

// Reveal shows path in the platform's file manager: the file selected in its
// folder where the platform can, or the folder itself when path is a
// directory. On macOS a folder is selected in its parent as a file is, so
// that a bundle such as Foo.app is shown, not launched. It does not wait for the file manager.
//
// path is checked with [CheckPath] first. On a platform with no file manager
// to ask, including js/wasm, it returns [ErrUnsupported].
func Reveal(path string) error {
	switch runtime.GOOS {
	case "darwin", "windows", "linux", "freebsd", "openbsd", "netbsd":
	default:
		return ErrUnsupported
	}
	isDir := false
	if info, err := os.Stat(path); err == nil {
		isDir = info.IsDir()
	}
	p, err := CheckPath(path, isDir)
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		// A folder is selected in its parent too: "open" on a bundle, such
		// as Foo.app, would launch it rather than show it.
		return Start([]string{"open", "-R", p})
	case "windows":
		if isDir {
			return Start([]string{"explorer.exe", p})
		}
		return selectInExplorer(p)
	}
	// XDG desktops have no "select this file": open the folder holding it.
	if !isDir {
		p = filepath.Dir(p)
	}
	return Start([]string{"xdg-open", p})
}

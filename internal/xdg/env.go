// Package xdg reads the freedesktop.org files that Linux and the BSDs use to
// say which apps handle which kinds of file: the Shared MIME-info globs, the
// desktop entries in the applications folders, and mimeapps.list.
//
// It only parses files, so it builds and tests on every OS. Nothing here
// starts a program: [ExpandExec] turns an entry's Exec line into an argument
// list and never a shell command.
package xdg

import (
	"os"
	"strings"
)

// Env says where the XDG files are. The zero value reads nothing.
type Env struct {
	// DataHome is $XDG_DATA_HOME, searched before DataDirs.
	DataHome string
	// DataDirs is $XDG_DATA_DIRS, most important first.
	DataDirs []string
	// ConfigHome is $XDG_CONFIG_HOME, searched before ConfigDirs.
	ConfigHome string
	// ConfigDirs is $XDG_CONFIG_DIRS, most important first.
	ConfigDirs []string
	// Desktops is $XDG_CURRENT_DESKTOP split on ":" and lower-cased, most
	// important first. It picks the <desktop>-mimeapps.list files and
	// decides OnlyShowIn and NotShowIn.
	Desktops []string
}

// EnvFromOS reads the XDG variables and HOME, with the specification's
// defaults for those that are unset or empty: ~/.local/share,
// /usr/local/share:/usr/share, ~/.config and /etc/xdg. A value that is not an
// absolute path is ignored, as the specification requires.
func EnvFromOS() Env {
	return envFrom(os.Getenv)
}

func envFrom(get func(string) string) Env {
	home := get("HOME")
	homeDir := func(env, rel string) string {
		if v := get(env); isAbs(v) {
			return v
		}
		if isAbs(home) {
			return home + "/" + rel
		}
		return ""
	}
	dirs := func(env string, def ...string) []string {
		var out []string
		for _, d := range strings.Split(get(env), ":") {
			if isAbs(d) {
				out = append(out, d)
			}
		}
		if len(out) == 0 {
			return def
		}
		return out
	}

	e := Env{
		DataHome:   homeDir("XDG_DATA_HOME", ".local/share"),
		DataDirs:   dirs("XDG_DATA_DIRS", "/usr/local/share", "/usr/share"),
		ConfigHome: homeDir("XDG_CONFIG_HOME", ".config"),
		ConfigDirs: dirs("XDG_CONFIG_DIRS", "/etc/xdg"),
	}
	for _, d := range strings.Split(get("XDG_CURRENT_DESKTOP"), ":") {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
			e.Desktops = append(e.Desktops, d)
		}
	}
	return e
}

func isAbs(p string) bool {
	return strings.HasPrefix(p, "/")
}

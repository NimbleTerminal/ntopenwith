package xdg

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"strings"
)

// Entry is a desktop entry (a .desktop file) that is fit to offer.
type Entry struct {
	// ID is the desktop file ID: the path under applications/ with "/"
	// turned into "-", such as "kde-foo.desktop".
	ID string
	// File is the path of the .desktop file, for the %k field code.
	File string
	Name string
	// Exec is the raw Exec line, after key file unescaping but before
	// quoting and field codes. [ExpandExec] turns it into arguments.
	Exec    string
	TryExec string
	Icon    string

	Terminal        bool
	NoDisplay       bool
	Hidden          bool
	DBusActivatable bool

	MimeTypes  []string
	Categories []string
	OnlyShowIn []string
	NotShowIn  []string
}

// parseDesktop reads the [Desktop Entry] group. It reports false for a file
// that has no such group or is not an Application. Localized keys such as
// Name[de] are ignored.
func parseDesktop(data []byte) (Entry, bool) {
	var e Entry
	var typ string
	found := false
	scanKeyFile(data, func(group, key, value string) {
		if group != "Desktop Entry" {
			return
		}
		found = true
		switch key {
		case "Type":
			typ = value
		case "Name":
			e.Name = unescape(value)
		case "Exec":
			e.Exec = unescape(value)
		case "TryExec":
			e.TryExec = unescape(value)
		case "Icon":
			e.Icon = unescape(value)
		case "Terminal":
			e.Terminal = value == "true"
		case "NoDisplay":
			e.NoDisplay = value == "true"
		case "Hidden":
			e.Hidden = value == "true"
		case "DBusActivatable":
			e.DBusActivatable = value == "true"
		case "MimeType":
			e.MimeTypes = splitList(value)
		case "Categories":
			e.Categories = splitList(value)
		case "OnlyShowIn":
			e.OnlyShowIn = splitList(value)
		case "NotShowIn":
			e.NotShowIn = splitList(value)
		}
	})
	if !found || (typ != "" && typ != "Application") {
		return Entry{}, false
	}
	return e, true
}

// shown reports whether the entry is fit to offer: not Hidden or NoDisplay,
// its TryExec program found, shown in the current desktops, and runnable.
func (e Entry) shown(desktops []string, lookPath func(string) (string, error)) bool {
	if e.Hidden || e.NoDisplay {
		return false
	}
	if e.DBusActivatable && strings.TrimSpace(e.Exec) == "" {
		return false
	}
	if e.TryExec != "" {
		if _, err := lookPath(e.TryExec); err != nil {
			return false
		}
	}
	if len(e.OnlyShowIn) > 0 && !anyDesktop(e.OnlyShowIn, desktops) {
		return false
	}
	if anyDesktop(e.NotShowIn, desktops) {
		return false
	}
	return true
}

func anyDesktop(list, desktops []string) bool {
	for _, l := range list {
		for _, d := range desktops {
			if strings.EqualFold(l, d) {
				return true
			}
		}
	}
	return false
}

// Largest files read, so a hostile or broken tree cannot exhaust memory.
const (
	maxGlobsSize    = 16 << 20
	maxDesktopSize  = 1 << 20
	maxMimeappsSize = 1 << 20
	maxLineSize     = 1 << 20
)

// readBounded reads a regular file no larger than limit. A missing,
// unreadable or oversized file reports ok false, and is skipped by callers.
func readBounded(path string, limit int64) ([]byte, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > limit {
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, false
	}
	return data, true
}

// scanKeyFile calls fn for each key=value line of a key file (desktop entry
// or mimeapps.list) with the group it sits in. Comments, blank lines and
// lines before the first group are skipped.
func scanKeyFile(data []byte, fn func(group, key, value string)) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(nil, maxLineSize)
	group, inGroup := "", false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "" || line[0] == '#':
		case line[0] == '[' && line[len(line)-1] == ']':
			group, inGroup = line[1:len(line)-1], true
		case inGroup:
			if k, v, ok := strings.Cut(line, "="); ok {
				fn(group, strings.TrimSpace(k), strings.TrimLeft(v, " \t"))
			}
		}
	}
}

// unescape applies the key file escapes for a string value: \s, \n, \t, \r
// and \\. Any other backslash is kept, so an Exec line's own \" survives.
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			if r, ok := escapes[s[i+1]]; ok {
				b.WriteByte(r)
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

var escapes = map[byte]byte{'s': ' ', 'n': '\n', 't': '\t', 'r': '\r', '\\': '\\'}

// splitList splits a ;-separated list value. "\;" is a literal semicolon,
// and the other string escapes apply. Empty items are dropped.
func splitList(s string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == ';':
			flush()
		case c == '\\' && i+1 < len(s) && s[i+1] == ';':
			b.WriteByte(';')
			i++
		case c == '\\' && i+1 < len(s) && escapes[s[i+1]] != 0:
			b.WriteByte(escapes[s[i+1]])
			i++
		default:
			b.WriteByte(c)
		}
	}
	flush()
	return out
}

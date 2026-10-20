package xdg

import (
	"errors"
	"fmt"
	"strings"
)

// ExpandExec turns an entry's Exec line into an argument list for launching
// path, following the Desktop Entry Specification. It never involves a shell:
// quoting only groups characters into one argument.
//
// Arguments are separated by spaces and may be double-quoted, with \" \` \$
// and \\ escaped inside the quotes. Then the field codes are replaced:
//
//	%f %F %u %U   the path (always one argument; a code inside a larger
//	              argument, such as "--file=%f", is replaced in place)
//	%i            --icon and the Icon, as two arguments; nothing if no Icon
//	%c            the Name
//	%k            the .desktop file's path
//	%%            a literal %
//	%d %D %n %N %v %m   deprecated, removed
//
// An argument left empty by a removed code is dropped. Any other code is an
// error. If no %f, %F, %u or %U appears, the path is appended as the last
// argument, since an app told nothing of the file could not open it.
func ExpandExec(e Entry, path string) ([]string, error) {
	if path == "" {
		return nil, errors.New("xdg: no file to open")
	}
	words, err := splitExec(e.Exec)
	if err != nil {
		return nil, fmt.Errorf("xdg: %s: %w", e.ID, err)
	}
	var argv []string
	usedPath := false
	for _, w := range words {
		args, used, err := expandWord(e, w, path)
		if err != nil {
			return nil, fmt.Errorf("xdg: %s: %w", e.ID, err)
		}
		usedPath = usedPath || used
		argv = append(argv, args...)
	}
	if len(argv) == 0 || argv[0] == "" {
		return nil, fmt.Errorf("xdg: %s: the Exec line names no program", e.ID)
	}
	if !usedPath {
		argv = append(argv, path)
	}
	return argv, nil
}

// splitExec applies the Exec quoting rules, giving the arguments before field
// codes are replaced.
func splitExec(s string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inWord, inQuote := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote {
			switch {
			case c == '"':
				inQuote = false
			case c == '\\' && i+1 < len(s) && strings.IndexByte("\"`$\\", s[i+1]) >= 0:
				cur.WriteByte(s[i+1])
				i++
			default:
				cur.WriteByte(c)
			}
			continue
		}
		switch c {
		case ' ', '\t', '\n':
			if inWord {
				args = append(args, cur.String())
				cur.Reset()
				inWord = false
			}
		case '"':
			inQuote, inWord = true, true
		default:
			cur.WriteByte(c)
			inWord = true
		}
	}
	if inQuote {
		return nil, errors.New("the Exec line has an unterminated quote")
	}
	if inWord {
		args = append(args, cur.String())
	}
	return args, nil
}

// expandWord replaces the field codes in one argument. It reports whether a
// code carried the path.
func expandWord(e Entry, w, path string) (args []string, usedPath bool, err error) {
	if w == "%i" {
		if e.Icon == "" {
			return nil, false, nil
		}
		return []string{"--icon", e.Icon}, false, nil
	}
	var b strings.Builder
	coded := false
	for i := 0; i < len(w); i++ {
		if w[i] != '%' {
			b.WriteByte(w[i])
			continue
		}
		if i+1 == len(w) {
			return nil, false, errors.New("the Exec line ends in a lone %")
		}
		i++
		switch w[i] {
		case '%':
			b.WriteByte('%')
		case 'f', 'F', 'u', 'U':
			b.WriteString(path)
			usedPath, coded = true, true
		case 'c':
			b.WriteString(e.Name)
			coded = true
		case 'k':
			b.WriteString(e.File)
			coded = true
		case 'd', 'D', 'n', 'N', 'v', 'm':
			coded = true
		case 'i':
			return nil, false, errors.New("the Exec line uses %i inside a larger argument")
		default:
			return nil, false, fmt.Errorf("the Exec line has the unknown field code %%%c", w[i])
		}
	}
	if coded && b.Len() == 0 {
		return nil, usedPath, nil
	}
	return []string{b.String()}, usedPath, nil
}

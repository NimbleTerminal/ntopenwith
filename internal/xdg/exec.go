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
//	%f %u         the path, in place: a whole argument or part of an unquoted
//	              one, such as --file=%f
//	%F %U         the path, as a whole argument
//	%i            --icon and the Icon, as two arguments; nothing if no Icon
//	%c            the Name
//	%k            the .desktop file's path
//	%%            a literal %
//	%d %D %n %N %v %m   deprecated, removed
//
// An argument left empty by a removed code is dropped. Any other code is an
// error. A file code (%f %F %u %U) is also an error in any argument that has
// a quoted part, where the specification leaves it undefined; so is a code
// split by a quote, a second file code, or any code in the program name.
// If no file code appears, the path is appended as the last argument, since
// an app told nothing of the file could not open it.
//
// These rules catch quoting mistakes in ordinary Exec lines. An Exec line that
// hands its argument to an interpreter (such as sh -c %f) is the desktop
// file's own choice, and field-code rules cannot make it safe.
func ExpandExec(e Entry, path string) ([]string, error) {
	if path == "" {
		return nil, errors.New("xdg: no file to open")
	}
	words, err := splitExec(e.Exec)
	if err != nil {
		return nil, fmt.Errorf("xdg: %s: %w", e.ID, err)
	}
	var argv []string
	files := 0
	for i, w := range words {
		args, n, err := expandWord(e, w, path, i == 0, files > 0)
		if err != nil {
			return nil, fmt.Errorf("xdg: %s: %w", e.ID, err)
		}
		files += n
		argv = append(argv, args...)
	}
	if len(argv) == 0 || argv[0] == "" {
		return nil, fmt.Errorf("xdg: %s: the Exec line names no program", e.ID)
	}
	if files == 0 {
		argv = append(argv, path)
	}
	return argv, nil
}

// execWord is one argument of an Exec line after quoting is removed.
type execWord struct {
	text     string
	hasQuote bool   // some part of the argument was in double quotes
	quoted   []bool // quoted[i] says whether text[i] was inside double quotes
}

// splitExec applies the Exec quoting rules, giving the arguments before field
// codes are replaced.
func splitExec(s string) ([]execWord, error) {
	var words []execWord
	var cur strings.Builder
	var quoted []bool
	hasQuote := false
	inWord, inQuote := false, false
	push := func() {
		words = append(words, execWord{text: cur.String(), hasQuote: hasQuote, quoted: quoted})
		cur.Reset()
		quoted, hasQuote, inWord = nil, false, false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote {
			switch {
			case c == '"':
				inQuote = false
			case c == '\\' && i+1 < len(s) && strings.IndexByte("\"`$\\", s[i+1]) >= 0:
				quoted = append(quoted, true)
				cur.WriteByte(s[i+1])
				i++
			default:
				quoted = append(quoted, true)
				cur.WriteByte(c)
			}
			continue
		}
		switch c {
		case ' ', '\t', '\n':
			if inWord {
				push()
			}
		case '"':
			inQuote, inWord, hasQuote = true, true, true
		default:
			quoted = append(quoted, false)
			cur.WriteByte(c)
			inWord = true
		}
	}
	if inQuote {
		return nil, errors.New("the Exec line has an unterminated quote")
	}
	if inWord {
		push()
	}
	return words, nil
}

// expandWord replaces the field codes in one argument and counts the file
// codes it held. program is true for the first argument, which no code may
// make, and haveFile that an earlier argument held a file code.
func expandWord(e Entry, w execWord, path string, program, haveFile bool) (args []string, files int, err error) {
	text := w.text
	if text == "%i" && !program {
		if e.Icon == "" {
			return nil, 0, nil
		}
		return []string{"--icon", e.Icon}, 0, nil
	}
	var b strings.Builder
	coded := false
	for i := 0; i < len(text); i++ {
		if text[i] != '%' {
			b.WriteByte(text[i])
			continue
		}
		if i+1 == len(text) {
			return nil, 0, errors.New("the Exec line ends in a lone %")
		}
		i++
		code := text[i]
		if w.quoted[i-1] != w.quoted[i] {
			return nil, 0, errors.New("the Exec line has a field code split by a quote")
		}
		if code == '%' {
			b.WriteByte('%')
			continue
		}
		if program {
			return nil, 0, fmt.Errorf("the Exec line takes its program from the field code %%%c", code)
		}
		switch code {
		case 'f', 'F', 'u', 'U':
			if w.hasQuote {
				return nil, 0, fmt.Errorf("the Exec line has %%%c in a quoted argument", code)
			}
			if haveFile || files > 0 {
				return nil, 0, errors.New("the Exec line has more than one file code")
			}
			if (code == 'F' || code == 'U') && text != "%"+string(code) {
				return nil, 0, fmt.Errorf("the Exec line has %%%c inside a larger argument", code)
			}
			b.WriteString(path)
			files++
			coded = true
		case 'c':
			b.WriteString(e.Name)
			coded = true
		case 'k':
			b.WriteString(e.File)
			coded = true
		case 'd', 'D', 'n', 'N', 'v', 'm':
			coded = true
		case 'i':
			return nil, 0, errors.New("the Exec line uses %i inside a larger argument")
		default:
			return nil, 0, fmt.Errorf("the Exec line has the unknown field code %%%c", code)
		}
	}
	if coded && b.Len() == 0 {
		return nil, files, nil
	}
	return []string{b.String()}, files, nil
}

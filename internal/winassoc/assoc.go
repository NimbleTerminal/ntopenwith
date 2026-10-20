// Package winassoc reads the Windows registry's file associations: which
// ProgIDs and programs open an extension, and which is its default. The
// logic builds on every OS over the Registry interface so that it can be
// tested with a fake; only the registry and shell calls are Windows-only.
package winassoc

import (
	"strings"
)

// Registry reads the Windows registry. root is "HKCU", "HKLM" or "HKCR";
// path is a key below it, with backslashes. A missing key or value is an
// error.
type Registry interface {
	// String is the string value name of the key (name "" is its default).
	String(root, path, name string) (string, error)
	// ValueNames lists the names of the key's values.
	ValueNames(root, path string) ([]string, error)
	// SubKeys lists the names of the key's subkeys.
	SubKeys(root, path string) ([]string, error)
}

// Handler is a program that opens a type: a ProgID, or a program named by
// its file name. Either way the shell runs the command its class registers
// (a program's class is [AppClass]), so no command line is ever built here.
type Handler struct {
	// ID is "win:<ProgID>" or "win:app:<exe name>".
	ID string
	// ProgID is the class to run; empty for a program.
	ProgID string
	// Exe is the program's file name, such as "gimp.exe"; empty for a ProgID.
	Exe string
}

// Class is the registry class whose command the shell runs for h.
func (h Handler) Class() string {
	if h.ProgID != "" {
		return h.ProgID
	}
	return AppClass(h.Exe)
}

// AppClass is the class a program registers under its file name,
// Applications\<exe>.
func AppClass(exe string) string { return `Applications\` + exe }

// fileExts holds the Explorer's per-user choices for each extension.
const fileExts = `Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\`

// normExt is ext lower-cased with its dot, or "" when there is none.
func normExt(ext string) string {
	ext = strings.ToLower(ext)
	if ext == "" || ext == "." {
		return ""
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return ext
}

// hasCommand reports whether class registers a command for verb.
func hasCommand(r Registry, class, verb string) bool {
	subs, err := r.SubKeys("HKCR", class+`\shell\`+verb)
	if err != nil {
		return false
	}
	for _, s := range subs {
		if strings.EqualFold(s, "command") {
			return true
		}
	}
	return false
}

// bareExe reports whether name is a program's file name alone, not a path,
// and names an .exe, as an Applications class does: an OpenWithList entry
// naming a batch file (.bat, .cmd) or a path is not offered.
func bareExe(name string) bool {
	return isExe(name) && !strings.ContainsAny(name, `\/:`+"\x00")
}

// isExe reports whether name ends in ".exe", whatever its case.
func isExe(name string) bool {
	return len(name) > len(".exe") && strings.EqualFold(name[len(name)-len(".exe"):], ".exe")
}

// Handlers lists the handlers of ext (case is ignored) for verb "open" or
// "edit", each once: the ProgIDs of the user's, the machine's and the merged
// classes' OpenWithProgids, then the programs of the user's OpenWithList and
// the class's. A program is listed only when its class, [AppClass], registers
// shell\<verb>\command; for "edit" a ProgID only when it registers
// shell\edit\command.
func Handlers(r Registry, ext, verb string) []Handler {
	ext = normExt(ext)
	if ext == "" || (verb != "open" && verb != "edit") {
		return nil
	}
	var out []Handler
	seen := map[string]bool{}
	add := func(h Handler) {
		k := strings.ToLower(h.ID)
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, h)
	}

	for _, key := range [][2]string{
		{"HKCU", `Software\Classes\` + ext + `\OpenWithProgids`},
		{"HKLM", `Software\Classes\` + ext + `\OpenWithProgids`},
		{"HKCR", ext + `\OpenWithProgids`},
	} {
		names, _ := r.ValueNames(key[0], key[1])
		for _, p := range names {
			if p == "" || strings.ContainsRune(p, 0) {
				continue
			}
			if verb == "edit" && !hasCommand(r, p, "edit") {
				continue
			}
			add(Handler{ID: "win:" + p, ProgID: p})
		}
	}
	// The user's list holds a value per program, named a, b, c… and ordered
	// by MRUList; the class's has a subkey per program.
	var exes []string
	names, _ := r.ValueNames("HKCU", fileExts+ext+`\OpenWithList`)
	for _, n := range names {
		if strings.EqualFold(n, "MRUList") {
			continue
		}
		if exe, err := r.String("HKCU", fileExts+ext+`\OpenWithList`, n); err == nil {
			exes = append(exes, exe)
		}
	}
	subs, _ := r.SubKeys("HKCR", ext+`\OpenWithList`)
	exes = append(exes, subs...)
	for _, exe := range exes {
		if bareExe(exe) && hasCommand(r, AppClass(exe), verb) {
			add(Handler{ID: "win:app:" + exe, Exe: exe})
		}
	}
	return out
}

// Default is the handler Windows uses for ext (case is ignored) and verb:
// for "open", the user's UserChoice ProgId, else the class's default value;
// for "edit", that ProgID when it registers shell\edit\command. ok is false
// when there is none.
func Default(r Registry, ext, verb string) (h Handler, ok bool) {
	ext = normExt(ext)
	if ext == "" || (verb != "open" && verb != "edit") {
		return Handler{}, false
	}
	p, err := r.String("HKCU", fileExts+ext+`\UserChoice`, "ProgId")
	if err != nil || p == "" {
		p, err = r.String("HKCR", ext, "")
	}
	if err != nil || p == "" || strings.ContainsRune(p, 0) {
		return Handler{}, false
	}
	if verb == "edit" && !hasCommand(r, p, "edit") {
		return Handler{}, false
	}
	return Handler{ID: "win:" + p, ProgID: p}, true
}

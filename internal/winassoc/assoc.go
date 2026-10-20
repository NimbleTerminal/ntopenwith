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

// Handler is a program that opens a type: a ProgID, whose registered
// command the shell runs, or a program named only by its file name.
type Handler struct {
	// ID is "win:<ProgID>" or "win:app:<exe name>".
	ID string
	// ProgID is the class to run; empty for a program.
	ProgID string
	// Exe is the program's file name, such as "gimp.exe"; empty for a ProgID.
	Exe string
}

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

// canEdit reports whether progID registers an edit command.
func canEdit(r Registry, progID string) bool {
	subs, err := r.SubKeys("HKCR", progID+`\shell\edit`)
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
// and names an .exe: a batch file (.bat, .cmd, or a bare name that PATHEXT
// could resolve to one) would run through cmd.exe, which is a shell.
func bareExe(name string) bool {
	return IsExe(name) && !strings.ContainsAny(name, `\/:`+"\x00")
}

// IsExe reports whether name ends in ".exe", whatever its case.
func IsExe(name string) bool {
	return len(name) > len(".exe") && strings.EqualFold(name[len(name)-len(".exe"):], ".exe")
}

// Handlers lists the handlers of ext (case is ignored) for verb "open" or
// "edit", each once: the ProgIDs of the user's, the machine's and the merged
// classes' OpenWithProgids, then for "open" the programs of the user's
// OpenWithList and the class's. For "edit" only ProgIDs that register
// shell\edit\command are listed, and no programs.
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
			if verb == "edit" && !canEdit(r, p) {
				continue
			}
			add(Handler{ID: "win:" + p, ProgID: p})
		}
	}
	if verb == "edit" {
		return out
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
		if bareExe(exe) {
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
	if verb == "edit" && !canEdit(r, p) {
		return Handler{}, false
	}
	return Handler{ID: "win:" + p, ProgID: p}, true
}

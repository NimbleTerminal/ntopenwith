package winassoc

import "strings"

const (
	assocstrExecutable      = 2 // ASSOCSTR_EXECUTABLE
	assocstrFriendlyAppName = 4 // ASSOCSTR_FRIENDLYAPPNAME
)

// nameLookups keeps the fallback order testable without Windows.
type nameLookups struct {
	query       func(class, verb string, kind uint32) string
	application func(class string) string
	indirect    func(string) string
	description func(path string) string
}

func friendlyName(class, verb string, look nameLookups) string {
	if name := look.query(class, verb, assocstrFriendlyAppName); name != "" {
		return name
	}
	name := look.application(class)
	if strings.HasPrefix(name, "@") {
		name = look.indirect(name)
	}
	if name != "" {
		return name
	}
	if exe := look.query(class, verb, assocstrExecutable); exe != "" {
		if name := look.description(exe); name != "" {
			return name
		}
		// Use Windows separators even when the fallback is tested elsewhere.
		base := exe[strings.LastIndexAny(exe, `/\`)+1:]
		if isExe(base) {
			base = base[:len(base)-len(".exe")]
		}
		if base != "" {
			return base
		}
	}
	return class
}

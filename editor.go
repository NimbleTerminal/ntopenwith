package ntopenwith

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/google/shlex"
)

// editorSetting returns the trimmed $VISUAL, else the trimmed $EDITOR.
func editorSetting() string {
	if value := strings.TrimSpace(os.Getenv("VISUAL")); value != "" {
		return value
	}
	return strings.TrimSpace(os.Getenv("EDITOR"))
}

// editorArgs turns an editor setting into an argv whose first word is a
// resolved executable path. No shell is involved: the setting is either one
// executable path or POSIX-quoted words, with no expansions.
func editorArgs(setting string) ([]string, error) {
	if setting == "" {
		return nil, errors.New("set VISUAL or EDITOR to a terminal editor, for example EDITOR=nano")
	}
	if len(setting) > 8192 || strings.ContainsRune(setting, 0) {
		return nil, errors.New("editor setting exceeds 8192 bytes or contains NUL")
	}
	// An exact executable path can contain spaces or Windows backslashes.
	// Otherwise parse arguments with POSIX quoting, without any expansions.
	var argv []string
	if executable, err := exec.LookPath(setting); err == nil {
		argv = []string{executable}
	} else {
		argv, err = shlex.Split(setting)
		if err != nil {
			return nil, fmt.Errorf("invalid editor setting: %w", err)
		}
	}
	if len(argv) == 0 || argv[0] == "" || len(argv) > 64 {
		return nil, errors.New("editor setting needs an executable and at most 64 words")
	}
	executable, err := exec.LookPath(argv[0])
	if err != nil {
		return nil, fmt.Errorf("terminal editor: %w", err)
	}
	argv[0] = executable
	return argv, nil
}

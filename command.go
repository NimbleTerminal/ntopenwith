package ntopenwith

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// placeholder in an argument stands for the file's path.
const placeholder = "{}"

// Command returns argv with path substituted, ready to run without a shell.
//
// An argument that is exactly "{}" becomes path; with none, path is appended.
// The program (argv[0]) must be non-empty and free of "{}", and no other
// argument may hold "{}" inside longer text. path must be absolute and free
// of NUL bytes. argv is not modified. Callers validate the path with
// [CheckPath] first.
func Command(argv []string, path string) ([]string, error) {
	if len(argv) == 0 || argv[0] == "" || strings.Contains(argv[0], placeholder) {
		return nil, errors.New("invalid command: no program")
	}
	if !filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
		return nil, errors.New("invalid path: must be absolute and free of NUL bytes")
	}
	out := make([]string, 0, len(argv)+1)
	out = append(out, argv[0])
	found := false
	for _, arg := range argv[1:] {
		if arg == placeholder {
			arg, found = path, true
		} else if strings.Contains(arg, placeholder) {
			return nil, errors.New("invalid command: {} must be a whole argument")
		}
		out = append(out, arg)
	}
	if !found {
		out = append(out, path)
	}
	return out, nil
}

// Start runs argv directly, with no shell and no input or output, and does
// not wait for it to finish: the child is reaped in the background. It is
// started in a session of its own (on Windows, a process group), so that it
// outlives the terminal it was started from.
func Start(argv []string) error {
	if runtime.GOOS == "js" {
		return ErrUnsupported
	}
	if len(argv) == 0 || argv[0] == "" {
		return errors.New("invalid command: no program")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	// Stdin, Stdout and Stderr stay nil: the child gets the null device.
	detach(cmd)
	if err := startCommand(cmd); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// startCommand starts cmd; tests replace it to capture a launch.
var startCommand = func(c *exec.Cmd) error { return c.Start() }

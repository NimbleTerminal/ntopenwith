//go:build unix

package ntopenwith

import (
	"os/exec"
	"syscall"
)

// detach puts a GUI child in a session of its own, so that closing the
// terminal it was started from does not hang it up.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

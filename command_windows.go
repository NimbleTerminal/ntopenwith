//go:build windows

package ntopenwith

import (
	"os/exec"
	"syscall"
)

// detach puts a GUI child in a process group of its own, so that the
// console's Ctrl+C and Ctrl+Break do not reach it.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

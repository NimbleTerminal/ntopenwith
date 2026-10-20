//go:build !unix && !windows

package ntopenwith

import "os/exec"

// detach does nothing where there is no session to leave.
func detach(*exec.Cmd) {}

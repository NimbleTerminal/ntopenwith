//go:build linux || freebsd || openbsd || netbsd

package system

import (
	"os/exec"

	"nimbleterminal.dev/ntopenwith"
	"nimbleterminal.dev/ntopenwith/internal/xdg"
)

// lookPath finds TryExec programs; tests replace it.
var lookPath = exec.LookPath

// newSource reads the XDG files that the environment names.
func newSource() ntopenwith.Source {
	return newXDGSource(func() (*xdg.Index, error) {
		return xdg.Load(xdg.EnvFromOS(), lookPath)
	}, &shared)
}

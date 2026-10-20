//go:build windows

package system

import (
	"os/exec"

	"nimbleterminal.dev/ntopenwith"
	"nimbleterminal.dev/ntopenwith/internal/winassoc"
)

// lookPath finds the programs of OpenWithList entries; tests replace it.
var lookPath = exec.LookPath

// newSource reads the registry's associations and launches through the shell.
func newSource() ntopenwith.Source {
	return newWinSource(winassoc.OS(), winassoc.FriendlyName, winassoc.ShellExecute,
		func(file string) (string, error) { return lookPath(file) }, &shared)
}

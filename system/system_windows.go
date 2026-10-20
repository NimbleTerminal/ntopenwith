//go:build windows

package system

import (
	"nimbleterminal.dev/ntopenwith"
	"nimbleterminal.dev/ntopenwith/internal/winassoc"
)

// newSource reads the registry's associations and launches through the shell.
func newSource() ntopenwith.Source {
	return newWinSource(winassoc.OS(), winassoc.FriendlyName, winassoc.ShellExecute, &shared)
}

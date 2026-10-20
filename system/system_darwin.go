//go:build darwin

package system

import (
	"nimbleterminal.dev/ntopenwith"
	"nimbleterminal.dev/ntopenwith/internal/launchservices"
)

// newServices loads Launch Services; tests replace it.
var newServices = launchservices.New

// newSource asks Launch Services, or lists nothing if its frameworks cannot
// be loaded.
func newSource() ntopenwith.Source {
	ls, err := newServices()
	if err != nil {
		return other{}
	}
	return newMacSource(ls, &shared)
}

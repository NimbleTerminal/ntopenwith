// Package system is the operating system's own source of apps: the handlers
// the OS registers for a file's type.
package system

import (
	"sync"

	"nimbleterminal.dev/ntopenwith"
)

// start launches a command; tests replace it.
var start = ntopenwith.Start

// shared holds the listings of the process's one Source.
var shared cache

var (
	once   sync.Once
	source ntopenwith.Source
)

// System returns the operating system's Source. There is one per process,
// built on first use. Its listings are kept for a few seconds; see [Refresh].
//
// On a platform with no OS integration, including js/wasm, it lists nothing
// and every launch returns [ntopenwith.ErrUnsupported].
func System() ntopenwith.Source {
	once.Do(func() { source = newSource() })
	return source
}

// Refresh forgets the listings kept so far, so the next ask queries the OS
// again. Call it after installing or removing an app.
func Refresh() {
	shared.clear()
}

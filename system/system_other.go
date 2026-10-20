//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd

// This file serves every OS without its own file. macOS has system_darwin.go,
// Linux and the BSDs system_xdg.go; Task 11 adds windows, narrowing the
// constraint above with "!windows" until it reads: not darwin, not windows,
// not linux, not freebsd, not openbsd, not netbsd. That also covers js/wasm.

package system

import "nimbleterminal.dev/ntopenwith"

func newSource() ntopenwith.Source { return other{} }

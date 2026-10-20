//go:build !darwin && !windows && !linux && !freebsd && !openbsd && !netbsd

// This file serves every OS without its own file, js/wasm among them. macOS
// has system_darwin.go, Windows system_windows.go, and Linux and the BSDs
// system_xdg.go.

package system

import "nimbleterminal.dev/ntopenwith"

func newSource() ntopenwith.Source { return other{} }

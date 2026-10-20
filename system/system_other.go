//go:build !linux && !freebsd && !openbsd && !netbsd

// This file serves every OS without its own file. Linux and the BSDs have
// system_xdg.go; Tasks 10 and 11 add darwin and windows, each narrowing the
// constraint above with its "!os" term, until it reads: not darwin, not
// windows, not linux, not freebsd, not openbsd, not netbsd. That also covers
// js/wasm.

package system

import (
	"context"
	"os/exec"

	"nimbleterminal.dev/ntopenwith"
)

// other is the Source for a platform with no OS integration.
type other struct{}

func newSource() ntopenwith.Source { return other{} }

func (other) Apps(context.Context, ntopenwith.Target, ntopenwith.Verb) ([]ntopenwith.App, error) {
	return nil, nil
}

func (other) Launch(context.Context, ntopenwith.Target, ntopenwith.Verb, ntopenwith.App) (*exec.Cmd, error) {
	return nil, ntopenwith.ErrUnsupported
}

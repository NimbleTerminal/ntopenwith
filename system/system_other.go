// This file has no build tag on purpose: for now it serves every OS. Tasks 9,
// 10 and 11 each add their own OS file (darwin, windows, XDG) and narrow this
// one by adding the matching "!os" term to a constraint, ending with
// //go:build !darwin && !windows && !linux && !freebsd && !openbsd && !netbsd
// which also covers js/wasm.

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

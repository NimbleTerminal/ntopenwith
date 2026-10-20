package system

import (
	"context"
	"os/exec"

	"nimbleterminal.dev/ntopenwith"
)

// other is the Source for a platform with no OS integration, and macOS's
// when Launch Services cannot be loaded. It lists nothing and launches
// nothing.
type other struct{}

func (other) Apps(context.Context, ntopenwith.Target, ntopenwith.Verb) ([]ntopenwith.App, error) {
	return nil, nil
}

func (other) Launch(context.Context, ntopenwith.Target, ntopenwith.Verb, ntopenwith.App) (*exec.Cmd, error) {
	return nil, ntopenwith.ErrUnsupported
}

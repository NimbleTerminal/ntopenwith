//go:build js

package ntopenwith

import (
	"context"
	"errors"
	"testing"
)

// Under js/wasm nothing launches, a terminal app included: it is not handed
// back as a command to run.
func TestListLaunchUnsupportedOnJS(t *testing.T) {
	stubLookPath(t, "ed", "viewer")
	src := List(
		ListApp{ID: "ed", Name: "Ed", Verbs: []Verb{Edit}, Text: true, Terminal: true, Command: []string{"ed"}},
		ListApp{ID: "viewer", Name: "Viewer", Verbs: []Verb{Open}, Command: []string{"viewer"}},
	)
	path := tempFile(t, "a.txt")
	for _, c := range []struct {
		v  Verb
		id string
	}{{Edit, "ed"}, {Open, "viewer"}} {
		cmd, err := src.Launch(context.Background(), Target{Path: path, Text: true}, c.v, App{ID: c.id})
		if cmd != nil || !errors.Is(err, ErrUnsupported) {
			t.Errorf("Launch %s = %v, %v; want ErrUnsupported", c.id, cmd, err)
		}
	}
}

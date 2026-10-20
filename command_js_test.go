//go:build js

package ntopenwith

import (
	"errors"
	"testing"
)

func TestStartUnsupportedOnJS(t *testing.T) {
	if err := Start([]string{"app", "/p/a.txt"}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Start = %v, want ErrUnsupported", err)
	}
}

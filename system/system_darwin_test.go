package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"nimbleterminal.dev/ntopenwith"
	"nimbleterminal.dev/ntopenwith/internal/launchservices"
)

func TestNewSourceFallsBackWithoutLaunchServices(t *testing.T) {
	old := newServices
	t.Cleanup(func() { newServices = old })
	newServices = func() (launchservices.Services, error) { return nil, errors.New("no frameworks") }
	if _, ok := newSource().(other); !ok {
		t.Fatalf("newSource = %T, want the empty source", newSource())
	}
}

func TestSystemListsRealApps(t *testing.T) {
	txt := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(txt, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	Refresh() // other tests leave listings in the shared cache
	t.Cleanup(Refresh)
	src := newSource()
	if _, ok := src.(*macSource); !ok {
		t.Fatalf("newSource = %T, want *macSource", src)
	}
	apps, err := src.Apps(context.Background(), ntopenwith.Target{Path: txt}, ntopenwith.Open)
	if err != nil {
		t.Fatal(err)
	}
	got := ids(apps)
	if !slices.Contains(got, "system") || !slices.Contains(got, "mac:com.apple.TextEdit") {
		t.Errorf("Open ids = %v, want system and TextEdit", got)
	}
	apps, err = src.Apps(context.Background(), ntopenwith.Target{Path: txt}, ntopenwith.Edit)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(apps); slices.Contains(got, "system") || !slices.Contains(got, "mac:com.apple.TextEdit") {
		t.Errorf("Edit ids = %v, want TextEdit and no system", got)
	}
}

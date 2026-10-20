//go:build linux

package system

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"nimbleterminal.dev/ntopenwith"
)

func TestSystemOnLinux(t *testing.T) {
	txt := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(txt, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	Refresh() // other tests leave listings in the shared cache
	t.Cleanup(Refresh)
	src := System()
	if _, ok := src.(*xdgSource); !ok {
		t.Fatalf("System = %T, want *xdgSource", src)
	}
	tgt := ntopenwith.Target{Path: txt, Text: true}
	apps, err := src.Apps(context.Background(), tgt, ntopenwith.Open)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(apps); !slices.Contains(got, ntopenwith.SystemID) {
		t.Errorf("Open ids = %v, want system", got)
	}
	apps, err = src.Apps(context.Background(), tgt, ntopenwith.Edit)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(apps); slices.Contains(got, ntopenwith.SystemID) {
		t.Errorf("Edit ids = %v, want no system", got)
	}
	t.Logf("Edit ids: %v", ids(apps))
}

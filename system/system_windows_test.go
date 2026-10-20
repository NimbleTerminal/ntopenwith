package system

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"nimbleterminal.dev/ntopenwith"
)

func TestWindowsSourceReal(t *testing.T) {
	txt := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(txt, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	Refresh() // other tests leave listings in the shared cache
	t.Cleanup(Refresh)
	src := newSource()
	if _, ok := src.(*winSource); !ok {
		t.Fatalf("newSource = %T, want *winSource", src)
	}
	for _, v := range []ntopenwith.Verb{ntopenwith.Open, ntopenwith.Edit} {
		apps, err := src.Apps(context.Background(), ntopenwith.Target{Path: txt}, v)
		if err != nil {
			t.Fatalf("%s: %v", v, err)
		}
		if v == ntopenwith.Open && len(apps) == 0 {
			t.Error("no app opens a .txt")
		}
		for _, a := range apps {
			if a.ID == "" || a.Name == "" {
				t.Errorf("%s: app without an ID or name: %+v", v, a)
			}
		}
		t.Logf("%s: %v", v, ids(apps))
	}
}

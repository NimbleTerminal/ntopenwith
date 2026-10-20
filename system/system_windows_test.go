package system

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
		if v == ntopenwith.Open {
			// "system" is always offered, so ask for the registry's own:
			// every Windows associates .txt with Notepad (txtfile).
			i := slices.IndexFunc(apps, func(a ntopenwith.App) bool { return strings.HasPrefix(a.ID, "win:") })
			if i < 0 {
				t.Errorf("no registry handler opens a .txt: %v", ids(apps))
			}
			// The default is a ProgID, named by FriendlyName, not by its ID.
			d := slices.IndexFunc(apps, func(a ntopenwith.App) bool { return a.Default && strings.HasPrefix(a.ID, "win:") })
			if d < 0 {
				t.Errorf("no registry default for .txt: %+v", apps)
			} else if progID := strings.TrimPrefix(apps[d].ID, "win:"); apps[d].Name == progID {
				t.Errorf("default %s is named by its ProgID %q, not a friendly name", apps[d].ID, apps[d].Name)
			}
		}
		for _, a := range apps {
			if a.ID == "" || a.Name == "" {
				t.Errorf("%s: app without an ID or name: %+v", v, a)
			}
		}
		t.Logf("%s: %v", v, ids(apps))
	}
}

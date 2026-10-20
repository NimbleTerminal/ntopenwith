package xdg

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func inheritanceIndex(t *testing.T) *Index {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("testdata", "inheritance"))
	if err != nil {
		t.Fatal(err)
	}
	ix, err := Load(Env{
		DataHome:   filepath.Join(root, "home"),
		DataDirs:   []string{filepath.Join(root, "data-1"), filepath.Join(root, "data-2")},
		ConfigHome: filepath.Join(root, "config"),
	}, fakeLook)
	if err != nil {
		t.Fatal(err)
	}
	return ix
}

func TestMIMEInheritanceHandlers(t *testing.T) {
	ix := inheritanceIndex(t)
	for _, tt := range []struct {
		mime string
		want []string
	}{
		{"text/x-go", []string{"added.desktop", "go.desktop", "plain.desktop"}},
		{"text/markdown", []string{"added.desktop", "plain.desktop"}},
		{"application/json", []string{"json.desktop", "added.desktop", "plain.desktop"}},
		{"APPLICATION/X-JSON", []string{"json.desktop", "added.desktop", "plain.desktop"}},
		{"application/x-json-chain", []string{"json.desktop", "added.desktop", "plain.desktop"}},
		{"application/x-last", []string{"json.desktop", "added.desktop", "plain.desktop"}},
		{"application/x-go-alias", []string{"json.desktop", "added.desktop", "plain.desktop"}},
		{"application/x-child", []string{"left.desktop", "right.desktop", "third.desktop", "deep.desktop"}},
		{"application/x-cycle-a", []string{"added.desktop", "plain.desktop"}},
		{"application/x-loop-a", nil},
		{"application/x-binary", nil},
		{"application/octet-stream", []string{"octet.desktop"}},
	} {
		t.Run(tt.mime, func(t *testing.T) {
			if got := ids(ix.Handlers(tt.mime)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Handlers = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMIMEInheritanceDefaults(t *testing.T) {
	ix := inheritanceIndex(t)
	for mime, want := range map[string]string{
		"text/x-go":                "plain.desktop",
		"text/markdown":            "plain.desktop",
		"application/json":         "json.desktop",
		"APPLICATION/X-JSON":       "json.desktop",
		"application/x-json-chain": "json.desktop",
		"application/x-child":      "right.desktop", // nearer than deep.desktop
		"application/x-cycle-a":    "plain.desktop",
		"application/x-loop-a":     "",
		"application/x-binary":     "",
	} {
		e, ok := ix.Default(mime)
		if e.ID != want || ok != (want != "") {
			t.Errorf("Default(%q) = %q, %v; want %q", mime, e.ID, ok, want)
		}
	}
}

func TestMIMEAliasTypeOf(t *testing.T) {
	ix := inheritanceIndex(t)
	if got := ix.TypeOf("notes.json"); got != "application/json" {
		t.Fatalf("TypeOf = %q, want application/json", got)
	}
}

func TestMIMEInheritanceOwnRemovalKeepsParentAssociation(t *testing.T) {
	ix := inheritanceIndex(t)
	ix.assocs[0].removed["text/x-go"] = []string{"plain.desktop"}
	if got := ids(ix.Handlers("text/x-go")); !reflect.DeepEqual(got, []string{"added.desktop", "go.desktop", "plain.desktop"}) {
		t.Fatalf("Handlers = %v", got)
	}
}

func TestMIMEDefaultSkipsRemovedAssociation(t *testing.T) {
	ix := inheritanceIndex(t)
	ix.assocs[0].defaults["application/json"] = []string{"removed.desktop"}
	if e, ok := ix.Default("application/json"); !ok || e.ID != "plain.desktop" {
		t.Fatalf("Default = %q, %v; want plain.desktop", e.ID, ok)
	}
}

func TestMIMEInheritanceDepthBound(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "mime"), 0o755); err != nil {
		t.Fatal(err)
	}
	var subclasses, aliases strings.Builder
	for i := range 100 {
		fmt.Fprintf(&subclasses, "application/x-level%d application/x-level%d\n", i, i+1)
		fmt.Fprintf(&aliases, "application/x-alias%d application/x-alias%d\n", i, i+1)
	}
	subclasses.WriteString("application/x-level100 text/plain\n")
	aliases.WriteString("application/x-alias100 text/plain\n")
	for name, data := range map[string]string{"subclasses": subclasses.String(), "aliases": aliases.String()} {
		if err := os.WriteFile(filepath.Join(root, "mime", name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "applications"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "applications", "plain.desktop"), []byte("[Desktop Entry]\nType=Application\nName=Plain\nExec=present-tool %f\nMimeType=text/plain;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ix, err := Load(Env{DataHome: root}, fakeLook)
	if err != nil {
		t.Fatal(err)
	}
	if got := ix.Handlers("text/plain"); len(got) != 1 {
		t.Fatalf("plain handlers = %v", ids(got))
	}
	if got := ix.Handlers("application/x-level0"); len(got) != 0 {
		t.Fatalf("deep handlers = %v", ids(got))
	}
	if got := ix.canonical("application/x-alias0"); got == "text/plain" {
		t.Fatal("alias traversal exceeded its depth bound")
	}
}

func TestMIMEInheritanceParentRemovalKeepsOwnAssociation(t *testing.T) {
	ix := inheritanceIndex(t)
	ix.assocs[0].removed["text/plain"] = append(ix.assocs[0].removed["text/plain"], "added.desktop")
	if got := ids(ix.Handlers("text/x-go")); !reflect.DeepEqual(got, []string{"added.desktop", "go.desktop", "plain.desktop"}) {
		t.Fatalf("Handlers = %v", got)
	}
}

func TestMIMEAliasCycleHasStableCanonicalName(t *testing.T) {
	ix := inheritanceIndex(t)
	if a, b := ix.canonical("application/x-loop-a"), ix.canonical("application/x-loop-b"); a != b {
		t.Fatalf("alias cycle canonical names differ: %q, %q", a, b)
	}
}

func TestMIMEInheritanceBreadthBound(t *testing.T) {
	ix := inheritanceIndex(t)
	for i := range maxMimeTypes + 10 {
		ix.parents["application/x-wide"] = append(ix.parents["application/x-wide"], fmt.Sprintf("application/x-parent%d", i))
	}
	if got := len(ix.ancestry("application/x-wide")); got != maxMimeTypes {
		t.Fatalf("ancestry length = %d, want %d", got, maxMimeTypes)
	}
}

func TestMIMEInfoFilesAreBounded(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "mime"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"aliases", "subclasses"} {
		path := filepath.Join(root, "mime", name)
		if err := os.WriteFile(path, []byte("application/x-example text/plain\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Truncate(path, maxMimeInfoSize+1); err != nil {
			t.Fatal(err)
		}
	}
	ix, err := Load(Env{DataHome: root}, fakeLook)
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.aliases) != 0 || len(ix.parents) != 0 {
		t.Fatal("oversized MIME files were read")
	}
}

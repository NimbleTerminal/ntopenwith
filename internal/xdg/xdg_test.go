package xdg

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

// treeEnv points at the fixture tree in testdata/tree.
func treeEnv(t *testing.T, desktops ...string) Env {
	t.Helper()
	abs := func(p string) string {
		a, err := filepath.Abs(filepath.Join("testdata", "tree", p))
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	return Env{
		DataHome:   abs("data-home"),
		DataDirs:   []string{abs("data-1"), abs("data-2")},
		ConfigHome: abs("config-home"),
		ConfigDirs: []string{abs("config-dir")},
		Desktops:   desktops,
	}
}

// fakeLook finds only present-tool, like a PATH with one program on it.
func fakeLook(name string) (string, error) {
	if name == "present-tool" {
		return "/bin/present-tool", nil
	}
	return "", errors.New("not found")
}

func load(t *testing.T, desktops ...string) *Index {
	t.Helper()
	ix, err := Load(treeEnv(t, desktops...), fakeLook)
	if err != nil {
		t.Fatal(err)
	}
	return ix
}

func ids(es []Entry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.ID)
	}
	return out
}

func TestLoadNeedsLookPath(t *testing.T) {
	if _, err := Load(Env{}, nil); err == nil {
		t.Fatal("want an error for a nil lookPath")
	}
}

func TestLoadEmptyEnv(t *testing.T) {
	ix, err := Load(Env{DataDirs: []string{"/nonexistent/xdg-data"}}, fakeLook)
	if err != nil {
		t.Fatal(err)
	}
	if got := ix.TypeOf("a.png"); got != "application/octet-stream" {
		t.Fatalf("TypeOf = %q", got)
	}
	if hs := ix.Handlers("image/png"); len(hs) != 0 {
		t.Fatalf("handlers %v", ids(hs))
	}
	if _, ok := ix.Default("image/png"); ok {
		t.Fatal("default from nothing")
	}
}

func TestTypeOfCaseAndMissing(t *testing.T) {
	ix := load(t)
	for name, want := range map[string]string{
		"a.png":           "image/png",
		"a.PNG":           "image/png",
		"/some/dir/A.Png": "image/png",
		".png":            "image/png",
		"README":          "application/octet-stream",
		"":                "application/octet-stream",
		"noext.":          "application/octet-stream",
		"x.dup":           "text/x-high", // weight 60 beats 50
		"x.tar.gz":        "application/x-compressed-tar",
		"x.gz":            "application/gzip",
		"x.jpeg":          "image/jpeg",
		"Makefile":        "application/octet-stream", // not a suffix glob
		"x.sa":            "application/octet-stream", // wildcard glob ignored
		"x.a":             "application/octet-stream", // bracket glob ignored
		"readme.txt":      "application/octet-stream", // prefix glob ignored
		"x.bad":           "application/octet-stream",
		"x.C":             "text/x-cfile", // the case-sensitive *.C is not used
	} {
		if got := ix.TypeOf(name); got != want {
			t.Errorf("TypeOf(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestTypeOfHigherWeightInLaterDir(t *testing.T) {
	// data-2 gives *.later weight 55, data-home weight 50: the weight wins
	// over the directory order.
	ix := load(t)
	if got := ix.TypeOf("a.later"); got != "text/x-later" {
		t.Fatalf("TypeOf = %q", got)
	}
}

func TestTypeOfEarlierDirWinsTie(t *testing.T) {
	ix := load(t)
	// data-1 and data-2 both say 50 for *.png; data-1 is earlier.
	if got := ix.TypeOf("a.png"); got != "image/png" {
		t.Fatalf("TypeOf = %q", got)
	}
}

func TestHandlersOrderAndRemoved(t *testing.T) {
	ix := load(t)
	// Added in the config home first (e, c), then MimeType= in load order
	// (a, c again, d). b is removed by the config home, so the config dir's
	// addition of b is hidden too.
	want := []string{"e.desktop", "c.desktop", "a.desktop", "d.desktop"}
	if got := ids(ix.Handlers("image/png")); !reflect.DeepEqual(got, want) {
		t.Fatalf("Handlers = %v, want %v", got, want)
	}
	if got := ids(ix.Handlers("IMAGE/PNG")); !reflect.DeepEqual(got, want) {
		t.Fatalf("Handlers with a capitalized type = %v", got)
	}
	if got := ix.Handlers("image/jpeg"); len(got) != 1 || got[0].ID != "d.desktop" {
		t.Fatalf("jpeg handlers %v", ids(got))
	}
	if got := ix.Handlers("video/x-none"); len(got) != 0 {
		t.Fatalf("handlers for nothing: %v", ids(got))
	}
}

func TestHandlersRemovalDoesNotHideHigherAddition(t *testing.T) {
	ix := &Index{
		entries: map[string]Entry{"x.desktop": {ID: "x.desktop"}},
		byMime:  map[string][]string{},
		assocs: []mimeapps{
			{added: map[string][]string{"a/b": {"x.desktop"}}},
			{removed: map[string][]string{"a/b": {"x.desktop"}}},
		},
	}
	if got := ids(ix.Handlers("a/b")); !reflect.DeepEqual(got, []string{"x.desktop"}) {
		t.Fatalf("Handlers = %v", got)
	}
}

func TestDefaultDesktopSpecificFirst(t *testing.T) {
	// Without a desktop: the config home's mimeapps.list names a missing
	// entry, then a.desktop.
	e, ok := load(t).Default("image/png")
	if !ok || e.ID != "a.desktop" {
		t.Fatalf("Default = %q, %v", e.ID, ok)
	}
	// Under GNOME its file comes first; the missing entry is skipped.
	e, ok = load(t, "gnome").Default("image/png")
	if !ok || e.ID != "d.desktop" {
		t.Fatalf("Default under gnome = %q, %v", e.ID, ok)
	}
	// Another desktop does not read gnome's file.
	e, ok = load(t, "kde").Default("image/png")
	if !ok || e.ID != "a.desktop" {
		t.Fatalf("Default under kde = %q, %v", e.ID, ok)
	}
	// A type only the config dir names.
	e, ok = load(t).Default("text/markdown")
	if !ok || e.ID != "e.desktop" {
		t.Fatalf("Default markdown = %q, %v", e.ID, ok)
	}
	if _, ok = load(t).Default("image/jpeg"); ok {
		t.Fatal("a default nobody named")
	}
}

func TestEntryFiltering(t *testing.T) {
	cases := []struct {
		desktops []string
		want     []string
	}{
		{[]string{"gnome"}, []string{
			"dbus-exec.desktop", "not-kde.desktop", "ok.desktop",
			"only-gnome.desktop", "tryexec-ok.desktop"}},
		{[]string{"kde"}, []string{
			"dbus-exec.desktop", "not-gnome.desktop", "ok.desktop",
			"only-gnome.desktop", "only-kde.desktop", "tryexec-ok.desktop"}},
		{nil, []string{
			"dbus-exec.desktop", "not-gnome.desktop", "not-kde.desktop",
			"ok.desktop", "tryexec-ok.desktop"}},
	}
	for _, c := range cases {
		got := ids(load(t, c.desktops...).Handlers("text/x-filter"))
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("desktops %v: got %v, want %v", c.desktops, got, c.want)
		}
	}
}

func TestEntryFilteringDroppedFromDefault(t *testing.T) {
	ix := &Index{
		entries: map[string]Entry{},
		assocs:  []mimeapps{{defaults: map[string][]string{"a/b": {"hidden.desktop"}}}},
	}
	if _, ok := ix.Default("a/b"); ok {
		t.Fatal("a dropped entry became the default")
	}
}

func TestDesktopIDs(t *testing.T) {
	ix := load(t)
	e, ok := ix.entries["kde-foo.desktop"]
	if !ok {
		t.Fatalf("no kde-foo.desktop in %v", ids(ix.Handlers("text/x-kde")))
	}
	if e.Name != "Foo" || !filepath.IsAbs(e.File) || filepath.Base(e.File) != "foo.desktop" {
		t.Fatalf("entry %+v", e)
	}
	if got := ids(ix.Handlers("text/x-kde")); !reflect.DeepEqual(got, []string{"kde-foo.desktop"}) {
		t.Fatalf("Handlers = %v", got)
	}

	// The data home shadows the data dirs.
	got := ix.Handlers("text/x-shadow")
	if len(got) != 1 || got[0].Name != "HomeShadow" {
		t.Fatalf("shadow handlers: %+v", got)
	}
	// A Hidden entry in an earlier folder deletes the later one.
	if _, ok := ix.entries["unshadow.desktop"]; ok {
		t.Fatal("a hidden entry did not hide the later file with its ID")
	}
}

func TestParseDesktop(t *testing.T) {
	e, ok := parseDesktop([]byte("\xef\xbb\xbf# comment\n" +
		"[Desktop Entry]\r\n" +
		"Type=Application\r\n" +
		"Name=Real\n" +
		"Name[de]=Echt\n" +
		"Exec=sh -c \"echo \\\\ \\\"%f\\\"\"\n" +
		"Icon = pic\n" +
		"Terminal=true\n" +
		"MimeType=a/b;c/d\\;e;;f/g\n" +
		"Categories=Graphics;3DGraphics;\n" +
		"[Desktop Action x]\n" +
		"Name=Not this\n"))
	if !ok {
		t.Fatal("not parsed")
	}
	want := Entry{
		Name:       "Real",
		Exec:       `sh -c "echo \ \"%f\""`,
		Icon:       "pic",
		Terminal:   true,
		MimeTypes:  []string{"a/b", "c/d;e", "f/g"},
		Categories: []string{"Graphics", "3DGraphics"},
	}
	if !reflect.DeepEqual(e, want) {
		t.Fatalf("got %+v\nwant %+v", e, want)
	}
	if _, ok := parseDesktop([]byte("[Other]\nName=x\n")); ok {
		t.Fatal("a file without [Desktop Entry] parsed")
	}
	if _, ok := parseDesktop([]byte("[Desktop Entry]\nType=Directory\nName=x\n")); ok {
		t.Fatal("a directory entry parsed")
	}
}

func TestExpandExec(t *testing.T) {
	const path = "/home/ann/My Files/a b.png"
	cases := []struct {
		name  string
		entry Entry
		want  []string // nil means an error
	}{
		{"F", Entry{Exec: "code --new-window %F"}, []string{"code", "--new-window", path}},
		{"u", Entry{Exec: "viewer %u"}, []string{"viewer", path}},
		{"quoted program", Entry{Exec: `"/opt/My App/bin/app" %u`}, []string{"/opt/My App/bin/app", path}},
		{"path inside a quoted word", Entry{Exec: `sh -c "echo \"%f\""`}, []string{"sh", "-c", `echo "` + path + `"`}},
		{"escapes", Entry{Exec: "p \"a\\$b\\`c\\\\d\""}, []string{"p", "a$b`c\\d", path}},
		{"literal percent", Entry{Exec: "foo %%f %f"}, []string{"foo", "%f", path}},
		{"embedded", Entry{Exec: "foo --file=%f"}, []string{"foo", "--file=" + path}},
		{"icon", Entry{Exec: "bar --icon %i", Icon: "pic"}, []string{"bar", "--icon", "--icon", "pic", path}},
		{"icon alone", Entry{Exec: "bar %i %f", Icon: "pic"}, []string{"bar", "--icon", "pic", path}},
		{"no icon", Entry{Exec: "bar %i %f"}, []string{"bar", path}},
		{"name", Entry{Exec: "qux %c %f", Name: "Qux App"}, []string{"qux", "Qux App", path}},
		{"file", Entry{Exec: "qux %k %f", File: "/a/qux.desktop"}, []string{"qux", "/a/qux.desktop", path}},
		{"deprecated dropped", Entry{Exec: "qux %d %D %n %N %v %m %f"}, []string{"qux", path}},
		{"unknown", Entry{Exec: "baz %x"}, nil},
		{"lone percent", Entry{Exec: "baz 50%"}, nil},
		{"embedded icon", Entry{Exec: "baz x%i", Icon: "pic"}, nil},
		{"path appended", Entry{Exec: "qux"}, []string{"qux", path}},
		{"only a non-file code", Entry{Exec: "qux %c", Name: "N"}, []string{"qux", "N", path}},
		{"escaped percent is no code", Entry{Exec: "qux %%f"}, []string{"qux", "%f", path}},
		{"empty quoted word kept", Entry{Exec: `qux "" %f`}, []string{"qux", "", path}},
		{"tabs", Entry{Exec: "qux\t-x   %f"}, []string{"qux", "-x", path}},
		{"unterminated quote", Entry{Exec: `qux "a %f`}, nil},
		{"empty", Entry{Exec: ""}, nil},
		{"blank", Entry{Exec: "   "}, nil},
	}
	for _, c := range cases {
		got, err := ExpandExec(c.entry, path)
		if c.want == nil {
			if err == nil {
				t.Errorf("%s: got %q, want an error", c.name, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestExpandExecPathHasPercent(t *testing.T) {
	// A path is data: a % in it is never read as a field code.
	got, err := ExpandExec(Entry{Exec: "v %f"}, "/tmp/100%x.png")
	if err != nil || !reflect.DeepEqual(got, []string{"v", "/tmp/100%x.png"}) {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := ExpandExec(Entry{Exec: "v %f"}, ""); err == nil {
		t.Fatal("want an error for no path")
	}
}

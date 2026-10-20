package winassoc

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// fakeRegistry is a registry in memory. Like the real one it ignores case in
// key paths and value names.
type fakeRegistry struct {
	// keys maps `ROOT\path` to the key's values in order, as name/data pairs.
	keys map[string][][2]string
}

var errNotFound = errors.New("not found")

func (f *fakeRegistry) key(root, path string) ([][2]string, bool) {
	want := strings.ToLower(root + `\` + path)
	for k, vals := range f.keys {
		if strings.ToLower(k) == want {
			return vals, true
		}
	}
	return nil, false
}

func (f *fakeRegistry) String(root, path, name string) (string, error) {
	vals, ok := f.key(root, path)
	if !ok {
		return "", errNotFound
	}
	for _, v := range vals {
		if strings.EqualFold(v[0], name) {
			return v[1], nil
		}
	}
	return "", errNotFound
}

func (f *fakeRegistry) ValueNames(root, path string) ([]string, error) {
	vals, ok := f.key(root, path)
	if !ok {
		return nil, errNotFound
	}
	var names []string
	for _, v := range vals {
		names = append(names, v[0])
	}
	return names, nil
}

func (f *fakeRegistry) SubKeys(root, path string) ([]string, error) {
	// A key exists when it is listed or any key below it is.
	prefix := strings.ToLower(root + `\` + path + `\`)
	_, ok := f.key(root, path)
	for k := range f.keys {
		ok = ok || strings.HasPrefix(strings.ToLower(k), prefix)
	}
	if !ok {
		return nil, errNotFound
	}
	var subs []string
	for k := range f.keys {
		if strings.HasPrefix(strings.ToLower(k), prefix) {
			sub, _, _ := strings.Cut(k[len(prefix):], `\`)
			if !slices.ContainsFunc(subs, func(s string) bool { return strings.EqualFold(s, sub) }) {
				subs = append(subs, sub)
			}
		}
	}
	slices.Sort(subs)
	return subs, nil
}

// registryFixture holds a .png that three ProgIDs and two programs open, of
// which one ProgID and the default can edit.
func registryFixture() *fakeRegistry {
	r := &fakeRegistry{keys: map[string][][2]string{
		`HKCU\Software\Classes\.png\OpenWithProgids`: {{"Paint.Picture", ""}, {"Viewer.PNG", ""}},
		`HKLM\Software\Classes\.png\OpenWithProgids`: {{"paint.picture", ""}, {"Photos.File", ""}},
		`HKCR\.png`:                              {{"", "pngfile"}},
		`HKCR\.png\OpenWithProgids`:              {{"", ""}, {"Viewer.png", ""}, {"Photos.File", ""}},
		`HKCR\.png\OpenWithList`:                 nil,
		`HKCR\.png\OpenWithList\mspaint.exe`:     nil,
		`HKCU\` + fileExts + `.png\OpenWithList`: {{"a", "gimp.exe"}, {"b", `C:\evil\x.exe`}, {"c", "GIMP.EXE"}, {"d", ""}, {"MRUList", "abcd"}},
		`HKCU\` + fileExts + `.png\UserChoice`:   {{"Hash", "x"}, {"ProgId", "Viewer.PNG"}},

		`HKCR\pngfile\shell\edit\command`:       {{"", "paint.exe %1"}},
		`HKCR\Paint.Picture\shell\edit\command`: {{"", "paint.exe %1"}},
		`HKCR\Paint.Picture\shell\open\command`: {{"", "paint.exe %1"}},
		`HKCR\Viewer.PNG\shell\open\command`:    {{"", "viewer.exe %1"}},
		`HKCR\Photos.File\shell\edit`:           nil, // a verb with no command
	}}
	return r
}

func handlerIDs(hs []Handler) []string {
	var out []string
	for _, h := range hs {
		out = append(out, h.ID)
	}
	return out
}

func TestHandlersFromRegistry(t *testing.T) {
	r := registryFixture()

	// Open: the ProgIDs of user, machine and merged classes, then the
	// programs; each once, whatever its case; a program named by a path, an
	// empty name and the MRU list are skipped.
	open := Handlers(r, ".png", "open")
	want := []Handler{
		{ID: "win:Paint.Picture", ProgID: "Paint.Picture"},
		{ID: "win:Viewer.PNG", ProgID: "Viewer.PNG"},
		{ID: "win:Photos.File", ProgID: "Photos.File"},
		{ID: "win:app:gimp.exe", Exe: "gimp.exe"},
		{ID: "win:app:mspaint.exe", Exe: "mspaint.exe"},
	}
	if !reflect.DeepEqual(open, want) {
		t.Fatalf("open = %+v\nwant %+v", open, want)
	}

	// The extension is matched whatever its case.
	if upper := Handlers(r, ".PNG", "open"); !reflect.DeepEqual(upper, open) {
		t.Errorf(".PNG = %v, want the same as .png: %v", handlerIDs(upper), handlerIDs(open))
	}

	// Edit: only ProgIDs with a shell\edit\command, and no programs.
	if got, want := handlerIDs(Handlers(r, ".png", "edit")), []string{"win:Paint.Picture"}; !reflect.DeepEqual(got, want) {
		t.Errorf("edit = %v, want %v", got, want)
	}

	// Nothing for an unknown type, an empty one, or another verb.
	for _, c := range [][2]string{{".xyz", "open"}, {"", "open"}, {".png", "print"}} {
		if got := Handlers(r, c[0], c[1]); len(got) != 0 {
			t.Errorf("Handlers(%q, %q) = %v, want none", c[0], c[1], handlerIDs(got))
		}
	}
}

func TestDefault(t *testing.T) {
	r := registryFixture()

	// The person's UserChoice beats the class's default value.
	if h, ok := Default(r, ".png", "open"); !ok || h != (Handler{ID: "win:Viewer.PNG", ProgID: "Viewer.PNG"}) {
		t.Errorf("open default = %+v, %v; want Viewer.PNG", h, ok)
	}
	if h, ok := Default(r, ".PNG", "open"); !ok || h.ID != "win:Viewer.PNG" {
		t.Errorf(".PNG open default = %+v, %v; want Viewer.PNG", h, ok)
	}
	// Viewer.PNG cannot edit, so there is no edit default.
	if h, ok := Default(r, ".png", "edit"); ok {
		t.Errorf("edit default = %+v, want none", h)
	}

	// Without a UserChoice, the class's default value; it edits too.
	delete(r.keys, `HKCU\`+fileExts+`.png\UserChoice`)
	if h, ok := Default(r, ".png", "open"); !ok || h != (Handler{ID: "win:pngfile", ProgID: "pngfile"}) {
		t.Errorf("open default = %+v, %v; want pngfile", h, ok)
	}
	if h, ok := Default(r, ".png", "edit"); !ok || h.ID != "win:pngfile" {
		t.Errorf("edit default = %+v, %v; want pngfile", h, ok)
	}

	// Nothing at all.
	delete(r.keys, `HKCR\.png`)
	for _, v := range []string{"open", "edit"} {
		if h, ok := Default(r, ".png", v); ok {
			t.Errorf("%s default = %+v, want none", v, h)
		}
	}
	if h, ok := Default(r, "", "open"); ok {
		t.Errorf("default for no type = %+v", h)
	}
}

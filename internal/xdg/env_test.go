package xdg

import (
	"reflect"
	"testing"
)

func fakeEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestEnvDefaults(t *testing.T) {
	got := envFrom(fakeEnv(map[string]string{"HOME": "/home/ann"}))
	want := Env{
		DataHome:   "/home/ann/.local/share",
		DataDirs:   []string{"/usr/local/share", "/usr/share"},
		ConfigHome: "/home/ann/.config",
		ConfigDirs: []string{"/etc/xdg"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestEnvSetAndInvalid(t *testing.T) {
	got := envFrom(fakeEnv(map[string]string{
		"HOME":                "/home/ann",
		"XDG_DATA_HOME":       "/d/home",
		"XDG_DATA_DIRS":       "/d/one::relative/dir:/d/two",
		"XDG_CONFIG_HOME":     "relative",
		"XDG_CONFIG_DIRS":     "/c/one:/c/two",
		"XDG_CURRENT_DESKTOP": "ubuntu:GNOME::",
	}))
	want := Env{
		DataHome:   "/d/home",
		DataDirs:   []string{"/d/one", "/d/two"},
		ConfigHome: "/home/ann/.config", // a relative value is ignored
		ConfigDirs: []string{"/c/one", "/c/two"},
		Desktops:   []string{"ubuntu", "gnome"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestEnvNoHome(t *testing.T) {
	got := envFrom(fakeEnv(nil))
	if got.DataHome != "" || got.ConfigHome != "" {
		t.Fatalf("homes without HOME: %+v", got)
	}
}

func TestEnvFromOS(t *testing.T) {
	t.Setenv("HOME", "/home/zed")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CURRENT_DESKTOP", "KDE")
	got := EnvFromOS()
	if got.DataHome != "/home/zed/.local/share" || !reflect.DeepEqual(got.Desktops, []string{"kde"}) {
		t.Fatalf("got %+v", got)
	}
}

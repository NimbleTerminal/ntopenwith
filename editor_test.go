package ntopenwith

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// fakeEditor writes an executable file named name into dir and returns its path.
func fakeEditor(t *testing.T, dir, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEditorArgs(t *testing.T) {
	bin := t.TempDir()
	nvim := fakeEditor(t, bin, "nvim")
	emacs := fakeEditor(t, bin, "emacs")
	t.Setenv("PATH", bin)
	dir := filepath.Join(t.TempDir(), "my editors")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	spacedEditor := fakeEditor(t, dir, "ed")

	tests := []struct {
		name    string
		setting string
		want    []string
	}{
		{"exact path with spaces kept whole", spacedEditor, []string{spacedEditor}},
		{"two words", "nvim -p", []string{nvim, "-p"}},
		{"quoted executable then flag", `"emacs" -nw`, []string{emacs, "-nw"}},
		{"quoted path with spaces and flag", `'` + spacedEditor + `' -f`, []string{spacedEditor, "-f"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := editorArgs(tt.setting)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("editorArgs(%q) = %q, want %q", tt.setting, got, tt.want)
			}
		})
	}

	words := strings.TrimSpace(strings.Repeat("nvim ", 64))
	if got, err := editorArgs(words); err != nil || len(got) != 64 {
		t.Fatalf("64 words: %d words, err %v", len(got), err)
	}

	failures := []struct {
		name    string
		setting string
		contain string
	}{
		{"empty", "", "VISUAL"},
		{"too long", strings.Repeat("x", 8193), "8192"},
		{"65 words", words + " nvim", "64 words"},
		{"NUL", "nvim\x00 -p", "NUL"},
		{"unterminated quote", `"nvim -p`, "invalid editor setting"},
		{"quoted pair is one word", `"emacs -nw"`, "terminal editor"},
		{"no such editor", "ntopenwith-no-such-editor-123456", "terminal editor"},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			got, err := editorArgs(tt.setting)
			if err == nil {
				t.Fatalf("editorArgs accepted %q as %q", tt.setting, got)
			}
			if !strings.Contains(err.Error(), tt.contain) {
				t.Fatalf("error %q does not mention %q", err, tt.contain)
			}
		})
	}
}

func TestEditorSetting(t *testing.T) {
	t.Setenv("VISUAL", "  preferred -f ")
	t.Setenv("EDITOR", "fallback")
	if got := editorSetting(); got != "preferred -f" {
		t.Fatalf("VISUAL should win and be trimmed, got %q", got)
	}
	t.Setenv("VISUAL", " \t ")
	if got := editorSetting(); got != "fallback" {
		t.Fatalf("whitespace VISUAL should fall to EDITOR, got %q", got)
	}
	t.Setenv("EDITOR", "  ")
	if got := editorSetting(); got != "" {
		t.Fatalf("both blank should give empty, got %q", got)
	}
}

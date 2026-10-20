//go:build windows

package ntopenwith

import (
	"slices"
	"testing"
)

func TestSplitWordsKeepsWindowsPaths(t *testing.T) {
	got, err := splitWords(`"C:\Program Files\Vim\vim.exe" -n "C:\a b\c"`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{`C:\Program Files\Vim\vim.exe`, "-n", `C:\a b\c`}; !slices.Equal(got, want) {
		t.Fatalf("splitWords = %q, want %q", got, want)
	}
}

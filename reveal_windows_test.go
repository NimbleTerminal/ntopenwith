//go:build windows

package ntopenwith

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Explorer reads /select, with the path quoted inside the one switch; a
// path with a space must not be quoted as a whole argument.
func TestRevealSelectsWithQuotedPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a b")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "f g.txt")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var got *exec.Cmd
	old := startCommand
	startCommand = func(c *exec.Cmd) error { got = c; return nil }
	t.Cleanup(func() { startCommand = old })
	if err := Reveal(file); err != nil {
		t.Fatal(err)
	}
	if got == nil || got.SysProcAttr == nil {
		t.Fatal("nothing started")
	}
	if want := `explorer.exe /select,"` + file + `"`; got.SysProcAttr.CmdLine != want {
		t.Fatalf("CmdLine = %q, want %q", got.SysProcAttr.CmdLine, want)
	}
	if got.SysProcAttr.CreationFlags&0x200 == 0 { // CREATE_NEW_PROCESS_GROUP
		t.Fatal("explorer is not in a process group of its own")
	}
}

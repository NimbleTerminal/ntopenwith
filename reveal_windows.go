//go:build windows

package ntopenwith

import (
	"errors"
	"os/exec"
	"strings"
	"syscall"
)

// selectInExplorer shows p selected in its folder. Explorer reads
// "/select," and the path as one switch with the path in quotes, which
// os/exec cannot write (it would quote the whole argument), so the command
// line is given whole. A Windows path cannot hold a quote; one is refused.
func selectInExplorer(p string) error {
	if strings.ContainsRune(p, '"') {
		return errors.New("path contains a quote")
	}
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + p + `"`}
	return startDetached(cmd)
}

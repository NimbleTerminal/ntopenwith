//go:build windows

package ntopenwith

import "golang.org/x/sys/windows"

// splitWords splits an editor setting as Windows splits a command line, so
// "C:\Program Files\Vim\vim.exe" -n keeps its backslashes.
func splitWords(s string) ([]string, error) { return windows.DecomposeCommandLine(s) }

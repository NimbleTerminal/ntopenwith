//go:build !windows

package ntopenwith

import "github.com/google/shlex"

// splitWords splits an editor setting with POSIX quoting.
func splitWords(s string) ([]string, error) { return shlex.Split(s) }

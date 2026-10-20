//go:build !windows

package ntopenwith

// selectInExplorer is reached only on Windows.
func selectInExplorer(string) error { return ErrUnsupported }

//go:build unix

package ntopenwith

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestHelperRecordsSession is not a test: when GO_NTOPENWITH_SID_OUT is set,
// this test binary, started by TestStartDetachesSession, records its session.
func TestHelperRecordsSession(t *testing.T) {
	out := os.Getenv("GO_NTOPENWITH_SID_OUT")
	if out == "" {
		return
	}
	sid, err := unix.Getsid(0)
	if err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile(out+".tmp", []byte(strconv.Itoa(sid)), 0o600); err != nil {
		os.Exit(2)
	}
	if err := os.Rename(out+".tmp", out); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

// A GUI app started from a terminal must outlive it: it gets a session of its
// own, so the terminal's hangup does not reach it.
func TestStartDetachesSession(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sid")
	t.Setenv("GO_NTOPENWITH_SID_OUT", out)
	if err := Start([]string{os.Args[0], "-test.run=^TestHelperRecordsSession$"}); err != nil {
		t.Fatal(err)
	}
	var (
		data []byte
		err  error
	)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if data, err = os.ReadFile(out); err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("no output from the helper: %v", err)
	}
	child, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	parent, err := unix.Getsid(0)
	if err != nil {
		t.Fatal(err)
	}
	if child == parent {
		t.Errorf("child session %d is the parent's", child)
	}
}

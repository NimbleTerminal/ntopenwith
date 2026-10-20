package system

import (
	"errors"
	"testing"
	"time"

	"nimbleterminal.dev/ntopenwith"
)

func TestCacheExpires(t *testing.T) {
	now := time.Unix(1000, 0)
	oldNow := clock
	t.Cleanup(func() { clock = oldNow })
	clock = func() time.Time { return now }

	var c cache
	calls := 0
	fill := func() ([]ntopenwith.App, error) {
		calls++
		return []ntopenwith.App{{ID: "system"}}, nil
	}
	get := func(key string, v ntopenwith.Verb) {
		t.Helper()
		apps, err := c.get(key, v, fill)
		if err != nil || len(apps) != 1 || apps[0].ID != "system" {
			t.Fatalf("get = %v, %v", apps, err)
		}
	}

	get(".txt", ntopenwith.Open)
	get(".txt", ntopenwith.Open)
	if calls != 1 {
		t.Fatalf("fill ran %d times within the lifetime, want 1", calls)
	}
	get(".txt", ntopenwith.Edit) // another verb is another entry
	get(".pdf", ntopenwith.Open) // another type is another entry
	if calls != 3 {
		t.Fatalf("fill ran %d times for new keys, want 3", calls)
	}

	now = now.Add(lifetime - time.Nanosecond)
	get(".txt", ntopenwith.Open)
	if calls != 3 {
		t.Fatalf("fill ran again just before expiry: %d", calls)
	}
	now = now.Add(time.Nanosecond)
	get(".txt", ntopenwith.Open)
	if calls != 4 {
		t.Fatalf("fill ran %d times after the lifetime, want 4", calls)
	}

	get(".txt", ntopenwith.Open)
	c.clear()
	get(".txt", ntopenwith.Open)
	if calls != 5 {
		t.Fatalf("fill ran %d times after clear, want 5", calls)
	}
}

func TestCacheKeepsNoErrors(t *testing.T) {
	var c cache
	calls := 0
	boom := errors.New("boom")
	fill := func() ([]ntopenwith.App, error) {
		calls++
		return nil, boom
	}
	for i := 0; i < 2; i++ {
		if _, err := c.get(".txt", ntopenwith.Open, fill); err != boom {
			t.Fatalf("err = %v, want boom", err)
		}
	}
	if calls != 2 {
		t.Errorf("fill ran %d times, want 2: errors are not cached", calls)
	}
}

func TestRefreshEmptiesTheCache(t *testing.T) {
	calls := 0
	fill := func() ([]ntopenwith.App, error) {
		calls++
		return nil, nil
	}
	shared.get(".txt", ntopenwith.Open, fill)
	shared.get(".txt", ntopenwith.Open, fill)
	Refresh()
	shared.get(".txt", ntopenwith.Open, fill)
	if calls != 2 {
		t.Errorf("fill ran %d times across Refresh, want 2", calls)
	}
}

func TestSystemIsOnePerProcess(t *testing.T) {
	if System() == nil || System() != System() {
		t.Error("System must return the same non-nil Source each call")
	}
}

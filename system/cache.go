package system

import (
	"sync"
	"time"

	"nimbleterminal.dev/ntopenwith"
)

// lifetime is how long a listing is reused before it is asked for again.
const lifetime = 5 * time.Second

// clock is the time source; tests replace it.
var clock = time.Now

type cacheKey struct {
	key  string // the type, or the path's type, as the caller builds it
	verb ntopenwith.Verb
}

type cacheEntry struct {
	apps    []ntopenwith.App
	expires time.Time
}

// cache keeps recent listings by (type, verb) so a file browser asking again
// for each row does not query the OS every time. It is safe for concurrent
// use; the zero value is ready.
type cache struct {
	mu      sync.Mutex
	entries map[cacheKey]cacheEntry
	gen     uint64 // bumped by clear, so other holders of old data can tell
}

// get returns the listing for key and v, calling fill when there is none or
// it is older than lifetime. A failed fill is not kept.
//
// fill runs with the lock held, so concurrent callers of one cache wait
// rather than ask the OS twice.
func (c *cache) get(key string, v ntopenwith.Verb, fill func() ([]ntopenwith.App, error)) ([]ntopenwith.App, error) {
	return c.lookup(key, v, nil, fill)
}

// getSince is get for a fill built from data read when the cache was at
// generation gen. If the cache has been cleared since, the fill's answer is
// returned but not kept, so a Refresh is never undone by older data.
func (c *cache) getSince(gen uint64, key string, v ntopenwith.Verb, fill func() ([]ntopenwith.App, error)) ([]ntopenwith.App, error) {
	return c.lookup(key, v, &gen, fill)
}

func (c *cache) lookup(key string, v ntopenwith.Verb, gen *uint64, fill func() ([]ntopenwith.App, error)) ([]ntopenwith.App, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := cacheKey{key, v}
	now := clock()
	if e, ok := c.entries[k]; ok && now.Before(e.expires) {
		return append([]ntopenwith.App(nil), e.apps...), nil
	}
	apps, err := fill()
	if err != nil {
		return nil, err
	}
	if gen != nil && *gen != c.gen {
		return append([]ntopenwith.App(nil), apps...), nil
	}
	if c.entries == nil {
		c.entries = make(map[cacheKey]cacheEntry)
	}
	c.entries[k] = cacheEntry{apps: apps, expires: now.Add(lifetime)}
	return append([]ntopenwith.App(nil), apps...), nil
}

// clear drops every listing.
func (c *cache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
	c.gen++
}

// generation counts the times the cache was cleared.
func (c *cache) generation() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gen
}

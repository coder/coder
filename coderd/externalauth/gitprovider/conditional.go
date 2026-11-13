package gitprovider

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// Defaults bound retained entries and individual response bodies.
const (
	// defaultResponseCacheEntries bounds retained entries. Inserts beyond the
	// limit evict the least-recently-used entry.
	defaultResponseCacheEntries = 2048

	// maxCachedBodyBytes bounds one retained body. Oversized entries are ignored.
	maxCachedBodyBytes = 1 << 20 // 1 MiB
)

// cachedResponse holds a validator, an immutable body, and optional expiry.
// Expiry metadata does not prevent conditional validation of a stored response.
type cachedResponse struct {
	key       string
	etag      string
	body      []byte
	expiresAt time.Time
}

// responseCache is a concurrency-safe LRU cache keyed by URL and token hash.
type responseCache struct {
	mu      sync.Mutex
	maxSize int
	ll      *list.List
	entries map[string]*list.Element
}

// newResponseCache constructs an empty cache retaining at most
// maxSize entries. A non-positive maxSize falls back to the default.
func newResponseCache(maxSize int) *responseCache {
	if maxSize <= 0 {
		maxSize = defaultResponseCacheEntries
	}
	return &responseCache{
		maxSize: maxSize,
		ll:      list.New(),
		entries: make(map[string]*list.Element),
	}
}

// load returns the cached ETag and body for key, if present, and
// marks the entry as most-recently-used. The returned body aliases
// the cache's copy and must not be mutated.
func (c *responseCache) load(key string) (etag string, body []byte, ok bool) {
	entry, ok := c.loadEntry(key)
	return entry.etag, entry.body, ok
}

// loadEntry returns a copy and marks it as most-recently-used. The body aliases
// the cache's copy and must not be mutated.
func (c *responseCache) loadEntry(key string) (cachedResponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, found := c.entries[key]
	if !found {
		return cachedResponse{}, false
	}
	c.ll.MoveToFront(elem)
	return *cachedEntry(elem), true
}

// store records the ETag and body for key, evicting the
// least-recently-used entry when the cache is full. Empty ETags and
// bodies larger than maxCachedBodyBytes are not stored.
func (c *responseCache) store(key, etag string, body []byte) {
	if etag == "" {
		return
	}
	c.storeEntry(cachedResponse{key: key, etag: etag, body: body})
}

// storeEntry records an entry regardless of ETag, subject to the body limit and
// LRU eviction.
func (c *responseCache) storeEntry(entry cachedResponse) {
	if len(entry.body) > maxCachedBodyBytes {
		return
	}
	entry.body = bytes.Clone(entry.body)
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, found := c.entries[entry.key]; found {
		c.ll.MoveToFront(elem)
		cr := cachedEntry(elem)
		// Replace the body slice entirely; never write into the
		// existing slice in place. A concurrent reader may hold a
		// reference to the old slice while json.Unmarshal is reading
		// it.
		*cr = entry
		return
	}

	elem := c.ll.PushFront(&entry)
	c.entries[entry.key] = elem

	if c.ll.Len() > c.maxSize {
		c.evictOldest()
	}
}

// active reports whether key is unexpired, deleting expired entries.
func (c *responseCache) active(key string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, found := c.entries[key]
	if !found {
		return false
	}
	if !now.Before(cachedEntry(elem).expiresAt) {
		c.ll.Remove(elem)
		delete(c.entries, key)
		return false
	}
	c.ll.MoveToFront(elem)
	return true
}

// evictOldest removes the least-recently-used entry. The caller must
// hold c.mu.
func (c *responseCache) evictOldest() {
	elem := c.ll.Back()
	if elem == nil {
		return
	}
	c.ll.Remove(elem)
	delete(c.entries, cachedEntry(elem).key)
}

func cachedEntry(elem *list.Element) *cachedResponse {
	//nolint:forcetypeassert // Only *cachedResponse values enter c.ll.
	return elem.Value.(*cachedResponse)
}

// responseCacheKey derives a cache key that isolates responses by
// request URL and token hash. The token is hashed rather than stored
// so that a cached entry for one user's token is never served to
// another, without keeping raw credentials in memory.
func responseCacheKey(requestURL, token string) string {
	sum := sha256.Sum256([]byte(token))
	return requestURL + "\x00" + hex.EncodeToString(sum[:])
}

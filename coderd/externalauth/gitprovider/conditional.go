package gitprovider

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

const (
	// defaultResponseCacheEntries bounds entry count, map and LRU list overhead.
	// It is about 24 times the 2,731 keys derived from the 2026-11-07
	// dogfood snapshot (CODAGT-1543).
	defaultResponseCacheEntries = 65536

	// defaultResponseCacheBytes limits the sum of cachedResponse.sizeBytes.
	// It is not an RSS limit.
	defaultResponseCacheBytes = 128 << 20

	// maxCachedBodyBytes is the largest encoded JSON projection retained.
	maxCachedBodyBytes = 1 << 20 // 1 MiB
)

// cachedResponse holds the ETag sent as If-None-Match.
// Its body field holds the JSON projection decoded after a 304.
type cachedResponse struct {
	key  string
	etag string
	body []byte
}

// sizeBytes counts key and ETag lengths and projection buffer capacity.
// It excludes entry, map, list and allocator overhead.
func (r *cachedResponse) sizeBytes() int {
	return len(r.key) + len(r.etag) + cap(r.body)
}

// responseCache is a concurrency-safe LRU of JSON projections for conditional
// requests. Authenticated 304 responses do not count against GitHub's primary
// rate limit.
type responseCache struct {
	mu         sync.Mutex
	maxEntries int
	maxBytes   int
	usedBytes  int
	ll         *list.List
	entries    map[string]*list.Element
}

// newResponseCache returns an empty response cache with entry and byte limits.
// Non-positive maxEntries or maxBytes select the defaults.
func newResponseCache(maxEntries, maxBytes int) *responseCache {
	if maxEntries <= 0 {
		maxEntries = defaultResponseCacheEntries
	}
	if maxBytes <= 0 {
		maxBytes = defaultResponseCacheBytes
	}
	return &responseCache{
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
		ll:         list.New(),
		entries:    make(map[string]*list.Element),
	}
}

// load returns the cached ETag and body for key, if present, and
// marks the entry as most-recently-used. The returned body aliases
// the cache's copy and must not be mutated.
func (c *responseCache) load(key string) (etag string, body []byte, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, found := c.entries[key]
	if !found {
		return "", nil, false
	}
	c.ll.MoveToFront(elem)
	cr := cachedEntry(elem)
	return cr.etag, cr.body, true
}

// store records the ETag and JSON projection for key and evicts the
// least-recently-used entries until both limits are met. Empty ETags,
// projections larger than maxCachedBodyBytes and entries exceeding the byte
// limit are not stored.
func (c *responseCache) store(key, etag string, body []byte) {
	if etag == "" || len(body) > maxCachedBodyBytes {
		return
	}

	stored := bytes.Clone(body)
	entry := &cachedResponse{
		key:  key,
		etag: etag,
		body: stored,
	}
	entryBytes := entry.sizeBytes()
	if entryBytes > c.maxBytes {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, found := c.entries[key]; found {
		c.ll.MoveToFront(elem)
		cr := cachedEntry(elem)

		c.usedBytes -= cr.sizeBytes()

		cr.etag = etag
		// Replace the body slice entirely; never write into the
		// existing slice in place. A concurrent reader may hold a
		// reference to the old slice while json.Unmarshal is reading
		// it.
		cr.body = stored
	} else {
		c.entries[key] = c.ll.PushFront(entry)
	}

	c.usedBytes += entryBytes

	for c.ll.Len() > 0 && c.overLimit() {
		c.evictOldest()
	}
}

// overLimit reports whether the response cache exceeds either limit.
// The caller must hold c.mu.
func (c *responseCache) overLimit() bool {
	return c.ll.Len() > c.maxEntries || c.usedBytes > c.maxBytes
}

// evictOldest removes the least-recently-used entry. The caller must
// hold c.mu.
func (c *responseCache) evictOldest() {
	elem := c.ll.Back()
	if elem == nil {
		return
	}
	c.ll.Remove(elem)
	cr := cachedEntry(elem)
	delete(c.entries, cr.key)
	c.usedBytes -= cr.sizeBytes()
}

func cachedEntry(elem *list.Element) *cachedResponse {
	//nolint:forcetypeassert // Only *cachedResponse values enter c.ll.
	return elem.Value.(*cachedResponse)
}

// responseCacheKey isolates JSON projections by request URL and token hash
// without retaining the raw token.
func responseCacheKey(requestURL, token string) string {
	sum := sha256.Sum256([]byte(token))
	return requestURL + "\x00" + hex.EncodeToString(sum[:])
}

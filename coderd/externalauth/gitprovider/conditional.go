package gitprovider

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"sync"
)

// Defaults for the conditional-request response cache.
const (
	// defaultResponseCacheEntries bounds entry count, map and LRU list overhead.
	// Entries averaging under 2 KiB reach this limit before the byte limit.
	defaultResponseCacheEntries = 65536

	// defaultResponseCacheBytes limits URL/token-hash string lengths, ETag
	// lengths and JSON projection buffer capacities. Entry, map, list and
	// allocator overhead are excluded.
	defaultResponseCacheBytes = 128 << 20 // 128 MiB

	// maxCachedBodyBytes is the largest encoded JSON projection retained.
	// Larger projections are not stored.
	maxCachedBodyBytes = 1 << 20 // 1 MiB
)

// responseKey identifies a JSON projection by URL, token hash and
// destination type. scope combines the URL and token hash.
type responseKey struct {
	scope    string
	destType reflect.Type
}

// cachedResponse holds the ETag for If-None-Match and body, the JSON
// projection used when GitHub returns 304 Not Modified.
type cachedResponse struct {
	key  responseKey
	etag string
	body []byte
}

func (r *cachedResponse) sizeBytes() int {
	return len(r.key.scope) + len(r.etag) + cap(r.body)
}

// responseCache stores JSON projections in a concurrency-safe LRU, keyed by
// request URL, token hash and destination type. The provider decodes a cached
// JSON projection after a 304 Not Modified response, which GitHub does not
// count against the primary rate limit.
type responseCache struct {
	mu        sync.Mutex
	maxSize   int
	maxBytes  int
	usedBytes int
	ll        *list.List
	entries   map[responseKey]*list.Element
}

// newResponseCache constructs an empty cache retaining at most maxSize
// entries and maxBytes of keys, ETags and projection buffers. Non-positive
// limits fall back to the defaults.
func newResponseCache(maxSize, maxBytes int) *responseCache {
	if maxSize <= 0 {
		maxSize = defaultResponseCacheEntries
	}
	if maxBytes <= 0 {
		maxBytes = defaultResponseCacheBytes
	}
	return &responseCache{
		maxSize:  maxSize,
		maxBytes: maxBytes,
		ll:       list.New(),
		entries:  make(map[responseKey]*list.Element),
	}
}

// load returns the cached ETag and body for key, if present, and
// marks the entry as most-recently-used. The returned body aliases
// the cache's copy and must not be mutated.
func (c *responseCache) load(key responseKey) (etag string, body []byte, ok bool) {
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
func (c *responseCache) store(key responseKey, etag string, body []byte) {
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
	return c.ll.Len() > c.maxSize || c.usedBytes > c.maxBytes
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

// responseCacheKey isolates JSON projections by request URL, token hash
// and destination type without retaining the raw token.
func responseCacheKey(requestURL, token string, dest any) responseKey {
	sum := sha256.Sum256([]byte(token))
	return responseKey{
		scope:    requestURL + "\x00" + hex.EncodeToString(sum[:]),
		destType: reflect.TypeOf(dest),
	}
}

package gitprovider

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
)

// Defaults for the conditional-request response cache.
const (
	// defaultResponseCacheEntries bounds map and LRU metadata independently
	// of response sizes.
	defaultResponseCacheEntries = 8192

	// defaultResponseCacheBytes bounds cached keys, ETags and body buffers.
	// A 128 MiB budget covers thousands of responses sized in tens of KiB.
	defaultResponseCacheBytes = 128 << 20 // 128 MiB

	// maxCachedBodyBytes is the largest response body that will be
	// cached. Larger bodies are still returned to the caller but are
	// not stored, so a single oversized response cannot blow up the
	// cache's memory footprint.
	maxCachedBodyBytes = 1 << 20 // 1 MiB
)

// cachedResponse holds the data needed to satisfy a future
// conditional request: the validator (ETag) to echo back via
// If-None-Match and the body to reuse on a 304 Not Modified.
type cachedResponse struct {
	key  string
	etag string
	body []byte
}

func (r *cachedResponse) sizeBytes() int {
	return len(r.key) + len(r.etag) + cap(r.body)
}

// responseCache is a concurrency-safe LRU cache of GitHub
// responses keyed by request URL and auth scope. It enables
// conditional requests (ETag / If-None-Match): when GitHub replies
// 304 Not Modified the cached body is reused, avoiding a full
// re-download. Conditional requests that return 304 also do not
// count against the primary REST rate limit, which matters for the
// diff-status worker that polls open PRs on a short interval.
type responseCache struct {
	mu        sync.Mutex
	maxSize   int
	maxBytes  int
	sizeBytes int
	ll        *list.List
	entries   map[string]*list.Element
}

// newResponseCache constructs an empty cache retaining at most maxSize
// entries and maxBytes of keys, ETags and body buffers. Non-positive limits
// fall back to the defaults.
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
		entries:  make(map[string]*list.Element),
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

// store records the ETag and body for key, evicting least-recently-used
// entries until both limits are met. Empty ETags, bodies larger than
// maxCachedBodyBytes and entries exceeding the byte budget are not stored.
func (c *responseCache) store(key, etag string, body []byte) {
	if etag == "" || len(body) > maxCachedBodyBytes {
		return
	}

	stored := bytes.Clone(body)
	sizeBytes := len(key) + len(etag) + cap(stored)
	if sizeBytes > c.maxBytes {
		return
	}

	etag = strings.Clone(etag)
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, found := c.entries[key]; found {
		c.ll.MoveToFront(elem)
		cr := cachedEntry(elem)
		c.sizeBytes -= cr.sizeBytes()
		cr.etag = etag
		// Replace the body slice entirely; never write into the
		// existing slice in place. A concurrent reader may hold a
		// reference to the old slice while json.Unmarshal is reading
		// it.
		cr.body = stored
		c.sizeBytes += cr.sizeBytes()
	} else {
		elem := c.ll.PushFront(&cachedResponse{key: key, etag: etag, body: stored})
		c.entries[key] = elem
		c.sizeBytes += sizeBytes
	}

	for c.ll.Len() > c.maxSize || c.sizeBytes > c.maxBytes {
		c.evictOldest()
	}
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
	c.sizeBytes -= cr.sizeBytes()
}

func cachedEntry(elem *list.Element) *cachedResponse {
	//nolint:forcetypeassert // Only *cachedResponse values enter c.ll.
	return elem.Value.(*cachedResponse)
}

// responseCacheKey derives a cache key that isolates responses by
// request URL and auth scope. The token is hashed rather than stored
// so that a cached entry for one user's token is never served to
// another, without keeping raw credentials in memory.
func responseCacheKey(requestURL, token string) string {
	sum := sha256.Sum256([]byte(token))
	return requestURL + "\x00" + hex.EncodeToString(sum[:])
}

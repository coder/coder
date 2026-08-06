package gitprovider

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// Defaults for the conditional-request response cache. These bound
// the memory used by cached GitHub responses while still covering a
// realistic working set of actively-polled pull requests.
const (
	// defaultResponseCacheEntries is the maximum number of cached
	// responses retained. Once exceeded, the least-recently-used
	// entry is evicted.
	defaultResponseCacheEntries = 2048

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

// responseCache is a small, concurrency-safe LRU cache of GitHub
// responses keyed by request URL and auth scope. It enables
// conditional requests (ETag / If-None-Match): when GitHub replies
// 304 Not Modified the cached body is reused, avoiding a full
// re-download. Conditional requests that return 304 also do not
// count against the primary REST rate limit, which matters for the
// diff-status worker that polls open PRs on a short interval.
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
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, found := c.entries[key]
	if !found {
		return "", nil, false
	}
	c.ll.MoveToFront(elem)
	cr := elem.Value.(*cachedResponse)
	return cr.etag, cr.body, true
}

// store records the ETag and body for key, evicting the
// least-recently-used entry when the cache is full. Empty ETags and
// bodies larger than maxCachedBodyBytes are not stored.
func (c *responseCache) store(key, etag string, body []byte) {
	if etag == "" || len(body) > maxCachedBodyBytes {
		return
	}

	stored := bytes.Clone(body)
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, found := c.entries[key]; found {
		c.ll.MoveToFront(elem)
		cr := elem.Value.(*cachedResponse)
		cr.etag = etag
		cr.body = stored
		return
	}

	elem := c.ll.PushFront(&cachedResponse{key: key, etag: etag, body: stored})
	c.entries[key] = elem

	if c.ll.Len() > c.maxSize {
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
	delete(c.entries, elem.Value.(*cachedResponse).key)
}

// responseCacheKey derives a cache key that isolates responses by
// request URL and auth scope. The token is hashed rather than stored
// so that a cached entry for one user's token is never served to
// another, without keeping raw credentials in memory.
func responseCacheKey(requestURL, token string) string {
	sum := sha256.Sum256([]byte(token))
	return requestURL + "\x00" + hex.EncodeToString(sum[:8])
}

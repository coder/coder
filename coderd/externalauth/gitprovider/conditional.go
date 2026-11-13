package gitprovider

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// Defaults for the conditional-request response cache. These bound
// the memory used by cached GitHub responses while still covering a
// realistic working set of actively-polled pull requests.
const (
	// defaultResponseCacheEntries is the maximum number of cached
	// entries retained. Once exceeded, the least-recently-used
	// entry is evicted. Sized above the gitsync worker's steady-state
	// working set: defaultBatchSize (50) rows re-acquired every
	// DiffStatusTTL (120s) over defaultInterval (10s) ticks, at 2-3
	// cache keys per row. See coderd/x/gitsync.
	defaultResponseCacheEntries = 2048

	// maxCachedBodyBytes is the largest response body that will be
	// cached. Larger bodies are still returned to the caller but are
	// not stored, so a single oversized response cannot blow up the
	// cache's memory footprint.
	maxCachedBodyBytes = 1 << 20 // 1 MiB
)

// cachedResponse holds a conditional response or a pull-request snapshot.
// Repository issue-access rejection markers use only key and issueRejectedUntil.
type cachedResponse struct {
	key                string
	etag               string
	body               []byte
	issueRejectedUntil time.Time
}

// responseCache is a small, concurrency-safe LRU cache of GitHub
// responses and issue-access rejection markers keyed by URL and token hash.
// It enables conditional requests (ETag / If-None-Match): when GitHub replies
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
	cr := cachedEntry(elem)
	return cr.etag, cr.body, true
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

// storeEntry also accepts snapshots without an issue ETag and rejection markers.
// They share the response cache's entry limit and LRU eviction.
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

func (c *responseCache) issueRejected(key string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, found := c.entries[key]
	if !found {
		return false
	}
	if !now.Before(cachedEntry(elem).issueRejectedUntil) {
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
// request URL and auth scope. The token is hashed rather than stored
// so that a cached entry for one user's token is never served to
// another, without keeping raw credentials in memory.
func responseCacheKey(requestURL, token string) string {
	sum := sha256.Sum256([]byte(token))
	return requestURL + "\x00" + hex.EncodeToString(sum[:])
}

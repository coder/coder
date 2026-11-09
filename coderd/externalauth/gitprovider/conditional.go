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
	// defaultResponseCacheEntries bounds map and LRU metadata when entries
	// average less than 2 KiB of cached data.
	defaultResponseCacheEntries = 65536

	// defaultResponseCacheBytes bounds keys, ETags and projection buffers.
	defaultResponseCacheBytes = 128 << 20 // 128 MiB

	// maxCachedBodyBytes is the largest encoded JSON projection retained.
	// Larger projections are not stored.
	maxCachedBodyBytes = 1 << 20 // 1 MiB
)

type responseKey struct {
	scope    string
	destType reflect.Type
}

// cachedResponse holds the data needed to satisfy a future
// conditional request: the validator (ETag) to echo back via
// If-None-Match and the body to reuse on a 304 Not Modified.
type cachedResponse struct {
	key  responseKey
	etag string
	body []byte
}

func (r *cachedResponse) sizeBytes() int {
	return len(r.key.scope) + len(r.etag) + cap(r.body)
}

// responseCache is a concurrency-safe LRU cache of GitHub
// responses keyed by request URL, auth scope and destination type. It enables
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

// store records the ETag and body for key, evicting least-recently-used
// entries until both limits are met. Empty ETags, bodies larger than
// maxCachedBodyBytes and entries exceeding the byte budget are not stored.
func (c *responseCache) store(key responseKey, etag string, body []byte) {
	if etag == "" || len(body) > maxCachedBodyBytes {
		return
	}

	stored := bytes.Clone(body)
	sizeBytes := len(key.scope) + len(etag) + cap(stored)
	if sizeBytes > c.maxBytes {
		return
	}

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

	for c.ll.Len() > 0 && (c.ll.Len() > c.maxSize || c.sizeBytes > c.maxBytes) {
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

// responseCacheKey isolates cached JSON projections by request URL, token
// and destination type. Different types may decode different response fields.
// The token is hashed so cached entries do not retain raw credentials.
func responseCacheKey(requestURL, token string, dest any) responseKey {
	sum := sha256.Sum256([]byte(token))
	return responseKey{
		scope:    requestURL + "\x00" + hex.EncodeToString(sum[:]),
		destType: reflect.TypeOf(dest),
	}
}

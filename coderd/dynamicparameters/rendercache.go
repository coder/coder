package dynamicparameters

import (
	"crypto/md5"
	"encoding/hex"
	"sort"
	"sync"

	"github.com/google/uuid"

	"github.com/coder/preview"
)

// RenderCache is a simple in-memory cache for preview.Preview results.
// It caches based on (templateVersionID, ownerID, parameterValues).
type RenderCache struct {
	mu      sync.RWMutex
	entries map[cacheKey]*preview.Output
}

type cacheKey struct {
	templateVersionID uuid.UUID
	ownerID          uuid.UUID
	parameterHash    string
}

// NewRenderCache creates a new render cache.
func NewRenderCache() *RenderCache {
	return &RenderCache{
		entries: make(map[cacheKey]*preview.Output),
	}
}

// NewRenderCacheForTest creates a new render cache for testing purposes.
func NewRenderCacheForTest() *RenderCache {
	return NewRenderCache()
}

func (c *RenderCache) get(templateVersionID, ownerID uuid.UUID, parameters map[string]string) (*preview.Output, bool) {
	key := c.makeKey(templateVersionID, ownerID, parameters)
	c.mu.RLock()
	defer c.mu.RUnlock()

	output, ok := c.entries[key]
	return output, ok
}

func (c *RenderCache) put(templateVersionID, ownerID uuid.UUID, parameters map[string]string, output *preview.Output) {
	key := c.makeKey(templateVersionID, ownerID, parameters)
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = output
}

func (c *RenderCache) makeKey(templateVersionID, ownerID uuid.UUID, parameters map[string]string) cacheKey {
	return cacheKey{
		templateVersionID: templateVersionID,
		ownerID:          ownerID,
		parameterHash:    hashParameters(parameters),
	}
}

// hashParameters creates a deterministic hash of the parameter map.
func hashParameters(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}

	// Sort keys for deterministic hashing
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Hash the sorted key-value pairs
	h := md5.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0}) // separator
		h.Write([]byte(params[k]))
		h.Write([]byte{0}) // separator
	}

	return hex.EncodeToString(h.Sum(nil))
}

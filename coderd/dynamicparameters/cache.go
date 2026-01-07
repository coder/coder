package dynamicparameters

import (
	"crypto/sha256"
	"sort"
	"time"

	"github.com/ammario/tlru"
	"github.com/google/uuid"

	"github.com/coder/preview"
)

// PreviewCacheKey uniquely identifies a preview.Preview call's inputs.
// TemplateVersionID determines PlanJSON, TFVars, and templateFS.
// OwnerID determines the Owner struct.
// ValuesHash captures the ParameterValues map.
type PreviewCacheKey struct {
	TemplateVersionID uuid.UUID
	OwnerID           uuid.UUID
	ValuesHash        [32]byte
}

// PreviewCache caches preview.Output results to avoid redundant computation.
// This is particularly beneficial for prebuilds where inputs are deterministic
// per preset.
type PreviewCache struct {
	cache *tlru.Cache[PreviewCacheKey, *preview.Output]
}

// DefaultPreviewCacheSize is the default number of entries in the cache.
// This is generous for workspace builds since outputs are relatively small.
const DefaultPreviewCacheSize = 64 * 1024

// DefaultPreviewCacheTTL is how long cache entries are valid.
// Template versions don't change frequently, so 5 minutes is reasonable.
const DefaultPreviewCacheTTL = 5 * time.Minute

// NewPreviewCache creates a new preview output cache.
func NewPreviewCache() *PreviewCache {
	return &PreviewCache{
		cache: tlru.New[PreviewCacheKey](tlru.ConstantCost[*preview.Output], DefaultPreviewCacheSize),
	}
}

// Get retrieves a cached preview output if it exists and is still valid.
func (c *PreviewCache) Get(key PreviewCacheKey) (*preview.Output, bool) {
	output, _, ok := c.cache.Get(key)
	return output, ok
}

// Set stores a preview output in the cache with the default TTL.
func (c *PreviewCache) Set(key PreviewCacheKey, output *preview.Output) {
	c.cache.Set(key, output, DefaultPreviewCacheTTL)
}

// HashParameterValues creates a deterministic hash of the parameter values map.
// The keys are sorted to ensure consistent hashing regardless of map iteration order.
func HashParameterValues(values map[string]string) [32]byte {
	// Sort keys for deterministic ordering
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		// Write key, separator, value, separator
		// Using null bytes as separators to avoid collisions.
		// sha256.Write never returns an error, so we ignore the return values.
		_, _ = h.Write([]byte(k))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(values[k]))
		_, _ = h.Write([]byte{0})
	}

	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

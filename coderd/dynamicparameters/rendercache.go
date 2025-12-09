package dynamicparameters

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/preview"
	"github.com/coder/quartz"
)

// RenderCache is a simple in-memory cache for preview.Preview results.
// It caches based on (templateVersionID, ownerID, parameterValues).
type RenderCache struct {
	mu      sync.RWMutex
	entries map[cacheKey]*cacheEntry

	// Metrics (optional)
	cacheHits   prometheus.Counter
	cacheMisses prometheus.Counter
	cacheSize   prometheus.Gauge

	// TTL cleanup
	clock    quartz.Clock
	ttl      time.Duration
	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

type cacheEntry struct {
	output    *preview.Output
	timestamp time.Time
}

type cacheKey struct {
	templateVersionID uuid.UUID
	ownerID          uuid.UUID
	parameterHash    string
}

// NewRenderCache creates a new render cache with a default TTL of 1 hour.
func NewRenderCache() *RenderCache {
	return newRenderCache(quartz.NewReal(), time.Hour, nil, nil, nil)
}

// NewRenderCacheWithMetrics creates a new render cache with Prometheus metrics.
func NewRenderCacheWithMetrics(cacheHits, cacheMisses prometheus.Counter, cacheSize prometheus.Gauge) *RenderCache {
	return newRenderCache(quartz.NewReal(), time.Hour, cacheHits, cacheMisses, cacheSize)
}

func newRenderCache(clock quartz.Clock, ttl time.Duration, cacheHits, cacheMisses prometheus.Counter, cacheSize prometheus.Gauge) *RenderCache {
	c := &RenderCache{
		entries:     make(map[cacheKey]*cacheEntry),
		clock:       clock,
		cacheHits:   cacheHits,
		cacheMisses: cacheMisses,
		cacheSize:   cacheSize,
		ttl:         ttl,
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}

	// Start cleanup goroutine
	go c.cleanupLoop(context.Background())

	return c
}

// NewRenderCacheForTest creates a new render cache for testing purposes.
func NewRenderCacheForTest() *RenderCache {
	return NewRenderCache()
}

// Close stops the cleanup goroutine and waits for it to finish.
func (c *RenderCache) Close() {
	c.stopOnce.Do(func() {
		close(c.stopCh)
		<-c.doneCh
	})
}

func (c *RenderCache) get(templateVersionID, ownerID uuid.UUID, parameters map[string]string) (*preview.Output, bool) {
	key := c.makeKey(templateVersionID, ownerID, parameters)
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok {
		// Record miss
		if c.cacheMisses != nil {
			c.cacheMisses.Inc()
		}
		return nil, false
	}

	// Check if entry has expired
	if c.clock.Since(entry.timestamp) > c.ttl {
		// Expired entry, treat as miss
		if c.cacheMisses != nil {
			c.cacheMisses.Inc()
		}
		return nil, false
	}

	// Record hit
	if c.cacheHits != nil {
		c.cacheHits.Inc()
	}

	return entry.output, true
}

func (c *RenderCache) put(templateVersionID, ownerID uuid.UUID, parameters map[string]string, output *preview.Output) {
	key := c.makeKey(templateVersionID, ownerID, parameters)
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &cacheEntry{
		output:    output,
		timestamp: c.clock.Now(),
	}

	// Update cache size metric
	if c.cacheSize != nil {
		c.cacheSize.Set(float64(len(c.entries)))
	}
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

// cleanupLoop runs periodically to remove expired cache entries.
func (c *RenderCache) cleanupLoop(ctx context.Context) {
	defer close(c.doneCh)

	// Run cleanup every 15 minutes
	cleanupFunc := func() error {
		c.cleanup()
		return nil
	}

	// Run once immediately
	_ = cleanupFunc()

	// Create a cancellable context for the ticker
	tickerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Create ticker for periodic cleanup
	tkr := c.clock.TickerFunc(tickerCtx, 15*time.Minute, cleanupFunc, "render-cache-cleanup")

	// Wait for stop signal
	<-c.stopCh
	cancel()

	_ = tkr.Wait()
}

// cleanup removes expired entries from the cache.
func (c *RenderCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.clock.Now()
	for key, entry := range c.entries {
		if now.Sub(entry.timestamp) > c.ttl {
			delete(c.entries, key)
		}
	}

	// Update cache size metric after cleanup
	if c.cacheSize != nil {
		c.cacheSize.Set(float64(len(c.entries)))
	}
}

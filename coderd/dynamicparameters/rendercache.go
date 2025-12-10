package dynamicparameters

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/preview"
	"github.com/coder/quartz"
)

// RenderCacheImpl is a simple in-memory cache for preview.Preview results.
// It caches based on (templateVersionID, ownerID, parameterValues).
type RenderCacheImpl struct {
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
	ownerID           uuid.UUID
	parameterHash     uint64
}

// NewRenderCache creates a new render cache with a default TTL of 1 hour.
func NewRenderCache() *RenderCacheImpl {
	return newCache(quartz.NewReal(), time.Hour, nil, nil, nil)
}

// NewRenderCacheWithMetrics creates a new render cache with Prometheus metrics.
func NewRenderCacheWithMetrics(cacheHits, cacheMisses prometheus.Counter, cacheSize prometheus.Gauge) *RenderCacheImpl {
	return newCache(quartz.NewReal(), time.Hour, cacheHits, cacheMisses, cacheSize)
}

func newCache(clock quartz.Clock, ttl time.Duration, cacheHits, cacheMisses prometheus.Counter, cacheSize prometheus.Gauge) *RenderCacheImpl {
	c := &RenderCacheImpl{
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
func NewRenderCacheForTest() *RenderCacheImpl {
	return NewRenderCache()
}

// Close stops the cleanup goroutine and waits for it to finish.
func (c *RenderCacheImpl) Close() {
	c.stopOnce.Do(func() {
		close(c.stopCh)
		<-c.doneCh
	})
}

func (c *RenderCacheImpl) get(templateVersionID, ownerID uuid.UUID, parameters map[string]string) (*preview.Output, bool) {
	key := makeKey(templateVersionID, ownerID, parameters)
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

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

	// Record hit and refresh timestamp
	if c.cacheHits != nil {
		c.cacheHits.Inc()
	}

	// Refresh timestamp on hit to keep frequently accessed entries alive
	c.mu.Lock()
	entry.timestamp = c.clock.Now()
	c.mu.Unlock()

	return entry.output, true
}

func (c *RenderCacheImpl) put(templateVersionID, ownerID uuid.UUID, parameters map[string]string, output *preview.Output) {
	key := makeKey(templateVersionID, ownerID, parameters)
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

func makeKey(templateVersionID, ownerID uuid.UUID, parameters map[string]string) cacheKey {
	return cacheKey{
		templateVersionID: templateVersionID,
		ownerID:           ownerID,
		parameterHash:     hashParameters(parameters),
	}
}

// hashParameters creates a deterministic hash of the parameter map.
func hashParameters(params map[string]string) uint64 {
	if len(params) == 0 {
		return 0
	}

	// Sort keys for deterministic hashing
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Hash the sorted key-value pairs
	var b string
	for _, k := range keys {
		b += fmt.Sprintf("%s:%s,", k, params[k])
	}

	return xxhash.Sum64String(b)
}

// cleanupLoop runs periodically to remove expired cache entries.
func (c *RenderCacheImpl) cleanupLoop(ctx context.Context) {
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
func (c *RenderCacheImpl) cleanup() {
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

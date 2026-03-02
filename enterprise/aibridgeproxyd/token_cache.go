package aibridgeproxyd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/coder/quartz"
)

// TokenCache caches validated tokens to avoid repeated validation calls.
// Tokens are hashed for security and cached with an expiry time.
// The cache runs a background goroutine to clean up expired entries.
type TokenCache struct {
	mu              sync.RWMutex
	entries         map[string]time.Time // SHA256(token) -> expiry time
	ttl             time.Duration
	cleanupInterval time.Duration
	clock           quartz.Clock
}

// NewTokenCache creates a new token cache with the specified TTL and starts
// a background cleanup goroutine that removes expired entries.
func NewTokenCache(ctx context.Context, ttl, cleanupInterval time.Duration, clock quartz.Clock) *TokenCache {
	c := &TokenCache{
		entries:         make(map[string]time.Time),
		ttl:             ttl,
		cleanupInterval: cleanupInterval,
		clock:           clock,
	}

	// Start background cleanup goroutine.
	// Runs every cleanupInterval to remove expired entries.
	go func() {
		ticker := clock.NewTicker(cleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.Cleanup()
			}
		}
	}()

	return c
}

// IsValid checks if a token is in the cache and not expired.
func (c *TokenCache) IsValid(tokenHash string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	expiry, ok := c.entries[tokenHash]
	if !ok {
		return false
	}

	return c.clock.Now().Before(expiry)
}

// Add adds a token to the cache with the configured TTL.
func (c *TokenCache) Add(tokenHash string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[tokenHash] = c.clock.Now().Add(c.ttl)
}

// Cleanup removes expired entries from the cache.
// Called periodically by background goroutine.
func (c *TokenCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.clock.Now()
	for hash, expiry := range c.entries {
		if now.After(expiry) {
			delete(c.entries, hash)
		}
	}
}

// HasEntry checks if a token hash exists in the cache, regardless of expiry.
// This is useful for testing to verify that expired tokens still exist before
// cleanup runs.
func (c *TokenCache) HasEntry(tokenHash string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, exists := c.entries[tokenHash]
	return exists
}

// HashToken creates a SHA256 hash of the token for cache keys.
// This avoids storing raw tokens in memory.
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

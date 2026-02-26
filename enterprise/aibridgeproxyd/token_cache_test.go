package aibridgeproxyd

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestTokenCache(t *testing.T) {
	t.Parallel()

	t.Run("AddAndIsValid", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		clock := quartz.NewMock(t)
		ttl := 100 * time.Millisecond
		cleanupInterval := 200 * time.Millisecond
		cache := newTokenCache(ctx, ttl, cleanupInterval, clock)

		tokenHash := hashToken("test-token")

		// Token not in cache initially.
		require.False(t, cache.isValid(tokenHash), "token should not be valid before adding")

		// Add token to cache.
		cache.add(tokenHash)

		// Token should now be valid.
		require.True(t, cache.isValid(tokenHash), "token should be valid after adding")
	})

	t.Run("TokenExpiry", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		clock := quartz.NewMock(t)
		ttl := 100 * time.Millisecond
		cleanupInterval := 200 * time.Millisecond
		cache := newTokenCache(ctx, ttl, cleanupInterval, clock)

		tokenHash := hashToken("test-token")
		cache.add(tokenHash)

		// Token valid immediately.
		require.True(t, cache.isValid(tokenHash), "token should be valid immediately after adding")

		// Advance time past TTL but before cleanup.
		clock.Advance(150 * time.Millisecond).MustWait(ctx)

		// Token should be expired (isValid returns false) but still in cache.
		require.False(t, cache.isValid(tokenHash), "token should be expired after TTL")
		cache.mu.RLock()
		_, exists := cache.entries[tokenHash]
		cache.mu.RUnlock()
		require.True(t, exists, "expired token should still exist in cache before cleanup")
	})

	t.Run("CleanupRemovesExpiredTokens", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		clock := quartz.NewMock(t)
		ttl := 100 * time.Millisecond
		cleanupInterval := 200 * time.Millisecond
		cache := newTokenCache(ctx, ttl, cleanupInterval, clock)

		tokenHash := hashToken("test-token")
		cache.add(tokenHash)

		// Advance time past TTL.
		clock.Advance(150 * time.Millisecond).MustWait(ctx)

		// Token expired but still in cache.
		require.False(t, cache.isValid(tokenHash), "token should be expired")
		cache.mu.RLock()
		_, exists := cache.entries[tokenHash]
		cache.mu.RUnlock()
		require.True(t, exists, "expired token should still exist before cleanup")

		// Manually trigger cleanup.
		cache.cleanup()

		// Token should now be removed from cache.
		cache.mu.RLock()
		_, exists = cache.entries[tokenHash]
		cache.mu.RUnlock()
		require.False(t, exists, "expired token should be removed after cleanup")
	})

	t.Run("CleanupKeepsValidTokens", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		clock := quartz.NewMock(t)
		ttl := 200 * time.Millisecond
		cleanupInterval := 200 * time.Millisecond
		cache := newTokenCache(ctx, ttl, cleanupInterval, clock)

		tokenHash := hashToken("test-token")
		cache.add(tokenHash)

		// Token valid initially.
		require.True(t, cache.isValid(tokenHash), "token should be valid initially")

		// Advance time partway through TTL.
		clock.Advance(100 * time.Millisecond).MustWait(ctx)

		// Token still valid.
		require.True(t, cache.isValid(tokenHash), "token should still be valid")

		// Manually trigger cleanup while token is still valid.
		cache.cleanup()

		// Token should still exist in cache.
		cache.mu.RLock()
		_, exists := cache.entries[tokenHash]
		cache.mu.RUnlock()
		require.True(t, exists, "valid token should not be removed by cleanup")
		require.True(t, cache.isValid(tokenHash), "token should still be valid after cleanup")
	})

	t.Run("MultipleTokens", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		clock := quartz.NewMock(t)
		ttl := 100 * time.Millisecond
		cleanupInterval := 200 * time.Millisecond
		cache := newTokenCache(ctx, ttl, cleanupInterval, clock)

		token1Hash := hashToken("token1")
		token2Hash := hashToken("token2")

		// Add first token at time 0.
		cache.add(token1Hash)

		// Advance time partway.
		clock.Advance(50 * time.Millisecond).MustWait(ctx)

		// Add second token at time 50ms.
		cache.add(token2Hash)

		// Both tokens valid at time 50ms.
		require.True(t, cache.isValid(token1Hash), "token1 should be valid")
		require.True(t, cache.isValid(token2Hash), "token2 should be valid")

		// Advance to 110ms total (token1 expires at 100ms, token2 expires at 150ms).
		clock.Advance(60 * time.Millisecond).MustWait(ctx)

		// Token1 expired, token2 still valid.
		require.False(t, cache.isValid(token1Hash), "token1 should be expired")
		require.True(t, cache.isValid(token2Hash), "token2 should still be valid")

		// Advance to 160ms and manually trigger cleanup.
		clock.Advance(50 * time.Millisecond).MustWait(ctx)
		cache.cleanup()

		// Cleanup removed token1 (expired at 100ms) and token2 (expired at 150ms).
		cache.mu.RLock()
		_, exists1 := cache.entries[token1Hash]
		_, exists2 := cache.entries[token2Hash]
		cache.mu.RUnlock()
		require.False(t, exists1, "token1 should be removed after cleanup")
		require.False(t, exists2, "token2 should be removed after cleanup")
	})

	t.Run("BackgroundCleanupGoroutine", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		ttl := 50 * time.Millisecond
		cleanupInterval := 100 * time.Millisecond
		clock := quartz.NewMock(t)

		// Trap the NewTicker call to synchronize with goroutine startup.
		trap := clock.Trap().NewTicker()
		defer trap.Close()

		cache := newTokenCache(ctx, ttl, cleanupInterval, clock)

		// Wait for the cleanup goroutine to create the ticker.
		call := trap.MustWait(ctx)
		call.MustRelease(ctx)

		tokenHash := hashToken("test-token")
		cache.add(tokenHash) // Token expires at 50ms.

		// Token should exist initially.
		cache.mu.RLock()
		_, exists := cache.entries[tokenHash]
		cache.mu.RUnlock()
		require.True(t, exists, "token should exist before cleanup")

		// Advance clock to trigger cleanup (at 100ms).
		// The token expires at 50ms, so cleanup will remove it.
		clock.Advance(cleanupInterval).MustWait(ctx)

		// Wait for background cleanup goroutine to process the tick.
		require.Eventually(t, func() bool {
			cache.mu.RLock()
			defer cache.mu.RUnlock()
			_, exists := cache.entries[tokenHash]
			return !exists
		}, testutil.WaitShort, testutil.IntervalFast, "background cleanup should remove expired token")
	})
}

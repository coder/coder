package aibridgeproxyd_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/enterprise/aibridgeproxyd"
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
		cache := aibridgeproxyd.NewTokenCache(ctx, ttl, cleanupInterval, clock)

		tokenHash := aibridgeproxyd.HashToken("test-token")

		// Token not in cache initially.
		require.False(t, cache.IsValid(tokenHash), "token should not be valid before adding")

		// Add token to cache.
		cache.Add(tokenHash)

		// Token should now be valid.
		require.True(t, cache.IsValid(tokenHash), "token should be valid after adding")
	})

	t.Run("TokenExpiry", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		clock := quartz.NewMock(t)
		ttl := 100 * time.Millisecond
		cleanupInterval := 200 * time.Millisecond
		cache := aibridgeproxyd.NewTokenCache(ctx, ttl, cleanupInterval, clock)

		tokenHash := aibridgeproxyd.HashToken("test-token")
		cache.Add(tokenHash)

		// Token valid immediately.
		require.True(t, cache.IsValid(tokenHash), "token should be valid immediately after adding")

		// Advance time past TTL but before cleanup.
		clock.Advance(150 * time.Millisecond).MustWait(ctx)

		// Token should be expired (isValid returns false) but still in cache.
		require.False(t, cache.IsValid(tokenHash), "token should be expired after TTL")
		require.True(t, cache.HasEntry(tokenHash), "expired token should still exist in cache before cleanup")
	})

	t.Run("CleanupRemovesExpiredTokens", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		clock := quartz.NewMock(t)
		ttl := 100 * time.Millisecond
		cleanupInterval := 200 * time.Millisecond
		cache := aibridgeproxyd.NewTokenCache(ctx, ttl, cleanupInterval, clock)

		tokenHash := aibridgeproxyd.HashToken("test-token")
		cache.Add(tokenHash)

		// Advance time past TTL.
		clock.Advance(150 * time.Millisecond).MustWait(ctx)

		// Token expired but still in cache.
		require.False(t, cache.IsValid(tokenHash), "token should be expired")
		require.True(t, cache.HasEntry(tokenHash), "expired token should still exist before cleanup")

		// Manually trigger cleanup.
		cache.Cleanup()

		// Token should now be removed from cache.
		require.False(t, cache.HasEntry(tokenHash), "expired token should be removed after cleanup")
	})

	t.Run("CleanupKeepsValidTokens", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		clock := quartz.NewMock(t)
		ttl := 200 * time.Millisecond
		cleanupInterval := 200 * time.Millisecond
		cache := aibridgeproxyd.NewTokenCache(ctx, ttl, cleanupInterval, clock)

		tokenHash := aibridgeproxyd.HashToken("test-token")
		cache.Add(tokenHash)

		// Token valid initially.
		require.True(t, cache.IsValid(tokenHash), "token should be valid initially")

		// Advance time partway through TTL.
		clock.Advance(100 * time.Millisecond).MustWait(ctx)

		// Token still valid.
		require.True(t, cache.IsValid(tokenHash), "token should still be valid")

		// Manually trigger cleanup while token is still valid.
		cache.Cleanup()

		// Token should still exist in cache.
		require.True(t, cache.HasEntry(tokenHash), "valid token should not be removed by cleanup")
		require.True(t, cache.IsValid(tokenHash), "token should still be valid after cleanup")
	})

	t.Run("MultipleTokens", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		clock := quartz.NewMock(t)
		ttl := 100 * time.Millisecond
		cleanupInterval := 200 * time.Millisecond
		cache := aibridgeproxyd.NewTokenCache(ctx, ttl, cleanupInterval, clock)

		token1Hash := aibridgeproxyd.HashToken("token1")
		token2Hash := aibridgeproxyd.HashToken("token2")

		// Add first token at time 0.
		cache.Add(token1Hash)

		// Advance time partway.
		clock.Advance(50 * time.Millisecond).MustWait(ctx)

		// Add second token at time 50ms.
		cache.Add(token2Hash)

		// Both tokens valid at time 50ms.
		require.True(t, cache.IsValid(token1Hash), "token1 should be valid")
		require.True(t, cache.IsValid(token2Hash), "token2 should be valid")

		// Advance to 110ms total (token1 expires at 100ms, token2 expires at 150ms).
		clock.Advance(60 * time.Millisecond).MustWait(ctx)

		// Token1 expired, token2 still valid.
		require.False(t, cache.IsValid(token1Hash), "token1 should be expired")
		require.True(t, cache.IsValid(token2Hash), "token2 should still be valid")

		// Advance to 160ms and manually trigger cleanup.
		clock.Advance(50 * time.Millisecond).MustWait(ctx)
		cache.Cleanup()

		// Cleanup removed token1 (expired at 100ms) and token2 (expired at 150ms).
		require.False(t, cache.HasEntry(token1Hash), "token1 should be removed after cleanup")
		require.False(t, cache.HasEntry(token2Hash), "token2 should be removed after cleanup")
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

		cache := aibridgeproxyd.NewTokenCache(ctx, ttl, cleanupInterval, clock)

		// Wait for the cleanup goroutine to create the ticker.
		call := trap.MustWait(ctx)
		call.MustRelease(ctx)

		tokenHash := aibridgeproxyd.HashToken("test-token")
		cache.Add(tokenHash) // Token expires at 50ms.

		// Token should exist initially.
		require.True(t, cache.HasEntry(tokenHash), "token should exist before cleanup")

		// Advance clock to trigger cleanup (at 100ms).
		// The token expires at 50ms, so cleanup will remove it.
		clock.Advance(cleanupInterval).MustWait(ctx)

		// Wait for background cleanup goroutine to process the tick.
		require.Eventually(t, func() bool {
			return !cache.HasEntry(tokenHash)
		}, testutil.WaitShort, testutil.IntervalFast, "background cleanup should remove expired token")
	})
}

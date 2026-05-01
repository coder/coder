package aibridged_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/mcpmock"
	"github.com/coder/coder/v2/coderd/aibridged"
	mock "github.com/coder/coder/v2/coderd/aibridged/aibridgedmock"
)

// TestPoolReload exercises CachedBridgePool.Reload, ensuring that
// the cache is cleared after a hot-swap so the next Acquire builds a
// fresh RequestBridge against the new providers.
func TestPoolReload(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	ctrl := gomock.NewController(t)
	client := mock.NewMockDRPCClient(ctrl)
	mcpProxy := mcpmock.NewMockServerProxier(ctrl)
	clientFn := func() (aibridged.DRPCClient, error) {
		return client, nil
	}

	mcpProxy.EXPECT().Init(gomock.Any()).AnyTimes().Return(nil)
	mcpProxy.EXPECT().Shutdown(gomock.Any()).AnyTimes().Return(nil)

	opts := aibridged.PoolOptions{MaxItems: 8, TTL: time.Minute}
	pool, err := aibridged.NewCachedBridgePool(opts, []aibridge.Provider{
		aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{
			Name:    "openai",
			BaseURL: "https://api.openai.com/v1",
			Key:     "sk-old",
		}),
	}, logger, nil, testTracer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	id, apiKeyID := uuid.New(), uuid.New()
	req := aibridged.Request{InitiatorID: id, APIKeyID: apiKeyID.String()}

	// Prime the cache.
	_, err = pool.Acquire(t.Context(), req, clientFn, newMockMCPFactory(mcpProxy))
	require.NoError(t, err)

	cm := pool.CacheMetrics()
	require.EqualValues(t, 1, cm.Misses())
	require.EqualValues(t, 1, cm.KeysAdded())

	// Reload with a new provider set. ristretto.Cache.Clear()
	// resets metrics to zero alongside emptying the cache.
	pool.Reload([]aibridge.Provider{
		aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{
			Name:    "openai",
			BaseURL: "https://api.openai.com/v1",
			Key:     "sk-new",
		}),
	})

	cm = pool.CacheMetrics()
	require.EqualValues(t, 0, cm.KeysAdded(), "expected metrics to be reset after Reload")

	// After Reload, the next Acquire should be a miss because the
	// cache was cleared, and a new RequestBridge gets built.
	_, err = pool.Acquire(t.Context(), req, clientFn, newMockMCPFactory(mcpProxy))
	require.NoError(t, err)

	cm = pool.CacheMetrics()
	require.EqualValues(t, 1, cm.Misses(), "expected one cache miss after reload")
	require.EqualValues(t, 1, cm.KeysAdded(), "expected new key added after reload")

	// Wait briefly for ristretto's eviction goroutines (spawned by
	// the OnEvict callback during Reload) to settle so gomock's
	// teardown does not race with their Shutdown calls. The Shutdown
	// expectation is set with AnyTimes() so the assertion does not
	// require an exact count, but ctrl.Finish does need to see the
	// call complete.
	time.Sleep(100 * time.Millisecond)
}

// TestPoolReloadAfterShutdown verifies Reload is a no-op when the
// pool has already been shut down.
func TestPoolReloadAfterShutdown(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	pool, err := aibridged.NewCachedBridgePool(
		aibridged.DefaultPoolOptions, nil, logger, nil, testTracer,
	)
	require.NoError(t, err)
	require.NoError(t, pool.Shutdown(context.Background()))

	// Should not panic or hang.
	pool.Reload([]aibridge.Provider{
		aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{
			Name:    "openai",
			BaseURL: "https://api.openai.com/v1",
			Key:     "sk",
		}),
	})
}

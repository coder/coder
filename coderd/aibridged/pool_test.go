package aibridged_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/mcp"
	"github.com/coder/coder/v2/aibridge/mcpmock"
	"github.com/coder/coder/v2/coderd/aibridged"
	mock "github.com/coder/coder/v2/coderd/aibridged/aibridgedmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// TestPool validates the published behavior of [aibridged.CachedBridgePool].
// It is not meant to be an exhaustive test of the internal cache's functionality,
// since that is already covered by its library.
func TestPool(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)

	ctrl := gomock.NewController(t)
	client := mock.NewMockDRPCClient(ctrl)
	mcpProxy := mcpmock.NewMockServerProxier(ctrl)

	opts := aibridged.PoolOptions{MaxItems: 1, TTL: time.Second}
	pool, err := aibridged.NewCachedBridgePool(opts, nil, logger, nil, testTracer)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Shutdown(context.Background()) })

	id, id2, apiKeyID1, apiKeyID2 := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	clientFn := func(context.Context) (aibridged.DRPCClient, error) {
		return client, nil
	}

	// Once a pool instance is initialized, it will try setup its MCP proxier(s).
	// This is called exactly once since the instance below is only created once.
	mcpProxy.EXPECT().Init(gomock.Any()).Times(1).Return(nil)
	// This is part of the lifecycle.
	mcpProxy.EXPECT().Shutdown(gomock.Any()).AnyTimes().Return(nil)

	// Serving a request for an initiator ID will create a bridge the first time
	// it is seen...
	req1 := aibridged.Request{
		SessionKey:  "key",
		InitiatorID: id,
		APIKeyID:    apiKeyID1.String(),
	}
	err = servePool(t.Context(), pool, req1, clientFn, newMockMCPFactory(mcpProxy))
	require.NoError(t, err, "serve pool instance")

	// ...and it will reuse it when served again.
	err = servePool(t.Context(), pool, req1, clientFn, newMockMCPFactory(mcpProxy))
	require.NoError(t, err, "serve pool instance")

	cacheMetrics := pool.CacheMetrics()
	require.EqualValues(t, 1, cacheMetrics.KeysAdded())
	require.EqualValues(t, 0, cacheMetrics.KeysEvicted())
	require.EqualValues(t, 1, cacheMetrics.Hits())
	require.EqualValues(t, 1, cacheMetrics.Misses())

	// This will get called again because a new instance will be created.
	mcpProxy.EXPECT().Init(gomock.Any()).Times(1).Return(nil)

	// But that key will be evicted when a new initiator is seen (maxItems=1):
	req2 := aibridged.Request{
		SessionKey:  "key",
		InitiatorID: id2,
		APIKeyID:    apiKeyID1.String(),
	}
	err = servePool(t.Context(), pool, req2, clientFn, newMockMCPFactory(mcpProxy))
	require.NoError(t, err, "serve pool instance")

	cacheMetrics = pool.CacheMetrics()
	require.EqualValues(t, 2, cacheMetrics.KeysAdded())
	require.EqualValues(t, 1, cacheMetrics.KeysEvicted())
	require.EqualValues(t, 1, cacheMetrics.Hits())
	require.EqualValues(t, 2, cacheMetrics.Misses())

	// This will get called again because a new instance will be created.
	mcpProxy.EXPECT().Init(gomock.Any()).Times(1).Return(nil)

	// New instance is created for different api key id
	req2B := aibridged.Request{
		SessionKey:  "key",
		InitiatorID: id2,
		APIKeyID:    apiKeyID2.String(),
	}
	err = servePool(t.Context(), pool, req2B, clientFn, newMockMCPFactory(mcpProxy))
	require.NoError(t, err, "serve pool instance 2B")

	cacheMetrics = pool.CacheMetrics()
	require.EqualValues(t, 3, cacheMetrics.KeysAdded())
	require.EqualValues(t, 2, cacheMetrics.KeysEvicted())
	require.EqualValues(t, 1, cacheMetrics.Hits())
	require.EqualValues(t, 3, cacheMetrics.Misses())
}

func TestPoolReplaceProvidersUsesNewProviders(t *testing.T) {
	t.Parallel()

	oldUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "old")
	}))
	t.Cleanup(oldUpstream.Close)
	newUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "new")
	}))
	t.Cleanup(newUpstream.Close)

	logger := slogtest.Make(t, nil)
	ctrl := gomock.NewController(t)
	client := mock.NewMockDRPCClient(ctrl)
	mcpProxy := mcpmock.NewMockServerProxier(ctrl)
	mcpProxy.EXPECT().Init(gomock.Any()).AnyTimes().Return(nil)
	mcpProxy.EXPECT().Shutdown(gomock.Any()).AnyTimes().Return(nil)

	opts := aibridged.PoolOptions{MaxItems: 1, TTL: time.Minute}
	pool, err := aibridged.NewCachedBridgePool(opts, []aibridge.Provider{
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "old", BaseURL: oldUpstream.URL}),
	}, logger, nil, testTracer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	req := aibridged.Request{
		SessionKey:  "key",
		InitiatorID: uuid.New(),
		APIKeyID:    uuid.New().String(),
	}
	clientFn := func(context.Context) (aibridged.DRPCClient, error) {
		return client, nil
	}

	assertPoolBody(t, pool, req, clientFn, newMockMCPFactory(mcpProxy), "/old/v1/models", "old")

	pool.ReplaceProviders([]aibridge.Provider{
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "new", BaseURL: newUpstream.URL}),
	})

	assertPoolBody(t, pool, req, clientFn, newMockMCPFactory(mcpProxy), "/new/v1/models", "new")
}

func TestPoolReplaceProvidersDuringCachePublication(t *testing.T) {
	t.Parallel()

	oldUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "old")
	}))
	t.Cleanup(oldUpstream.Close)
	newUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "new")
	}))
	t.Cleanup(newUpstream.Close)

	ctx := testutil.Context(t, testutil.WaitShort)
	clk := quartz.NewMock(t)
	ctrl := gomock.NewController(t)
	client := mock.NewMockDRPCClient(ctrl)
	mcpProxy := mcpmock.NewMockServerProxier(ctrl)
	mcpProxy.EXPECT().Init(gomock.Any()).AnyTimes().Return(nil)
	mcpProxy.EXPECT().Shutdown(gomock.Any()).AnyTimes().Return(nil)

	pool, err := aibridged.NewCachedBridgePool(aibridged.PoolOptions{
		MaxItems: 1,
		TTL:      time.Minute,
		Clock:    clk,
	}, []aibridge.Provider{
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "old", BaseURL: oldUpstream.URL}),
	}, slogtest.Make(t, nil), nil, testTracer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	publishTrap := clk.Trap().Now("bridge_cache_publish")
	replaceTrap := clk.Trap().Now("provider_generation_publish")
	defer replaceTrap.Close()

	req := aibridged.Request{
		SessionKey:  "key",
		InitiatorID: uuid.New(),
		APIKeyID:    uuid.New().String(),
	}
	type serveResult struct {
		code int
		body string
		err  error
	}
	serveDone := make(chan serveResult, 1)
	go func() {
		rw := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/new/v1/models", nil).WithContext(ctx)
		err := pool.Serve(ctx, req, func(context.Context) (aibridged.DRPCClient, error) {
			return client, nil
		}, newMockMCPFactory(mcpProxy), rw, r)
		serveDone <- serveResult{code: rw.Code, body: rw.Body.String(), err: err}
	}()

	publish := publishTrap.MustWait(ctx)
	replaceDone := make(chan struct{})
	go func() {
		defer close(replaceDone)
		pool.ReplaceProviders([]aibridge.Provider{
			aibridge.NewOpenAIProvider(config.OpenAI{Name: "new", BaseURL: newUpstream.URL}),
		})
	}()
	replace := replaceTrap.MustWait(ctx)
	replace.MustRelease(ctx)
	publish.MustRelease(ctx)
	publishTrap.Close()

	_ = testutil.TryReceive(ctx, t, replaceDone)
	result := testutil.RequireReceive(ctx, t, serveDone)
	require.NoError(t, result.err)
	require.Equal(t, http.StatusOK, result.code)
	require.Equal(t, "new", result.body)
}

func TestPoolReplaceProvidersDoesNotJoinStaleBuild(t *testing.T) {
	t.Parallel()

	oldUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "old")
	}))
	t.Cleanup(oldUpstream.Close)
	newUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "new")
	}))
	t.Cleanup(newUpstream.Close)

	logger := slogtest.Make(t, nil)
	ctrl := gomock.NewController(t)
	client := mock.NewMockDRPCClient(ctrl)

	opts := aibridged.PoolOptions{MaxItems: 1, TTL: time.Minute}
	pool, err := aibridged.NewCachedBridgePool(opts, []aibridge.Provider{
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "old", BaseURL: oldUpstream.URL}),
	}, logger, nil, testTracer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	req := aibridged.Request{
		SessionKey:  "key",
		InitiatorID: uuid.New(),
		APIKeyID:    uuid.New().String(),
	}
	clientFn := func(context.Context) (aibridged.DRPCClient, error) {
		return client, nil
	}

	factory := newBlockingMCPFactory()
	firstDone := make(chan serveResult, 1)
	go func() {
		err := servePool(t.Context(), pool, req, clientFn, factory)
		firstDone <- serveResult{err: err}
	}()

	require.Eventually(t, factory.firstBuildStarted, testutil.WaitShort, testutil.IntervalFast)

	pool.ReplaceProviders([]aibridge.Provider{
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "new", BaseURL: newUpstream.URL}),
	})

	// The second serve is concurrent with ReplaceProviders. It must be routed
	// through the new provider, not the stale bridge being built by the first
	// serve. Capture its recorder body and assert on it directly.
	secondDone := make(chan serveResult, 1)
	go func() {
		rw := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/new/v1/models", nil).WithContext(t.Context())
		err := pool.Serve(t.Context(), req, clientFn, factory, rw, r)
		secondDone <- serveResult{err: err, code: rw.Code, body: rw.Body.String()}
	}()

	var second serveResult
	require.Eventually(t, func() bool {
		select {
		case second = <-secondDone:
			return true
		default:
			return false
		}
	}, testutil.WaitShort, testutil.IntervalFast)
	require.NoError(t, second.err)
	require.Equal(t, http.StatusOK, second.code)
	require.Equal(t, "new", second.body)

	close(factory.releaseFirst)
	var first serveResult
	require.Eventually(t, func() bool {
		select {
		case first = <-firstDone:
			return true
		default:
			return false
		}
	}, testutil.WaitShort, testutil.IntervalFast)
	require.NoError(t, first.err)

	err = servePool(t.Context(), pool, req, clientFn, factory)
	require.NoError(t, err)
}

func TestPoolReplaceProvidersAfterShutdownIsNoop(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	opts := aibridged.PoolOptions{MaxItems: 1, TTL: time.Minute}
	pool, err := aibridged.NewCachedBridgePool(opts, nil, logger, nil, testTracer)
	require.NoError(t, err)

	require.NoError(t, pool.Shutdown(t.Context()))
	require.NotPanics(t, func() {
		pool.ReplaceProviders([]aibridge.Provider{
			aibridge.NewOpenAIProvider(config.OpenAI{Name: "new", BaseURL: "https://example.com"}),
		})
	})

	err = servePool(t.Context(), pool, aibridged.Request{
		SessionKey:  "key",
		InitiatorID: uuid.New(),
		APIKeyID:    uuid.New().String(),
	}, func(context.Context) (aibridged.DRPCClient, error) {
		return nil, context.Canceled
	}, newMockMCPFactory(nil))
	require.ErrorContains(t, err, "pool shutting down")
}

func TestPool_Expiry(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		logger := slogtest.Make(t, nil)
		ctrl := gomock.NewController(t)
		client := mock.NewMockDRPCClient(ctrl)
		mcpProxy := mcpmock.NewMockServerProxier(ctrl)
		mcpProxy.EXPECT().Init(gomock.Any()).AnyTimes().Return(nil)
		mcpProxy.EXPECT().Shutdown(gomock.Any()).AnyTimes().Return(nil)

		const ttl = time.Second
		opts := aibridged.PoolOptions{MaxItems: 1, TTL: ttl}
		pool, err := aibridged.NewCachedBridgePool(opts, nil, logger, nil, testTracer)
		require.NoError(t, err)
		t.Cleanup(func() { pool.Shutdown(context.Background()) })

		req := aibridged.Request{
			SessionKey:  "key",
			InitiatorID: uuid.New(),
			APIKeyID:    uuid.New().String(),
		}
		clientFn := func(context.Context) (aibridged.DRPCClient, error) {
			return client, nil
		}

		ctx := t.Context()

		// First serve is a cache miss.
		err = servePool(ctx, pool, req, clientFn, newMockMCPFactory(mcpProxy))
		require.NoError(t, err)

		// Second serve is a cache hit.
		err = servePool(ctx, pool, req, clientFn, newMockMCPFactory(mcpProxy))
		require.NoError(t, err)

		metrics := pool.CacheMetrics()
		require.EqualValues(t, 1, metrics.Misses())
		require.EqualValues(t, 1, metrics.Hits())

		// TTL expires
		time.Sleep(ttl + time.Millisecond)

		// Third serve is a cache miss because the entry expired.
		err = servePool(ctx, pool, req, clientFn, newMockMCPFactory(mcpProxy))
		require.NoError(t, err)

		metrics = pool.CacheMetrics()
		require.EqualValues(t, 2, metrics.Misses())
		require.EqualValues(t, 1, metrics.Hits())

		// Wait for all eviction goroutines to complete before gomock's ctrl.Finish()
		// runs in test cleanup. ristretto's OnEvict callback spawns goroutines that
		// need to finish calling mcpProxy.Shutdown() before ctrl.finish clears the
		// expectations.
		synctest.Wait()
	})
}

func servePool(ctx context.Context, pool aibridged.Pooler, req aibridged.Request, clientFn aibridged.ClientFunc, factory aibridged.MCPProxyBuilder) error {
	rw := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	return pool.Serve(ctx, req, clientFn, factory, rw, r)
}

func assertPoolBody(t *testing.T, pool aibridged.Pooler, req aibridged.Request, clientFn aibridged.ClientFunc, factory aibridged.MCPProxyBuilder, path string, body string) {
	t.Helper()

	ctx := t.Context()
	rw := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
	err := pool.Serve(ctx, req, clientFn, factory, rw, r)
	require.NoError(t, err)
	resp := rw.Result()
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, body, string(got))
}

var _ aibridged.MCPProxyBuilder = &mockMCPFactory{}

type mockMCPFactory struct {
	proxy *mcpmock.MockServerProxier
}

func newMockMCPFactory(proxy *mcpmock.MockServerProxier) *mockMCPFactory {
	return &mockMCPFactory{proxy: proxy}
}

// TestPoolShutdownReplaceProviders ensures that concurrent
// pool shutdown does not race with provider replacement.
func TestPoolShutdownReplaceProviders(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(upstream.Close)

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	ctrl := gomock.NewController(t)
	client := mock.NewMockDRPCClient(ctrl)
	mcpProxy := mcpmock.NewMockServerProxier(ctrl)
	mcpProxy.EXPECT().Init(gomock.Any()).AnyTimes().Return(nil)
	mcpProxy.EXPECT().Shutdown(gomock.Any()).AnyTimes().Return(nil)

	ctx := testutil.Context(t, testutil.WaitShort)
	clk := quartz.NewMock(t)

	opts := aibridged.PoolOptions{MaxItems: 16, TTL: time.Minute, Clock: clk}
	pool, err := aibridged.NewCachedBridgePool(opts, []aibridge.Provider{
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "p", BaseURL: upstream.URL}),
	}, logger, nil, testTracer)
	require.NoError(t, err)

	clientFn := func(context.Context) (aibridged.DRPCClient, error) { return client, nil }

	// Populate the cache so ReplaceProviders' generation retirement has an
	// entry to retire.
	err = servePool(ctx, pool, aibridged.Request{
		SessionKey:  "key",
		InitiatorID: uuid.New(),
		APIKeyID:    uuid.New().String(),
	}, clientFn, newMockMCPFactory(mcpProxy))
	require.NoError(t, err)

	trap := clk.Trap().Now("provider_generation_publish")
	defer trap.Close()

	replaceDone := make(chan struct{})
	go func() {
		defer close(replaceDone)
		pool.ReplaceProviders([]aibridge.Provider{
			aibridge.NewOpenAIProvider(config.OpenAI{Name: "p2", BaseURL: upstream.URL}),
		})
	}()

	// ReplaceProviders is parked immediately before publishing the new
	// generation. Deterministic readiness, no require.Eventually.
	call := trap.MustWait(ctx)

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		_ = pool.Shutdown(context.Background())
	}()
	call.MustRelease(ctx)

	_ = testutil.TryReceive(ctx, t, replaceDone)
	_ = testutil.TryReceive(ctx, t, shutdownDone)
}

func (m *mockMCPFactory) Build(ctx context.Context, req aibridged.Request, tracer trace.Tracer) (mcp.ServerProxier, error) {
	return m.proxy, nil
}

type serveResult struct {
	err  error
	code int
	body string
}

type blockingMCPFactory struct {
	calls        atomic.Int32
	firstStarted chan struct{}
	releaseFirst chan struct{}
}

func newBlockingMCPFactory() *blockingMCPFactory {
	return &blockingMCPFactory{
		firstStarted: make(chan struct{}),
		releaseFirst: make(chan struct{}),
	}
}

func (m *blockingMCPFactory) firstBuildStarted() bool {
	select {
	case <-m.firstStarted:
		return true
	default:
		return false
	}
}

func (m *blockingMCPFactory) Build(ctx context.Context, _ aibridged.Request, _ trace.Tracer) (mcp.ServerProxier, error) {
	if m.calls.Add(1) == 1 {
		close(m.firstStarted)
		select {
		case <-m.releaseFirst:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, context.Canceled
}

// TestPoolKeyPools verifies KeyPools returns the providers' pools, the pool
// wires failover metrics into them, and the state collector reflects live
// pool state, on both the initial set and reload.
func TestPoolKeyPools(t *testing.T) {
	t.Parallel()

	// Setup.
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	opts := aibridged.PoolOptions{MaxItems: 1, TTL: time.Minute}
	clk := quartz.NewMock(t)
	reg := prometheus.NewRegistry()
	m := aibridge.NewMetrics(reg)

	// markRateLimited drives one rate-limit transition on the pool's first
	// key, recording a metric only if the pool has metrics attached.
	markRateLimited := func(t *testing.T, pool *keypool.Pool) {
		key, kpErr := pool.Walker().Next()
		require.Nil(t, kpErr)
		pool.MarkKeyOnStatus(context.Background(), key,
			&http.Response{StatusCode: http.StatusTooManyRequests, Header: make(http.Header)}, logger)
	}

	// Given: provider "a" (2 keys), a BYOK provider with no key pool, and
	// provider "b" (1 key).
	poolA, err := keypool.New("a", []string{"a-key-0", "a-key-1"}, clk, m)
	require.NoError(t, err)
	poolB, err := keypool.New("b", []string{"b-key-0"}, clk, m)
	require.NoError(t, err)

	// When: the providers are loaded into a new bridge pool.
	aibridgePool, err := aibridged.NewCachedBridgePool(opts, []aibridge.Provider{
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "a", KeyPool: poolA}),
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "byok"}),
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "b", KeyPool: poolB}),
	}, logger, m, testTracer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = aibridgePool.Shutdown(context.Background()) })

	reg.MustRegister(keypool.NewStateCollector(aibridgePool.KeyPools))

	// Then: KeyPools returns the non-BYOK pools, and the collector reports
	// every key as valid.
	require.Equal(t, []*keypool.Pool{poolA, poolB}, aibridgePool.KeyPools())
	gathered, err := reg.Gather()
	require.NoError(t, err)
	assert.True(t, testutil.PromGaugeHasValue(t, gathered, 2, "key_pool_state", "a", "valid"))
	assert.True(t, testutil.PromGaugeHasValue(t, gathered, 1, "key_pool_state", "b", "valid"))

	// When: a key in pool "a" is rate-limited.
	markRateLimited(t, poolA)

	// Then: the transition is recorded (metrics were attached) and the key
	// moves to temporary, which the collector reflects.
	gathered, err = reg.Gather()
	require.NoError(t, err)
	assert.True(t, testutil.PromCounterHasValue(t, gathered, 1, "key_pool_state_transitions_total", "a", "rate_limited"))
	assert.True(t, testutil.PromGaugeHasValue(t, gathered, 1, "key_pool_state", "a", "valid"))
	assert.True(t, testutil.PromGaugeHasValue(t, gathered, 1, "key_pool_state", "a", "temporary"))

	// When: the providers reload, dropping a key from "a", adding one to "b",
	// and introducing a new provider "c".
	poolA, err = keypool.New("a", []string{"a-key-0"}, clk, m)
	require.NoError(t, err)
	poolB, err = keypool.New("b", []string{"b-key-0", "b-key-1"}, clk, m)
	require.NoError(t, err)
	poolC, err := keypool.New("c", []string{"c-key-0"}, clk, m)
	require.NoError(t, err)
	aibridgePool.ReplaceProviders([]aibridge.Provider{
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "a", KeyPool: poolA}),
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "b", KeyPool: poolB}),
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "c", KeyPool: poolC}),
	})

	// Then: KeyPools, metric wiring, and pool state all follow the new set.
	require.Equal(t, []*keypool.Pool{poolA, poolB, poolC}, aibridgePool.KeyPools())
	gathered, err = reg.Gather()
	require.NoError(t, err)
	assert.True(t, testutil.PromGaugeHasValue(t, gathered, 1, "key_pool_state", "a", "valid"))
	assert.True(t, testutil.PromGaugeHasValue(t, gathered, 2, "key_pool_state", "b", "valid"))
	assert.True(t, testutil.PromGaugeHasValue(t, gathered, 1, "key_pool_state", "c", "valid"))

	// When: a key in the new pool "c" is rate-limited.
	markRateLimited(t, poolC)

	// Then: the transition is recorded and the key moves to temporary.
	gathered, err = reg.Gather()
	require.NoError(t, err)
	assert.True(t, testutil.PromCounterHasValue(t, gathered, 1, "key_pool_state_transitions_total", "c", "rate_limited"))
	assert.True(t, testutil.PromGaugeHasValue(t, gathered, 1, "key_pool_state", "c", "temporary"))
}

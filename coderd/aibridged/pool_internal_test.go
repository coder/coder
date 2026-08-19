package aibridged

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/mcp"
	"github.com/coder/coder/v2/aibridge/mcpmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// TestPoolServeRetriesRetiredCachedBridgeAdmission covers the retirement
// window after a cached bridge is selected but before TryServe admits the
// request. The pool must retry instead of serving through the closed bridge.
func TestPoolServeRetriesRetiredCachedBridgeAdmission(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "served")
	}))
	t.Cleanup(upstream.Close)

	ctrl := gomock.NewController(t)
	mcpProxy := mcpmock.NewMockServerProxier(ctrl)
	mcpProxy.EXPECT().Init(gomock.Any()).AnyTimes().Return(nil)
	mcpProxy.EXPECT().Shutdown(gomock.Any()).AnyTimes().Return(nil)

	clk := quartz.NewMock(t)
	metrics := aibridge.NewMetrics(prometheus.NewRegistry())
	pool, err := NewCachedBridgePool(PoolOptions{MaxItems: 1, TTL: time.Minute, Clock: clk}, []aibridge.Provider{
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "p", BaseURL: upstream.URL}),
	}, slogtest.Make(t, nil), metrics, otel.Tracer("pool_internal_test"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	req := Request{
		SessionKey:  "key",
		InitiatorID: uuid.New(),
		APIKeyID:    uuid.New().String(),
	}
	// The passthrough below is not recorded, so no DRPC client is needed.
	clientFn := func(context.Context) (DRPCClient, error) {
		return nil, xerrors.New("no DRPC client")
	}
	factory := &countingMCPFactory{proxy: mcpProxy}

	// Given: a cached bridge which served a request.
	rw := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/p/v1/models", nil).WithContext(ctx)
	require.NoError(t, pool.Serve(ctx, req, clientFn, factory, rw, r))
	require.Equal(t, http.StatusOK, rw.Code)

	trap := clk.Trap().Now("bridge_serve_admission")
	defer trap.Close()

	// When: a second request selects that bridge and provider replacement or
	// eviction retires it before TryServe admission.
	serveDone := make(chan poolServeResult, 1)
	go func() {
		rw := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/p/v1/models", nil).WithContext(ctx)
		err := pool.Serve(ctx, req, clientFn, factory, rw, r)
		serveDone <- poolServeResult{err: err, code: rw.Code, body: rw.Body.String()}
	}()

	firstAdmission := trap.MustWait(ctx)
	cacheKey := bridgeCacheKey(pool.generation.Load().id, req)
	cached, ok := pool.cache.Get(cacheKey)
	require.True(t, ok)
	cached.retire(pool)
	firstAdmission.MustRelease(ctx)

	// The retry selects a rebuilt bridge. Let that admission proceed.
	trap.MustWait(ctx).MustRelease(ctx)

	result := testutil.RequireReceive(ctx, t, serveDone)
	require.NoError(t, result.err)
	require.Equal(t, http.StatusOK, result.code)
	require.Equal(t, "served", result.body)
	require.EqualValues(t, 2, factory.builds.Load())
	require.Equal(t, 1.0, promtest.ToFloat64(metrics.BridgePoolRetries.WithLabelValues("admission")))
	require.Equal(t, 0.0, promtest.ToFloat64(metrics.BridgePoolRetryExhausted.WithLabelValues("admission")))

	rebuilt, ok := pool.cache.Get(cacheKey)
	require.True(t, ok)
	require.NotSame(t, cached.bridge, rebuilt.bridge)
}

func TestPoolServeUncachedFallback(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "served")
	}))
	t.Cleanup(upstream.Close)

	ctrl := gomock.NewController(t)
	mcpProxy := mcpmock.NewMockServerProxier(ctrl)
	mcpProxy.EXPECT().Init(gomock.Any()).AnyTimes().Return(nil)
	mcpProxy.EXPECT().Shutdown(gomock.Any()).AnyTimes().Return(nil)

	// A negative TTL makes Ristretto's SetWithTTL a deterministic no-op that
	// returns false. The rejected build itself must serve the request without a
	// second MCP build.
	metrics := aibridge.NewMetrics(prometheus.NewRegistry())
	pool, err := NewCachedBridgePool(PoolOptions{MaxItems: 1, TTL: -time.Second}, []aibridge.Provider{
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "p", BaseURL: upstream.URL}),
	}, slogtest.Make(t, nil), metrics, otel.Tracer("pool_internal_test"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	req := Request{
		SessionKey:  "key",
		InitiatorID: uuid.New(),
		APIKeyID:    uuid.New().String(),
	}
	clientFn := func(context.Context) (DRPCClient, error) {
		return nil, xerrors.New("no DRPC client")
	}
	factory := &countingMCPFactory{proxy: mcpProxy}

	rw := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/p/v1/models", nil).WithContext(ctx)
	require.NoError(t, pool.Serve(ctx, req, clientFn, factory, rw, r))

	require.Equal(t, http.StatusOK, rw.Code)
	require.Equal(t, "served", rw.Body.String())
	require.EqualValues(t, 1, factory.builds.Load())
	require.EqualValues(t, 0, pool.CacheMetrics().KeysAdded())
	require.Equal(t, 1.0, promtest.ToFloat64(metrics.BridgePoolUncachedServeAttempts))
}

func TestPoolServeUncachedFallbackCoalescesConcurrentBuilds(t *testing.T) {
	t.Parallel()

	const requestCount = 16

	ctx := testutil.Context(t, testutil.WaitShort)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "served")
	}))
	t.Cleanup(upstream.Close)

	ctrl := gomock.NewController(t)
	mcpProxy := mcpmock.NewMockServerProxier(ctrl)
	shutdown := make(chan struct{})
	mcpProxy.EXPECT().Init(gomock.Any()).Times(1).Return(nil)
	mcpProxy.EXPECT().Shutdown(gomock.Any()).DoAndReturn(func(context.Context) error {
		close(shutdown)
		return nil
	}).Times(1)

	metrics := aibridge.NewMetrics(prometheus.NewRegistry())
	pool, err := NewCachedBridgePool(PoolOptions{MaxItems: 1, TTL: -time.Second}, []aibridge.Provider{
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "p", BaseURL: upstream.URL}),
	}, slogtest.Make(t, nil), metrics, otel.Tracer("pool_internal_test"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	req := Request{
		SessionKey:  "key",
		InitiatorID: uuid.New(),
		APIKeyID:    uuid.New().String(),
	}
	clientFn := func(context.Context) (DRPCClient, error) {
		return nil, xerrors.New("no DRPC client")
	}
	factory := newBlockingCountingMCPFactory(mcpProxy)
	results := make(chan poolServeResult, requestCount)
	serve := func() {
		rw := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/p/v1/models", nil).WithContext(ctx)
		err := pool.Serve(ctx, req, clientFn, factory, rw, r)
		results <- poolServeResult{err: err, code: rw.Code, body: rw.Body.String()}
	}

	go serve()
	_ = testutil.TryReceive(ctx, t, factory.started)

	cacheKey := bridgeCacheKey(pool.generation.Load().id, req)
	for users := 2; users <= requestCount; users++ {
		go serve()
		require.Eventually(t, func() bool {
			pool.builds.mu.Lock()
			defer pool.builds.mu.Unlock()
			call := pool.builds.calls[cacheKey]
			return call != nil && call.users == users
		}, testutil.WaitShort, testutil.IntervalFast)
	}
	close(factory.release)

	for range requestCount {
		result := testutil.RequireReceive(ctx, t, results)
		require.NoError(t, result.err)
		require.Equal(t, http.StatusOK, result.code)
		require.Equal(t, "served", result.body)
	}

	require.EqualValues(t, 1, factory.builds.Load())
	require.EqualValues(t, 0, pool.CacheMetrics().KeysAdded())
	require.Equal(t, float64(requestCount), promtest.ToFloat64(metrics.BridgePoolUncachedServeAttempts))
	_ = testutil.TryReceive(ctx, t, shutdown)
}

func TestPoolPolicyRejectedEntryRemainsUsableUntilRetired(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "served")
	}))
	t.Cleanup(upstream.Close)

	ctrl := gomock.NewController(t)
	mcpProxy := mcpmock.NewMockServerProxier(ctrl)
	mcpProxy.EXPECT().Init(gomock.Any()).Return(nil)
	mcpProxy.EXPECT().Shutdown(gomock.Any()).AnyTimes().Return(nil)

	pool, err := NewCachedBridgePool(PoolOptions{MaxItems: 1, TTL: time.Minute}, []aibridge.Provider{
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "p", BaseURL: upstream.URL}),
	}, slogtest.Make(t, nil), nil, otel.Tracer("pool_internal_test"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	req := Request{SessionKey: "key", InitiatorID: uuid.New(), APIKeyID: uuid.New().String()}
	generation := pool.generation.Load()
	bridge, err := pool.buildBridge(ctx, req, func(context.Context) (DRPCClient, error) {
		return nil, xerrors.New("no DRPC client")
	}, &poolTestMCPFactory{proxy: mcpProxy}, generation.providers)
	require.NoError(t, err)

	entry := &bridgeEntry{key: "policy-rejected", generation: generation, bridge: bridge}
	require.True(t, generation.register(entry))

	// A cost above MaxCost is accepted into the set buffer, then rejected by
	// policy. OnReject marks the entry before OnExit runs, preserving it for
	// callers that already joined the build.
	require.True(t, pool.cache.SetWithTTL(entry.key, entry, cacheCost+1, time.Minute))
	pool.cache.Wait()
	require.True(t, entry.cacheRejected.Load())
	require.False(t, entry.retired.Load())
	require.EqualValues(t, 0, pool.cache.Metrics.KeysAdded())

	rw := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/p/v1/models", nil).WithContext(ctx)
	require.True(t, entry.bridge.TryServe(rw, r))
	require.Equal(t, http.StatusOK, rw.Code)
	require.Equal(t, "served", rw.Body.String())

	entry.retire(pool)
	require.True(t, entry.retired.Load())
}

func TestPoolServeAdmissionRetriesAreBounded(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "unexpected")
	}))
	t.Cleanup(upstream.Close)

	ctrl := gomock.NewController(t)
	mcpProxy := mcpmock.NewMockServerProxier(ctrl)
	mcpProxy.EXPECT().Init(gomock.Any()).AnyTimes().Return(nil)
	mcpProxy.EXPECT().Shutdown(gomock.Any()).AnyTimes().Return(nil)

	clk := quartz.NewMock(t)
	metrics := aibridge.NewMetrics(prometheus.NewRegistry())
	pool, err := NewCachedBridgePool(PoolOptions{MaxItems: 1, TTL: time.Minute, Clock: clk}, []aibridge.Provider{
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "p", BaseURL: upstream.URL}),
	}, slogtest.Make(t, nil), metrics, otel.Tracer("pool_internal_test"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	req := Request{SessionKey: "key", InitiatorID: uuid.New(), APIKeyID: uuid.New().String()}
	clientFn := func(context.Context) (DRPCClient, error) {
		return nil, xerrors.New("no DRPC client")
	}
	factory := &countingMCPFactory{proxy: mcpProxy}

	// Populate the cache before trapping admission for the request under test.
	rw := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/p/v1/models", nil).WithContext(ctx)
	require.NoError(t, pool.Serve(ctx, req, clientFn, factory, rw, r))

	trap := clk.Trap().Now("bridge_serve_admission")
	defer trap.Close()
	serveDone := make(chan error, 1)
	go func() {
		rw := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/p/v1/models", nil).WithContext(ctx)
		serveDone <- pool.Serve(ctx, req, clientFn, factory, rw, r)
	}()

	cacheKey := bridgeCacheKey(pool.generation.Load().id, req)
	for range maxBridgeServeAttempts {
		admission := trap.MustWait(ctx)
		entry, ok := pool.cache.Get(cacheKey)
		require.True(t, ok)
		require.False(t, entry.retired.Load())
		entry.retire(pool)
		admission.MustRelease(ctx)
	}

	err = testutil.RequireReceive(ctx, t, serveDone)
	require.ErrorIs(t, err, errBridgeServeRetriesExhausted)
	require.EqualValues(t, maxBridgeServeAttempts, factory.builds.Load())
	require.Equal(t, float64(maxBridgeServeAttempts-1), promtest.ToFloat64(metrics.BridgePoolRetries.WithLabelValues("admission")))
	require.Equal(t, 1.0, promtest.ToFloat64(metrics.BridgePoolRetryExhausted.WithLabelValues("admission")))
}

func TestBridgeBuildGroupReleasesWaitersAfterPanic(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	var group bridgeBuildGroup
	started := make(chan struct{})
	panicBuild := make(chan struct{})
	panics := make(chan any, 2)

	call := func(fn func() (*bridgeEntry, func(), error)) {
		defer func() { panics <- recover() }()
		_, release, _ := group.Do("key", fn)
		release()
	}

	go call(func() (*bridgeEntry, func(), error) {
		close(started)
		<-panicBuild
		panic("build panic")
	})
	_ = testutil.TryReceive(ctx, t, started)

	go call(func() (*bridgeEntry, func(), error) {
		t.Error("duplicate caller unexpectedly built a bridge")
		return nil, nil, nil
	})
	require.Eventually(t, func() bool {
		group.mu.Lock()
		defer group.mu.Unlock()
		call := group.calls["key"]
		return call != nil && call.users == 2
	}, testutil.WaitShort, testutil.IntervalFast)
	close(panicBuild)

	require.Equal(t, "build panic", testutil.RequireReceive(ctx, t, panics))
	require.Equal(t, "build panic", testutil.RequireReceive(ctx, t, panics))

	entry := &bridgeEntry{}
	got, release, err := group.Do("key", func() (*bridgeEntry, func(), error) {
		return entry, nil, nil
	})
	require.NoError(t, err)
	require.Same(t, entry, got)
	release()
}

func TestBridgeBuildGroupReleasesWaitersAfterGoexit(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	var group bridgeBuildGroup
	started := make(chan struct{})
	exitBuild := make(chan struct{})
	leaderDone := make(chan struct{})
	waiterDone := make(chan error, 1)

	go func() {
		defer close(leaderDone)
		_, release, _ := group.Do("key", func() (*bridgeEntry, func(), error) {
			close(started)
			<-exitBuild
			runtime.Goexit()
			return nil, nil, nil
		})
		release()
	}()
	_ = testutil.TryReceive(ctx, t, started)

	go func() {
		_, release, err := group.Do("key", func() (*bridgeEntry, func(), error) {
			t.Error("duplicate caller unexpectedly built a bridge")
			return nil, nil, nil
		})
		release()
		waiterDone <- err
	}()
	require.Eventually(t, func() bool {
		group.mu.Lock()
		defer group.mu.Unlock()
		call := group.calls["key"]
		return call != nil && call.users == 2
	}, testutil.WaitShort, testutil.IntervalFast)
	close(exitBuild)

	_ = testutil.TryReceive(ctx, t, leaderDone)
	require.ErrorIs(t, testutil.RequireReceive(ctx, t, waiterDone), errBridgeBuildExited)

	entry := &bridgeEntry{}
	got, release, err := group.Do("key", func() (*bridgeEntry, func(), error) {
		return entry, nil, nil
	})
	require.NoError(t, err)
	require.Same(t, entry, got)
	release()
}

func TestPoolServeUncachedFallbackReleasesAfterPanic(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "served")
	}))
	t.Cleanup(upstream.Close)

	ctrl := gomock.NewController(t)
	mcpProxy := mcpmock.NewMockServerProxier(ctrl)
	shutdown := make(chan struct{})
	mcpProxy.EXPECT().Init(gomock.Any()).Return(nil)
	mcpProxy.EXPECT().Shutdown(gomock.Any()).DoAndReturn(func(context.Context) error {
		close(shutdown)
		return nil
	}).Times(1)

	pool, err := NewCachedBridgePool(PoolOptions{MaxItems: 1, TTL: -time.Second}, []aibridge.Provider{
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "p", BaseURL: upstream.URL}),
	}, slogtest.Make(t, nil), nil, otel.Tracer("pool_internal_test"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	req := Request{SessionKey: "key", InitiatorID: uuid.New(), APIKeyID: uuid.New().String()}
	var panicValue any
	func() {
		defer func() { panicValue = recover() }()
		r := httptest.NewRequest(http.MethodGet, "/p/v1/models", nil).WithContext(ctx)
		_ = pool.Serve(ctx, req, func(context.Context) (DRPCClient, error) {
			return nil, xerrors.New("no DRPC client")
		}, &poolTestMCPFactory{proxy: mcpProxy}, panicResponseWriter{}, r)
	}()

	require.Equal(t, "response writer panic", panicValue)
	_ = testutil.TryReceive(ctx, t, shutdown)
}

func TestPoolShutdownCancelsBridgeBuild(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	buildStarted := make(chan struct{})
	buildCanceled := make(chan struct{})
	factory := MCPProxyBuilderFunc(func(ctx context.Context, _ Request, _ trace.Tracer) (mcp.ServerProxier, error) {
		close(buildStarted)
		<-ctx.Done()
		close(buildCanceled)
		return nil, ctx.Err()
	})

	pool, err := NewCachedBridgePool(PoolOptions{MaxItems: 1, TTL: time.Minute}, nil, slogtest.Make(t, nil), nil, otel.Tracer("pool_internal_test"))
	require.NoError(t, err)

	serveDone := make(chan error, 1)
	go func() {
		rw := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		serveDone <- pool.Serve(ctx, Request{
			SessionKey:  "key",
			InitiatorID: uuid.New(),
			APIKeyID:    uuid.New().String(),
		}, func(context.Context) (DRPCClient, error) {
			return nil, xerrors.New("no DRPC client")
		}, factory, rw, r)
	}()
	_ = testutil.TryReceive(ctx, t, buildStarted)

	shutdownCtx, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, pool.Shutdown(shutdownCtx), context.Canceled)
	_ = testutil.TryReceive(ctx, t, buildCanceled)
	require.ErrorIs(t, testutil.RequireReceive(ctx, t, serveDone), context.Canceled)
}

func TestPoolShutdownCancelsAdmittedRequest(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	started := make(chan struct{})
	canceled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	t.Cleanup(func() {
		upstream.CloseClientConnections()
		upstream.Close()
	})

	ctrl := gomock.NewController(t)
	mcpProxy := mcpmock.NewMockServerProxier(ctrl)
	mcpProxy.EXPECT().Init(gomock.Any()).Return(nil)
	mcpProxy.EXPECT().Shutdown(gomock.Any()).Return(nil)

	pool, err := NewCachedBridgePool(PoolOptions{MaxItems: 1, TTL: time.Minute}, []aibridge.Provider{
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "p", BaseURL: upstream.URL}),
	}, slogtest.Make(t, nil), nil, otel.Tracer("pool_internal_test"))
	require.NoError(t, err)

	req := Request{
		SessionKey:  "key",
		InitiatorID: uuid.New(),
		APIKeyID:    uuid.New().String(),
	}
	serveDone := make(chan error, 1)
	go func() {
		rw := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/p/v1/models", nil).WithContext(ctx)
		serveDone <- pool.Serve(ctx, req, func(context.Context) (DRPCClient, error) {
			return nil, xerrors.New("no DRPC client")
		}, &poolTestMCPFactory{proxy: mcpProxy}, rw, r)
	}()
	_ = testutil.TryReceive(ctx, t, started)

	shutdownCtx, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, pool.Shutdown(shutdownCtx), context.Canceled)
	_ = testutil.TryReceive(ctx, t, canceled)
	require.NoError(t, testutil.RequireReceive(ctx, t, serveDone))
}

type panicResponseWriter struct{}

func (panicResponseWriter) Header() http.Header {
	return make(http.Header)
}

func (panicResponseWriter) Write([]byte) (int, error) {
	panic("response writer panic")
}

func (panicResponseWriter) WriteHeader(int) {
	panic("response writer panic")
}

type poolServeResult struct {
	err  error
	code int
	body string
}

type blockingCountingMCPFactory struct {
	proxy   *mcpmock.MockServerProxier
	builds  atomic.Int32
	started chan struct{}
	release chan struct{}
}

func newBlockingCountingMCPFactory(proxy *mcpmock.MockServerProxier) *blockingCountingMCPFactory {
	return &blockingCountingMCPFactory{
		proxy:   proxy,
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (f *blockingCountingMCPFactory) Build(ctx context.Context, _ Request, _ trace.Tracer) (mcp.ServerProxier, error) {
	if f.builds.Add(1) == 1 {
		close(f.started)
		select {
		case <-f.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.proxy, nil
}

type MCPProxyBuilderFunc func(context.Context, Request, trace.Tracer) (mcp.ServerProxier, error)

func (f MCPProxyBuilderFunc) Build(ctx context.Context, req Request, tracer trace.Tracer) (mcp.ServerProxier, error) {
	return f(ctx, req, tracer)
}

type poolTestMCPFactory struct {
	proxy *mcpmock.MockServerProxier
}

func (f *poolTestMCPFactory) Build(context.Context, Request, trace.Tracer) (mcp.ServerProxier, error) {
	return f.proxy, nil
}

type countingMCPFactory struct {
	proxy  *mcpmock.MockServerProxier
	builds atomic.Int32
}

func (f *countingMCPFactory) Build(context.Context, Request, trace.Tracer) (mcp.ServerProxier, error) {
	f.builds.Add(1)
	return f.proxy, nil
}

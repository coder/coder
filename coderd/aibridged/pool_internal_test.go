package aibridged

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
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
)

// TestPoolServeRebuildsShutdownCachedBridge covers a request which finds a
// cached bridge that a provider reload has already shut down. The pool must
// rebuild instead of serving through the closed bridge, which responds 500.
func TestPoolServeRebuildsShutdownCachedBridge(t *testing.T) {
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

	pool, err := NewCachedBridgePool(PoolOptions{MaxItems: 1, TTL: time.Minute}, []aibridge.Provider{
		aibridge.NewOpenAIProvider(config.OpenAI{Name: "p", BaseURL: upstream.URL}),
	}, slogtest.Make(t, nil), nil, otel.Tracer("pool_internal_test"))
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
	factory := &poolTestMCPFactory{proxy: mcpProxy}

	serve := func() *httptest.ResponseRecorder {
		t.Helper()

		rw := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/p/v1/models", nil).WithContext(ctx)
		require.NoError(t, pool.Serve(ctx, req, clientFn, factory, rw, r))
		return rw
	}

	// Given: a cached bridge which served a request.
	require.Equal(t, http.StatusOK, serve().Code)

	// When: that bridge is retired while it is still cached, as a cache exit
	// does before the buffered deletion becomes visible.
	cacheKey := bridgeCacheKey(pool.generation.Load().id, req)
	cached, ok := pool.cache.Get(cacheKey)
	require.True(t, ok)
	cached.retire(pool)

	// Then: the request is served by a rebuilt bridge.
	rw := serve()
	require.Equal(t, http.StatusOK, rw.Code)
	require.Equal(t, "served", rw.Body.String())

	rebuilt, ok := pool.cache.Get(cacheKey)
	require.True(t, ok)
	require.NotSame(t, cached.bridge, rebuilt.bridge)
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

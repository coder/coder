package aibridged_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"
	"storj.io/drpc"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/coderd/aibridged"
	mock "github.com/coder/coder/v2/coderd/aibridged/aibridgedmock"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// countingDRPCConn counts Close calls so tests can assert that a connection is
// discarded when startup mode selection fails.
type countingDRPCConn struct {
	*mockDRPCConn
	closes atomic.Int32
}

func newCountingDRPCConn(closedCh chan struct{}) *countingDRPCConn {
	return &countingDRPCConn{mockDRPCConn: &mockDRPCConn{closedCh: closedCh}}
}

func (c *countingDRPCConn) Close() error {
	c.closes.Add(1)
	return nil
}

var _ drpc.Conn = &countingDRPCConn{}

// mcpConfigsFn answers a GetMCPServerConfigs call. call is 1-based.
type mcpConfigsFn func(call int32) (*proto.GetMCPServerConfigsResponse, error)

// expectMCPConfigs wires client.GetMCPServerConfigs to fn and returns the call
// counter. Startup mode selection queries MCP configs at most once per
// successful selection, so tests assert on this counter directly.
func expectMCPConfigs(client *mock.MockDRPCClient, fn mcpConfigsFn) *atomic.Int32 {
	var calls atomic.Int32
	client.EXPECT().GetMCPServerConfigs(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(context.Context, *proto.GetMCPServerConfigsRequest) (*proto.GetMCPServerConfigsResponse, error) {
			return fn(calls.Add(1))
		})
	return &calls
}

// noMCPConfigs is the response which selects the proxy backend.
func noMCPConfigs() *proto.GetMCPServerConfigsResponse {
	return &proto.GetMCPServerConfigsResponse{}
}

// staticConfigs answers every MCP discovery call with the same response.
func staticConfigs(response *proto.GetMCPServerConfigsResponse) mcpConfigsFn {
	return func(int32) (*proto.GetMCPServerConfigsResponse, error) {
		return response, nil
	}
}

func coderMCPConfig() *proto.GetMCPServerConfigsResponse {
	return &proto.GetMCPServerConfigsResponse{
		CoderMcpConfig: &proto.MCPServerConfig{Id: "coder", Url: "http://coder.test/api/experimental/mcp/http"},
	}
}

func externalMCPConfig() *proto.GetMCPServerConfigsResponse {
	return &proto.GetMCPServerConfigsResponse{
		ExternalAuthMcpConfigs: []*proto.MCPServerConfig{{Id: "github", Url: "http://github.test/mcp"}},
	}
}

// sentinelHandler identifies the handler a test expects to be returned.
type sentinelHandler struct {
	body string
}

func (h *sentinelHandler) ServeHTTP(rw http.ResponseWriter, _ *http.Request) {
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write([]byte(h.body))
}

// backendFixture holds the collaborators of a server under test.
type backendFixture struct {
	srv      *aibridged.Server
	client   *mock.MockDRPCClient
	pool     *mock.MockPooler
	conn     *countingDRPCConn
	mcpCalls *atomic.Int32
	// poolShutdowns counts Shutdown calls on the supplied pool, including the
	// unused one handed to a proxy-mode server.
	poolShutdowns *atomic.Int32
}

// newBackendServer starts a server whose single mock client answers MCP
// discovery with fn. The pool is a strict mock: any unexpected use fails the
// test, which is how the proxy tests assert that no pool is instantiated.
func newBackendServer(t *testing.T, experiments codersdk.Experiments, fn mcpConfigsFn, ignoreErrors bool) *backendFixture {
	t.Helper()

	ctrl := gomock.NewController(t)
	client := mock.NewMockDRPCClient(ctrl)
	pool := mock.NewMockPooler(ctrl)
	conn := newCountingDRPCConn(nil)
	client.EXPECT().DRPCConn().AnyTimes().Return(conn)
	calls := expectMCPConfigs(client, fn)

	var shutdowns atomic.Int32
	pool.EXPECT().Shutdown(gomock.Any()).AnyTimes().DoAndReturn(func(context.Context) error {
		shutdowns.Add(1)
		return nil
	})

	srv, err := aibridged.New(t.Context(), pool,
		func(context.Context) (aibridged.DRPCClient, error) { return client, nil },
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: ignoreErrors}), testTracer,
		aibridged.WithExperiments(experiments))
	require.NoError(t, err, "create aibridged server")
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	return &backendFixture{srv: srv, client: client, pool: pool, conn: conn, mcpCalls: calls, poolShutdowns: &shutdowns}
}

func proxyExperiments() codersdk.Experiments {
	return codersdk.Experiments{codersdk.ExperimentAIGatewayReverseProxy}
}

func waitReady(t *testing.T, srv *aibridged.Server) {
	t.Helper()
	require.Eventually(t, srv.Ready, testutil.WaitShort, testutil.IntervalFast,
		"server never reported an active connection")
}

func serveHandler(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
	return rec
}

func openAIProvider(name string, pool *keypool.Pool) aibridge.Provider {
	return aibridge.NewOpenAIProvider(config.OpenAI{Name: name, BaseURL: "http://upstream.test", KeyPool: pool})
}

// TestBackendMode_ExperimentDisabled asserts that without the experiment the
// server selects interception before it ever talks to coderd: no MCP discovery
// query is issued, and requests are served from the pool rather than a proxy
// router.
func TestBackendMode_ExperimentDisabled(t *testing.T) {
	t.Parallel()

	f := newBackendServer(t, nil, func(int32) (*proto.GetMCPServerConfigsResponse, error) {
		t.Error("MCP discovery must not run when the experiment is disabled")
		return noMCPConfigs(), nil
	}, false)

	bridge := &sentinelHandler{body: "bridge"}
	f.pool.EXPECT().Acquire(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(bridge, nil)

	ctx := testutil.Context(t, testutil.WaitShort)
	waitReady(t, f.srv)

	h, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
	require.NoError(t, err)
	require.Same(t, http.Handler(bridge), h, "requests must be served by the pool")
	require.Equal(t, int32(0), f.mcpCalls.Load())

	// Reloads reach the pool, and no proxy router is ever built.
	providers := []aibridge.Provider{openAIProvider("openai", singleKeyPool(t, "openai", "key"))}
	f.pool.EXPECT().ReplaceProviders(gomock.Any()).Times(1)
	require.NoError(t, f.srv.ReplaceProviders(ctx, providers))
	require.Nil(t, f.srv.KeyPools(), "a mock pool exposes no key pools; the proxy router must not be consulted")
}

// TestBackendMode_ProxyWhenNoMCPConfigs pins the proxy selection path: with the
// experiment on and no MCP configuration, requests are refused with 503 until
// the first provider snapshot lands, after which the router serves and answers
// unregistered routes with 404. The strict pool mock has no expectations, so
// any pool use fails the test.
func TestBackendMode_ProxyWhenNoMCPConfigs(t *testing.T) {
	t.Parallel()

	f := newBackendServer(t, proxyExperiments(), staticConfigs(noMCPConfigs()), false)

	ctx := testutil.Context(t, testutil.WaitShort)
	waitReady(t, f.srv)
	require.Equal(t, int32(1), f.mcpCalls.Load())

	h, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
	require.NoError(t, err, "the proxy backend must answer requests instead of erroring")
	rec := serveHandler(t, h, "/openai/v1/chat/completions")
	require.Equal(t, http.StatusServiceUnavailable, rec.Code, "requests before the first snapshot must be refused")
	require.Nil(t, f.srv.KeyPools())

	pool := singleKeyPool(t, "openai", "key")
	require.NoError(t, f.srv.ReplaceProviders(ctx, []aibridge.Provider{openAIProvider("openai", pool)}))

	h, err = f.srv.GetRequestHandler(ctx, aibridged.Request{})
	require.NoError(t, err)
	rec = serveHandler(t, h, "/openai/v1/chat/completions")
	require.Equal(t, http.StatusNotFound, rec.Code, "the router has no provider routes registered yet")
	require.Equal(t, []*keypool.Pool{pool}, f.srv.KeyPools())
}

// TestBackendMode_InterceptionWhenMCPConfigured asserts that any MCP
// configuration, from the Coder MCP server or from external auth, selects
// interception even with the experiment enabled.
func TestBackendMode_InterceptionWhenMCPConfigured(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		response *proto.GetMCPServerConfigsResponse
	}{
		{name: "CoderMCP", response: coderMCPConfig()},
		{name: "ExternalAuthMCP", response: externalMCPConfig()},
		{name: "Both", response: &proto.GetMCPServerConfigsResponse{
			CoderMcpConfig:         coderMCPConfig().GetCoderMcpConfig(),
			ExternalAuthMcpConfigs: externalMCPConfig().GetExternalAuthMcpConfigs(),
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			response := tc.response
			f := newBackendServer(t, proxyExperiments(), staticConfigs(response), false)

			bridge := &sentinelHandler{body: "bridge"}
			f.pool.EXPECT().Acquire(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(bridge, nil)
			f.pool.EXPECT().ReplaceProviders(gomock.Any()).Times(1)

			ctx := testutil.Context(t, testutil.WaitShort)
			waitReady(t, f.srv)

			h, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
			require.NoError(t, err)
			require.Same(t, http.Handler(bridge), h, "MCP configuration must select interception")

			require.NoError(t, f.srv.ReplaceProviders(ctx, []aibridge.Provider{
				openAIProvider("openai", singleKeyPool(t, "openai", "key")),
			}))
			require.Nil(t, f.srv.KeyPools(), "interception must not publish a proxy router")
		})
	}
}

// TestBackendMode_SelectedOnceAcrossReconnects asserts that mode selection is a
// startup decision: after a reconnect and further reloads the server keeps the
// backend it chose, and it does not re-query MCP discovery.
func TestBackendMode_SelectedOnceAcrossReconnects(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		first    *proto.GetMCPServerConfigsResponse
		second   *proto.GetMCPServerConfigsResponse
		wantPool bool
	}{
		{name: "InterceptionStaysInterception", first: coderMCPConfig(), second: noMCPConfigs(), wantPool: true},
		{name: "ProxyStaysProxy", first: noMCPConfigs(), second: coderMCPConfig(), wantPool: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			client := mock.NewMockDRPCClient(ctrl)
			pool := mock.NewMockPooler(ctrl)
			closedCh := make(chan struct{})
			conn := newCountingDRPCConn(closedCh)
			client.EXPECT().DRPCConn().AnyTimes().Return(conn)

			first, second := tc.first, tc.second
			calls := expectMCPConfigs(client, func(call int32) (*proto.GetMCPServerConfigsResponse, error) {
				if call == 1 {
					return first, nil
				}
				return second, nil
			})

			var dials atomic.Int32
			srv, err := aibridged.New(t.Context(), pool, func(context.Context) (aibridged.DRPCClient, error) {
				dials.Add(1)
				return client, nil
			}, slogtest.Make(t, nil), testTracer, aibridged.WithExperiments(proxyExperiments()))
			require.NoError(t, err)
			t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

			bridge := &sentinelHandler{body: "bridge"}
			if tc.wantPool {
				pool.EXPECT().Acquire(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).MinTimes(1).Return(bridge, nil)
				pool.EXPECT().ReplaceProviders(gomock.Any()).MinTimes(1)
			}
			pool.EXPECT().Shutdown(gomock.Any()).AnyTimes().Return(nil)

			ctx := testutil.Context(t, testutil.WaitShort)
			waitReady(t, srv)
			require.Equal(t, int32(1), calls.Load())

			require.NoError(t, srv.ReplaceProviders(ctx, []aibridge.Provider{
				openAIProvider("openai", singleKeyPool(t, "openai", "key")),
			}))

			// Force a reconnect and wait for the loop to dial again.
			close(closedCh)
			require.Eventually(t, func() bool { return dials.Load() > 1 }, testutil.WaitShort, testutil.IntervalFast,
				"connection loop did not redial")

			// Another reload after the reconnect must not change the mode.
			require.NoError(t, srv.ReplaceProviders(ctx, []aibridge.Provider{
				openAIProvider("openai", singleKeyPool(t, "openai", "key")),
			}))

			require.Equal(t, int32(1), calls.Load(), "MCP discovery must run once, at startup")

			h, err := srv.GetRequestHandler(ctx, aibridged.Request{})
			require.NoError(t, err)
			if tc.wantPool {
				require.Same(t, http.Handler(bridge), h)
				require.Nil(t, srv.KeyPools())
				return
			}
			require.NotSame(t, http.Handler(bridge), h)
			require.Len(t, srv.KeyPools(), 1, "proxy mode must keep serving from its router")
			require.Equal(t, http.StatusNotFound, serveHandler(t, h, "/openai/v1/chat/completions").Code)
		})
	}
}

// TestBackendMode_MCPDiscoveryFailureRetries asserts that a failed or malformed
// discovery response is never read as "no MCP configured": no backend is
// published, the connection is dropped, and selection is retried on the next
// connection.
func TestBackendMode_MCPDiscoveryFailureRetries(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		fail mcpConfigsFn
	}{
		{
			name: "RPCError",
			fail: func(call int32) (*proto.GetMCPServerConfigsResponse, error) {
				if call <= 2 {
					return nil, xerrors.New("boom")
				}
				return noMCPConfigs(), nil
			},
		},
		{
			name: "NilResponse",
			fail: func(call int32) (*proto.GetMCPServerConfigsResponse, error) {
				if call <= 2 {
					//nolint:nilnil // exercises a malformed response
					return nil, nil
				}
				return noMCPConfigs(), nil
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newBackendServer(t, proxyExperiments(), tc.fail, true)
			ctx := testutil.Context(t, testutil.WaitShort)

			// While selection keeps failing the server is not ready and
			// requests are refused rather than served by a guessed backend.
			require.Eventually(t, func() bool { return f.mcpCalls.Load() >= 1 }, testutil.WaitShort, testutil.IntervalFast)
			require.False(t, f.srv.Ready())
			h, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
			require.NoError(t, err)
			require.Equal(t, http.StatusServiceUnavailable, serveHandler(t, h, "/openai/v1/chat/completions").Code)
			require.Nil(t, f.srv.KeyPools())

			waitReady(t, f.srv)
			require.Equal(t, int32(3), f.mcpCalls.Load(), "selection must be retried on each new connection")
			require.GreaterOrEqual(t, f.conn.closes.Load(), int32(2), "a connection whose selection failed must be discarded")

			require.NoError(t, f.srv.ReplaceProviders(ctx, []aibridge.Provider{
				openAIProvider("openai", singleKeyPool(t, "openai", "key")),
			}))
			h, err = f.srv.GetRequestHandler(ctx, aibridged.Request{})
			require.NoError(t, err)
			require.Equal(t, http.StatusNotFound, serveHandler(t, h, "/openai/v1/chat/completions").Code)
		})
	}
}

// TestReplaceProviders_SwapsRouter asserts the swap is atomic from a caller's
// point of view: the published snapshot changes wholesale, and a handler
// acquired before the swap keeps serving from the router it holds.
func TestReplaceProviders_SwapsRouter(t *testing.T) {
	t.Parallel()

	f := newBackendServer(t, proxyExperiments(), staticConfigs(noMCPConfigs()), false)

	ctx := testutil.Context(t, testutil.WaitShort)
	waitReady(t, f.srv)

	firstPool := singleKeyPool(t, "openai", "key")
	require.NoError(t, f.srv.ReplaceProviders(ctx, []aibridge.Provider{openAIProvider("openai", firstPool)}))
	require.Equal(t, []*keypool.Pool{firstPool}, f.srv.KeyPools())

	oldHandler, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
	require.NoError(t, err)

	secondPool := singleKeyPool(t, "anthropic", "key")
	require.NoError(t, f.srv.ReplaceProviders(ctx, []aibridge.Provider{
		openAIProvider("openai", firstPool),
		openAIProvider("anthropic", secondPool),
	}))
	require.Equal(t, []*keypool.Pool{firstPool, secondPool}, f.srv.KeyPools(),
		"KeyPools must report the new snapshot only")

	newHandler, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
	require.NoError(t, err)
	require.NotSame(t, oldHandler, newHandler, "a reload must publish a new router")

	// Handlers already acquired keep serving from the router they hold.
	require.Equal(t, http.StatusNotFound, serveHandler(t, oldHandler, "/openai/v1/chat/completions").Code)
	require.Equal(t, http.StatusNotFound, serveHandler(t, newHandler, "/openai/v1/chat/completions").Code)
}

// TestReplaceProviders_FailureRetainsRouter asserts an invalid snapshot leaves
// the previously published router serving.
func TestReplaceProviders_FailureRetainsRouter(t *testing.T) {
	t.Parallel()

	f := newBackendServer(t, proxyExperiments(), staticConfigs(noMCPConfigs()), false)

	ctx := testutil.Context(t, testutil.WaitShort)
	waitReady(t, f.srv)

	good := singleKeyPool(t, "openai", "key")
	require.NoError(t, f.srv.ReplaceProviders(ctx, []aibridge.Provider{openAIProvider("openai", good)}))
	handler, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
	require.NoError(t, err)

	err = f.srv.ReplaceProviders(ctx, []aibridge.Provider{
		openAIProvider("dupe", singleKeyPool(t, "a", "key")),
		openAIProvider("dupe", singleKeyPool(t, "b", "key")),
	})
	require.ErrorContains(t, err, "duplicate provider name")

	require.Equal(t, []*keypool.Pool{good}, f.srv.KeyPools(), "a failed reload must retain the previous snapshot")
	retained, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
	require.NoError(t, err)
	require.Same(t, handler, retained)
	require.Equal(t, http.StatusNotFound, serveHandler(t, retained, "/openai/v1/chat/completions").Code)
}

// TestReplaceProviders_ContextAndShutdown asserts a canceled context or a
// shutdown server aborts the reload instead of publishing a snapshot, and that
// shutdown stops serving the router even when it cannot finish gracefully.
func TestReplaceProviders_ContextAndShutdown(t *testing.T) {
	t.Parallel()

	t.Run("CanceledContext", func(t *testing.T) {
		t.Parallel()

		f := newBackendServer(t, proxyExperiments(), staticConfigs(noMCPConfigs()), false)
		waitReady(t, f.srv)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		require.ErrorIs(t, f.srv.ReplaceProviders(ctx, []aibridge.Provider{
			openAIProvider("openai", singleKeyPool(t, "openai", "key")),
		}), context.Canceled)
		require.Nil(t, f.srv.KeyPools(), "a canceled reload must not publish a snapshot")
	})

	t.Run("AfterShutdown", func(t *testing.T) {
		t.Parallel()

		f := newBackendServer(t, proxyExperiments(), staticConfigs(noMCPConfigs()), false)
		ctx := testutil.Context(t, testutil.WaitShort)
		waitReady(t, f.srv)
		require.NoError(t, f.srv.ReplaceProviders(ctx, []aibridge.Provider{
			openAIProvider("openai", singleKeyPool(t, "openai", "key")),
		}))
		retired, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
		require.NoError(t, err)

		require.NoError(t, f.srv.Shutdown(context.Background()))
		require.ErrorIs(t, f.srv.ReplaceProviders(ctx, []aibridge.Provider{
			openAIProvider("openai", singleKeyPool(t, "openai", "key")),
		}), aibridged.ErrShutdown)

		// The pool handed to a proxy-mode server never serves requests, but
		// it must still be released on shutdown.
		require.GreaterOrEqual(t, f.poolShutdowns.Load(), int32(1))

		// Shutdown refuses proxy requests, whether the handler was acquired
		// before or after it.
		h, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
		require.NoError(t, err)
		require.Equal(t, http.StatusServiceUnavailable, serveHandler(t, h, "/openai/v1/chat/completions").Code)
		require.Equal(t, http.StatusServiceUnavailable, serveHandler(t, retired, "/openai/v1/chat/completions").Code)
	})

	t.Run("CanceledShutdown", func(t *testing.T) {
		t.Parallel()

		f := newBackendServer(t, proxyExperiments(), staticConfigs(noMCPConfigs()), false)
		waitReady(t, f.srv)
		require.NoError(t, f.srv.ReplaceProviders(t.Context(), nil))
		acquired, err := f.srv.GetRequestHandler(t.Context(), aibridged.Request{})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		require.ErrorIs(t, f.srv.Shutdown(ctx), context.Canceled)

		// A shutdown that cannot finish gracefully still refuses proxy
		// requests instead of serving from a snapshot that is being released.
		handler, err := f.srv.GetRequestHandler(t.Context(), aibridged.Request{})
		require.NoError(t, err)
		require.Equal(t, http.StatusServiceUnavailable, serveHandler(t, handler, "/openai/v1/models").Code)
		require.Equal(t, http.StatusServiceUnavailable, serveHandler(t, acquired, "/openai/v1/models").Code)
	})
}

// TestBackend_ConcurrentUse exercises request serving, reloads and shutdown
// concurrently. It is meaningful under -race.
func TestBackend_ConcurrentUse(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		response *proto.GetMCPServerConfigsResponse
		proxy    bool
	}{
		{name: "Proxy", response: noMCPConfigs(), proxy: true},
		{name: "Interception", response: coderMCPConfig()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			response := tc.response
			f := newBackendServer(t, proxyExperiments(), staticConfigs(response), true)
			if !tc.proxy {
				f.pool.EXPECT().Acquire(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
					Return(&sentinelHandler{body: "bridge"}, nil)
				f.pool.EXPECT().ReplaceProviders(gomock.Any()).AnyTimes()
			}

			ctx := testutil.Context(t, testutil.WaitShort)
			waitReady(t, f.srv)

			const workers = 8
			var wg sync.WaitGroup
			start := make(chan struct{})

			providers := []aibridge.Provider{openAIProvider("openai", singleKeyPool(t, "openai", "key"))}
			for i := 0; i < workers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					for j := 0; j < 20; j++ {
						h, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
						if err != nil {
							continue
						}
						rec := httptest.NewRecorder()
						h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil))
					}
				}()

				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					for j := 0; j < 20; j++ {
						// Errors are expected once shutdown wins the race.
						_ = f.srv.ReplaceProviders(ctx, providers)
						_ = f.srv.KeyPools()
					}
				}()
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_ = f.srv.Shutdown(context.Background())
			}()

			close(start)
			wg.Wait()

			if tc.proxy {
				h, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
				require.NoError(t, err)
				require.Equal(t, http.StatusServiceUnavailable, serveHandler(t, h, "/openai/v1/chat/completions").Code,
					"shutdown must refuse proxy requests")
			}
		})
	}
}

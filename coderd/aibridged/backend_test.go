package aibridged_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"
	"storj.io/drpc"

	"cdr.dev/slog/v3"
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

// countingDRPCConn tracks connections discarded after failed mode selection.
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

// expectMCPConfigs counts discovery calls and supplies their responses.
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

// backendFixture holds the collaborators of a server under test.
type backendFixture struct {
	srv      *aibridged.Server
	conn     *countingDRPCConn
	mcpCalls *atomic.Int32
	logSink  *testutil.FakeSink
}

// newBackendServer starts a server whose single mock client answers MCP
// discovery with fn. Interception pools are created by the server under test.
func newBackendServer(t *testing.T, experiments codersdk.Experiments, fn mcpConfigsFn, ignoreErrors bool) *backendFixture {
	t.Helper()

	ctrl := gomock.NewController(t)
	client := mock.NewMockDRPCClient(ctrl)
	conn := newCountingDRPCConn(nil)
	client.EXPECT().DRPCConn().AnyTimes().Return(conn)
	calls := expectMCPConfigs(client, fn)
	logSink := testutil.NewFakeSink(t)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: ignoreErrors}).AppendSinks(logSink)

	srv, err := aibridged.New(t.Context(),
		func(context.Context) (aibridged.DRPCClient, error) { return client, nil },
		logger, testTracer,
		experiments, nil)
	require.NoError(t, err, "create aibridged server")
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	return &backendFixture{srv: srv, conn: conn, mcpCalls: calls, logSink: logSink}
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

func collectServerMetrics(t *testing.T, server *aibridged.Server) []*dto.MetricFamily {
	t.Helper()

	registry := prometheus.NewRegistry()
	require.NoError(t, registry.Register(server.KeyPoolStateCollector()))
	metrics, err := registry.Gather()
	require.NoError(t, err)
	return metrics
}

func requireKeyPoolState(t *testing.T, server *aibridged.Server, value float64, provider, state string) {
	t.Helper()
	require.True(t, testutil.PromGaugeHasValue(t, collectServerMetrics(t, server), value, "key_pool_state", provider, state))
}

func requireNoKeyPoolState(t *testing.T, server *aibridged.Server, provider, state string) {
	t.Helper()
	require.False(t, testutil.PromGaugeGathered(t, collectServerMetrics(t, server), "key_pool_state", provider, state))
}

func openAIProvider(name string, pool *keypool.Pool) aibridge.Provider {
	return aibridge.NewOpenAIProvider(config.OpenAI{Name: name, BaseURL: "http://upstream.test", KeyPool: pool})
}

func TestBackendMode_Interception(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		experiments codersdk.Experiments
		response    *proto.GetMCPServerConfigsResponse
		wantCalls   int32
	}{
		{name: "ExperimentDisabled", response: noMCPConfigs()},
		{name: "CoderMCP", experiments: proxyExperiments(), response: coderMCPConfig(), wantCalls: 1},
		{name: "ExternalAuthMCP", experiments: proxyExperiments(), response: externalMCPConfig(), wantCalls: 1},
		{name: "Both", experiments: proxyExperiments(), wantCalls: 1, response: &proto.GetMCPServerConfigsResponse{
			CoderMcpConfig:         coderMCPConfig().GetCoderMcpConfig(),
			ExternalAuthMcpConfigs: externalMCPConfig().GetExternalAuthMcpConfigs(),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newBackendServer(t, tc.experiments, func(int32) (*proto.GetMCPServerConfigsResponse, error) {
				if tc.wantCalls == 0 {
					t.Error("MCP discovery must not run when the experiment is disabled")
				}
				return tc.response, nil
			}, false)
			ctx := testutil.Context(t, testutil.WaitShort)
			waitReady(t, f.srv)
			require.NotNil(t, f.srv.InterceptionPoolForTest())
			require.Equal(t, tc.wantCalls, f.mcpCalls.Load())

			require.NoError(t, f.srv.ReplaceProviders(ctx, []aibridge.Provider{
				openAIProvider("openai", singleKeyPool(t, "openai", "key")),
			}))
			requireKeyPoolState(t, f.srv, 1, "openai", "valid")
		})
	}
}

// Proxy mode returns 503 until providers load, then 404 for unregistered routes.
func TestBackendMode_ProxyWhenNoMCPConfigs(t *testing.T) {
	t.Parallel()

	f := newBackendServer(t, proxyExperiments(), staticConfigs(noMCPConfigs()), false)

	ctx := testutil.Context(t, testutil.WaitShort)
	waitReady(t, f.srv)
	require.Equal(t, int32(1), f.mcpCalls.Load())
	require.Nil(t, f.srv.InterceptionPoolForTest())
	warnings := f.logSink.Entries(func(entry slog.SinkEntry) bool {
		return entry.Message == "selected experimental reverse proxy routing; only request start and end are recorded; token/prompt/tool/model accounting and spend accrual, Bedrock, and actor-header injection are unsupported; remove ai-gateway-reverse-proxy and restart to restore interception mode"
	})
	require.Len(t, warnings, 1)
	require.Equal(t, slog.LevelWarn, warnings[0].Level)

	h, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
	require.NoError(t, err, "the proxy backend must answer requests instead of erroring")
	rec := serveHandler(t, h, "/openai/v1/chat/completions")
	require.Equal(t, http.StatusServiceUnavailable, rec.Code, "requests before the first snapshot must be refused")
	require.Equal(t, "AI Gateway is starting up, retry shortly\n", rec.Body.String())
	requireNoKeyPoolState(t, f.srv, "openai", "valid")

	pool := singleKeyPool(t, "openai", "key")
	require.NoError(t, f.srv.ReplaceProviders(ctx, []aibridge.Provider{openAIProvider("openai", pool)}))

	h, err = f.srv.GetRequestHandler(ctx, aibridged.Request{})
	require.NoError(t, err)
	rec = serveHandler(t, h, "/openai/v1/chat/completions")
	require.Equal(t, http.StatusBadRequest, rec.Code, "bridged routes require authenticated actor context")
	require.Equal(t, "no actor found\n", rec.Body.String())
	requireKeyPoolState(t, f.srv, 1, "openai", "valid")
}

// Reconnects and reloads must not repeat mode selection.
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
			closedCh := make(chan struct{})
			// Redial switches to a fresh connection after the first is closed.
			firstConn := newCountingDRPCConn(closedCh)
			secondConn := newCountingDRPCConn(nil)
			var activeConn atomic.Pointer[countingDRPCConn]
			activeConn.Store(firstConn)
			client.EXPECT().DRPCConn().AnyTimes().DoAndReturn(func() drpc.Conn { return activeConn.Load() })

			calls := expectMCPConfigs(client, func(call int32) (*proto.GetMCPServerConfigsResponse, error) {
				if call == 1 {
					return tc.first, nil
				}
				return tc.second, nil
			})

			var dials atomic.Int32
			srv, err := aibridged.New(t.Context(), func(context.Context) (aibridged.DRPCClient, error) {
				if dials.Add(1) >= 2 {
					activeConn.Store(secondConn)
				}
				return client, nil
			}, slogtest.Make(t, nil), testTracer, proxyExperiments(), nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

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

			if tc.wantPool {
				require.NotNil(t, srv.InterceptionPoolForTest())
				requireKeyPoolState(t, srv, 1, "openai", "valid")
				return
			}

			h, err := srv.GetRequestHandler(ctx, aibridged.Request{})
			require.NoError(t, err)
			require.Nil(t, srv.InterceptionPoolForTest())
			requireKeyPoolState(t, srv, 1, "openai", "valid")
			require.Equal(t, http.StatusBadRequest, serveHandler(t, h, "/openai/v1/chat/completions").Code)
		})
	}
}

// Failed discovery must discard the connection, not select proxy mode.
func TestBackendMode_MCPDiscoveryFailureRetries(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "RPCError", err: xerrors.New("boom")},
		{name: "NilResponse"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newBackendServer(t, proxyExperiments(), func(call int32) (*proto.GetMCPServerConfigsResponse, error) {
				if call <= 2 {
					return nil, tc.err // A nil error exercises a malformed response.
				}
				return noMCPConfigs(), nil
			}, true)
			ctx := testutil.Context(t, testutil.WaitShort)

			// While selection keeps failing the server is not ready and
			// requests are refused rather than served by a guessed backend.
			require.Eventually(t, func() bool { return f.mcpCalls.Load() >= 1 }, testutil.WaitShort, testutil.IntervalFast)
			require.False(t, f.srv.Ready())
			h, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
			require.NoError(t, err)
			require.Equal(t, http.StatusServiceUnavailable, serveHandler(t, h, "/openai/v1/chat/completions").Code)
			requireNoKeyPoolState(t, f.srv, "openai", "valid")
			require.Nil(t, f.srv.InterceptionPoolForTest())

			waitReady(t, f.srv)
			require.Equal(t, int32(3), f.mcpCalls.Load(), "selection must be retried on each new connection")
			require.GreaterOrEqual(t, f.conn.closes.Load(), int32(2), "a connection whose selection failed must be discarded")

			require.NoError(t, f.srv.ReplaceProviders(ctx, []aibridge.Provider{
				openAIProvider("openai", singleKeyPool(t, "openai", "key")),
			}))
			h, err = f.srv.GetRequestHandler(ctx, aibridged.Request{})
			require.NoError(t, err)
			require.Equal(t, http.StatusBadRequest, serveHandler(t, h, "/openai/v1/chat/completions").Code)
		})
	}
}

// Reloads publish a new router while acquired handlers retain their snapshot.
func TestReplaceProviders_SwapsRouter(t *testing.T) {
	t.Parallel()

	f := newBackendServer(t, proxyExperiments(), staticConfigs(noMCPConfigs()), false)

	ctx := testutil.Context(t, testutil.WaitShort)
	waitReady(t, f.srv)

	firstPool := singleKeyPool(t, "openai", "key")
	require.NoError(t, f.srv.ReplaceProviders(ctx, []aibridge.Provider{openAIProvider("openai", firstPool)}))
	requireKeyPoolState(t, f.srv, 1, "openai", "valid")

	oldHandler, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
	require.NoError(t, err)

	secondPool := singleKeyPool(t, "anthropic", "key")
	require.NoError(t, f.srv.ReplaceProviders(ctx, []aibridge.Provider{
		openAIProvider("openai", firstPool),
		openAIProvider("anthropic", secondPool),
		aibridge.NewDisabledProviderStub("disabled", "openai"),
	}))
	requireKeyPoolState(t, f.srv, 1, "openai", "valid")
	requireKeyPoolState(t, f.srv, 1, "anthropic", "valid")

	newHandler, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
	require.NoError(t, err)

	// Handlers already acquired keep serving from the router they hold.
	require.Equal(t, http.StatusNotFound, serveHandler(t, oldHandler, "/disabled/v1/models").Code)
	require.Equal(t, http.StatusServiceUnavailable, serveHandler(t, newHandler, "/disabled/v1/models").Code)
}

func TestReplaceProvidersClosesIdleConnectionsWithoutCancelingInflight(t *testing.T) {
	t.Parallel()

	activeStarted := make(chan struct{})
	activeRelease := make(chan struct{})
	activeCanceled := make(chan struct{}, 1)
	idleConnections := make(chan string, 4)
	closedConnections := make(chan string, 4)
	firstUpstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models/active" {
			close(activeStarted)
			select {
			case <-activeRelease:
			case <-r.Context().Done():
				activeCanceled <- struct{}{}
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	firstUpstream.Config.ConnState = func(conn net.Conn, state http.ConnState) {
		remoteAddr := conn.RemoteAddr()
		if remoteAddr == nil {
			return
		}
		switch state {
		case http.StateIdle:
			idleConnections <- remoteAddr.String()
		case http.StateClosed:
			closedConnections <- remoteAddr.String()
		default:
		}
	}
	firstUpstream.Start()
	t.Cleanup(firstUpstream.Close)

	var secondHits atomic.Int32
	secondUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondHits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(secondUpstream.Close)

	f := newBackendServer(t, proxyExperiments(), staticConfigs(noMCPConfigs()), false)
	ctx := testutil.Context(t, testutil.WaitShort)
	waitReady(t, f.srv)
	firstProvider := aibridge.NewOpenAIProvider(config.OpenAI{
		BaseURL: firstUpstream.URL,
		KeyPool: singleKeyPool(t, "openai", "first-key"),
	})
	require.NoError(t, f.srv.ReplaceProviders(ctx, []aibridge.Provider{firstProvider}))
	oldHandler, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
	require.NoError(t, err)

	activeDone := make(chan int, 1)
	go func() {
		rec := serveHandler(t, oldHandler, "/openai/v1/models/active")
		activeDone <- rec.Code
	}()
	testutil.TryReceive(ctx, t, activeStarted)

	require.Equal(t, http.StatusNoContent, serveHandler(t, oldHandler, "/openai/v1/models").Code)
	idleAddr := testutil.TryReceive(ctx, t, idleConnections)

	secondProvider := aibridge.NewOpenAIProvider(config.OpenAI{
		BaseURL: secondUpstream.URL,
		KeyPool: singleKeyPool(t, "openai", "second-key"),
	})
	require.NoError(t, f.srv.ReplaceProviders(ctx, []aibridge.Provider{secondProvider}))

	for {
		closedAddr := testutil.TryReceive(ctx, t, closedConnections)
		if closedAddr == idleAddr {
			break
		}
	}
	select {
	case <-activeCanceled:
		t.Fatal("provider replacement canceled an in-flight old-router request")
	default:
	}

	newHandler, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, serveHandler(t, newHandler, "/openai/v1/models").Code)
	require.Equal(t, int32(1), secondHits.Load())

	close(activeRelease)
	require.Equal(t, http.StatusNoContent, testutil.TryReceive(ctx, t, activeDone))
}

// Invalid snapshots leave the previous router serving.
func TestReplaceProviders_FailureRetainsRouter(t *testing.T) {
	t.Parallel()

	f := newBackendServer(t, proxyExperiments(), staticConfigs(noMCPConfigs()), false)

	ctx := testutil.Context(t, testutil.WaitShort)
	waitReady(t, f.srv)

	good := singleKeyPool(t, "openai", "key")
	require.NoError(t, f.srv.ReplaceProviders(ctx, []aibridge.Provider{
		openAIProvider("openai", good),
		aibridge.NewDisabledProviderStub("disabled", "openai"),
	}))
	handler, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
	require.NoError(t, err)

	err = f.srv.ReplaceProviders(ctx, []aibridge.Provider{
		openAIProvider("dupe", singleKeyPool(t, "a", "key")),
		openAIProvider("dupe", singleKeyPool(t, "b", "key")),
	})
	require.ErrorContains(t, err, "duplicate provider name")

	requireKeyPoolState(t, f.srv, 1, "openai", "valid")
	retained, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, serveHandler(t, handler, "/disabled/v1/models").Code)
	require.Equal(t, http.StatusServiceUnavailable, serveHandler(t, retained, "/disabled/v1/models").Code)
	require.Equal(t, http.StatusBadRequest, serveHandler(t, retained, "/openai/v1/chat/completions").Code)
}

// Cancellation and shutdown prevent publication; shutdown also stops serving.
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
		requireNoKeyPoolState(t, f.srv, "openai", "valid")
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

// Exercise serving, reloads, metrics, and shutdown together under -race.
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

			f := newBackendServer(t, proxyExperiments(), staticConfigs(tc.response), true)

			ctx := testutil.Context(t, testutil.WaitShort)
			waitReady(t, f.srv)

			const workers = 8
			registry := prometheus.NewRegistry()
			require.NoError(t, registry.Register(f.srv.KeyPoolStateCollector()))
			gatherErrors := make(chan error, workers)
			var wg sync.WaitGroup
			start := make(chan struct{})

			providers := []aibridge.Provider{openAIProvider("openai", singleKeyPool(t, "openai", "key"))}
			for range workers {
				wg.Go(func() {
					<-start
					for range 20 {
						h, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
						if err != nil {
							continue
						}
						rec := httptest.NewRecorder()
						h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil))
					}
				})

				wg.Go(func() {
					<-start
					for range 20 {
						// Errors are expected once shutdown wins the race.
						_ = f.srv.ReplaceProviders(ctx, providers)
						if _, err := registry.Gather(); err != nil {
							gatherErrors <- err
							return
						}
					}
				})
			}

			wg.Go(func() {
				<-start
				_ = f.srv.Shutdown(context.Background())
			})

			close(start)
			wg.Wait()
			close(gatherErrors)
			for err := range gatherErrors {
				require.NoError(t, err)
			}

			if tc.proxy {
				h, err := f.srv.GetRequestHandler(ctx, aibridged.Request{})
				require.NoError(t, err)
				require.Equal(t, http.StatusServiceUnavailable, serveHandler(t, h, "/openai/v1/chat/completions").Code,
					"shutdown must refuse proxy requests")
			}
		})
	}
}

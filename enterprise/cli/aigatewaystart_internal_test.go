//go:build !slim

package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"golang.org/x/xerrors"
	"storj.io/drpc"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/cli/clitest"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// failThenSucceedReloader fails the first failUntil reloads, then succeeds,
// modeling a coderd connection or provider fetch that recovers after a few
// transient failures.
type failThenSucceedReloader struct {
	calls     atomic.Int32
	failUntil int32
}

func (r *failThenSucceedReloader) Reload(_ context.Context) error {
	if r.calls.Add(1) <= r.failUntil {
		return xerrors.New("transient failure")
	}
	return nil
}

type failingReloader struct {
	after func()
	calls atomic.Int32
	err   error
}

func (r *failingReloader) Reload(context.Context) error {
	r.calls.Add(1)
	if r.after != nil {
		r.after()
	}
	return r.err
}

type connectedDRPCConn struct {
	drpc.Conn
	closed chan struct{}
	once   sync.Once
}

func (c *connectedDRPCConn) Close() error {
	c.once.Do(func() {
		close(c.closed)
	})
	return nil
}

func (c *connectedDRPCConn) Closed() <-chan struct{} {
	return c.closed
}

type controlledShutdownPool struct {
	*aibridged.CachedBridgePool
	err     error
	release <-chan struct{}
	started chan<- struct{}
}

func (p *controlledShutdownPool) Shutdown(ctx context.Context) error {
	if p.started != nil {
		p.started <- struct{}{}
	}
	if p.release != nil {
		select {
		case <-p.release:
		case <-ctx.Done():
			return errors.Join(ctx.Err(), p.err)
		}
	}
	return errors.Join(p.CachedBridgePool.Shutdown(ctx), p.err)
}

type standaloneGatewayTestParams struct {
	address string
	params  standaloneGatewayParams
	pool    *controlledShutdownPool
}

func newStandaloneGatewayTestParams(t *testing.T) *standaloneGatewayTestParams {
	t.Helper()

	logger := slog.Make()
	tracer := sdktrace.NewTracerProvider().Tracer("test")
	cachedPool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, nil, logger, nil, tracer)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, shutdownWithTimeout(cachedPool.Shutdown, testutil.WaitShort))
	})

	pool := &controlledShutdownPool{CachedBridgePool: cachedPool}
	address := fmt.Sprintf("127.0.0.1:%d", testutil.RandomPort(t))
	return &standaloneGatewayTestParams{
		address: address,
		params: standaloneGatewayParams{
			httpAddress: address,

			dialer: blockingStandaloneDaemonDialer,
			pool:   pool,

			logger: logger,
			tracer: tracer,
		},
		pool: pool,
	}
}

func TestStandaloneGatewayLoadProviders(t *testing.T) {
	t.Parallel()

	reloadErr := xerrors.New("reload failed")
	tests := []struct {
		name       string
		setup      func(*testing.T, *aibridged.Server, context.CancelFunc) (aibridged.ProviderReloader, *atomic.Int32)
		wantErr    error
		wantCalls  int32
		wantLoaded bool
	}{
		{
			name: "Retry succeeds",
			setup: func(_ *testing.T, _ *aibridged.Server, _ context.CancelFunc) (aibridged.ProviderReloader, *atomic.Int32) {
				reloader := &failThenSucceedReloader{failUntil: 2}
				return reloader, &reloader.calls
			},
			wantCalls:  3,
			wantLoaded: true,
		},
		{
			name: "Daemon stops retry",
			setup: func(t *testing.T, daemon *aibridged.Server, _ context.CancelFunc) (aibridged.ProviderReloader, *atomic.Int32) {
				reloader := &failingReloader{
					after: func() {
						require.NoError(t, daemon.Close())
					},
					err: reloadErr,
				}
				return reloader, &reloader.calls
			},
			wantErr:   reloadErr,
			wantCalls: 1,
		},
		{
			name: "Context cancellation stops retry",
			setup: func(_ *testing.T, _ *aibridged.Server, cancel context.CancelFunc) (aibridged.ProviderReloader, *atomic.Int32) {
				reloader := &failingReloader{after: cancel, err: reloadErr}
				return reloader, &reloader.calls
			},
			wantErr:   context.Canceled,
			wantCalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
			defer cancel()
			logger := slog.Make()
			daemon := newTestStandaloneDaemon(t, logger)
			reloader, calls := tc.setup(t, daemon, cancel)
			gateway := &standaloneGateway{
				daemon:         daemon,
				providerLogger: logger,
				reloader:       reloader,
			}

			err := gateway.loadProviders(ctx)
			if tc.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.wantErr)
			}
			require.Equal(t, tc.wantCalls, calls.Load())
			require.Equal(t, tc.wantLoaded, gateway.providersLoaded.Load())
		})
	}
}

func TestStandaloneGatewayHealthAndReadiness(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	logger := slog.Make()
	tracer := sdktrace.NewTracerProvider().Tracer("test")
	pool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, nil, logger, nil, tracer)
	require.NoError(t, err)
	connections := make(chan drpc.Conn, 2)
	dialer := func(ctx context.Context) (aibridged.DRPCClient, error) {
		select {
		case conn := <-connections:
			return &aibridged.Client{Conn: conn}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	daemon, err := aibridged.New(ctx, pool, dialer, logger, tracer)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, shutdownWithTimeout(daemon.Shutdown, testutil.WaitShort))
	})

	gateway := &standaloneGateway{
		daemon:         daemon,
		providerLogger: logger,
		reloader:       &failThenSucceedReloader{},
	}
	gateway.httpServer = &http.Server{
		Handler:           newGatewayMux(daemon, gateway.ready, func(next http.Handler) http.Handler { return next }),
		ReadHeaderTimeout: testutil.WaitShort,
	}

	// The HTTP server is healthy before the daemon connects or providers load.
	require.Equal(t, http.StatusOK, healthzStatus(t, gateway))
	require.Equal(t, http.StatusServiceUnavailable, readyzStatus(t, gateway))

	// A daemon connection alone does not make the gateway ready.
	firstConn := &connectedDRPCConn{closed: make(chan struct{})}
	connections <- firstConn
	require.Eventually(t, daemon.Ready, testutil.WaitShort, testutil.IntervalFast)
	require.Equal(t, http.StatusOK, healthzStatus(t, gateway))
	require.Equal(t, http.StatusServiceUnavailable, readyzStatus(t, gateway))

	// The gateway becomes ready after the initial provider load completes.
	require.NoError(t, gateway.loadProviders(ctx))
	require.Equal(t, http.StatusOK, healthzStatus(t, gateway))
	require.Equal(t, http.StatusOK, readyzStatus(t, gateway))

	// Losing the daemon connection affects readiness but not HTTP health.
	require.NoError(t, firstConn.Close())
	require.Eventually(t, func() bool { return !daemon.Ready() }, testutil.WaitShort, testutil.IntervalFast)
	require.Equal(t, http.StatusOK, healthzStatus(t, gateway))
	require.Equal(t, http.StatusServiceUnavailable, readyzStatus(t, gateway))

	// Readiness recovers when the daemon reconnects; providers remain loaded.
	connections <- &connectedDRPCConn{closed: make(chan struct{})}
	require.Eventually(t, daemon.Ready, testutil.WaitShort, testutil.IntervalFast)
	require.Equal(t, http.StatusOK, healthzStatus(t, gateway))
	require.Equal(t, http.StatusOK, readyzStatus(t, gateway))
}

func healthzStatus(t *testing.T, gateway *standaloneGateway) int {
	t.Helper()
	return probeStatus(t, gateway, healthzPath)
}

func readyzStatus(t *testing.T, gateway *standaloneGateway) int {
	t.Helper()
	return probeStatus(t, gateway, readyzPath)
}

func probeStatus(t *testing.T, gateway *standaloneGateway, path string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	gateway.httpServer.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code
}

func TestAIGatewayStart_HealthBeforeProviders(t *testing.T) {
	t.Parallel()

	gatewayAddress := fmt.Sprintf("127.0.0.1:%d", testutil.RandomPort(t))
	coderAddress := fmt.Sprintf("127.0.0.1:%d", testutil.RandomPort(t))

	var root RootCmd
	cmd, err := root.Command(root.enterpriseOnly())
	require.NoError(t, err)
	inv, _ := clitest.NewWithCommand(t, cmd,
		"--url", "http://"+coderAddress,
		"ai-gateway", "start",
		"--key", "test-key",
		"--http-address", gatewayAddress,
	)
	clitest.Start(t, inv.WithContext(testutil.Context(t, testutil.WaitShort)))

	client := &http.Client{Timeout: testutil.WaitShort}
	baseURL := "http://" + gatewayAddress
	require.Eventually(t, func() bool {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+healthzPath, nil)
		if err != nil {
			return false
		}
		resp, err := client.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, testutil.WaitShort, testutil.IntervalFast)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+readyzPath, nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestRunStandaloneGateway_ContextCanceled(t *testing.T) {
	t.Parallel()

	testCtx := testutil.Context(t, testutil.WaitShort)
	runCtx, cancelRun := context.WithCancel(testCtx)
	defer cancelRun()
	test := newStandaloneGatewayTestParams(t)

	runDone := make(chan error, 1)
	go func() {
		runDone <- runStandaloneGateway(runCtx, test.params)
	}()
	requireListenerReady(t, test.address)
	cancelRun()

	require.NoError(t, testutil.RequireReceive(testCtx, t, runDone))
	requireListenerAvailable(t, test.address, "HTTP listener must be closed before run returns")
}

func TestRunStandaloneGateway_DaemonExited(t *testing.T) {
	t.Parallel()

	test := newStandaloneGatewayTestParams(t)
	test.params.dialer = func(context.Context) (aibridged.DRPCClient, error) {
		return nil, codersdk.NewError(http.StatusUnauthorized, codersdk.Response{Message: "invalid gateway key"})
	}

	err := runStandaloneGateway(testutil.Context(t, testutil.WaitShort), test.params)
	require.ErrorContains(t, err, "AI Gateway daemon exited")
	requireListenerAvailable(t, test.address, "HTTP listener must be closed before run returns")
}

func TestRunStandaloneGateway_HTTPStopsBeforeDaemonShutdown(t *testing.T) {
	t.Parallel()

	testCtx := testutil.Context(t, testutil.WaitShort)
	test := newStandaloneGatewayTestParams(t)
	shutdownErr := xerrors.New("pool shutdown failed")
	shutdownStarted := make(chan struct{}, 1)
	shutdownRelease := make(chan struct{})
	test.pool.err = shutdownErr
	test.pool.started = shutdownStarted
	test.pool.release = shutdownRelease
	test.params.tlsCertFile = filepath.Join(t.TempDir(), "missing.crt")
	test.params.tlsKeyFile = filepath.Join(t.TempDir(), "missing.key")

	runDone := make(chan error, 1)
	go func() {
		runDone <- runStandaloneGateway(testCtx, test.params)
	}()
	testutil.RequireReceive(testCtx, t, shutdownStarted)
	requireListenerAvailable(t, test.address, "HTTP listener must close before daemon shutdown")
	close(shutdownRelease)

	err := testutil.RequireReceive(testCtx, t, runDone)
	require.ErrorContains(t, err, "serve:")
	require.ErrorContains(t, err, "shutdown AI Gateway daemon:")
	require.ErrorContains(t, err, shutdownErr.Error())
}

func TestRunStandaloneGateway_ListenAndShutdownErrors(t *testing.T) {
	t.Parallel()

	test := newStandaloneGatewayTestParams(t)
	shutdownErr := xerrors.New("pool shutdown failed")
	test.pool.err = shutdownErr
	listener, err := net.Listen("tcp", test.address)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = listener.Close()
	})

	err = runStandaloneGateway(testutil.Context(t, testutil.WaitShort), test.params)
	require.NoError(t, listener.Close())
	require.ErrorContains(t, err, "listen on")
	require.ErrorContains(t, err, "shutdown AI Gateway daemon:")
	require.ErrorContains(t, err, shutdownErr.Error())
}

func TestStandaloneGatewayServe_ShutdownOrder(t *testing.T) {
	t.Parallel()

	// Set up a running daemon, provider reloader, and blocked HTTP request.
	testCtx := testutil.Context(t, testutil.WaitShort)
	logger := slog.Make()
	pool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, nil, logger, nil, sdktrace.NewTracerProvider().Tracer("test"))
	require.NoError(t, err)

	dialCtxCh := make(chan context.Context, 1)
	dialer := func(ctx context.Context) (aibridged.DRPCClient, error) {
		select {
		case dialCtxCh <- ctx:
		default:
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	daemon, err := aibridged.New(context.Background(), pool, dialer, logger, sdktrace.NewTracerProvider().Tracer("test"))
	require.NoError(t, err)

	httpAddress := fmt.Sprintf("127.0.0.1:%d", testutil.RandomPort(t))
	reloader := &failThenSucceedReloader{}
	handlerStarted := make(chan struct{}, 1)
	httpShutdownStarted := make(chan struct{}, 1)
	releaseHandler := make(chan struct{})
	gateway := &standaloneGateway{
		daemon: daemon,
		httpServer: &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				select {
				case handlerStarted <- struct{}{}:
				default:
				}
				<-releaseHandler
				w.WriteHeader(http.StatusNoContent)
			}),
			ReadHeaderTimeout: testutil.WaitShort,
		},
		httpAddress:    httpAddress,
		logger:         logger,
		providerLogger: logger,
		reloader:       reloader,
	}
	gateway.httpServer.RegisterOnShutdown(func() {
		httpShutdownStarted <- struct{}{}
	})

	serveCtx, cancelServe := context.WithCancel(testCtx)
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- gateway.serve(serveCtx)
	}()

	dialCtx := testutil.RequireReceive(testCtx, t, dialCtxCh)
	require.Eventually(t, gateway.providersLoaded.Load, testutil.WaitShort, testutil.IntervalFast)
	requireListenerReady(t, httpAddress)

	requestDone := make(chan error, 1)
	go func() {
		req, err := http.NewRequestWithContext(testCtx, http.MethodGet, "http://"+httpAddress, nil)
		if err != nil {
			requestDone <- err
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusNoContent {
				err = xerrors.Errorf("unexpected status code: %d", resp.StatusCode)
			}
		}
		requestDone <- err
	}()
	testutil.RequireReceive(testCtx, t, handlerStarted)

	// Trigger shutdown after the initial load enters the provider watch loop.
	cancelServe()
	testutil.RequireReceive(testCtx, t, httpShutdownStarted)
	select {
	case <-dialCtx.Done():
		t.Fatal("daemon context canceled before the in-flight HTTP request drained")
	case err := <-serveDone:
		t.Fatalf("server returned before the in-flight HTTP request drained: %v", err)
	default:
	}

	// Expect provider reload to stop while HTTP draining keeps the daemon alive.
	close(releaseHandler)
	require.NoError(t, testutil.RequireReceive(testCtx, t, requestDone))
	require.NoError(t, testutil.RequireReceive(testCtx, t, serveDone))
	select {
	case <-dialCtx.Done():
		t.Fatal("daemon context canceled by serve")
	case <-daemon.Done():
		t.Fatal("daemon stopped before its runtime owner shut it down")
	default:
	}

	// Expect the runtime owner to shut down the daemon after HTTP serving stops.
	require.NoError(t, shutdownWithTimeout(daemon.Shutdown, daemonShutdownTimeout))
	testutil.TryReceive(testCtx, t, dialCtx.Done())
	testutil.TryReceive(testCtx, t, daemon.Done())

	requireListenerAvailable(t, httpAddress, "HTTP listener must be closed before serve returns")
}

func requireListenerReady(t *testing.T, address string) {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitShort)
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, testutil.IntervalFast)
}

func requireListenerAvailable(t *testing.T, address, message string) {
	t.Helper()

	listener, err := net.Listen("tcp", address)
	require.NoError(t, err, message)
	require.NoError(t, listener.Close())
}

func newTestStandaloneDaemon(t *testing.T, logger slog.Logger) *aibridged.Server {
	t.Helper()

	tracer := sdktrace.NewTracerProvider().Tracer("test")
	pool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, nil, logger, nil, tracer)
	require.NoError(t, err)
	daemon, err := aibridged.New(context.Background(), pool, blockingStandaloneDaemonDialer, logger, tracer)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, daemon.Close())
	})
	return daemon
}

func blockingStandaloneDaemonDialer(ctx context.Context) (aibridged.DRPCClient, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestResolveAIGatewayKey(t *testing.T) {
	t.Parallel()

	keyFile := filepath.Join(t.TempDir(), "gateway.key")
	require.NoError(t, os.WriteFile(keyFile, []byte("file-key\n"), 0o600))

	tests := []struct {
		name    string
		key     string
		keyFile string
		want    string
		wantErr string
	}{
		{
			name:    "Nothing set",
			wantErr: keyFlagsMissingErr,
		},
		{
			name: "Key",
			key:  "flag-key",
			want: "flag-key",
		},
		{
			name:    "KeyFile",
			keyFile: keyFile,
			want:    "file-key",
		},
		{
			name:    "MutuallyExclusive",
			key:     "flag-key",
			keyFile: keyFile,
			wantErr: keyFlagsExclusiveErr,
		},
		{
			name:    "MissingKeyFile",
			keyFile: filepath.Join(t.TempDir(), "missing.key"),
			wantErr: "read AI Gateway key file",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveAIGatewayKey(tc.key, tc.keyFile)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestAIGatewayStart_TracingMiddleware verifies the gateway mux built by
// newGatewayMux traces the LLM routes while leaving the health probes untraced.
func TestAIGatewayStart_TracingMiddleware(t *testing.T) {
	t.Parallel()

	tracer := sdktrace.NewTracerProvider().Tracer("test")
	for _, tc := range []struct {
		name       string
		path       string
		ready      bool
		traced     bool
		wantStatus int
	}{
		{name: "root LLM route", path: "/anthropic/v1/messages", ready: true, traced: true, wantStatus: http.StatusTeapot},
		{name: "aibridge alias", path: "/api/v2/aibridge/v1/messages", ready: true, traced: true, wantStatus: http.StatusTeapot},
		{name: "healthz", path: healthzPath, ready: true, traced: false, wantStatus: http.StatusOK},
		{name: "readyz ready", path: readyzPath, ready: true, traced: false, wantStatus: http.StatusOK},
		{name: "readyz not ready", path: readyzPath, ready: false, traced: false, wantStatus: http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			})
			mux := newGatewayMux(handler, func() bool { return tc.ready }, tracingMiddleware(tracer))

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			require.NotPanics(t, func() {
				mux.ServeHTTP(rec, req)
			})
			require.Equal(t, tc.wantStatus, rec.Code)

			if tc.traced {
				require.NotEmpty(t, rec.Header().Get("X-Trace-ID"), "expected a span to be created")
			} else {
				require.Empty(t, rec.Header().Get("X-Trace-ID"), "health probes must not be traced")
			}
		})
	}
}

// TestAIGatewayStart_TracingOutermost verifies the request
// rejected by AIGatewayDataPlaneMiddleware middleware is still traced.
func TestAIGatewayStart_TracingOutermost(t *testing.T) {
	t.Parallel()

	tracer := sdktrace.NewTracerProvider().Tracer("test")

	cfg := codersdk.AIBridgeConfig{
		AllowBYOK: false,
	}

	var handlerCalls atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	wrapped := gatewayMiddleware(cfg, tracer)(handler)

	// BYOK request
	req := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", nil)
	req.Header.Set(agplaibridge.HeaderCoderToken, "byok-token")

	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	// req rejected but still traced
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.NotEmpty(t, rec.Header().Get("X-Trace-ID"), "rejected requests must still be traced")
	require.Equal(t, int32(0), handlerCalls.Load(), "rejected request must not reach the handler")
}

// TestAIGatewayStart_InheritedOptions verifies that options inherited
// from coderd's deployment values are consciously used or dropped.
// A newly added option in these groups fails this test until it
// is consciously placed in one bucket, preventing silent drift
// in what the gateway exposes.
func TestAIGatewayStart_InheritedOptions(t *testing.T) {
	t.Parallel()

	// Groups the gateway sources options from.
	sourceGroups := map[string]struct{}{
		"Logging":    {},
		"Tracing":    {},
		"AI Gateway": {},
		"Prometheus": {},
	}

	// Options in the source groups that the gateway intentionally does not
	// inherit because they only apply to coderd.
	dropped := map[string]struct{}{
		// Logging
		"CODER_ENABLE_TERRAFORM_DEBUG_MODE": {},

		// AI Gateway (coderd-only: provider seeding, budgets, retention, etc.)
		"CODER_AI_BUDGET_PERIOD":                     {},
		"CODER_AI_BUDGET_POLICY":                     {},
		"CODER_AI_GATEWAY_ANTHROPIC_BASE_URL":        {},
		"CODER_AI_GATEWAY_ANTHROPIC_KEY":             {},
		"CODER_AI_GATEWAY_BEDROCK_ACCESS_KEY":        {},
		"CODER_AI_GATEWAY_BEDROCK_ACCESS_KEY_SECRET": {},
		"CODER_AI_GATEWAY_BEDROCK_BASE_URL":          {},
		"CODER_AI_GATEWAY_BEDROCK_MODEL":             {},
		"CODER_AI_GATEWAY_BEDROCK_REGION":            {},
		"CODER_AI_GATEWAY_BEDROCK_SMALL_FAST_MODEL":  {},
		"CODER_AI_GATEWAY_ENABLED":                   {},
		"CODER_AI_GATEWAY_INJECT_CODER_MCP_TOOLS":    {},
		"CODER_AI_GATEWAY_OPENAI_BASE_URL":           {},
		"CODER_AI_GATEWAY_OPENAI_KEY":                {},
		"CODER_AI_GATEWAY_RETENTION":                 {},
		"CODER_AI_GATEWAY_STRUCTURED_LOGGING":        {},

		// Prometheus (coderd-only: agent/database collectors)
		"CODER_PROMETHEUS_AGGREGATE_AGENT_STATS_BY": {},
		"CODER_PROMETHEUS_COLLECT_AGENT_STATS":      {},
		"CODER_PROMETHEUS_COLLECT_DB_METRICS":       {},
	}

	dv := codersdk.DeploymentValues{}
	var unclassified []string
	for _, opt := range dv.Options() {
		if opt.Group == nil || opt.Env == "" {
			continue
		}
		if _, ok := sourceGroups[opt.Group.Name]; !ok {
			continue
		}
		_, inherited := aiGatewayInheritedEnvs[opt.Env]
		_, drop := dropped[opt.Env]
		require.Falsef(t, inherited && drop, "%s option is both inherited and dropped", opt.Env)
		if !inherited && !drop {
			unclassified = append(unclassified, opt.Env)
		}
	}
	require.Emptyf(t, unclassified,
		"options from source groups are neither inherited nor dropped.\n"+
			"Check if option is applicable for standalone AI Gateway.\n"+
			"If so, add it to aiGatewayInheritedEnvs, otherwise add it to the dropped set: %v", unclassified)
}

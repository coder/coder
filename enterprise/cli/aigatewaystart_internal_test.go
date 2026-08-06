//go:build !slim

package cli

import (
	"context"
	"errors"
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
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// mockReloader fails the first failUntil reloads with err, then succeeds.
// after runs at the end of every reload, letting a test hang the retry loop.
type mockReloader struct {
	calls     atomic.Int32
	failUntil int32
	err       error
	after     func()
}

func (r *mockReloader) Reload(context.Context) error {
	failed := r.calls.Add(1) <= r.failUntil
	if r.after != nil {
		r.after()
	}
	if failed {
		return r.err
	}
	return nil
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

// testGatewayOption mutates the default standaloneGatewayParams before the
// gateway is constructed.
type testGatewayOption func(*standaloneGatewayParams)

// newTestStandaloneGateway constructs a standalone gateway for
// testing, with a controllable shutdown pool and optional customizations.
// Uses blockingStandaloneDaemonDialer to mock coderd.
func newTestStandaloneGateway(t *testing.T, opts ...testGatewayOption) (*standaloneGateway, *controlledShutdownPool) {
	t.Helper()

	logger := slog.Make()
	tracer := sdktrace.NewTracerProvider().Tracer("test")
	cachedPool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, nil, logger, nil, tracer)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, shutdownWithTimeout(cachedPool.Shutdown, testutil.WaitShort))
	})

	pool := &controlledShutdownPool{CachedBridgePool: cachedPool}
	params := standaloneGatewayParams{
		httpAddress: "127.0.0.1:0",

		dialer: blockingStandaloneDaemonDialer,
		pool:   pool,

		logger: logger,
		tracer: tracer,
	}
	for _, m := range opts {
		m(&params)
	}

	gateway, err := newStandaloneGateway(params)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, shutdownWithTimeout(gateway.daemon.Shutdown, testutil.WaitShort))
	})
	return gateway, pool
}

func TestStandaloneGatewayLoadProviders(t *testing.T) {
	t.Parallel()

	reloadErr := xerrors.New("reload failed")
	tests := []struct {
		name              string
		reloaderFailUntil int32
		reloaderErr       error
		reloaderAfter     func(t *testing.T, daemon *aibridged.Server, cancel context.CancelFunc)
		wantErr           error
		wantCalls         int32
		wantLoaded        bool
	}{
		{
			name:              "Retry succeeds",
			reloaderFailUntil: 2,
			reloaderErr:       xerrors.New("transient failure"),
			wantCalls:         3,
			wantLoaded:        true,
		},
		{
			name:              "Daemon stops retry",
			reloaderFailUntil: 1,
			reloaderErr:       reloadErr,
			reloaderAfter: func(t *testing.T, daemon *aibridged.Server, _ context.CancelFunc) {
				require.NoError(t, daemon.Close())
			},
			wantErr:   reloadErr,
			wantCalls: 1,
		},
		{
			name:              "Context cancellation stops retry",
			reloaderFailUntil: 1,
			reloaderErr:       reloadErr,
			reloaderAfter: func(_ *testing.T, _ *aibridged.Server, cancel context.CancelFunc) {
				cancel()
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
			reloader := &mockReloader{failUntil: tc.reloaderFailUntil, err: tc.reloaderErr}
			gateway, _ := newTestStandaloneGateway(t)
			gateway.reloader = reloader
			// reloaderAfter needs daemon which only exists once the gateway is constructed,
			// nothing reloads until loadProviders below.
			if tc.reloaderAfter != nil {
				reloader.after = func() { tc.reloaderAfter(t, gateway.daemon, cancel) }
			}

			err := gateway.loadProviders(ctx)
			if tc.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.wantErr)
			}
			require.Equal(t, tc.wantCalls, reloader.calls.Load())
			require.Equal(t, tc.wantLoaded, gateway.providersLoaded.Load())
		})
	}
}

func TestStandaloneGatewayHealthAndReadiness(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	connections := make(chan drpc.Conn, 2)
	modifyDialer := func(p *standaloneGatewayParams) {
		p.dialer = func(ctx context.Context) (aibridged.DRPCClient, error) {
			select {
			case conn := <-connections:
				return &aibridged.Client{Conn: conn}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	gateway, _ := newTestStandaloneGateway(t, modifyDialer)
	gateway.reloader = &mockReloader{}

	// The HTTP server is healthy before the daemon connects or providers load.
	require.Equal(t, http.StatusOK, healthzStatus(t, gateway))
	require.Equal(t, http.StatusServiceUnavailable, readyzStatus(t, gateway))

	// A daemon connection alone does not make the gateway ready.
	firstConn := &connectedDRPCConn{closed: make(chan struct{})}
	connections <- firstConn
	require.Eventually(t, gateway.daemon.Ready, testutil.WaitShort, testutil.IntervalFast)
	require.Equal(t, http.StatusOK, healthzStatus(t, gateway))
	require.Equal(t, http.StatusServiceUnavailable, readyzStatus(t, gateway))

	// The gateway becomes ready after the initial provider load completes.
	require.NoError(t, gateway.loadProviders(ctx))
	require.Equal(t, http.StatusOK, healthzStatus(t, gateway))
	require.Equal(t, http.StatusOK, readyzStatus(t, gateway))

	// Losing the daemon connection affects readiness but not HTTP health.
	require.NoError(t, firstConn.Close())
	require.Eventually(t, func() bool { return !gateway.daemon.Ready() }, testutil.WaitShort, testutil.IntervalFast)
	require.Equal(t, http.StatusOK, healthzStatus(t, gateway))
	require.Equal(t, http.StatusServiceUnavailable, readyzStatus(t, gateway))

	// Readiness recovers when the daemon reconnects; providers remain loaded.
	connections <- &connectedDRPCConn{closed: make(chan struct{})}
	require.Eventually(t, gateway.daemon.Ready, testutil.WaitShort, testutil.IntervalFast)
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

func TestRunStandaloneGateway_ContextCanceled(t *testing.T) {
	t.Parallel()

	testCtx := testutil.Context(t, testutil.WaitShort)
	runCtx, cancelRun := context.WithCancel(testCtx)
	defer cancelRun()
	gateway, _ := newTestStandaloneGateway(t)
	runDone := make(chan error, 1)
	go func() {
		runDone <- gateway.run(runCtx)
	}()
	requireListening(testCtx, t, gateway, runDone)
	cancelRun()

	require.NoError(t, testutil.RequireReceive(testCtx, t, runDone))
	require.True(t, gateway.drpcClosed.Load(), "DRPC connection must be closed before run returns")
	require.True(t, gateway.providerRefreshStopped.Load(), "provider refresh must stop before run returns")
	require.True(t, gateway.listenerClosed.Load(), "HTTP listener must be closed before run returns")
}

func TestRunStandaloneGateway_DaemonExited(t *testing.T) {
	t.Parallel()

	modifyDialer := func(p *standaloneGatewayParams) {
		p.dialer = func(context.Context) (aibridged.DRPCClient, error) {
			return nil, codersdk.NewError(http.StatusUnauthorized, codersdk.Response{Message: "invalid gateway key"})
		}
	}
	gateway, _ := newTestStandaloneGateway(t, modifyDialer)
	err := gateway.run(testutil.Context(t, testutil.WaitShort))
	require.ErrorContains(t, err, "AI Gateway daemon exited")
	require.True(t, gateway.drpcClosed.Load(), "DRPC connection must be closed before run returns")
	require.True(t, gateway.providerRefreshStopped.Load(), "provider refresh must stop before run returns")
	require.True(t, gateway.listenerClosed.Load(), "HTTP listener must be closed before run returns")
}

func TestRunStandaloneGateway_HTTPStopsBeforeDaemonShutdown(t *testing.T) {
	t.Parallel()

	testCtx := testutil.Context(t, testutil.WaitShort)
	modifyTLS := func(p *standaloneGatewayParams) {
		p.tlsCertFile = filepath.Join(t.TempDir(), "missing.crt")
		p.tlsKeyFile = filepath.Join(t.TempDir(), "missing.key")
	}
	gateway, pool := newTestStandaloneGateway(t, modifyTLS)
	shutdownErr := xerrors.New("pool shutdown failed")
	shutdownStarted := make(chan struct{}, 1)
	shutdownRelease := make(chan struct{})
	pool.err = shutdownErr
	pool.started = shutdownStarted
	pool.release = shutdownRelease

	runDone := make(chan error, 1)
	go func() {
		runDone <- gateway.run(testCtx)
	}()
	testutil.RequireReceive(testCtx, t, shutdownStarted)
	require.True(t, gateway.providerRefreshStopped.Load(), "provider refresh must stop before daemon shutdown")
	require.True(t, gateway.listenerClosed.Load(), "HTTP listener must close before daemon shutdown")
	require.False(t, gateway.drpcClosed.Load(), "DRPC connection must remain open until daemon shutdown completes")
	close(shutdownRelease)

	err := testutil.RequireReceive(testCtx, t, runDone)
	require.False(t, gateway.drpcClosed.Load(), "DRPC connection shutdown must not be marked successful after an error")
	require.ErrorContains(t, err, "serve:")
	require.ErrorContains(t, err, "shutdown AI Gateway daemon:")
	require.ErrorContains(t, err, shutdownErr.Error())
}

func TestRunStandaloneGateway_ListenAndShutdownErrors(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = listener.Close()
	})
	// Occupy an address so binding the gateway listener fails.
	modifyAddr := func(p *standaloneGatewayParams) {
		p.httpAddress = listener.Addr().String()
	}
	gateway, pool := newTestStandaloneGateway(t, modifyAddr)
	shutdownErr := xerrors.New("pool shutdown failed")
	pool.err = shutdownErr

	err = gateway.run(testutil.Context(t, testutil.WaitShort))
	require.NoError(t, listener.Close())
	require.ErrorContains(t, err, "listen on")
	require.ErrorContains(t, err, "shutdown AI Gateway daemon:")
	require.ErrorContains(t, err, shutdownErr.Error())
}

func TestStandaloneGatewayServe_ShutdownOrder(t *testing.T) {
	t.Parallel()

	// Set up a running daemon, provider reloader, and blocked HTTP request.
	testCtx := testutil.Context(t, testutil.WaitShort)

	// inFlightPath scopes the blocking handler to the request this test keeps in
	// flight. Another test may probe a port the OS later assigns to this
	// listener, and such traffic must not stand in for that request.
	const inFlightPath = "/in-flight"
	reloader := &mockReloader{}
	handlerStarted := make(chan struct{}, 1)
	httpShutdownStarted := make(chan struct{}, 1)
	releaseHandler := make(chan struct{})
	gateway, _ := newTestStandaloneGateway(t)
	gateway.reloader = reloader
	// The gateway mux is replaced with a handler this test can block, so an
	// in-flight request is observable during shutdown.
	gateway.httpServer.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != inFlightPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		select {
		case handlerStarted <- struct{}{}:
		default:
		}
		<-releaseHandler
		w.WriteHeader(http.StatusNoContent)
	})
	gateway.httpServer.RegisterOnShutdown(func() {
		httpShutdownStarted <- struct{}{}
	})

	serveCtx, cancelServe := context.WithCancel(testCtx)
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- gateway.serve(serveCtx)
	}()

	listenAddress := requireListening(testCtx, t, gateway, serveDone)
	require.Eventually(t, gateway.providersLoaded.Load, testutil.WaitShort, testutil.IntervalFast)

	requestDone := make(chan error, 1)
	go func() {
		req, err := http.NewRequestWithContext(testCtx, http.MethodGet, "http://"+listenAddress+inFlightPath, nil)
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
	require.True(t, gateway.providerRefreshStopped.Load(), "provider refresh must stop before HTTP draining")
	require.False(t, gateway.listenerClosed.Load(), "HTTP listener must remain open while requests drain")
	require.False(t, gateway.drpcClosed.Load(), "DRPC connection must remain open while requests drain")

	// Expect provider reload to stop while HTTP draining keeps the daemon alive.
	close(releaseHandler)
	require.NoError(t, testutil.RequireReceive(testCtx, t, requestDone))
	require.NoError(t, testutil.RequireReceive(testCtx, t, serveDone))
	require.True(t, gateway.providerRefreshStopped.Load(), "provider refresh must stop before serve returns")
	require.True(t, gateway.listenerClosed.Load(), "HTTP listener must be closed before serve returns")
	require.False(t, gateway.drpcClosed.Load(), "DRPC connection must remain open until its runtime owner shuts it down")

	// Expect the runtime owner to shut down the daemon after HTTP serving stops.
	require.NoError(t, gateway.shutdownDaemon())
	require.True(t, gateway.drpcClosed.Load(), "DRPC connection must close during daemon shutdown")
}

// requireListening waits until the gateway's HTTP listener is bound and returns
// its resolved address. It reports the serve error, such as a port clash, rather
// than leaving callers to time out on an unrelated assertion.
func requireListening(ctx context.Context, t *testing.T, gateway *standaloneGateway, done <-chan error) string {
	t.Helper()

	select {
	case <-gateway.listenerReady:
		return gateway.httpAddr.String()
	case err := <-done:
		t.Fatalf("gateway stopped before listening: %v", err)
	case <-ctx.Done():
		t.Fatalf("gateway never started listening: %v", ctx.Err())
	}
	return ""
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

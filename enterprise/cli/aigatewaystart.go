//go:build !slim

package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.14.0"
	"go.opentelemetry.io/otel/semconv/v1.14.0/httpconv"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/keypool"
	agpl "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clilog"
	"github.com/coder/coder/v2/coderd/aibridged"
	coderdtracing "github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd"
	"github.com/coder/retry"
	"github.com/coder/serpent"
)

const (
	// The sum of daemonShutdownTimeout, httpShutdownTimeout,
	// providerReloadShutdownTimeout, and traceShutdownTimeout must stay below
	// terminationGracePeriodSeconds in helm/ai-gateway/values.yaml so the
	// process can complete graceful shutdown before Kubernetes sends SIGKILL.
	daemonShutdownTimeout         = 5 * time.Second
	httpShutdownTimeout           = 5 * time.Minute
	providerReloadShutdownTimeout = 5 * time.Second
	traceShutdownTimeout          = 5 * time.Second

	healthzPath = "/healthz"
	readyzPath  = "/readyz"

	keyFlagsExclusiveErr = "--key and --key-file options are mutually exclusive"
	keyFlagsMissingErr   = "an AI Gateway key is required, set --key (CODER_AI_GATEWAY_KEY) or --key-file (CODER_AI_GATEWAY_KEY_FILE)"
)

// aiGatewayInheritedEnvs are the coderd deployment options, keyed by env var,
// that the standalone Gateway inherits.
var aiGatewayInheritedEnvs = map[string]struct{}{
	// Logging
	"CODER_LOGGING_HUMAN":       {},
	"CODER_LOGGING_JSON":        {},
	"CODER_LOGGING_STACKDRIVER": {},
	"CODER_LOG_FILTER":          {},
	"CODER_VERBOSE":             {},

	// Tracing
	"CODER_TRACE_DATADOG":           {},
	"CODER_TRACE_ENABLE":            {},
	"CODER_TRACE_HONEYCOMB_API_KEY": {},
	"CODER_TRACE_LOGS":              {},

	// AI Gateway
	"CODER_AI_GATEWAY_ALLOW_BYOK":                        {},
	"CODER_AI_GATEWAY_CIRCUIT_BREAKER_ENABLED":           {},
	"CODER_AI_GATEWAY_CIRCUIT_BREAKER_FAILURE_THRESHOLD": {},
	"CODER_AI_GATEWAY_CIRCUIT_BREAKER_INTERVAL":          {},
	"CODER_AI_GATEWAY_CIRCUIT_BREAKER_MAX_REQUESTS":      {},
	"CODER_AI_GATEWAY_CIRCUIT_BREAKER_TIMEOUT":           {},
	"CODER_AI_GATEWAY_DUMP_DIR":                          {},
	"CODER_AI_GATEWAY_MAX_CONCURRENCY":                   {},
	"CODER_AI_GATEWAY_RATE_LIMIT":                        {},
	"CODER_AI_GATEWAY_SEND_ACTOR_HEADERS":                {},

	// Prometheus
	"CODER_PROMETHEUS_ADDRESS": {},
	"CODER_PROMETHEUS_ENABLE":  {},
}

// aiGatewayStart runs the AI Gateway as a standalone process.
func (r *RootCmd) aiGatewayStart() *serpent.Command {
	var (
		key         string
		keyFile     string
		httpAddress string
		tlsCertFile string
		tlsKeyFile  string
	)

	vals := new(codersdk.DeploymentValues)

	cmd := &serpent.Command{
		Use:   "start",
		Short: "Run a standalone AI Gateway server",
		Long: "Runs a standalone replica of the AI Gateway. Standalone replicas " +
			"serve LLM client traffic on a dedicated HTTP listener and connect " +
			"to coderd using the Coder deployment URL and an AI Gateway key.\n\n" +
			"Set --url or CODER_URL to the Coder deployment address, and set " +
			"--key (CODER_AI_GATEWAY_KEY) or --key-file " +
			"(CODER_AI_GATEWAY_KEY_FILE). A user login or session token is " +
			"not required.",
		Handler: func(inv *serpent.Invocation) error {
			signalCtx, stop := inv.SignalNotifyContext(inv.Context(), agpl.StopSignals...)
			defer stop()

			resolvedKey, err := resolveAIGatewayKey(key, keyFile)
			if err != nil {
				return err
			}

			// TLS is opt-in and requires both files; setting only one is
			// an error. Default is plain HTTP.
			if (tlsCertFile == "") != (tlsKeyFile == "") {
				return xerrors.New("--tls-cert-file and --tls-key-file options must be provided together")
			}

			serverURL, transport, err := r.ResolveClientConnection()
			if err != nil {
				if errors.Is(err, agpl.ErrClientURLNotConfigured) {
					return xerrors.New("AI Gateway requires --url or CODER_URL to point at the Coder deployment")
				}
				return xerrors.Errorf("configure Coder deployment connection: %w", err)
			}

			logger, closeLogger, err := clilog.New(clilog.FromDeploymentValues(vals)).Build(inv)
			if err != nil {
				return xerrors.Errorf("make logger: %w", err)
			}
			defer closeLogger()

			logger.Debug(signalCtx, "started debug logging")
			logger.Sync()

			registry := prometheus.NewRegistry()
			registry.MustRegister(collectors.NewGoCollector())
			registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

			metrics := aibridge.NewMetrics(registry)
			providerMetrics := aibridged.NewMetrics(registry)

			tracerProvider, _, closeTracing := agpl.ConfigureTraceProviderWithService(signalCtx, logger, vals, "coder-ai-gateway")
			// The tracer is shared by the gateway's HTTP middleware, pool, and
			// daemon, so it must be flushed only after runStandaloneGateway returns
			// and all span producers have stopped, hence the handler-level defer.
			defer func() {
				logger.Debug(signalCtx, "closing tracing")
				traceCloseErr := shutdownWithTimeout(closeTracing, traceShutdownTimeout)
				logger.Debug(signalCtx, "tracing closed", slog.Error(traceCloseErr))
			}()
			tracer := tracerProvider.Tracer("ai-gateway")

			if vals.Prometheus.Enable.Value() {
				logger.Info(signalCtx, "starting Prometheus endpoint", slog.F("address", vals.Prometheus.Address.String()))
				closeFunc := agpl.ServeHandler(signalCtx, logger, promhttp.InstrumentMetricHandler(
					registry, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
				), vals.Prometheus.Address.String(), "prometheus")
				defer closeFunc()
			}

			gatewayLogger := logger.Named("ai-gateway")
			// Standalone Gateway starts with an empty pool. Providers are
			// fetched later via GetAIProviders DRPC and pool is updated.
			pool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, nil, gatewayLogger.Named("pool"), metrics, tracer)
			if err != nil {
				return xerrors.Errorf("create request pool: %w", err)
			}
			registry.MustRegister(keypool.NewStateCollector(pool.KeyPools))

			return runStandaloneGateway(signalCtx, standaloneGatewayParams{
				bridgeConfig: vals.AI.BridgeConfig,
				coderURL:     serverURL.String(),
				httpAddress:  httpAddress,
				tlsCertFile:  tlsCertFile,
				tlsKeyFile:   tlsKeyFile,

				dialer: aibridged.NewWebsocketDialer(serverURL, transport, resolvedKey),
				pool:   pool,

				logger:          gatewayLogger,
				metrics:         metrics,
				providerMetrics: providerMetrics,
				tracer:          tracer,
			})
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "key",
			Env:         "CODER_AI_GATEWAY_KEY",
			Description: "The AI Gateway key used to authenticate to coderd.",
			Value:       serpent.StringOf(&key),
		},
		{
			Flag:        "key-file",
			Env:         "CODER_AI_GATEWAY_KEY_FILE",
			Description: "Path to a file containing the AI Gateway key used to authenticate to coderd.",
			Value:       serpent.StringOf(&keyFile),
		},
		{
			Flag:        "http-address",
			Env:         "CODER_AI_GATEWAY_HTTP_ADDRESS",
			Description: "The bind address to serve incoming AI Gateway client traffic.",
			Default:     "127.0.0.1:4001",
			Value:       serpent.StringOf(&httpAddress),
		},
		{
			Flag:        "tls-cert-file",
			Env:         "CODER_AI_GATEWAY_TLS_CERT_FILE",
			Description: "Path to a PEM-encoded TLS certificate. Enables TLS termination when set together with --tls-key-file.",
			Value:       serpent.StringOf(&tlsCertFile),
		},
		{
			Flag:        "tls-key-file",
			Env:         "CODER_AI_GATEWAY_TLS_KEY_FILE",
			Description: "Path to a PEM-encoded TLS private key. Enables TLS termination when set together with --tls-cert-file.",
			Value:       serpent.StringOf(&tlsKeyFile),
		},
	}

	for _, opt := range vals.Options() {
		if _, ok := aiGatewayInheritedEnvs[opt.Env]; ok {
			cmd.Options = append(cmd.Options, opt)
		}
	}

	return cmd
}

type standaloneGatewayParams struct {
	// Configuration.
	bridgeConfig codersdk.AIBridgeConfig
	coderURL     string
	httpAddress  string
	tlsCertFile  string
	tlsKeyFile   string

	// Runtime dependencies.
	dialer aibridged.Dialer
	pool   aibridged.Pooler

	// Observability.
	// logger is the gateway-scoped logger; derived loggers (daemon,
	// providers) are named under it.
	logger          slog.Logger
	metrics         *aibridge.Metrics
	providerMetrics *aibridged.Metrics
	tracer          trace.Tracer
}

type standaloneGateway struct {
	// Services.
	daemon     *aibridged.Server
	httpServer *http.Server
	reloader   aibridged.ProviderReloader

	// Configuration.
	coderURL    string
	httpAddress string
	tlsCertFile string
	tlsKeyFile  string

	// State.
	// providersLoaded is an initial-load latch. Reconnects refresh providers
	// through the watch loop without resetting readiness.
	providersLoaded atomic.Bool

	// Observability.
	logger         slog.Logger
	providerLogger slog.Logger
}

// runStandaloneGateway starts the aibridged daemon and serves the standalone
// AI Gateway. The daemon dials coderd asynchronously, so HTTP serving does not
// wait for the DRPC connection. It manages the daemon life cycle.
func runStandaloneGateway(ctx context.Context, params standaloneGatewayParams) error {
	// The aibridged daemon must outlive ctx so in-flight HTTP requests
	// retain their DRPC connection during graceful HTTP shutdown.
	daemon, err := aibridged.New(context.Background(), params.pool, params.dialer, params.logger.Named("aibridged"), params.tracer)
	if err != nil {
		return xerrors.Errorf("start AI Gateway daemon: %w", err)
	}

	providerLogger := params.logger.Named("providers")
	gateway := &standaloneGateway{
		daemon:   daemon,
		reloader: agpl.NewPoolRPCReloader(params.pool, daemon.ClientContext, params.bridgeConfig, providerLogger, params.metrics, params.providerMetrics),

		coderURL:    params.coderURL,
		httpAddress: params.httpAddress,
		tlsCertFile: params.tlsCertFile,
		tlsKeyFile:  params.tlsKeyFile,

		logger:         params.logger,
		providerLogger: providerLogger,
	}
	gateway.httpServer = &http.Server{
		Handler:           newGatewayMux(gateway.daemon, gateway.ready, gatewayMiddleware(params.bridgeConfig, params.tracer)),
		ReadHeaderTimeout: time.Minute,
	}

	serveErr := gateway.serve(ctx)
	var daemonShutdownErr error
	if err := shutdownWithTimeout(daemon.Shutdown, daemonShutdownTimeout); err != nil {
		daemonShutdownErr = xerrors.Errorf("shutdown AI Gateway daemon: %w", err)
	}
	return errors.Join(serveErr, daemonShutdownErr)
}

func (s *standaloneGateway) serve(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.httpAddress)
	if err != nil {
		return xerrors.Errorf("listen on %q: %w", s.httpAddress, err)
	}

	serveErr := make(chan error, 1)
	var serveWG sync.WaitGroup
	serveWG.Go(func() {
		defer listener.Close()
		if s.tlsCertFile != "" {
			serveErr <- s.httpServer.ServeTLS(listener, s.tlsCertFile, s.tlsKeyFile)
			return
		}
		serveErr <- s.httpServer.Serve(listener)
	})

	s.logger.Info(ctx, "standalone AI Gateway listening",
		slog.F("address", listener.Addr().String()),
		slog.F("coder_url", s.coderURL),
		slog.F("tls", s.tlsCertFile != ""),
	)

	provReloadCtx, provReloadCancel := context.WithCancel(ctx)
	provReloadDone := make(chan struct{})
	go func() {
		defer close(provReloadDone)
		if err := s.loadProviders(provReloadCtx); err != nil {
			if provReloadCtx.Err() == nil {
				s.providerLogger.Error(provReloadCtx, "initial ai provider load stopped", slog.Error(err))
			}
			return
		}
		// WatchProviderReload reconnects internally and normally returns only when canceled.
		err := aibridged.WatchProviderReload(provReloadCtx, s.daemon.ClientContext, s.reloader, s.providerLogger)
		if err != nil && provReloadCtx.Err() == nil {
			s.providerLogger.Error(provReloadCtx, "ai provider reload watch stopped", slog.Error(err))
		}
	}()

	var runErr error
	select {
	case <-ctx.Done():
	case <-s.daemon.Done():
		// daemon uses context.Background() so no race with ctx.Done() is possible, ctx.Err() check is not needed.
		runErr = xerrors.Errorf("AI Gateway daemon exited: %w", s.daemon.Err())
	case <-provReloadDone:
		if ctx.Err() == nil {
			select {
			// reload can exit due to daemon failure
			// covering race with previous daemon.Done() case.
			case <-s.daemon.Done():
				runErr = xerrors.Errorf("AI Gateway daemon exited: %w", s.daemon.Err())
			default:
				runErr = xerrors.New("provider reload stopped unexpectedly")
			}
		}
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			runErr = xerrors.Errorf("serve: %w", err)
		}
	}
	s.logger.Info(ctx, "shutting down standalone AI Gateway")

	provReloadCancel()
	provReloadShutdownCtx, provReloadShutdownCancel := context.WithTimeout(context.Background(), providerReloadShutdownTimeout)
	defer provReloadShutdownCancel()

	var provReloadStopErr error
	select {
	case <-provReloadDone:
	case <-provReloadShutdownCtx.Done():
		provReloadStopErr = xerrors.Errorf("provider reload did not stop within %s, continuing gateway shutdown", providerReloadShutdownTimeout)
	}

	// Provider reload normally stops before HTTP draining so it cannot clear the
	// bridge cache while requests are draining. If it does not stop within its
	// timeout, continue with best-effort graceful HTTP shutdown.
	// The daemon remains connected so in-flight requests retain their DRPC connection.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer shutdownCancel()
	var httpShutdownErr error
	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		httpShutdownErr = xerrors.Errorf("shutdown http server: %w", err)
		if closeErr := s.httpServer.Close(); closeErr != nil {
			httpShutdownErr = errors.Join(httpShutdownErr, xerrors.Errorf("force close http server: %w", closeErr))
		}
	}
	serveWG.Wait()
	return errors.Join(runErr, provReloadStopErr, httpShutdownErr)
}

// loadProviders retries the initial provider load until it succeeds or the
// context or daemon stops. A successful empty provider list completes the
// initial load. Subsequent changes are handled by the watch loop.
func (s *standaloneGateway) loadProviders(ctx context.Context) error {
	for r := retry.New(50*time.Millisecond, 10*time.Second); r.Wait(ctx); {
		if err := s.reloader.Reload(ctx); err != nil {
			select {
			case <-s.daemon.Done():
				return err
			default:
			}
			s.providerLogger.Warn(ctx, "failed to load ai providers, will retry", slog.Error(err))
			continue
		}
		s.providersLoaded.Store(true)
		s.providerLogger.Info(ctx, "loaded ai providers from coderd")
		return nil
	}
	return context.Cause(ctx)
}

func (s *standaloneGateway) ready() bool {
	return s.daemon.Ready() && s.providersLoaded.Load()
}

func gatewayMiddleware(cfg codersdk.AIBridgeConfig, tracer trace.Tracer) func(http.Handler) http.Handler {
	mw := coderd.AIGatewayDataPlaneMiddleware(cfg)
	// Tracing wraps outermost so rejected requests are still traced.
	traced := tracingMiddleware(tracer)
	return func(next http.Handler) http.Handler {
		return traced(mw(next))
	}
}

// tracingMiddleware traces every request to the wrapped handler, unlike
// tracing.Middleware which only spans coderd's route patterns.
func tracingMiddleware(tracer trace.Tracer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			sw := &coderdtracing.StatusWriter{ResponseWriter: rw}
			r, span := coderdtracing.StartHTTPSpan(tracer, sw, r, fmt.Sprintf("%s %s", r.Method, r.URL.Path))
			defer span.End()

			next.ServeHTTP(sw, r)

			status := sw.Status
			if status == 0 {
				status = http.StatusOK
			}
			span.SetAttributes(
				semconv.HTTPMethodKey.String(r.Method),
				semconv.HTTPTargetKey.String(r.URL.RequestURI()),
				semconv.HTTPStatusCodeKey.Int(status),
			)
			span.SetStatus(httpconv.ServerStatus(status))
		})
	}
}

func newGatewayMux(aibridgedHandler http.Handler, aibridgedReady func() bool, middleware func(http.Handler) http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/api/v2/aibridge/", middleware(http.StripPrefix("/api/v2/aibridge", aibridgedHandler)))
	mux.Handle("/api/v2/ai-gateway/", middleware(http.StripPrefix("/api/v2/ai-gateway", aibridgedHandler)))
	mux.Handle("/", middleware(aibridgedHandler))

	// Health probes are registered without middleware.
	mux.HandleFunc(healthzPath, func(w http.ResponseWriter, _ *http.Request) {
		// healthz: returns 200 once the HTTP server is listening.
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc(readyzPath, func(w http.ResponseWriter, _ *http.Request) {
		// readyz: returns 200 after the initial provider load while the
		// DRPC connection to coderd remains active.
		if aibridgedReady() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	return mux
}

// resolveAIGatewayKey resolves key from --key or --key-file flags.
// If both are set, an error is returned. If neither is set, an empty string is returned.
func resolveAIGatewayKey(key string, keyFile string) (string, error) {
	if key != "" && keyFile != "" {
		return "", xerrors.New(keyFlagsExclusiveErr)
	}
	if key == "" && keyFile == "" {
		return "", xerrors.New(keyFlagsMissingErr)
	}
	if keyFile == "" {
		return key, nil
	}
	data, err := os.ReadFile(keyFile)
	if err != nil {
		return "", xerrors.Errorf("read AI Gateway key file %q: %w", keyFile, err)
	}
	return strings.TrimSpace(string(data)), nil
}

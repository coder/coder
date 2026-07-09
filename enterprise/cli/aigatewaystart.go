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
	shutdownTimeout = 5 * time.Minute

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
			logger = logger.Named("ai-gateway")

			logger.Debug(signalCtx, "started debug logging")
			logger.Sync()

			registry := prometheus.NewRegistry()
			registry.MustRegister(collectors.NewGoCollector())
			registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

			metrics := aibridge.NewMetrics(registry)
			providerMetrics := aibridged.NewMetrics(registry)

			tracerProvider, _, closeTracing := agpl.ConfigureTraceProviderWithService(signalCtx, logger, vals, "coder-ai-gateway")
			defer func() {
				logger.Debug(signalCtx, "closing tracing")
				traceCloseErr := shutdownWithTimeout(closeTracing, 5*time.Second)
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

			dialer := aibridged.NewWebsocketDialer(serverURL, transport, resolvedKey)
			aibridgedCtx, aibridgedCancel := context.WithCancel(context.Background())
			defer aibridgedCancel()
			srv, err := aibridged.New(aibridgedCtx, pool, dialer, gatewayLogger, tracer)
			if err != nil {
				return xerrors.Errorf("start AI Gateway daemon: %w", err)
			}
			defer srv.Close()

			// Fetch the initial provider set from coderd, retrying until
			// success. Subsequent changes are delivered by the watch loop
			// started below. The reloader's client acquisition honors the
			// context of each Reload call, so loadProviders is bounded by
			// signalCtx and the watch loop by watchCtx.
			providerLogger := gatewayLogger.Named("providers")
			reloader := agpl.NewPoolRPCReloader(pool, srv.ClientContext, vals.AI.BridgeConfig, providerLogger, metrics, providerMetrics)
			if err := loadProviders(signalCtx, reloader, providerLogger, srv.Done()); err != nil {
				if signalCtx.Err() != nil {
					logger.Info(signalCtx, "shutting down standalone AI Gateway")
					return nil
				}
				return xerrors.Errorf("initialize ai providers: %w", err)
			}

			mw := gatewayMiddleware(vals.AI.BridgeConfig, tracer)

			// Watch coderd for provider changes and refresh the pool on each
			// signal.
			watchCtx, watchCancel := context.WithCancel(signalCtx)
			var watchWG sync.WaitGroup
			watchWG.Go(func() {
				// srv.ClientContext observes watchCtx, so watchCancel below
				// unblocks a pending client acquisition and drains this
				// goroutine without relying on srv.Close.
				if err := aibridged.WatchProviderReload(watchCtx, srv.ClientContext, reloader, providerLogger); err != nil && watchCtx.Err() == nil {
					providerLogger.Warn(watchCtx, "ai provider watch loop exited", slog.Error(err))
				}
			})
			defer func() {
				watchCancel()
				watchWG.Wait()
			}()

			mux := newGatewayMux(srv, srv.Ready, mw)

			listener, err := net.Listen("tcp", httpAddress)
			if err != nil {
				return xerrors.Errorf("listen on %q: %w", httpAddress, err)
			}
			defer listener.Close()

			logger.Info(signalCtx, "standalone AI Gateway listening",
				slog.F("address", listener.Addr().String()),
				slog.F("coder_url", serverURL.String()),
				slog.F("tls", tlsCertFile != ""),
			)

			httpServer := &http.Server{
				Handler:           mux,
				ReadHeaderTimeout: time.Minute,
			}

			serveErr := make(chan error, 1)
			go func() {
				if tlsCertFile != "" {
					serveErr <- httpServer.ServeTLS(listener, tlsCertFile, tlsKeyFile)
				} else {
					serveErr <- httpServer.Serve(listener)
				}
			}()

			var aibridgedErr error
			select {
			case <-signalCtx.Done():
				logger.Info(signalCtx, "shutting down standalone AI Gateway")
			case <-srv.Done():
				aibridgedErr = srv.Err()
			case err := <-serveErr:
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					return xerrors.Errorf("serve: %w", err)
				}
			}

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer shutdownCancel()
			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				return xerrors.Errorf("shutdown http server: %w", err)
			}
			if aibridgedErr != nil {
				return xerrors.Errorf("AI Gateway daemon exited: %w", aibridgedErr)
			}
			return nil
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

// gatewayMiddleware composes the standalone gateway's per-request middleware.
// Tracing is outermost so request is traced even when the other guards short-circuit.
func gatewayMiddleware(cfg codersdk.AIBridgeConfig, tracer trace.Tracer) func(http.Handler) http.Handler {
	mw := coderd.AIGatewayDataPlaneMiddleware(cfg)
	traced := tracingMiddleware(tracer)
	return func(next http.Handler) http.Handler {
		return traced(mw(next))
	}
}

// newGatewayMux builds the standalone gateway's HTTP routes.
// The middleware is applied only to the LLM data-plane routes.
func newGatewayMux(aibridgedHandler http.Handler, aibridgedReady func() bool, middleware func(http.Handler) http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/api/v2/aibridge/", middleware(http.StripPrefix("/api/v2/aibridge", aibridgedHandler)))
	mux.Handle("/api/v2/ai-gateway/", middleware(http.StripPrefix("/api/v2/ai-gateway", aibridgedHandler)))
	mux.Handle("/", middleware(aibridgedHandler))

	// healthz: returns 200 once the HTTP server is listening.
	mux.HandleFunc(healthzPath, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// readyz: returns 200 only when the DRPC connection to coderd is established.
	mux.HandleFunc(readyzPath, func(w http.ResponseWriter, _ *http.Request) {
		if aibridgedReady() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	return mux
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

// loadProviders performs the standalone gateway's initial provider
// load by driving reloader until it succeeds or ctx is canceled. The reloader
// owns the actual fetch/build/replace/metrics work; the reloader's underlying
// client blocks until the daemon connects to coderd, and the fetch may still
// fail transiently (e.g. mid-seed contention or a dropped connection), so the
// reload is retried with backoff. A successful empty provider list is a valid
// result and ends the loop.
//
// Subsequent provider changes are delivered by WatchProviderReload, started
// after this initial load returns.
func loadProviders(ctx context.Context, reloader aibridged.ProviderReloader, logger slog.Logger, aibridgedDone <-chan struct{}) error {
	for r := retry.New(50*time.Millisecond, 10*time.Second); r.Wait(ctx); {
		if err := reloader.Reload(ctx); err != nil {
			select {
			case <-aibridgedDone:
				return err
			default:
			}
			logger.Warn(ctx, "failed to load ai providers, will retry", slog.Error(err))
			continue
		}
		logger.Info(ctx, "loaded ai providers from coderd")
		return nil
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return ctx.Err()
}

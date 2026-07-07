//go:build !slim

package cli

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/aibridge"
	agpl "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd"
	"github.com/coder/retry"
	"github.com/coder/serpent"
)

const (
	shutdownTimeout = 5 * time.Minute

	keyFlagsExclusiveErr = "--key and --key-file options are mutually exclusive"
	keyFlagsMissingErr   = "an AI Gateway key is required, set --key (CODER_AI_GATEWAY_KEY) or --key-file (CODER_AI_GATEWAY_KEY_FILE)"
)

// aiGatewayStart runs the AI Gateway as a standalone process.
func (r *RootCmd) aiGatewayStart() *serpent.Command {
	var (
		key         string
		keyFile     string
		httpAddress string
		tlsCertFile string
		tlsKeyFile  string
		verbose     bool
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

			logger := slog.Make(sloghuman.Sink(inv.Stderr))
			if verbose {
				logger = logger.Leveled(slog.LevelDebug)
			}

			// Metrics and tracing are not exposed by standalone mode yet
			// (TODO AIGOV-317), but the pool and the reloader require a metrics
			// object and a tracer.
			registry := prometheus.NewRegistry()
			metrics := aibridge.NewMetrics(registry)
			providerMetrics := aibridged.NewMetrics(registry)
			tracer := tracenoop.NewTracerProvider().Tracer("aibridged")

			// Standalone Gateway starts with an empty pool. Providers are
			// fetched later via GetAIProviders DRPC and pool is updated.
			pool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, nil, logger.Named("pool"), metrics, tracer)
			if err != nil {
				return xerrors.Errorf("create request pool: %w", err)
			}

			dialer := aibridged.NewWebsocketDialer(serverURL, transport, resolvedKey)
			aibridgedCtx, aibridgedCancel := context.WithCancel(context.Background())
			defer aibridgedCancel()
			srv, err := aibridged.New(aibridgedCtx, pool, dialer, logger.Named("aibridged"), tracer)
			if err != nil {
				return xerrors.Errorf("start AI Gateway daemon: %w", err)
			}
			defer srv.Close()

			// Fetch the initial provider set from coderd, retrying until
			// success.
			// TODO(AIGOV-465): the standalone gateway has no refresh trigger
			// yet, so this runs once on startup.
			clientFn := func() (aibridged.DRPCClient, error) {
				return srv.ClientContext(signalCtx)
			}
			providerLogger := logger.Named("aibridge.providers")
			reloader := agpl.NewPoolRPCReloader(pool, clientFn, vals.AI.BridgeConfig, providerLogger, metrics, providerMetrics)
			if err := loadProviders(signalCtx, reloader, providerLogger, srv.Done()); err != nil {
				if signalCtx.Err() != nil {
					logger.Info(signalCtx, "shutting down standalone AI Gateway")
					return nil
				}
				return xerrors.Errorf("initialize ai providers: %w", err)
			}

			mw := coderd.AIGatewayDataPlaneMiddleware(vals.AI.BridgeConfig)

			// The standalone listener is dedicated to Gateway traffic, so
			// the daemon is served at the root. The /api/v2/ai-gateway
			// and /api/v2/aibridge/ aliases are added for compatibility
			// with the embedded route.
			mux := http.NewServeMux()
			mux.Handle("/api/v2/aibridge/", mw(http.StripPrefix("/api/v2/aibridge", srv)))
			mux.Handle("/api/v2/ai-gateway/", mw(http.StripPrefix("/api/v2/ai-gateway", srv)))
			mux.Handle("/", mw(srv))

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
		{
			Flag:        "verbose",
			Env:         "CODER_AI_GATEWAY_VERBOSE",
			Description: "Output debug-level logs.",
			Value:       serpent.BoolOf(&verbose),
			Default:     "false",
		},
	}

	// Standalone Gateway only uses part of the options from "AI Gateway" group.
	// Other options from the group are coderd-only (eg. budget, provider-seeding).
	standaloneOpts := map[string]struct{}{
		"CODER_AI_GATEWAY_ALLOW_BYOK":                        {},
		"CODER_AI_GATEWAY_SEND_ACTOR_HEADERS":                {},
		"CODER_AI_GATEWAY_DUMP_DIR":                          {},
		"CODER_AI_GATEWAY_CIRCUIT_BREAKER_ENABLED":           {},
		"CODER_AI_GATEWAY_CIRCUIT_BREAKER_FAILURE_THRESHOLD": {},
		"CODER_AI_GATEWAY_CIRCUIT_BREAKER_INTERVAL":          {},
		"CODER_AI_GATEWAY_CIRCUIT_BREAKER_TIMEOUT":           {},
		"CODER_AI_GATEWAY_CIRCUIT_BREAKER_MAX_REQUESTS":      {},
		"CODER_AI_GATEWAY_MAX_CONCURRENCY":                   {},
		"CODER_AI_GATEWAY_RATE_LIMIT":                        {},
	}

	var aiGatewayOpts serpent.OptionSet
	for _, opt := range vals.Options() {
		if opt.Group == nil || opt.Group.Name != "AI Gateway" {
			continue
		}
		if _, ok := standaloneOpts[opt.Env]; !ok {
			continue
		}
		aiGatewayOpts = append(aiGatewayOpts, opt)
	}

	cmd.Options = append(cmd.Options, aiGatewayOpts...)

	return cmd
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
// TODO(AIGOV-465): the standalone gateway has no provider-change refresh
// trigger yet, so this runs once on startup; provider add/enable will not
// propagate to a running standalone gateway.
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

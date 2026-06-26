//go:build !slim

package cli

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/aibridge"
	agpl "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/retry"
	"github.com/coder/serpent"
)

// aiGatewayStart runs the AI Gateway as a standalone process.
// It connects to coderd over DRPC via /api/v2/ai-gateway/serve for
// authentication, recording, and MCP configuration, and listens on its
// own HTTP address for incoming LLM client traffic. Providers are built
// from the deployment configuration; the standalone process does not read
// the database directly.
func (r *RootCmd) aiGatewayStart() *serpent.Command {
	var (
		key         string
		httpAddress string
		tlsCertFile string
		tlsKeyFile  string
		verbose     bool
	)

	// Reuse the shared AI Gateway deployment options (CODER_AI_GATEWAY_*)
	// so standalone mode is configured exactly like embedded mode. The
	// option Values point into vals, which is captured by the handler.
	vals := new(codersdk.DeploymentValues)
	var aiGatewayOpts serpent.OptionSet
	for _, opt := range vals.Options() {
		if opt.Group != nil && opt.Group.Name == "AI Gateway" {
			aiGatewayOpts = append(aiGatewayOpts, opt)
		}
	}

	cmd := &serpent.Command{
		Use:   "start",
		Short: "Run a standalone AI Gateway server",
		Long: "The standalone AI Gateway connects to a Coder deployment over DRPC to " +
			"authenticate users, record interceptions, and configure MCP, while serving " +
			"LLM client traffic on its own HTTP listener.\n\n" +
			"The deployment address is taken from the global --url flag (CODER_URL) and " +
			"is required. The gateway authenticates with the key from --key " +
			"(CODER_AI_GATEWAY_KEY). Provider and other AI Gateway settings use the same " +
			"CODER_AI_GATEWAY_* options as embedded mode.",
		Handler: func(inv *serpent.Invocation) error {
			// Derive a single signal-aware context so a stop signal interrupts
			// every phase, including connecting to coderd and the initial
			// provider fetch, not just the serving select below. Using a
			// non-signal context for startup left Ctrl+C ignored until the
			// gateway had finished starting up.
			ctx, stop := inv.SignalNotifyContext(inv.Context(), agpl.StopSignals...)
			defer stop()

			if key == "" {
				return xerrors.New("an AI Gateway key is required; set --key or CODER_AI_GATEWAY_KEY")
			}
			// TLS is opt-in and requires both files; setting only one is
			// an error. Default is plain HTTP.
			if (tlsCertFile == "") != (tlsKeyFile == "") {
				return xerrors.New("--tls-cert-file and --tls-key-file must be provided together")
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			logger := slog.Make(sloghuman.Sink(inv.Stderr))
			if verbose {
				logger = logger.Leveled(slog.LevelDebug)
			}

			// Metrics and tracing are not yet exposed by standalone mode
			// (future work), but the pool and the reloader require a metrics
			// object and a tracer, so wire up no-op sinks registered against a
			// throwaway registry until standalone metrics are exported.
			metrics := aibridge.NewMetrics(prometheus.NewRegistry())
			providerMetrics := aibridged.NewMetrics(prometheus.NewRegistry())
			tracer := trace.NewNoopTracerProvider().Tracer("aibridged")

			// The standalone gateway has no provider env vars and no database
			// access. It starts with an empty pool, connects to coderd over
			// DRPC, then fetches the provider set via GetAIProviders and builds
			// the pool from it.
			pool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, nil, logger.Named("pool"), metrics, tracer)
			if err != nil {
				return xerrors.Errorf("create request pool: %w", err)
			}

			dialer := aibridged.NewWebsocketDialer(client, key)
			srv, err := aibridged.New(ctx, pool, dialer, logger.Named("aibridged"), tracer)
			if err != nil {
				return xerrors.Errorf("start aibridge daemon: %w", err)
			}
			defer srv.Close()

			// Fetch the initial provider set from coderd, retrying until
			// success. The reloader owns the fetch/build/replace/metrics work;
			// the standalone gateway just drives it once at startup.
			// TODO(AIGOV-465): the standalone gateway has no refresh trigger
			// yet, so this runs once on startup.
			providerLogger := logger.Named("aibridge.providers")
			reloader := agpl.NewPoolRPCReloader(pool, srv.Client, vals.AI.BridgeConfig, providerLogger, metrics, providerMetrics)
			if err := loadProviders(ctx, reloader, providerLogger); err != nil {
				// A stop signal during startup cancels ctx (and the daemon's
				// lifecycle). Treat that as a graceful shutdown rather than a
				// failure, so interrupting before the gateway is serving still
				// exits cleanly.
				if ctx.Err() != nil {
					logger.Info(ctx, "shutting down standalone AI Gateway")
					return nil
				}
				return xerrors.Errorf("initialize ai providers: %w", err)
			}

			// The standalone listener is dedicated to Gateway traffic, so
			// the daemon is served at the root. The /api/v2/ai-gateway alias
			// keeps parity with the embedded route, so a Gateway proxy
			// pointed here with the embedded path still works.
			mux := http.NewServeMux()
			mux.Handle("/api/v2/aibridge/", http.StripPrefix("/api/v2/aibridge", srv))
			mux.Handle("/api/v2/ai-gateway/", http.StripPrefix("/api/v2/ai-gateway", srv))
			mux.Handle("/", srv)

			listener, err := net.Listen("tcp", httpAddress)
			if err != nil {
				return xerrors.Errorf("listen on %q: %w", httpAddress, err)
			}
			defer listener.Close()

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

			logger.Info(ctx, "standalone AI Gateway listening",
				slog.F("address", listener.Addr().String()),
				slog.F("tls", tlsCertFile != ""),
			)

			select {
			case <-ctx.Done():
				logger.Info(ctx, "shutting down standalone AI Gateway")
			case err := <-serveErr:
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					return xerrors.Errorf("serve: %w", err)
				}
			}

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()
			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				return xerrors.Errorf("shutdown http server: %w", err)
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
	cmd.Options = append(cmd.Options, aiGatewayOpts...)

	return cmd
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
func loadProviders(ctx context.Context, reloader aibridged.ProviderReloader, logger slog.Logger) error {
	for r := retry.New(50*time.Millisecond, 10*time.Second); r.Wait(ctx); {
		if err := reloader.Reload(ctx); err != nil {
			logger.Warn(ctx, "failed to load ai providers, will retry", slog.Error(err))
			continue
		}
		logger.Info(ctx, "loaded ai providers from coderd")
		return nil
	}
	return ctx.Err()
}

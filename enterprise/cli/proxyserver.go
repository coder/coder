//go:build !slim

package cli

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"os/signal"
	"regexp"
	rpprof "runtime/pprof"
	"time"

	"github.com/coreos/go-systemd/daemon"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/wsproxy"
)

type closers []func()

func (c closers) Close() {
	for _, closeF := range c {
		closeF()
	}
}

func (c *closers) Add(f func()) {
	*c = append(*c, f)
}

func (*RootCmd) proxyServer() *clibase.Cmd {
	var (
		cfg = new(codersdk.DeploymentValues)
		// Filter options for only relevant ones.
		opts = cfg.Options().Filter(codersdk.IsWorkspaceProxies)

		externalProxyOptionGroup = clibase.Group{
			Name: "External Workspace Proxy",
			YAML: "externalWorkspaceProxy",
		}
		proxySessionToken clibase.String
		primaryAccessURL  clibase.URL
		derpOnly          clibase.Bool
	)
	opts.Add(
		// Options only for external workspace proxies

		clibase.Option{
			Name:        "Proxy Session Token",
			Description: "Authentication token for the workspace proxy to communicate with coderd.",
			Flag:        "proxy-session-token",
			Env:         "CODER_PROXY_SESSION_TOKEN",
			YAML:        "proxySessionToken",
			Required:    true,
			Value:       &proxySessionToken,
			Group:       &externalProxyOptionGroup,
			Hidden:      false,
		},

		clibase.Option{
			Name:        "Coderd (Primary) Access URL",
			Description: "URL to communicate with coderd. This should match the access URL of the Coder deployment.",
			Flag:        "primary-access-url",
			Env:         "CODER_PRIMARY_ACCESS_URL",
			YAML:        "primaryAccessURL",
			Required:    true,
			Value: clibase.Validate(&primaryAccessURL, func(value *clibase.URL) error {
				if !(value.Scheme == "http" || value.Scheme == "https") {
					return xerrors.Errorf("'--primary-access-url' value must be http or https: url=%s", primaryAccessURL.String())
				}
				return nil
			}),
			Group:  &externalProxyOptionGroup,
			Hidden: false,
		},
		clibase.Option{
			Name:        "DERP-only proxy",
			Description: "Run a proxy server that only supports DERP connections and does not proxy workspace app/terminal traffic.",
			Flag:        "derp-only",
			Env:         "CODER_PROXY_DERP_ONLY",
			YAML:        "derpOnly",
			Required:    false,
			Value:       &derpOnly,
			Group:       &externalProxyOptionGroup,
			Hidden:      false,
		},
	)

	cmd := &clibase.Cmd{
		Use:     "server",
		Short:   "Start a workspace proxy server",
		Options: opts,
		Middleware: clibase.Chain(
			cli.WriteConfigMW(cfg),
			cli.PrintDeprecatedOptions(),
			clibase.RequireNArgs(0),
		),
		Handler: func(inv *clibase.Invocation) error {
			var closers closers
			// Main command context for managing cancellation of running
			// services.
			ctx, topCancel := context.WithCancel(inv.Context())
			defer topCancel()
			closers.Add(topCancel)

			go cli.DumpHandler(ctx)

			cli.PrintLogo(inv, "Coder Workspace Proxy")
			logger, logCloser, err := cli.BuildLogger(inv, cfg)
			if err != nil {
				return xerrors.Errorf("make logger: %w", err)
			}
			defer logCloser()
			closers.Add(logCloser)

			logger.Debug(ctx, "started debug logging")
			logger.Sync()

			// Register signals early on so that graceful shutdown can't
			// be interrupted by additional signals. Note that we avoid
			// shadowing cancel() (from above) here because notifyStop()
			// restores default behavior for the signals. This protects
			// the shutdown sequence from abruptly terminating things
			// like: database migrations, provisioner work, workspace
			// cleanup in dev-mode, etc.
			//
			// To get out of a graceful shutdown, the user can send
			// SIGQUIT with ctrl+\ or SIGKILL with `kill -9`.
			notifyCtx, notifyStop := signal.NotifyContext(ctx, cli.InterruptSignals...)
			defer notifyStop()

			// Clean up idle connections at the end, e.g.
			// embedded-postgres can leave an idle connection
			// which is caught by goleaks.
			defer http.DefaultClient.CloseIdleConnections()
			closers.Add(http.DefaultClient.CloseIdleConnections)

			tracer, _, closeTracing := cli.ConfigureTraceProvider(ctx, logger, cfg)
			defer func() {
				logger.Debug(ctx, "closing tracing")
				traceCloseErr := shutdownWithTimeout(closeTracing, 5*time.Second)
				logger.Debug(ctx, "tracing closed", slog.Error(traceCloseErr))
			}()

			httpServers, err := cli.ConfigureHTTPServers(inv, cfg)
			if err != nil {
				return xerrors.Errorf("configure http(s): %w", err)
			}
			defer httpServers.Close()
			closers.Add(httpServers.Close)

			// If no access url given, use the local address.
			if cfg.AccessURL.String() == "" {
				// Prefer TLS
				if httpServers.TLSUrl != nil {
					cfg.AccessURL = clibase.URL(*httpServers.TLSUrl)
				} else if httpServers.HTTPUrl != nil {
					cfg.AccessURL = clibase.URL(*httpServers.HTTPUrl)
				}
			}

			if derpOnly.Value() && !cfg.DERP.Server.Enable.Value() {
				return xerrors.Errorf("cannot use --derp-only with DERP server disabled")
			}

			// TODO: @emyrk I find this strange that we add this to the context
			// at the root here.
			ctx, httpClient, err := cli.ConfigureHTTPClient(
				ctx,
				cfg.TLS.ClientCertFile.String(),
				cfg.TLS.ClientKeyFile.String(),
				cfg.TLS.ClientCAFile.String(),
			)
			if err != nil {
				return xerrors.Errorf("configure http client: %w", err)
			}
			defer httpClient.CloseIdleConnections()
			closers.Add(httpClient.CloseIdleConnections)

			// A newline is added before for visibility in terminal output.
			cliui.Infof(inv.Stdout, "\nView the Web UI: %s", cfg.AccessURL.String())

			var appHostnameRegex *regexp.Regexp
			appHostname := cfg.WildcardAccessURL.String()
			if appHostname != "" {
				appHostnameRegex, err = httpapi.CompileHostnamePattern(appHostname)
				if err != nil {
					return xerrors.Errorf("parse wildcard access URL %q: %w", appHostname, err)
				}
			}

			realIPConfig, err := httpmw.ParseRealIPConfig(cfg.ProxyTrustedHeaders, cfg.ProxyTrustedOrigins)
			if err != nil {
				return xerrors.Errorf("parse real ip config: %w", err)
			}

			if cfg.Pprof.Enable {
				// This prevents the pprof import from being accidentally deleted.
				// pprof has an init function that attaches itself to the default handler.
				// By passing a nil handler to 'serverHandler', it will automatically use
				// the default, which has pprof attached.
				_ = pprof.Handler
				//nolint:revive
				closeFunc := cli.ServeHandler(ctx, logger, nil, cfg.Pprof.Address.String(), "pprof")
				defer closeFunc()
				closers.Add(closeFunc)
			}

			prometheusRegistry := prometheus.NewRegistry()
			if cfg.Prometheus.Enable {
				prometheusRegistry.MustRegister(collectors.NewGoCollector())
				prometheusRegistry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

				//nolint:revive
				closeFunc := cli.ServeHandler(ctx, logger, promhttp.InstrumentMetricHandler(
					prometheusRegistry, promhttp.HandlerFor(prometheusRegistry, promhttp.HandlerOpts{}),
				), cfg.Prometheus.Address.String(), "prometheus")
				defer closeFunc()
				closers.Add(closeFunc)
			}

			proxy, err := wsproxy.New(ctx, &wsproxy.Options{
				Logger:                 logger,
				Experiments:            coderd.ReadExperiments(logger, cfg.Experiments.Value()),
				HTTPClient:             httpClient,
				DashboardURL:           primaryAccessURL.Value(),
				AccessURL:              cfg.AccessURL.Value(),
				AppHostname:            appHostname,
				AppHostnameRegex:       appHostnameRegex,
				RealIPConfig:           realIPConfig,
				Tracing:                tracer,
				PrometheusRegistry:     prometheusRegistry,
				APIRateLimit:           int(cfg.RateLimit.API.Value()),
				SecureAuthCookie:       cfg.SecureAuthCookie.Value(),
				DisablePathApps:        cfg.DisablePathApps.Value(),
				ProxySessionToken:      proxySessionToken.Value(),
				AllowAllCors:           cfg.Dangerous.AllowAllCors.Value(),
				DERPEnabled:            cfg.DERP.Server.Enable.Value(),
				DERPOnly:               derpOnly.Value(),
				DERPServerRelayAddress: cfg.DERP.Server.RelayURL.String(),
			})
			if err != nil {
				return xerrors.Errorf("create workspace proxy: %w", err)
			}
			closers.Add(func() { _ = proxy.Close() })

			shutdownConnsCtx, shutdownConns := context.WithCancel(ctx)
			defer shutdownConns()
			closers.Add(shutdownConns)
			// ReadHeaderTimeout is purposefully not enabled. It caused some
			// issues with websockets over the dev tunnel.
			// See: https://github.com/coder/coder/pull/3730
			//nolint:gosec
			httpServer := &http.Server{
				// These errors are typically noise like "TLS: EOF". Vault does
				// similar:
				// https://github.com/hashicorp/vault/blob/e2490059d0711635e529a4efcbaa1b26998d6e1c/command/server.go#L2714
				ErrorLog: log.New(io.Discard, "", 0),
				Handler:  proxy.Handler,
				BaseContext: func(_ net.Listener) context.Context {
					return shutdownConnsCtx
				},
			}
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = httpServer.Shutdown(ctx)
			}()

			// TODO: So this obviously is not going to work well.
			errCh := make(chan error, 1)
			go rpprof.Do(ctx, rpprof.Labels("service", "workspace-proxy"), func(ctx context.Context) {
				errCh <- httpServers.Serve(httpServer)
			})

			cliui.Infof(inv.Stdout, "\n==> Logs will stream in below (press ctrl+c to gracefully exit):")

			// Updates the systemd status from activating to activated.
			_, err = daemon.SdNotify(false, daemon.SdNotifyReady)
			if err != nil {
				return xerrors.Errorf("notify systemd: %w", err)
			}

			// Currently there is no way to ask the server to shut
			// itself down, so any exit signal will result in a non-zero
			// exit of the server.
			var exitErr error
			select {
			case exitErr = <-errCh:
			case <-notifyCtx.Done():
				exitErr = notifyCtx.Err()
				_, _ = fmt.Fprintln(inv.Stdout, cliui.Bold(
					"Interrupt caught, gracefully exiting. Use ctrl+\\ to force quit",
				))
			}

			if exitErr != nil && !xerrors.Is(exitErr, context.Canceled) {
				cliui.Errorf(inv.Stderr, "Unexpected error, shutting down server: %s\n", exitErr)
			}

			// Begin clean shut down stage, we try to shut down services
			// gracefully in an order that gives the best experience.
			// This procedure should not differ greatly from the order
			// of `defer`s in this function, but allows us to inform
			// the user about what's going on and handle errors more
			// explicitly.

			_, err = daemon.SdNotify(false, daemon.SdNotifyStopping)
			if err != nil {
				cliui.Errorf(inv.Stderr, "Notify systemd failed: %s", err)
			}

			// Stop accepting new connections without interrupting
			// in-flight requests, give in-flight requests 5 seconds to
			// complete.
			cliui.Info(inv.Stdout, "Shutting down API server..."+"\n")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err = httpServer.Shutdown(shutdownCtx)
			if err != nil {
				cliui.Errorf(inv.Stderr, "API server shutdown took longer than 3s: %s\n", err)
			} else {
				cliui.Info(inv.Stdout, "Gracefully shut down API server\n")
			}
			// Cancel any remaining in-flight requests.
			shutdownConns()

			// Trigger context cancellation for any remaining services.
			closers.Close()

			switch {
			case xerrors.Is(exitErr, context.DeadlineExceeded):
				cliui.Warnf(inv.Stderr, "Graceful shutdown timed out")
				// Errors here cause a significant number of benign CI failures.
				return nil
			case xerrors.Is(exitErr, context.Canceled):
				return nil
			case exitErr != nil:
				return xerrors.Errorf("graceful shutdown: %w", exitErr)
			default:
				return nil
			}
		},
	}

	return cmd
}

func shutdownWithTimeout(shutdown func(context.Context) error, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return shutdown(ctx)
}

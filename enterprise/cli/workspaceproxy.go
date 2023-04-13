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
	"net/url"
	"os/signal"
	"regexp"
	rpprof "runtime/pprof"
	"time"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/httpmw"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/coreos/go-systemd/daemon"

	"github.com/coder/coder/cli/cliui"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli"
	"github.com/coder/coder/coderd/workspaceapps"
	"github.com/coder/coder/enterprise/wsproxy"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) workspaceProxy() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:     "workspace-proxy",
		Short:   "Manage workspace proxies",
		Aliases: []string{"proxy"},
		Hidden:  true,
		Handler: func(inv *clibase.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*clibase.Cmd{
			r.proxyServer(),
			r.registerProxy(),
		},
	}

	return cmd
}

func (r *RootCmd) registerProxy() *clibase.Cmd {
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:   "register",
		Short: "Register a workspace proxy",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(i *clibase.Invocation) error {
			ctx := i.Context()
			name := i.Args[0]
			// TODO: Fix all this
			resp, err := client.CreateWorkspaceProxy(ctx, codersdk.CreateWorkspaceProxyRequest{
				Name:             name,
				DisplayName:      name,
				Icon:             "whocares.png",
				URL:              "http://localhost:6005",
				WildcardHostname: "",
			})
			if err != nil {
				return xerrors.Errorf("create workspace proxy: %w", err)
			}

			fmt.Println(resp.ProxyToken)
			return nil
		},
	}
	return cmd
}

type closers []func()

func (c closers) Close() {
	for _, closeF := range c {
		closeF()
	}
}

func (c *closers) Add(f func()) {
	*c = append(*c, f)
}

func (r *RootCmd) proxyServer() *clibase.Cmd {
	var (
		// TODO: Remove options that we do not need
		cfg  = new(codersdk.DeploymentValues)
		opts = cfg.Options()
	)
	var _ = opts

	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:     "server",
		Short:   "Start a workspace proxy server",
		Options: opts,
		Middleware: clibase.Chain(
			cli.WriteConfigMW(cfg),
			cli.PrintDeprecatedOptions(),
			clibase.RequireNArgs(0),
			// We need a client to connect with the primary coderd instance.
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			var closers closers
			// Main command context for managing cancellation of running
			// services.
			ctx, topCancel := context.WithCancel(inv.Context())
			defer topCancel()
			closers.Add(topCancel)

			go cli.DumpHandler(ctx)

			cli.PrintLogo(inv)
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

			tracer, _ := cli.ConfigureTraceProvider(ctx, logger, inv, cfg)

			httpServers, err := cli.ConfigureHTTPServers(inv, cfg)
			if err != nil {
				return xerrors.Errorf("configure http(s): %w", err)
			}
			defer httpServers.Close()
			closers.Add(httpServers.Close)

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

			// Warn the user if the access URL appears to be a loopback address.
			isLocal, err := cli.IsLocalURL(ctx, cfg.AccessURL.Value())
			if isLocal || err != nil {
				reason := "could not be resolved"
				if isLocal {
					reason = "isn't externally reachable"
				}
				cliui.Warnf(
					inv.Stderr,
					"The access URL %s %s, this may cause unexpected problems when creating workspaces. Generate a unique *.try.coder.app URL by not specifying an access URL.\n",
					cliui.Styles.Field.Render(cfg.AccessURL.String()), reason,
				)
			}

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

			pu, _ := url.Parse("http://localhost:3000")
			proxy, err := wsproxy.New(&wsproxy.Options{
				Logger: logger,
				// TODO: PrimaryAccessURL
				PrimaryAccessURL: pu,
				AccessURL:        cfg.AccessURL.Value(),
				AppHostname:      appHostname,
				AppHostnameRegex: appHostnameRegex,
				RealIPConfig:     realIPConfig,
				// TODO: AppSecurityKey
				AppSecurityKey:     workspaceapps.SecurityKey{},
				Tracing:            tracer,
				PrometheusRegistry: prometheusRegistry,
				APIRateLimit:       int(cfg.RateLimit.API.Value()),
				SecureAuthCookie:   cfg.SecureAuthCookie.Value(),
				// TODO: DisablePathApps
				DisablePathApps: false,
				// TODO: ProxySessionToken
				ProxySessionToken: "",
			})
			if err != nil {
				return xerrors.Errorf("create workspace proxy: %w", err)
			}

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
				_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Bold.Render(
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

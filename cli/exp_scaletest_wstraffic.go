//go:build !slim

package cli

import (
	"fmt"
	"net/url"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/scaletest/workspacetraffic"
	"github.com/coder/serpent"
)

func (r *RootCmd) scaletestWorkspaceTraffic() *serpent.Command {
	var (
		tickInterval      time.Duration
		bytesPerTick      int64
		ssh               bool
		disableDirect     bool
		app               string
		workspaceProxyURL string

		targetFlags     = &workspaceTargetFlags{}
		tracingFlags    = &scaletestTracingFlags{}
		strategy        = &scaletestStrategyFlags{}
		cleanupStrategy = newScaletestCleanupStrategy()
		output          = &scaletestOutputFlags{}
		prometheusFlags = &scaletestPrometheusFlags{}
	)

	cmd := &serpent.Command{
		Use:   "workspace-traffic",
		Short: "Generate traffic to scaletest workspaces through coderd",
		Handler: func(inv *serpent.Invocation) (err error) {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			ctx := inv.Context()

			notifyCtx, stop := signal.NotifyContext(ctx, StopSignals...) // Checked later.
			defer stop()
			ctx = notifyCtx

			me, err := RequireAdmin(ctx, client)
			if err != nil {
				return err
			}

			reg := prometheus.NewRegistry()
			metrics := workspacetraffic.NewMetrics(reg, "username", "workspace_name", "agent_name")

			logger := inv.Logger
			prometheusSrvClose := ServeHandler(ctx, logger, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), prometheusFlags.Address, "prometheus")
			defer prometheusSrvClose()

			workspaces, err := targetFlags.getTargetedWorkspaces(ctx, client, me.OrganizationIDs, inv.Stdout)
			if err != nil {
				return err
			}

			appHost, err := client.AppHost(ctx)
			if err != nil {
				return xerrors.Errorf("get app host: %w", err)
			}

			tracerProvider, closeTracing, tracingEnabled, err := tracingFlags.provider(ctx)
			if err != nil {
				return xerrors.Errorf("create tracer provider: %w", err)
			}
			defer func() {
				// Allow time for traces to flush even if command context is
				// canceled. This is a no-op if tracing is not enabled.
				_, _ = fmt.Fprintln(inv.Stderr, "\nUploading traces...")
				if err := closeTracing(ctx); err != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "\nError uploading traces: %+v\n", err)
				}
				// Wait for prometheus metrics to be scraped
				_, _ = fmt.Fprintf(inv.Stderr, "Waiting %s for prometheus metrics to be scraped\n", prometheusFlags.Wait)
				<-time.After(prometheusFlags.Wait)
			}()
			tracer := tracerProvider.Tracer(scaletestTracerName)

			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("could not parse --output flags")
			}

			th := harness.NewTestHarness(strategy.toStrategy(), cleanupStrategy.toStrategy())
			for idx, ws := range workspaces {
				var (
					agent codersdk.WorkspaceAgent
					name  = "workspace-traffic"
					id    = strconv.Itoa(idx)
				)

				for _, res := range ws.LatestBuild.Resources {
					if len(res.Agents) == 0 {
						continue
					}
					agent = res.Agents[0]
				}

				if agent.ID == uuid.Nil {
					_, _ = fmt.Fprintf(inv.Stderr, "WARN: skipping workspace %s: no agent\n", ws.Name)
					continue
				}

				appConfig, err := createWorkspaceAppConfig(client, appHost.Host, app, ws, agent)
				if err != nil {
					return xerrors.Errorf("configure workspace app: %w", err)
				}

				var webClient *codersdk.Client
				if workspaceProxyURL != "" {
					u, err := url.Parse(workspaceProxyURL)
					if err != nil {
						return xerrors.Errorf("parse workspace proxy URL: %w", err)
					}

					webClient = codersdk.New(u,
						codersdk.WithHTTPClient(client.HTTPClient),
						codersdk.WithSessionToken(client.SessionToken()),
					)

					appConfig, err = createWorkspaceAppConfig(webClient, appHost.Host, app, ws, agent)
					if err != nil {
						return xerrors.Errorf("configure proxy workspace app: %w", err)
					}
				}

				// Setup our workspace agent connection.
				config := workspacetraffic.Config{
					AgentID:       agent.ID,
					WorkspaceID:   ws.ID,
					WorkspaceName: ws.Name,
					AgentName:     agent.Name,
					BytesPerTick:  bytesPerTick,
					Duration:      strategy.timeout,
					TickInterval:  tickInterval,
					ReadMetrics:   metrics.ReadMetrics(ws.OwnerName, ws.Name, agent.Name),
					WriteMetrics:  metrics.WriteMetrics(ws.OwnerName, ws.Name, agent.Name),
					SSH:           ssh,
					DisableDirect: disableDirect,
					Echo:          ssh,
					App:           appConfig,
				}

				if webClient != nil {
					config.WebClient = webClient
				}

				if err := config.Validate(); err != nil {
					return xerrors.Errorf("validate config: %w", err)
				}
				// use an independent client for each Runner, so they don't reuse TCP connections. This can lead to
				// requests being unbalanced among Coder instances.
				runnerClient, err := loadtestutil.DupClientCopyingHeaders(client, BypassHeader)
				if err != nil {
					return xerrors.Errorf("create runner client: %w", err)
				}
				var runner harness.Runnable = workspacetraffic.NewRunner(runnerClient, config)
				if tracingEnabled {
					runner = &runnableTraceWrapper{
						tracer:   tracer,
						spanName: fmt.Sprintf("%s/%s", name, id),
						runner:   runner,
					}
				}

				th.AddRun(name, id, runner)
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Running load test...")
			testCtx, testCancel := strategy.toContext(ctx)
			defer testCancel()
			err = th.Run(testCtx)
			if err != nil {
				return xerrors.Errorf("run test harness (harness failure, not a test failure): %w", err)
			}

			// If the command was interrupted, skip stats.
			if notifyCtx.Err() != nil {
				return notifyCtx.Err()
			}

			res := th.Results()
			for _, o := range outputs {
				err = o.write(res, inv.Stdout)
				if err != nil {
					return xerrors.Errorf("write output %q to %q: %w", o.format, o.path, err)
				}
			}

			if res.TotalFail > 0 {
				return xerrors.New("load test failed, see above for more details")
			}

			return nil
		},
	}

	cmd.Options = []serpent.Option{
		{
			Flag:        "bytes-per-tick",
			Env:         "CODER_SCALETEST_WORKSPACE_TRAFFIC_BYTES_PER_TICK",
			Default:     "1024",
			Description: "How much traffic to generate per tick.",
			Value:       serpent.Int64Of(&bytesPerTick),
		},
		{
			Flag:        "tick-interval",
			Env:         "CODER_SCALETEST_WORKSPACE_TRAFFIC_TICK_INTERVAL",
			Default:     "100ms",
			Description: "How often to send traffic.",
			Value:       serpent.DurationOf(&tickInterval),
		},
		{
			Flag:        "ssh",
			Env:         "CODER_SCALETEST_WORKSPACE_TRAFFIC_SSH",
			Default:     "",
			Description: "Send traffic over SSH, cannot be used with --app.",
			Value:       serpent.BoolOf(&ssh),
		},
		{
			Flag:        "disable-direct",
			Env:         "CODER_SCALETEST_WORKSPACE_TRAFFIC_DISABLE_DIRECT_CONNECTIONS",
			Default:     "false",
			Description: "Disable direct connections for SSH traffic to workspaces. Does nothing if `--ssh` is not also set.",
			Value:       serpent.BoolOf(&disableDirect),
		},
		{
			Flag:        "app",
			Env:         "CODER_SCALETEST_WORKSPACE_TRAFFIC_APP",
			Default:     "",
			Description: "Send WebSocket traffic to a workspace app (proxied via coderd), cannot be used with --ssh.",
			Value:       serpent.StringOf(&app),
		},
		{
			Flag:        "workspace-proxy-url",
			Env:         "CODER_SCALETEST_WORKSPACE_PROXY_URL",
			Default:     "",
			Description: "URL for workspace proxy to send web traffic to.",
			Value:       serpent.StringOf(&workspaceProxyURL),
		},
	}

	targetFlags.attach(&cmd.Options)
	tracingFlags.attach(&cmd.Options)
	strategy.attach(&cmd.Options)
	cleanupStrategy.attach(&cmd.Options)
	output.attach(&cmd.Options)
	prometheusFlags.attach(&cmd.Options)

	return cmd
}

func createWorkspaceAppConfig(client *codersdk.Client, appHost, app string, workspace codersdk.Workspace, agent codersdk.WorkspaceAgent) (workspacetraffic.AppConfig, error) {
	if app == "" {
		return workspacetraffic.AppConfig{}, nil
	}

	i := slices.IndexFunc(agent.Apps, func(a codersdk.WorkspaceApp) bool { return a.Slug == app })
	if i == -1 {
		return workspacetraffic.AppConfig{}, xerrors.Errorf("app %q not found in workspace %q", app, workspace.Name)
	}

	c := workspacetraffic.AppConfig{
		Name: agent.Apps[i].Slug,
	}
	if agent.Apps[i].Subdomain {
		if appHost == "" {
			return workspacetraffic.AppConfig{}, xerrors.Errorf("app %q is a subdomain app but no app host is configured", app)
		}

		c.URL = fmt.Sprintf("%s://%s", client.URL.Scheme, strings.Replace(appHost, "*", agent.Apps[i].SubdomainName, 1))
	} else {
		// Path-based apps are served at a trailing-slash URL: coderd (and
		// workspace proxies) 307-redirect "/apps/<slug>" to "/apps/<slug>/"
		// (see coderd/workspaceapps/proxy.go). The scaletest client rejects
		// redirects, so the WebSocket handshake fails on the redirect unless we
		// request the already-normalized URL.
		c.URL = fmt.Sprintf("%s/@%s/%s.%s/apps/%s/", client.URL.String(), workspace.OwnerName, workspace.Name, agent.Name, agent.Apps[i].Slug)
	}

	return c, nil
}

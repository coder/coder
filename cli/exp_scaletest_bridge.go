//go:build !slim

package cli

import (
	"fmt"
	"net/http"
	"os/signal"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/bridge"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/serpent"
)

func (r *RootCmd) scaletestBridge() *serpent.Command {
	var (
		userCount    int64
		noCleanup    bool
		mode         string
		upstreamURL  string
		directToken  string
		requestCount int64
		model        string
		stream       bool
		tracingFlags = &scaletestTracingFlags{}

		// This test requires unlimited concurrency.
		timeoutStrategy = &timeoutFlags{}
		cleanupStrategy = newScaletestCleanupStrategy()
		output          = &scaletestOutputFlags{}
		prometheusFlags = &scaletestPrometheusFlags{}
	)

	cmd := &serpent.Command{
		Use:   "bridge",
		Short: "Generate load on the AI Bridge service.",
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			notifyCtx, stop := signal.NotifyContext(ctx, StopSignals...)
			defer stop()
			ctx = notifyCtx

			// Validate mode
			if mode != "bridge" && mode != "direct" {
				return xerrors.Errorf("--mode must be either 'bridge' or 'direct', got %q", mode)
			}

			var me codersdk.User
			if mode == "bridge" {
				// Bridge mode requires admin access to create users
				var err error
				me, err = requireAdmin(ctx, client)
				if err != nil {
					return err
				}
			} else if upstreamURL == "" {
				// Direct mode requires upstream URL
				return xerrors.Errorf("--upstream-url must be set when using --mode direct")
			}

			client.HTTPClient = &http.Client{
				Transport: &codersdk.HeaderTransport{
					Transport: http.DefaultTransport,
					Header: map[string][]string{
						codersdk.BypassRatelimitHeader: {"true"},
					},
				},
			}

			// Validate: user count is always required (controls concurrency)
			if userCount <= 0 {
				return xerrors.Errorf("--user-count must be greater than 0")
			}

			// Set defaults
			if requestCount <= 0 {
				requestCount = 1
			}
			if model == "" {
				model = "gpt-4"
			}

			// userCount always controls the number of runners (concurrency)
			// Each runner makes requestCount requests
			runnerCount := userCount

			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("could not parse --output flags")
			}

			tracerProvider, closeTracing, tracingEnabled, err := tracingFlags.provider(ctx)
			if err != nil {
				return xerrors.Errorf("create tracer provider: %w", err)
			}
			tracer := tracerProvider.Tracer(scaletestTracerName)

			reg := prometheus.NewRegistry()
			metrics := bridge.NewMetrics(reg)

			logger := inv.Logger
			prometheusSrvClose := ServeHandler(ctx, logger, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), prometheusFlags.Address, "prometheus")
			defer prometheusSrvClose()

			defer func() {
				_, _ = fmt.Fprintln(inv.Stderr, "\nUploading traces...")
				if err := closeTracing(ctx); err != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "\nError uploading traces: %+v\n", err)
				}
				// Wait for prometheus metrics to be scraped
				_, _ = fmt.Fprintf(inv.Stderr, "Waiting %s for prometheus metrics to be scraped\n", prometheusFlags.Wait)
				<-time.After(prometheusFlags.Wait)
			}()

			if mode == "bridge" {
				_, _ = fmt.Fprintln(inv.Stderr, "Bridge mode: creating users and making requests through AI Bridge...")
			} else {
				_, _ = fmt.Fprintf(inv.Stderr, "Direct mode: making requests directly to %s\n", upstreamURL)
			}

			configs := make([]bridge.Config, 0, runnerCount)
			for range runnerCount {
				config := bridge.Config{
					Mode:         bridge.RequestMode(mode),
					Metrics:      metrics,
					RequestCount: int(requestCount),
					Model:        model,
					Stream:       stream,
				}

				if mode == "direct" {
					// Direct mode
					config.UpstreamURL = upstreamURL
					config.DirectToken = directToken
				} else {
					// Bridge mode
					if len(me.OrganizationIDs) == 0 {
						return xerrors.Errorf("admin user must have at least one organization")
					}
					config.User = createusers.Config{
						OrganizationID: me.OrganizationIDs[0],
					}
				}

				if err := config.Validate(); err != nil {
					return xerrors.Errorf("validate config: %w", err)
				}
				configs = append(configs, config)
			}

			th := harness.NewTestHarness(timeoutStrategy.wrapStrategy(harness.ConcurrentExecutionStrategy{}), cleanupStrategy.toStrategy())

			for i, config := range configs {
				id := strconv.Itoa(i)
				name := fmt.Sprintf("bridge-%s", id)
				var runner harness.Runnable = bridge.NewRunner(client, config)
				if tracingEnabled {
					runner = &runnableTraceWrapper{
						tracer:   tracer,
						spanName: name,
						runner:   runner,
					}
				}

				th.AddRun(name, id, runner)
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Running bridge scaletest...")
			testCtx, testCancel := timeoutStrategy.toContext(ctx)
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

			if !noCleanup {
				_, _ = fmt.Fprintln(inv.Stderr, "\nCleaning up...")
				cleanupCtx, cleanupCancel := cleanupStrategy.toContext(ctx)
				defer cleanupCancel()
				err = th.Cleanup(cleanupCtx)
				if err != nil {
					return xerrors.Errorf("cleanup tests: %w", err)
				}
			}

			if res.TotalFail > 0 {
				return xerrors.New("load test failed, see above for more details")
			}

			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:          "user-count",
			FlagShorthand: "c",
			Env:           "CODER_SCALETEST_BRIDGE_USER_COUNT",
			Description:   "Required: Number of concurrent runners (in bridge mode, each creates a user).",
			Value:         serpent.Int64Of(&userCount),
			Required:      true,
		},
		{
			Flag:        "mode",
			Env:         "CODER_SCALETEST_BRIDGE_MODE",
			Default:     "direct",
			Description: "Request mode: 'bridge' (create users and use AI Bridge) or 'direct' (make requests directly to upstream-url).",
			Value:       serpent.StringOf(&mode),
		},
		{
			Flag:        "upstream-url",
			Env:         "CODER_SCALETEST_BRIDGE_UPSTREAM_URL",
			Description: "URL to make requests to directly (required in direct mode, e.g., http://localhost:8080/v1/chat/completions).",
			Value:       serpent.StringOf(&upstreamURL),
		},
		{
			Flag:        "direct-token",
			Env:         "CODER_SCALETEST_BRIDGE_DIRECT_TOKEN",
			Description: "Bearer token for direct mode (optional, uses client token if not set).",
			Value:       serpent.StringOf(&directToken),
		},
		{
			Flag:        "request-count",
			Env:         "CODER_SCALETEST_BRIDGE_REQUEST_COUNT",
			Default:     "1",
			Description: "Number of requests to make per runner.",
			Value:       serpent.Int64Of(&requestCount),
		},
		{
			Flag:        "model",
			Env:         "CODER_SCALETEST_BRIDGE_MODEL",
			Default:     "gpt-4",
			Description: "Model to use for requests.",
			Value:       serpent.StringOf(&model),
		},
		{
			Flag:        "stream",
			Env:         "CODER_SCALETEST_BRIDGE_STREAM",
			Description: "Enable streaming requests.",
			Value:       serpent.BoolOf(&stream),
		},
		{
			Flag:        "no-cleanup",
			Env:         "CODER_SCALETEST_NO_CLEANUP",
			Description: "Do not clean up resources after the test completes.",
			Value:       serpent.BoolOf(&noCleanup),
		},
	}

	tracingFlags.attach(&cmd.Options)
	timeoutStrategy.attach(&cmd.Options)
	cleanupStrategy.attach(&cmd.Options)
	output.attach(&cmd.Options)
	prometheusFlags.attach(&cmd.Options)
	return cmd
}

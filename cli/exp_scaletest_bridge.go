//go:build !slim

package cli

import (
	"fmt"
	"net/http"
	"os/signal"
	"strconv"
	"text/tabwriter"
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
		concurrentUsers    int64
		noCleanup          bool
		mode               string
		upstreamURL        string
		provider           string
		requestsPerUser    int64
		useStreamingAPI    bool
		requestPayloadSize int64
		numMessages        int64
		httpTimeout        time.Duration

		timeoutStrategy = &timeoutFlags{}
		cleanupStrategy = newScaletestCleanupStrategy()
		output          = &scaletestOutputFlags{}
		prometheusFlags = &scaletestPrometheusFlags{}
	)

	cmd := &serpent.Command{
		Use:   "bridge",
		Short: "Generate load on the AI Bridge service.",
		Long: `Generate load for AI Bridge testing. Supports two modes: 'bridge' mode routes requests through the Coder AI Bridge, 'direct' mode makes requests directly to an upstream URL (useful for baseline comparisons).

Examples:
  # Test OpenAI API through bridge
  coder scaletest bridge --mode bridge --provider openai --concurrent-users 10 --request-count 5 --num-messages 10

  # Test Anthropic API through bridge
  coder scaletest bridge --mode bridge --provider anthropic --concurrent-users 10 --request-count 5 --num-messages 10

  # Test directly against mock server
  coder scaletest bridge --mode direct --provider openai --upstream-url http://localhost:8080/v1/chat/completions
`,
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			client.HTTPClient = &http.Client{
				Transport: &codersdk.HeaderTransport{
					Transport: http.DefaultTransport,
					Header: map[string][]string{
						codersdk.BypassRatelimitHeader: {"true"},
					},
				},
			}
			reg := prometheus.NewRegistry()
			metrics := bridge.NewMetrics(reg)

			logger := inv.Logger
			prometheusSrvClose := ServeHandler(ctx, logger, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), prometheusFlags.Address, "prometheus")
			defer prometheusSrvClose()

			defer func() {
				_, _ = fmt.Fprintf(inv.Stderr, "Waiting %s for prometheus metrics to be scraped\n", prometheusFlags.Wait)
				<-time.After(prometheusFlags.Wait)
			}()

			notifyCtx, stop := signal.NotifyContext(ctx, StopSignals...)
			defer stop()
			ctx = notifyCtx

			var userConfig createusers.Config
			if bridge.RequestMode(mode) == bridge.RequestModeBridge {
				me, err := requireAdmin(ctx, client)
				if err != nil {
					return err
				}
				if len(me.OrganizationIDs) == 0 {
					return xerrors.Errorf("admin user must have at least one organization")
				}
				userConfig = createusers.Config{
					OrganizationID: me.OrganizationIDs[0],
				}
				_, _ = fmt.Fprintln(inv.Stderr, "Bridge mode: creating users and making requests through AI Bridge...")
			} else {
				_, _ = fmt.Fprintf(inv.Stderr, "Direct mode: making requests directly to %s\n", upstreamURL)
			}

			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("parse output flags: %w", err)
			}

			config := bridge.Config{
				Mode:               bridge.RequestMode(mode),
				Metrics:            metrics,
				Provider:           provider,
				RequestCount:       int(requestsPerUser),
				Stream:             useStreamingAPI,
				RequestPayloadSize: int(requestPayloadSize),
				NumMessages:        int(numMessages),
				HTTPTimeout:        httpTimeout,
				UpstreamURL:        upstreamURL,
				User:               userConfig,
			}
			if err := config.Validate(); err != nil {
				return xerrors.Errorf("validate config: %w", err)
			}
			if err := config.PrepareRequestBody(); err != nil {
				return xerrors.Errorf("prepare request body: %w", err)
			}

			th := harness.NewTestHarness(timeoutStrategy.wrapStrategy(harness.ConcurrentExecutionStrategy{}), cleanupStrategy.toStrategy())

			for i := range concurrentUsers {
				id := strconv.Itoa(int(i))
				name := fmt.Sprintf("bridge-%s", id)
				var runner harness.Runnable = bridge.NewRunner(client, config)
				th.AddRun(name, id, runner)
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Bridge scaletest configuration:")
			tw := tabwriter.NewWriter(inv.Stderr, 0, 0, 2, ' ', 0)
			for _, opt := range inv.Command.Options {
				if opt.Hidden || opt.ValueSource == serpent.ValueSourceNone {
					continue
				}
				_, _ = fmt.Fprintf(tw, "  %s:\t%s", opt.Name, opt.Value.String())
				if opt.ValueSource != serpent.ValueSourceDefault {
					_, _ = fmt.Fprintf(tw, "\t(from %s)", opt.ValueSource)
				}
				_, _ = fmt.Fprintln(tw)
			}
			_ = tw.Flush()

			_, _ = fmt.Fprintln(inv.Stderr, "\nRunning bridge scaletest...")
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
			Flag:          "concurrent-users",
			FlagShorthand: "c",
			Env:           "CODER_SCALETEST_BRIDGE_CONCURRENT_USERS",
			Description:   "Required: Number of concurrent users.",
			Value: serpent.Validate(serpent.Int64Of(&concurrentUsers), func(value *serpent.Int64) error {
				if value == nil || value.Value() <= 0 {
					return xerrors.Errorf("--concurrent-users must be greater than 0")
				}
				return nil
			}),
			Required: true,
		},
		{
			Flag:        "mode",
			Env:         "CODER_SCALETEST_BRIDGE_MODE",
			Default:     "direct",
			Description: "Request mode: 'bridge' (create users and use AI Bridge) or 'direct' (make requests directly to upstream-url).",
			Value:       serpent.EnumOf(&mode, string(bridge.RequestModeBridge), string(bridge.RequestModeDirect)),
		},
		{
			Flag:        "upstream-url",
			Env:         "CODER_SCALETEST_BRIDGE_UPSTREAM_URL",
			Description: "URL to make requests to directly (required in direct mode, e.g., http://localhost:8080/v1/chat/completions).",
			Value:       serpent.StringOf(&upstreamURL),
		},
		{
			Flag:        "provider",
			Env:         "CODER_SCALETEST_BRIDGE_PROVIDER",
			Default:     "openai",
			Description: "API provider to use.",
			Value:       serpent.EnumOf(&provider, "openai", "anthropic"),
		},
		{
			Flag:        "request-count",
			Env:         "CODER_SCALETEST_BRIDGE_REQUEST_COUNT",
			Default:     "1",
			Description: "Number of sequential requests to make per runner.",
			Value: serpent.Validate(serpent.Int64Of(&requestsPerUser), func(value *serpent.Int64) error {
				if value == nil || value.Value() <= 0 {
					return xerrors.Errorf("--request-count must be greater than 0")
				}
				return nil
			}),
		},
		{
			Flag:        "stream",
			Env:         "CODER_SCALETEST_BRIDGE_STREAM",
			Description: "Enable streaming requests.",
			Value:       serpent.BoolOf(&useStreamingAPI),
		},
		{
			Flag:        "request-payload-size",
			Env:         "CODER_SCALETEST_BRIDGE_REQUEST_PAYLOAD_SIZE",
			Default:     "1024",
			Description: "Size in bytes of the request payload (user message content). If 0, uses default message content.",
			Value:       serpent.Int64Of(&requestPayloadSize),
		},
		{
			Flag:        "num-messages",
			Env:         "CODER_SCALETEST_BRIDGE_NUM_MESSAGES",
			Default:     "1",
			Description: "Number of messages to include in the conversation.",
			Value:       serpent.Int64Of(&numMessages),
		},
		{
			Flag:        "no-cleanup",
			Env:         "CODER_SCALETEST_NO_CLEANUP",
			Description: "Do not clean up resources after the test completes.",
			Value:       serpent.BoolOf(&noCleanup),
		},
		{
			Flag:        "http-timeout",
			Env:         "CODER_SCALETEST_BRIDGE_HTTP_TIMEOUT",
			Default:     "30s",
			Description: "Timeout for individual HTTP requests to the upstream provider.",
			Value:       serpent.DurationOf(&httpTimeout),
		},
	}

	timeoutStrategy.attach(&cmd.Options)
	cleanupStrategy.attach(&cmd.Options)
	output.attach(&cmd.Options)
	prometheusFlags.attach(&cmd.Options)
	return cmd
}

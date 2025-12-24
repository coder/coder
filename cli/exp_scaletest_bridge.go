//go:build !slim

package cli

import (
	"fmt"
	"net/http"
	"os/signal"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/bridge"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/serpent"
)

func (r *RootCmd) scaletestBridge() *serpent.Command {
	var (
		userCount          int64
		noCleanup          bool
		mode               string
		upstreamURL        string
		directToken        string
		provider           string
		requestCount       int64
		model              string
		stream             bool
		requestPayloadSize int64

		timeoutStrategy = &timeoutFlags{}
		cleanupStrategy = newScaletestCleanupStrategy()
		output          = &scaletestOutputFlags{}
	)

	cmd := &serpent.Command{
		Use:   "bridge",
		Short: "Generate load on the AI Bridge service.",
		Long: `Generate load on the AI Bridge service by making requests to OpenAI or Anthropic APIs.

Examples:
  # Test OpenAI API through bridge
  coder scaletest bridge --mode bridge --provider openai --user-count 10 --request-count 5

  # Test Anthropic API through bridge
  coder scaletest bridge --mode bridge --provider anthropic --user-count 10 --request-count 5

  # Test directly against mock server
  coder scaletest bridge --mode direct --provider openai --upstream-url http://localhost:8080/v1/chat/completions

The load generator builds conversation history over time, with each request including
all previous messages in the conversation.`,
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			notifyCtx, stop := signal.NotifyContext(ctx, StopSignals...)
			defer stop()
			ctx = notifyCtx

			if mode != "bridge" && mode != "direct" {
				return xerrors.Errorf("--mode must be either 'bridge' or 'direct', got %q", mode)
			}

			var me codersdk.User
			if mode == "bridge" {
				var err error
				me, err = requireAdmin(ctx, client)
				if err != nil {
					return err
				}
			} else if upstreamURL == "" {
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

			if userCount <= 0 {
				return xerrors.Errorf("--user-count must be greater than 0")
			}

			if requestCount <= 0 {
				requestCount = 1
			}
			if provider == "" {
				provider = "openai"
			}
			if model == "" {
				if provider == "anthropic" {
					model = "claude-3-opus-20240229"
				} else {
					model = "gpt-4"
				}
			}

			runnerCount := userCount

			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("could not parse --output flags")
			}

			reg := prometheus.NewRegistry()
			metrics := bridge.NewMetrics(reg)

			if mode == "bridge" {
				_, _ = fmt.Fprintln(inv.Stderr, "Bridge mode: creating users and making requests through AI Bridge...")
			} else {
				_, _ = fmt.Fprintf(inv.Stderr, "Direct mode: making requests directly to %s\n", upstreamURL)
			}

			configs := make([]bridge.Config, 0, runnerCount)
			for range runnerCount {
				config := bridge.Config{
					Mode:               bridge.RequestMode(mode),
					Metrics:            metrics,
					Provider:           provider,
					RequestCount:       int(requestCount),
					Model:              model,
					Stream:             stream,
					RequestPayloadSize: int(requestPayloadSize),
				}

				if mode == "direct" {
					config.UpstreamURL = upstreamURL
					config.DirectToken = directToken
				} else {
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
			Description:   "Required: Number of concurrent runners (in bridge mode, each creates a coder user).",
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
			Flag:        "provider",
			Env:         "CODER_SCALETEST_BRIDGE_PROVIDER",
			Default:     "openai",
			Description: "API provider to use: 'openai' or 'anthropic'.",
			Value:       serpent.StringOf(&provider),
		},
		{
			Flag:        "request-count",
			Env:         "CODER_SCALETEST_BRIDGE_REQUEST_COUNT",
			Default:     "1",
			Description: "Number of sequential requests to make per runner.",
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
			Flag:        "request-payload-size",
			Env:         "CODER_SCALETEST_BRIDGE_REQUEST_PAYLOAD_SIZE",
			Default:     "0",
			Description: "Size in bytes of the request payload (user message content). If 0, uses default message content.",
			Value:       serpent.Int64Of(&requestPayloadSize),
		},
		{
			Flag:        "no-cleanup",
			Env:         "CODER_SCALETEST_NO_CLEANUP",
			Description: "Do not clean up resources after the test completes.",
			Value:       serpent.BoolOf(&noCleanup),
		},
	}

	timeoutStrategy.attach(&cmd.Options)
	cleanupStrategy.attach(&cmd.Options)
	output.attach(&cmd.Options)
	return cmd
}

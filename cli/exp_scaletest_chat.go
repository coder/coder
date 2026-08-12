//go:build !slim

package cli

import (
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/chat"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/serpent"
)

func (r *RootCmd) scaletestChat() *serpent.Command {
	var (
		chatsPerWorkspace       int64
		prompt                  string
		turns                   int64
		turnStartDelay          time.Duration
		llmMockURL              string
		providerPropagationWait time.Duration
		targetFlags             = &workspaceTargetFlags{allowEmpty: true}
		tracingFlags            = &scaletestTracingFlags{}
		prometheusFlags         = &scaletestPrometheusFlags{}
		timeoutStrategy         = &timeoutFlags{}
		cleanupStrategy         = newScaletestCleanupStrategy()
		output                  = &scaletestOutputFlags{}
	)

	cmd := &serpent.Command{
		Use:   "chat",
		Short: "Generate Coder Agents load.",
		Handler: func(inv *serpent.Invocation) error {
			baseCtx := inv.Context()
			ctx, stop := inv.SignalNotifyContext(baseCtx, StopSignals...)
			defer stop()

			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("could not parse --output flags: %w", err)
			}
			switch {
			case turns < 1:
				return xerrors.Errorf("--turns must be at least 1")
			case chatsPerWorkspace < 1:
				return xerrors.Errorf("--chats-per-workspace must be at least 1")
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			me, err := RequireAdmin(ctx, client)
			if err != nil {
				return err
			}
			client.HTTPClient.Transport = &codersdk.HeaderTransport{
				Transport: client.HTTPClient.Transport,
				Header:    BypassHeader,
			}

			workspaces, err := targetFlags.getTargetedWorkspaces(ctx, client, me.OrganizationIDs, inv.Stdout)
			if err != nil {
				return err
			}

			if len(workspaces) == 0 {
				workspaces = append(workspaces, codersdk.Workspace{OrganizationID: me.OrganizationIDs[0]})
				_, _ = fmt.Fprintln(inv.Stderr, "No scaletest workspaces found; running chats without workspace context.")
			}

			logger := inv.Logger
			modelConfigID, err := chat.EnsureScaletestModelConfig(ctx, client, logger, llmMockURL, providerPropagationWait)
			if err != nil {
				return err
			}

			// Start metrics and tracing before creating runners.
			reg := prometheus.NewRegistry()
			metrics := chat.NewMetrics(reg)

			prometheusSrvClose := ServeHandler(baseCtx, logger, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), prometheusFlags.Address, "prometheus")

			tracerProvider, closeTracing, tracingEnabled, err := tracingFlags.provider(baseCtx)
			if err != nil {
				prometheusSrvClose()
				return xerrors.Errorf("create tracer provider: %w", err)
			}
			defer func() {
				if tracingEnabled {
					_, _ = fmt.Fprintln(inv.Stderr, "Uploading traces...")
				}
				if err := closeTracing(baseCtx); err != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "Error uploading traces: %+v\n", err)
				}
				_, _ = fmt.Fprintf(inv.Stderr, "Waiting %s for prometheus metrics to be scraped\n", prometheusFlags.Wait)
				<-time.After(prometheusFlags.Wait)
				prometheusSrvClose()
			}()

			tracer := tracerProvider.Tracer(scaletestTracerName)

			var turnStartReadyWaitGroup *sync.WaitGroup
			var startTurnsChan chan struct{}
			if turnStartDelay > 0 && turns > 1 {
				turnStartReadyWaitGroup = &sync.WaitGroup{}
				startTurnsChan = make(chan struct{})
			}

			chatHarness := harness.NewTestHarness(
				timeoutStrategy.wrapStrategy(harness.ConcurrentExecutionStrategy{}),
				cleanupStrategy.toStrategy(),
			)
			for workspaceIndex, targetWorkspace := range workspaces {
				for chatIndex := int64(0); chatIndex < chatsPerWorkspace; chatIndex++ {
					if turnStartReadyWaitGroup != nil {
						turnStartReadyWaitGroup.Add(1)
					}

					cfg := chat.Config{
						OrganizationID:          targetWorkspace.OrganizationID,
						WorkspaceID:             targetWorkspace.ID,
						Prompt:                  prompt,
						ModelConfigID:           modelConfigID,
						Turns:                   int(turns),
						TurnStartDelay:          turnStartDelay,
						TurnStartReadyWaitGroup: turnStartReadyWaitGroup,
						StartTurnsChan:          startTurnsChan,
						Metrics:                 metrics,
					}
					if err := cfg.Validate(); err != nil {
						return xerrors.Errorf("validate config for workspace %d chat %d: %w", workspaceIndex, chatIndex, err)
					}

					runnerClient, err := loadtestutil.DupClientCopyingHeaders(client, BypassHeader)
					if err != nil {
						return xerrors.Errorf("duplicate client for workspace %d chat %d: %w", workspaceIndex, chatIndex, err)
					}
					var runner harness.Runnable = chat.NewRunner(runnerClient, cfg)
					if tracingEnabled {
						runner = &runnableTraceWrapper{
							tracer:   tracer,
							runner:   runner,
							spanName: fmt.Sprintf("chat/workspace-%d-chat-%d", workspaceIndex, chatIndex),
						}
					}
					chatHarness.AddRun("chat", fmt.Sprintf("workspace-%d-chat-%d", workspaceIndex, chatIndex), runner)
				}
			}

			// Run the chat harness in the background so the CLI can release the
			// follow-up turns after every runner finishes its initial turn.
			totalChats := int64(len(workspaces)) * chatsPerWorkspace
			_, _ = fmt.Fprintf(inv.Stderr, "Starting chat scale test with %d chats across %d targets...\n", totalChats, len(workspaces))
			testCtx, testCancel := timeoutStrategy.toContext(ctx)
			defer testCancel()
			testDone := make(chan error, 1)
			go func() {
				testDone <- chatHarness.Run(testCtx)
			}()

			if turnStartReadyWaitGroup != nil {
				initialTurnsDone := make(chan struct{})
				go func() {
					turnStartReadyWaitGroup.Wait()
					close(initialTurnsDone)
				}()

				select {
				case <-testCtx.Done():
					return testCtx.Err()
				case <-initialTurnsDone:
				}

				_, _ = fmt.Fprintf(inv.Stderr, "All %d initial turns completed, waiting %s before starting the follow-up turns...\n", totalChats, turnStartDelay)
				select {
				case <-testCtx.Done():
					return testCtx.Err()
				case <-time.After(turnStartDelay):
				}

				close(startTurnsChan)
			}

			if err := <-testDone; err != nil {
				return xerrors.Errorf("run harness: %w", err)
			}

			results := chatHarness.Results()
			for _, o := range outputs {
				if err := o.write(results, inv.Stdout); err != nil {
					return xerrors.Errorf("write output %q to %q: %w", o.format, o.path, err)
				}
			}

			_, _ = fmt.Fprintln(inv.Stderr, "\nCleaning up (archiving chats)...")
			cleanupCtx, cleanupCancel := cleanupStrategy.toContext(ctx)
			defer cleanupCancel()
			if err := chatHarness.Cleanup(cleanupCtx); err != nil {
				return xerrors.Errorf("cleanup chats: %w", err)
			}

			if results.TotalFail > 0 {
				return xerrors.Errorf("scale test failed: %d/%d runs failed", results.TotalFail, results.TotalRuns)
			}

			_, _ = fmt.Fprintf(inv.Stderr, "Scale test passed: %d/%d runs succeeded\n", results.TotalPass, results.TotalRuns)
			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "chats-per-workspace",
			Description: "Number of chats to run against each targeted workspace. Required and must be greater than 0.",
			Value:       serpent.Int64Of(&chatsPerWorkspace),
			Required:    true,
		},
		{
			Flag:        "prompt",
			Description: "Text prompt to send on every turn in each chat.",
			Default:     "Reply with one short sentence.",
			Value:       serpent.StringOf(&prompt),
		},
		{
			Flag:        "turns",
			Description: "Number of user to assistant exchanges per chat conversation.",
			Default:     "10",
			Value:       serpent.Int64Of(&turns),
		},
		{
			Flag:        "turn-start-delay",
			Description: "Delay between every chat completing its initial turn and starting the follow-up turns. Use this to separate initial-turn load from follow-up-turn load.",
			Default:     "0s",
			Value:       serpent.DurationOf(&turnStartDelay),
		},
		{
			Flag:        "llm-mock-url",
			Description: "URL of the mock LLM server (e.g. http://127.0.0.1:8080/v1). Creates or updates the Scaletest LLM Mock openai-compat provider and model config to point at this URL.",
			Value:       serpent.StringOf(&llmMockURL),
			Required:    true,
		},
		{
			Flag:        "provider-propagation-wait",
			Description: "Time to wait after creating or updating the mock LLM provider so every coderd replica's cached provider config expires. The default exceeds the server-side cache TTL.",
			Default:     chat.DefaultProviderPropagationWait.String(),
			Value:       serpent.DurationOf(&providerPropagationWait),
			Hidden:      true,
		},
	}
	targetFlags.attach(&cmd.Options)
	output.attach(&cmd.Options)
	tracingFlags.attach(&cmd.Options)
	prometheusFlags.attach(&cmd.Options)
	timeoutStrategy.attach(&cmd.Options)
	cleanupStrategy.attach(&cmd.Options)
	return cmd
}

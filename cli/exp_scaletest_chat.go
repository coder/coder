//go:build !slim

package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/chat"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/coder/v2/scaletest/workspacebuild"
	"github.com/coder/serpent"
)

func (r *RootCmd) scaletestChat() *serpent.Command {
	var (
		workspaceCount    int64
		chatsPerWorkspace int64
		workspaceID       string
		template          string
		prompt            string
		turns             int64
		turnStartDelay    time.Duration
		llmMockURL        string
		parameterFlags    workspaceParameterFlags
		tracingFlags      = &scaletestTracingFlags{}
		prometheusFlags   = &scaletestPrometheusFlags{}
		timeoutStrategy   = &timeoutFlags{}
		cleanupStrategy   = newScaletestCleanupStrategy()
		output            = &scaletestOutputFlags{}
	)

	cmd := &serpent.Command{
		Use:   "chat",
		Short: "Run a chat scale test against the Coder API",
		Long:  "Creates chats-per-workspace concurrent chats against each selected workspace. Use --workspace-id to target one existing workspace or --template with --workspace-count to pre-create workspaces before the chat storm begins. --chats-per-workspace must be at least 1.",
		Handler: func(inv *serpent.Invocation) (runErr error) {
			baseCtx := inv.Context()
			ctx, stop := inv.SignalNotifyContext(baseCtx, StopSignals...)
			defer stop()

			// Parse and validate command inputs.
			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("could not parse --output flags: %w", err)
			}
			switch {
			case turns < 1:
				return xerrors.Errorf("--turns must be at least 1")
			case workspaceID == "" && template == "":
				return xerrors.Errorf("exactly one of --workspace-id or --template is required")
			case workspaceID != "" && template != "":
				return xerrors.Errorf("--workspace-id and --template are mutually exclusive")
			case chatsPerWorkspace < 1:
				return xerrors.Errorf("--chats-per-workspace must be at least 1")
			case template != "" && workspaceCount < 1:
				return xerrors.Errorf("--workspace-count must be at least 1 when --template is set")
			case template == "" && workspaceCount != 0:
				return xerrors.Errorf("--workspace-count may only be used with --template")
			}

			// Initialize clients and the bounded test context.
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

			testCtx, testCancel := timeoutStrategy.toContext(ctx)
			defer testCancel()

			// Resolve workspace inputs. Existing workspaces run chats directly;
			// template inputs first build workspaces, then run chats against them.
			var (
				organizationID       uuid.UUID
				workspaceIDs         []uuid.UUID
				workspaceBuildConfig *workspacebuild.Config
				templateName         string
			)
			if workspaceID != "" {
				wsID, err := uuid.Parse(workspaceID)
				if err != nil {
					return xerrors.Errorf("parse workspace-id: %w", err)
				}
				ws, err := client.Workspace(testCtx, wsID)
				if err != nil {
					return xerrors.Errorf("fetch workspace: %w", err)
				}
				hasAgents := slices.ContainsFunc(ws.LatestBuild.Resources, func(resource codersdk.WorkspaceResource) bool {
					return len(resource.Agents) > 0
				})
				if !hasAgents {
					return xerrors.Errorf("workspace %s has no agents in its latest build", ws.Name)
				}
				organizationID = ws.OrganizationID
				workspaceIDs = []uuid.UUID{wsID}
			} else {
				tpl, err := parseTemplate(testCtx, client, me.OrganizationIDs, template)
				if err != nil {
					return xerrors.Errorf("parse template: %w", err)
				}
				organizationID = tpl.OrganizationID
				templateName = tpl.Name

				cliRichParameters, err := asWorkspaceBuildParameters(parameterFlags.richParameters)
				if err != nil {
					return xerrors.Errorf("can't parse given parameter values: %w", err)
				}

				// Scaletest commands should not stop for interactive parameter prompts.
				// Accept template defaults unless the caller overrides them explicitly.
				richParameters, err := prepWorkspaceBuild(inv, client, prepWorkspaceBuildArgs{
					Action:               WorkspaceCreate,
					TemplateVersionID:    tpl.ActiveVersionID,
					NewWorkspaceName:     "scaletest-chat",
					Owner:                codersdk.Me,
					RichParameterFile:    parameterFlags.richParameterFile,
					RichParameters:       cliRichParameters,
					UseParameterDefaults: true,
				})
				if err != nil {
					return xerrors.Errorf("prepare build: %w", err)
				}

				workspaceConfig := workspacebuild.Config{
					OrganizationID: tpl.OrganizationID,
					UserID:         codersdk.Me,
					Request: codersdk.CreateWorkspaceRequest{
						TemplateID:          tpl.ID,
						RichParameterValues: richParameters,
					},
				}
				if err := workspaceConfig.Validate(); err != nil {
					return xerrors.Errorf("validate workspace config: %w", err)
				}
				workspaceBuildConfig = &workspaceConfig
			}

			// Bootstrap the optional mock model config before runners start.
			var modelConfigID *uuid.UUID
			if llmMockURL != "" {
				modelConfigID, err = chat.EnsureScaletestModelConfig(testCtx, codersdk.NewExperimentalClient(client), inv.Stderr, llmMockURL)
				if err != nil {
					return err
				}
			}

			// Start metrics and tracing before creating runners.
			runID := uuid.NewString()
			metricLabelValues := chat.MetricLabelValues(runID)
			reg := prometheus.NewRegistry()
			metrics := chat.NewMetrics(reg, chat.MetricLabelNames()...)

			logger := slog.Make(sloghuman.Sink(inv.Stderr)).Leveled(slog.LevelDebug)
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

			var tracer trace.Tracer
			if tracingEnabled {
				tracer = tracerProvider.Tracer(scaletestTracerName)
			}

			// Register cleanup before provisioning resources.
			var (
				workspaceHarness *harness.TestHarness
				chatHarness      *harness.TestHarness
			)
			defer func() {
				if chatHarness == nil && workspaceHarness == nil {
					return
				}

				cleanupCtx, cleanupCancel := cleanupStrategy.toContext(context.Background())
				defer cleanupCancel()

				var cleanupErr error
				if chatHarness != nil {
					_, _ = fmt.Fprintln(inv.Stderr, "Cleaning up (archiving chats)...")
					if err := chatHarness.Cleanup(cleanupCtx); err != nil {
						cleanupErr = errors.Join(cleanupErr, xerrors.Errorf("cleanup chats: %w", err))
					}
				}
				if workspaceHarness != nil {
					_, _ = fmt.Fprintln(inv.Stderr, "Cleaning up created workspaces...")
					if err := workspaceHarness.Cleanup(cleanupCtx); err != nil {
						cleanupErr = errors.Join(cleanupErr, xerrors.Errorf("cleanup workspaces: %w", err))
					}
				}
				if cleanupErr != nil {
					runErr = errors.Join(runErr, cleanupErr)
				}
			}()

			// Provision template workspaces before the chat storm starts.
			if workspaceBuildConfig != nil {
				_, _ = fmt.Fprintf(inv.Stderr, "Creating %d workspaces from template %q before starting chats...\n", workspaceCount, templateName)
				workspaceHarness = harness.NewTestHarness(
					timeoutStrategy.wrapStrategy(harness.ConcurrentExecutionStrategy{}),
					cleanupStrategy.toStrategy(),
				)

				workspaceRunners := make([]*workspacebuild.Runner, 0, int(workspaceCount))
				for i := int64(0); i < workspaceCount; i++ {
					runnerClient, err := loadtestutil.DupClientCopyingHeaders(client, BypassHeader)
					if err != nil {
						return xerrors.Errorf("duplicate client for workspace runner %d: %w", i, err)
					}
					runner := workspacebuild.NewRunner(runnerClient, *workspaceBuildConfig)
					workspaceRunners = append(workspaceRunners, runner)
					workspaceHarness.AddRun("workspace", fmt.Sprintf("workspace-%d", i), runner)
				}

				if err := workspaceHarness.Run(testCtx); err != nil {
					return xerrors.Errorf("create template workspaces: %w", err)
				}

				workspaceResults := workspaceHarness.Results()
				if workspaceResults.TotalFail > 0 {
					return xerrors.Errorf("workspace provisioning failed: %d/%d workspace builds failed", workspaceResults.TotalFail, workspaceResults.TotalRuns)
				}

				workspaceIDs = make([]uuid.UUID, 0, len(workspaceRunners))
				for i, runner := range workspaceRunners {
					createdWorkspaceID := runner.WorkspaceID()
					if createdWorkspaceID == uuid.Nil {
						return xerrors.Errorf("workspace runner %d did not record a created workspace ID", i)
					}
					workspaceIDs = append(workspaceIDs, createdWorkspaceID)
				}
				if int64(len(workspaceIDs)) != workspaceCount {
					return xerrors.Errorf("workspace provisioning completed with %d workspaces, expected %d", len(workspaceIDs), workspaceCount)
				}
				_, _ = fmt.Fprintf(inv.Stderr, "Created %d workspaces from template %q\n", len(workspaceIDs), templateName)
			}

			// Build chat runners and their phase barriers.
			readyWaitGroup := &sync.WaitGroup{}
			startChan := make(chan struct{})
			var turnStartReadyWaitGroup *sync.WaitGroup
			var startTurnsChan chan struct{}
			if turnStartDelay > 0 {
				turnStartReadyWaitGroup = &sync.WaitGroup{}
				startTurnsChan = make(chan struct{})
			}

			// Validate all chat configs before constructing the harness so
			// that validation failures (for example, an empty --prompt) return
			// cleanly instead of triggering the deferred cleanup against a
			// harness whose Run never started.
			type chatRunConfig struct {
				workspaceIndex int
				chatIndex      int64
				cfg            chat.Config
			}
			chatRunConfigs := make([]chatRunConfig, 0, len(workspaceIDs)*int(chatsPerWorkspace))
			for workspaceIndex, targetWorkspaceID := range workspaceIDs {
				for chatIndex := int64(0); chatIndex < chatsPerWorkspace; chatIndex++ {
					readyWaitGroup.Add(1)
					if turnStartReadyWaitGroup != nil {
						turnStartReadyWaitGroup.Add(1)
					}

					cfg := chat.Config{
						RunID:                   runID,
						OrganizationID:          organizationID,
						WorkspaceID:             targetWorkspaceID,
						Prompt:                  prompt,
						ModelConfigID:           modelConfigID,
						Turns:                   int(turns),
						TurnStartDelay:          turnStartDelay,
						ReadyWaitGroup:          readyWaitGroup,
						StartChan:               startChan,
						TurnStartReadyWaitGroup: turnStartReadyWaitGroup,
						StartTurnsChan:          startTurnsChan,
						Metrics:                 metrics,
						MetricLabelValues:       slices.Clone(metricLabelValues),
					}
					if err := cfg.Validate(); err != nil {
						return xerrors.Errorf("validate config for workspace %d chat %d: %w", workspaceIndex, chatIndex, err)
					}
					chatRunConfigs = append(chatRunConfigs, chatRunConfig{
						workspaceIndex: workspaceIndex,
						chatIndex:      chatIndex,
						cfg:            cfg,
					})
				}
			}

			chatHarness = harness.NewTestHarness(
				timeoutStrategy.wrapStrategy(harness.ConcurrentExecutionStrategy{}),
				cleanupStrategy.toStrategy(),
			)
			for _, run := range chatRunConfigs {
				runnerClient, err := loadtestutil.DupClientCopyingHeaders(client, BypassHeader)
				if err != nil {
					return xerrors.Errorf("duplicate client for workspace %d chat %d: %w", run.workspaceIndex, run.chatIndex, err)
				}
				var runner harness.Runnable = chat.NewRunner(runnerClient, run.cfg)
				if tracer != nil {
					runner = &runnableTraceWrapper{
						tracer:   tracer,
						runner:   runner,
						spanName: fmt.Sprintf("chat/workspace-%d-chat-%d", run.workspaceIndex, run.chatIndex),
					}
				}
				chatHarness.AddRun("chat", fmt.Sprintf("workspace-%d-chat-%d", run.workspaceIndex, run.chatIndex), runner)
			}

			// Run the chat harness, optionally pausing between chat creation and the turn storm.
			totalChats := int64(len(workspaceIDs)) * chatsPerWorkspace
			_, _ = fmt.Fprintf(inv.Stderr, "Starting chat scale test with %d chats across %d workspaces...\n", totalChats, len(workspaceIDs))
			done := make(chan error, 1)
			go func() {
				done <- chatHarness.Run(testCtx)
			}()

			drainChatHarnessAfterCancel := func() error {
				if err := <-done; err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					_, _ = fmt.Fprintf(inv.Stderr, "chat harness exited after context cancellation: %+v\n", err)
				}
				return testCtx.Err()
			}
			waitForRunners := func(waitGroup *sync.WaitGroup) error {
				ready := make(chan struct{})
				go func() {
					waitGroup.Wait()
					close(ready)
				}()
				select {
				case <-testCtx.Done():
					return testCtx.Err()
				case <-ready:
					return nil
				}
			}

			if err := waitForRunners(readyWaitGroup); err != nil {
				return drainChatHarnessAfterCancel()
			}
			_, _ = fmt.Fprintf(inv.Stderr, "All %d chat runners ready across %d workspaces, creating chats...\n", totalChats, len(workspaceIDs))
			close(startChan)

			if turnStartReadyWaitGroup != nil {
				if err := waitForRunners(turnStartReadyWaitGroup); err != nil {
					return drainChatHarnessAfterCancel()
				}
				_, _ = fmt.Fprintf(inv.Stderr, "All %d initial turns completed, waiting %s before starting the follow-up turn storm...\n", totalChats, turnStartDelay)
				select {
				case <-testCtx.Done():
					return drainChatHarnessAfterCancel()
				case <-time.After(turnStartDelay):
				}
				close(startTurnsChan)
			}

			if err := <-done; err != nil {
				return xerrors.Errorf("run harness: %w", err)
			}

			results := chatHarness.Results()
			for _, o := range outputs {
				if err := o.write(results, inv.Stdout); err != nil {
					return xerrors.Errorf("write output %q to %q: %w", o.format, o.path, err)
				}
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
			Flag:        "workspace-count",
			Description: "Number of workspaces to create before starting chats. Required with --template and rejected with --workspace-id.",
			Value:       serpent.Int64Of(&workspaceCount),
		},
		{
			Flag:        "chats-per-workspace",
			Description: "Number of chats to run against each selected workspace. Required and must be greater than 0.",
			Value:       serpent.Int64Of(&chatsPerWorkspace),
		},
		{
			Flag:        "workspace-id",
			Description: "UUID of the pre-existing workspace to create chats against. Mutually exclusive with --template.",
			Value:       serpent.StringOf(&workspaceID),
		},
		{
			Flag:        "template",
			Description: "Template name or UUID. When set, the command first creates --workspace-count workspaces and then fans out chats across them.",
			Value:       serpent.StringOf(&template),
		},
		{
			Flag:        "prompt",
			Description: "Text prompt to send on every turn in each chat.",
			Default:     "Reply with one short sentence.",
			Value:       serpent.StringOf(&prompt),
		},
		{
			Flag:        "turns",
			Description: "Number of user→assistant exchanges per chat conversation.",
			Default:     "10",
			Value:       serpent.Int64Of(&turns),
		},
		{
			Flag:        "turn-start-delay",
			Description: "Delay between every chat completing its initial turn and starting the follow-up turn storm. Use this to separate initial-turn load from follow-up-turn load.",
			Default:     "0s",
			Value:       serpent.DurationOf(&turnStartDelay),
		},
		{
			Flag:        "llm-mock-url",
			Description: "URL of the mock LLM server (e.g. http://127.0.0.1:8080/v1). When set, creates or reconciles the Scaletest LLM Mock openai-compat provider and model config to point at this URL. Refuses to overwrite a non-scaletest openai-compat provider.",
			Value:       serpent.StringOf(&llmMockURL),
		},
	}
	cmd.Options = append(cmd.Options, parameterFlags.cliParameters()...)
	output.attach(&cmd.Options)
	tracingFlags.attach(&cmd.Options)
	prometheusFlags.attach(&cmd.Options)
	timeoutStrategy.attach(&cmd.Options)
	cleanupStrategy.attach(&cmd.Options)
	return cmd
}

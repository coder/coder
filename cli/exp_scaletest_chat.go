//go:build !slim

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

const (
	scaletestProviderDisplayName = "Scaletest LLM Mock"
	scaletestModelName           = "scaletest-model"
	scaletestModelDisplayName    = "Scaletest Model"
)

type chatWorkspaceSelection struct {
	WorkspaceID          *uuid.UUID
	TemplateID           *uuid.UUID
	TemplateName         string
	WorkspaceBuildConfig *workspacebuild.Config
}

func (r *RootCmd) scaletestChat() *serpent.Command {
	var (
		workspaceCount     int64
		chatsPerWorkspace  int64
		workspaceID        string
		template           string
		prompt             string
		turns              int64
		followUpPrompt     string
		followUpStartDelay time.Duration
		llmMockURL         string
		summaryOutput      string
		parameterFlags     workspaceParameterFlags
		tracingFlags       = &scaletestTracingFlags{}
		prometheusFlags    = &scaletestPrometheusFlags{}
		timeoutStrategy    = &timeoutFlags{}
		cleanupStrategy    = newScaletestCleanupStrategy()
		output             = &scaletestOutputFlags{}
	)

	cmd := &serpent.Command{
		Use:   "chat",
		Short: "Run a chat scale test against the Coder API",
		Long:  "Creates chats-per-workspace concurrent chats against each selected workspace. Use --workspace-id to target one existing workspace or --template with --workspace-count to pre-create workspaces before the chat storm begins.",
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("could not parse --output flags: %w", err)
			}

			if turns < 1 {
				return xerrors.Errorf("--turns must be at least 1")
			}

			if followUpStartDelay > 0 && turns < 2 {
				return xerrors.Errorf("--follow-up-start-delay requires --turns to be at least 2")
			}
			if summaryOutput == "-" {
				return xerrors.Errorf("--summary-output must be a file path, not stdout")
			}
			if err := validateChatWorkspaceSelection(workspaceID, template, workspaceCount, chatsPerWorkspace); err != nil {
				return err
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			me, err := requireAdmin(ctx, client)
			if err != nil {
				return err
			}

			client.HTTPClient.Transport = &codersdk.HeaderTransport{
				Transport: client.HTTPClient.Transport,
				Header:    BypassHeader,
			}

			testCtx, testCancel := timeoutStrategy.toContext(ctx)
			defer testCancel()

			workspaceSelection, err := resolveChatWorkspaceSelection(testCtx, inv, client, me.OrganizationIDs, workspaceID, template, parameterFlags)
			if err != nil {
				return err
			}

			experimentalClient := codersdk.NewExperimentalClient(client)
			var modelConfigID *uuid.UUID
			if llmMockURL != "" {
				_, _ = fmt.Fprintf(inv.Stderr, "Bootstrapping mock LLM provider at %s...\n", llmMockURL)

				// Try to create a DB-backed openai-compat provider. If one
				// already exists the server returns 409 and we proceed.
				enabled := true
				_, err = experimentalClient.CreateChatProvider(testCtx, codersdk.CreateChatProviderConfigRequest{
					Provider:    "openai-compat",
					DisplayName: scaletestProviderDisplayName,
					APIKey:      "scaletest-api-key",
					BaseURL:     llmMockURL,
					Enabled:     &enabled,
				})
				if err != nil {
					// A 409 means the provider already exists in the DB — that's fine.
					var sdkErr *codersdk.Error
					if !xerrors.As(err, &sdkErr) || sdkErr.StatusCode() != http.StatusConflict {
						return xerrors.Errorf("create scaletest chat provider: %w", err)
					}
					_, _ = fmt.Fprintf(inv.Stderr, "openai-compat provider already exists, proceeding...\n")
				} else {
					_, _ = fmt.Fprintf(inv.Stderr, "Created openai-compat provider pointing at %s\n", llmMockURL)
				}

				modelConfigs, err := experimentalClient.ListChatModelConfigs(testCtx)
				if err != nil {
					return xerrors.Errorf("list chat model configs: %w", err)
				}

				var existingModelConfig *codersdk.ChatModelConfig
				for i := range modelConfigs {
					if modelConfigs[i].Provider == "openai-compat" && modelConfigs[i].Model == scaletestModelName {
						existingModelConfig = &modelConfigs[i]
						break
					}
				}

				if existingModelConfig != nil {
					modelConfigID = &existingModelConfig.ID
					_, _ = fmt.Fprintf(inv.Stderr, "Reusing existing scaletest model config %s\n", existingModelConfig.ID)
				} else {
					enabled := true
					isDefault := false
					contextLimit := int64(4096)
					created, err := experimentalClient.CreateChatModelConfig(testCtx, codersdk.CreateChatModelConfigRequest{
						Provider:     "openai-compat",
						Model:        scaletestModelName,
						DisplayName:  scaletestModelDisplayName,
						Enabled:      &enabled,
						IsDefault:    &isDefault,
						ContextLimit: &contextLimit,
					})
					if err != nil {
						return xerrors.Errorf("create scaletest chat model config: %w", err)
					}
					modelConfigID = &created.ID
					_, _ = fmt.Fprintf(inv.Stderr, "Created scaletest model config %s\n", created.ID)
				}
			}

			runID := uuid.NewString()
			metricLabelValues := chat.MetricLabelValues(runID)

			reg := prometheus.NewRegistry()
			metrics := chat.NewMetrics(reg, chat.MetricLabelNames()...)

			logger := slog.Make(sloghuman.Sink(inv.Stdout)).Leveled(slog.LevelDebug)
			prometheusSrvClose := ServeHandler(ctx, logger, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), prometheusFlags.Address, "prometheus")
			defer prometheusSrvClose()

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
				// Wait for prometheus metrics to be scraped.
				_, _ = fmt.Fprintf(inv.Stderr, "Waiting %s for prometheus metrics to be scraped\n", prometheusFlags.Wait)
				<-time.After(prometheusFlags.Wait)
			}()
			tracer := tracerProvider.Tracer(scaletestTracerName)

			var (
				workspaceHarness         *harness.TestHarness
				workspaceHarnessFinished bool
				chatHarness              *harness.TestHarness
				chatHarnessFinished      bool
				workspaceIDs             []uuid.UUID
				results                  harness.Results
			)

			runErr := func() error {
				if workspaceSelection.WorkspaceBuildConfig != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "Creating %d workspaces from template %q before starting chats...\n", workspaceCount, workspaceSelection.TemplateName)
					workspaceHarness = harness.NewTestHarness(
						timeoutStrategy.wrapStrategy(harness.ConcurrentExecutionStrategy{}),
						cleanupStrategy.toStrategy(),
					)

					workspaceRunners := make([]*chatTemplateWorkspaceRunner, 0, int(workspaceCount))
					for i := range workspaceCount {
						runnerClient, err := loadtestutil.DupClientCopyingHeaders(client, BypassHeader)
						if err != nil {
							return xerrors.Errorf("duplicate client for workspace runner %d: %w", i, err)
						}
						runner := newChatTemplateWorkspaceRunner(runnerClient, *workspaceSelection.WorkspaceBuildConfig)
						workspaceRunners = append(workspaceRunners, runner)
						workspaceHarness.AddRun("workspace", fmt.Sprintf("workspace-%d", i), runner)
					}

					if err := workspaceHarness.Run(testCtx); err != nil {
						workspaceHarnessFinished = true
						return xerrors.Errorf("create template workspaces: %w", err)
					}
					workspaceHarnessFinished = true

					workspaceResults := workspaceHarness.Results()
					if workspaceResults.TotalFail > 0 {
						return xerrors.Errorf("workspace provisioning failed: %d/%d workspace builds failed", workspaceResults.TotalFail, workspaceResults.TotalRuns)
					}

					workspaceIDs, err = chatTemplateWorkspaceIDs(workspaceRunners)
					if err != nil {
						return err
					}
					if int64(len(workspaceIDs)) != workspaceCount {
						return xerrors.Errorf("workspace provisioning completed with %d workspaces, expected %d", len(workspaceIDs), workspaceCount)
					}
					_, _ = fmt.Fprintf(inv.Stderr, "Created %d workspaces from template %q\n", len(workspaceIDs), workspaceSelection.TemplateName)
				} else {
					workspaceIDs = []uuid.UUID{*workspaceSelection.WorkspaceID}
				}

				totalChats := int64(len(workspaceIDs)) * chatsPerWorkspace
				readyWG := &sync.WaitGroup{}
				startChan := make(chan struct{})
				var followUpReadyWG *sync.WaitGroup
				var startFollowUpChan chan struct{}
				if followUpStartDelay > 0 && turns > 1 {
					followUpReadyWG = &sync.WaitGroup{}
					startFollowUpChan = make(chan struct{})
				}

				chatHarness = harness.NewTestHarness(
					timeoutStrategy.wrapStrategy(harness.ConcurrentExecutionStrategy{}),
					cleanupStrategy.toStrategy(),
				)

				for workspaceIndex, targetWorkspaceID := range workspaceIDs {
					for chatIndex := range chatsPerWorkspace {
						readyWG.Add(1)
						if followUpReadyWG != nil {
							followUpReadyWG.Add(1)
						}
						cfg := chat.Config{
							RunID:                  runID,
							WorkspaceID:            targetWorkspaceID,
							Prompt:                 prompt,
							ModelConfigID:          modelConfigID,
							Turns:                  int(turns),
							FollowUpPrompt:         followUpPrompt,
							FollowUpStartDelay:     followUpStartDelay,
							ReadyWaitGroup:         readyWG,
							StartChan:              startChan,
							FollowUpReadyWaitGroup: followUpReadyWG,
							StartFollowUpChan:      startFollowUpChan,
							Metrics:                metrics,
							MetricLabelValues:      append([]string(nil), metricLabelValues...),
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
								spanName: "ChatRun",
							}
						}
						chatHarness.AddRun("chat", fmt.Sprintf("workspace-%d-chat-%d", workspaceIndex, chatIndex), runner)
					}
				}

				_, _ = fmt.Fprintf(inv.Stderr, "Starting chat scale test with %d chats across %d workspaces...\n", totalChats, len(workspaceIDs))
				done := make(chan error, 1)
				go func() {
					done <- chatHarness.Run(testCtx)
				}()

				waitForChatHarness := func() error {
					err := <-done
					chatHarnessFinished = true
					return err
				}

				readyDone := make(chan struct{})
				go func() {
					readyWG.Wait()
					close(readyDone)
				}()
				select {
				case <-readyDone:
					_, _ = fmt.Fprintf(inv.Stderr, "All %d chat runners ready across %d workspaces, starting chat storm...\n", totalChats, len(workspaceIDs))
				case <-time.After(5 * time.Minute):
					testCancel()
					if err := waitForChatHarness(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
						_, _ = fmt.Fprintf(inv.Stderr, "chat harness exited after readiness timeout: %+v\n", err)
					}
					return xerrors.Errorf("timed out waiting for chat runners to become ready")
				case <-testCtx.Done():
					if err := waitForChatHarness(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
						_, _ = fmt.Fprintf(inv.Stderr, "chat harness exited after context cancellation: %+v\n", err)
					}
					return testCtx.Err()
				}
				loadStartedAt := time.Now().UTC()
				close(startChan)

				var followUpPhaseReleasedAt *time.Time
				if followUpReadyWG != nil {
					followUpReadyDone := make(chan struct{})
					go func() {
						followUpReadyWG.Wait()
						close(followUpReadyDone)
					}()
					select {
					case <-followUpReadyDone:
						_, _ = fmt.Fprintf(inv.Stderr, "All %d chat runners finished the initial turn, waiting %s before follow-up turns...\n", totalChats, followUpStartDelay)
					case <-time.After(10 * time.Minute):
						testCancel()
						if err := waitForChatHarness(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
							_, _ = fmt.Fprintf(inv.Stderr, "chat harness exited after follow-up timeout: %+v\n", err)
						}
						return xerrors.Errorf("timed out waiting for chat runners to finish the initial turn")
					case <-testCtx.Done():
						if err := waitForChatHarness(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
							_, _ = fmt.Fprintf(inv.Stderr, "chat harness exited after context cancellation: %+v\n", err)
						}
						return testCtx.Err()
					}
					if followUpStartDelay > 0 {
						select {
						case <-testCtx.Done():
							if err := waitForChatHarness(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
								_, _ = fmt.Fprintf(inv.Stderr, "chat harness exited after context cancellation: %+v\n", err)
							}
							return testCtx.Err()
						case <-time.After(followUpStartDelay):
						}
					}
					releasedAt := time.Now().UTC()
					followUpPhaseReleasedAt = &releasedAt
					close(startFollowUpChan)
				}

				if err := waitForChatHarness(); err != nil {
					return xerrors.Errorf("run harness: %w", err)
				}
				loadCompletedAt := time.Now().UTC()

				results = chatHarness.Results()
				summaryCfg := chat.SummaryConfig{
					RunID:                 runID,
					WorkspaceID:           workspaceSelection.WorkspaceID,
					TemplateID:            workspaceSelection.TemplateID,
					TemplateName:          workspaceSelection.TemplateName,
					WorkspaceCount:        int64(len(workspaceIDs)),
					ChatsPerWorkspace:     chatsPerWorkspace,
					CreatedWorkspaceCount: int64(len(workspaceIDs)),
					ModelConfigID:         modelConfigID,
					Count:                 totalChats,
					Turns:                 int(turns),
					Prompt:                prompt,
					FollowUpPrompt:        followUpPrompt,
					FollowUpStartDelay:    followUpStartDelay,
					LLMMockURL:            llmMockURL,
					OutputSpecs:           scaletestOutputSpecs(outputs),
				}
				if workspaceSelection.WorkspaceBuildConfig != nil {
					summaryCfg.WorkspaceMode = chat.WorkspaceModeTemplate
				} else {
					summaryCfg.WorkspaceMode = chat.WorkspaceModeSharedWorkspace
					summaryCfg.CreatedWorkspaceCount = 0
				}
				summary := chat.NewSummary(summaryCfg, results, loadStartedAt, loadCompletedAt, followUpPhaseReleasedAt)
				summaryJSON, err := summary.CompactJSON()
				if err != nil {
					return xerrors.Errorf("marshal chat scaletest summary: %w", err)
				}
				_, _ = fmt.Fprintf(inv.Stderr, "CHAT_SCALETEST_SUMMARY=%s\n", summaryJSON)
				if summaryOutput != "" {
					if err := summary.Write(summaryOutput); err != nil {
						return xerrors.Errorf("write chat scaletest summary to %q: %w", summaryOutput, err)
					}
					_, _ = fmt.Fprintf(inv.Stderr, "Wrote chat scaletest summary to %s\n", summaryOutput)
				}

				for _, o := range outputs {
					if err := o.write(results, inv.Stdout); err != nil {
						return xerrors.Errorf("write output %q to %q: %w", o.format, o.path, err)
					}
				}

				return nil
			}()
			cleanupChatHarness := chatHarness
			if !chatHarnessFinished {
				cleanupChatHarness = nil
			}
			cleanupWorkspaceHarness := workspaceHarness
			if !workspaceHarnessFinished {
				cleanupWorkspaceHarness = nil
			}
			cleanupErr := cleanupChatScaleTestResources(ctx, cleanupStrategy, inv.Stderr, cleanupChatHarness, cleanupWorkspaceHarness)
			if cleanupErr != nil {
				if runErr != nil {
					return errors.Join(runErr, cleanupErr)
				}
				return cleanupErr
			}
			if runErr != nil {
				return runErr
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
			Description: "Text prompt to send for the first turn in each chat.",
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
			Flag:        "follow-up-prompt",
			Description: "Text prompt to send for follow-up turns (turns 2 through N).",
			Default:     "Continue.",
			Value:       serpent.StringOf(&followUpPrompt),
		},
		{
			Flag:        "follow-up-start-delay",
			Description: "Delay between the first completed turn and the release of turns 2 through N.",
			Default:     "0s",
			Value:       serpent.DurationOf(&followUpStartDelay),
		},
		{
			Flag:        "llm-mock-url",
			Description: "URL of the mock LLM server (e.g. http://127.0.0.1:8080/v1). When set, bootstraps an openai-compat chat provider and model config pointing at this URL.",
			Value:       serpent.StringOf(&llmMockURL),
		},
		{
			Flag:        "summary-output",
			Description: "Optional file path for a compact chat scaletest run summary JSON artifact.",
			Value:       serpent.StringOf(&summaryOutput),
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

func validateChatWorkspaceSelection(workspaceID string, template string, workspaceCount int64, chatsPerWorkspace int64) error {
	switch {
	case workspaceID == "" && template == "":
		return xerrors.Errorf("exactly one of --workspace-id or --template is required")
	case workspaceID != "" && template != "":
		return xerrors.Errorf("--workspace-id and --template are mutually exclusive")
	}

	if chatsPerWorkspace < 1 {
		return xerrors.Errorf("--chats-per-workspace must be at least 1")
	}

	if template != "" {
		if workspaceCount < 1 {
			return xerrors.Errorf("--workspace-count must be at least 1 when --template is set")
		}
		return nil
	}

	if workspaceCount != 0 {
		return xerrors.Errorf("--workspace-count may only be used with --template")
	}

	return nil
}

func resolveChatWorkspaceSelection(ctx context.Context, inv *serpent.Invocation, client *codersdk.Client, organizationIDs []uuid.UUID, workspaceID string, template string, parameterFlags workspaceParameterFlags) (chatWorkspaceSelection, error) {
	if workspaceID != "" {
		wsID, err := uuid.Parse(workspaceID)
		if err != nil {
			return chatWorkspaceSelection{}, xerrors.Errorf("parse workspace-id: %w", err)
		}
		ws, err := client.Workspace(ctx, wsID)
		if err != nil {
			return chatWorkspaceSelection{}, xerrors.Errorf("fetch workspace: %w", err)
		}
		if !workspaceHasAgents(ws) {
			return chatWorkspaceSelection{}, xerrors.Errorf("workspace %s has no agents in its latest build", ws.Name)
		}
		sharedWorkspaceID := wsID
		return chatWorkspaceSelection{
			WorkspaceID: &sharedWorkspaceID,
		}, nil
	}

	tpl, err := parseTemplate(ctx, client, organizationIDs, template)
	if err != nil {
		return chatWorkspaceSelection{}, xerrors.Errorf("parse template: %w", err)
	}

	workspaceCfg, err := prepareChatTemplateWorkspaceConfig(inv, client, tpl, parameterFlags)
	if err != nil {
		return chatWorkspaceSelection{}, err
	}

	templateID := tpl.ID
	return chatWorkspaceSelection{
		TemplateID:           &templateID,
		TemplateName:         tpl.Name,
		WorkspaceBuildConfig: &workspaceCfg,
	}, nil
}

func prepareChatTemplateWorkspaceConfig(inv *serpent.Invocation, client *codersdk.Client, template codersdk.Template, parameterFlags workspaceParameterFlags) (workspacebuild.Config, error) {
	cliRichParameters, err := asWorkspaceBuildParameters(parameterFlags.richParameters)
	if err != nil {
		return workspacebuild.Config{}, xerrors.Errorf("can't parse given parameter values: %w", err)
	}

	// Scaletest commands should not stop for interactive parameter prompts.
	// Accept template defaults unless the caller overrides them explicitly.
	richParameters, err := prepWorkspaceBuild(inv, client, prepWorkspaceBuildArgs{
		Action:               WorkspaceCreate,
		TemplateVersionID:    template.ActiveVersionID,
		NewWorkspaceName:     "scaletest-chat-N",
		Owner:                codersdk.Me,
		RichParameterFile:    parameterFlags.richParameterFile,
		RichParameters:       cliRichParameters,
		UseParameterDefaults: true,
	})
	if err != nil {
		return workspacebuild.Config{}, xerrors.Errorf("prepare build: %w", err)
	}

	cfg := workspacebuild.Config{
		OrganizationID: template.OrganizationID,
		UserID:         codersdk.Me,
		Request: codersdk.CreateWorkspaceRequest{
			TemplateID:          template.ID,
			RichParameterValues: richParameters,
		},
	}
	if err := cfg.Validate(); err != nil {
		return workspacebuild.Config{}, xerrors.Errorf("validate workspace config: %w", err)
	}

	return cfg, nil
}

func workspaceHasAgents(workspace codersdk.Workspace) bool {
	for _, resource := range workspace.LatestBuild.Resources {
		if len(resource.Agents) > 0 {
			return true
		}
	}
	return false
}

func scaletestOutputSpecs(outputs []scaleTestOutput) []string {
	specs := make([]string, 0, len(outputs))
	for _, output := range outputs {
		spec := string(output.format)
		if output.path != "-" {
			spec += ":" + output.path
		}
		specs = append(specs, spec)
	}
	return specs
}

func cleanupChatScaleTestResources(ctx context.Context, cleanupStrategy *scaletestStrategyFlags, stderr io.Writer, chatHarness *harness.TestHarness, workspaceHarness *harness.TestHarness) error {
	if chatHarness == nil && workspaceHarness == nil {
		return nil
	}

	cleanupCtx, cleanupCancel := cleanupStrategy.toContext(ctx)
	defer cleanupCancel()

	var cleanupErr error
	if chatHarness != nil {
		_, _ = fmt.Fprintln(stderr, "Cleaning up (archiving chats)...")
		if err := chatHarness.Cleanup(cleanupCtx); err != nil {
			cleanupErr = errors.Join(cleanupErr, xerrors.Errorf("cleanup chats: %w", err))
		}
	}
	if workspaceHarness != nil {
		_, _ = fmt.Fprintln(stderr, "Cleaning up created workspaces...")
		if err := workspaceHarness.Cleanup(cleanupCtx); err != nil {
			cleanupErr = errors.Join(cleanupErr, xerrors.Errorf("cleanup workspaces: %w", err))
		}
	}

	return cleanupErr
}

type chatTemplateWorkspaceRunner struct {
	runner *workspacebuild.Runner

	mu          sync.Mutex
	workspaceID uuid.UUID
}

func newChatTemplateWorkspaceRunner(client *codersdk.Client, cfg workspacebuild.Config) *chatTemplateWorkspaceRunner {
	return &chatTemplateWorkspaceRunner{
		runner: workspacebuild.NewRunner(client, cfg),
	}
}

func (r *chatTemplateWorkspaceRunner) Run(ctx context.Context, id string, logs io.Writer) error {
	workspace, err := r.runner.RunReturningWorkspace(ctx, id, logs)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.workspaceID = workspace.ID
	return nil
}

func (r *chatTemplateWorkspaceRunner) Cleanup(ctx context.Context, id string, logs io.Writer) error {
	return r.runner.Cleanup(ctx, id, logs)
}

func (r *chatTemplateWorkspaceRunner) WorkspaceID() uuid.UUID {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.workspaceID
}

func chatTemplateWorkspaceIDs(runners []*chatTemplateWorkspaceRunner) ([]uuid.UUID, error) {
	workspaceIDs := make([]uuid.UUID, 0, len(runners))
	for i, runner := range runners {
		workspaceID := runner.WorkspaceID()
		if workspaceID == uuid.Nil {
			return nil, xerrors.Errorf("workspace runner %d did not record a created workspace ID", i)
		}
		workspaceIDs = append(workspaceIDs, workspaceID)
	}
	return workspaceIDs, nil
}

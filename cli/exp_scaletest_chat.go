//go:build !slim

package cli

import (
	"context"
	"fmt"
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
		count              int64
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
		Long:  "Creates N concurrent chats against either one shared pre-existing workspace or one workspace per chat from a template, then streams each conversation to completion.",
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			outputs, err := output.parse()
			if err != nil {
				return xerrors.Errorf("could not parse --output flags: %w", err)
			}

			if count < 1 {
				return xerrors.Errorf("--count must be at least 1")
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
			if err := validateChatWorkspaceSelection(workspaceID, template); err != nil {
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

			readyWG := &sync.WaitGroup{}
			startChan := make(chan struct{})
			var followUpReadyWG *sync.WaitGroup
			var startFollowUpChan chan struct{}
			if followUpStartDelay > 0 && turns > 1 {
				followUpReadyWG = &sync.WaitGroup{}
				startFollowUpChan = make(chan struct{})
			}

			th := harness.NewTestHarness(
				timeoutStrategy.wrapStrategy(harness.ConcurrentExecutionStrategy{}),
				cleanupStrategy.toStrategy(),
			)

			for i := range count {
				readyWG.Add(1)
				if followUpReadyWG != nil {
					followUpReadyWG.Add(1)
				}
				cfg := chat.Config{
					RunID:                  runID,
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
				if workspaceSelection.WorkspaceID != nil {
					cfg.WorkspaceID = *workspaceSelection.WorkspaceID
				}
				if workspaceSelection.WorkspaceBuildConfig != nil {
					cfg.Workspace = *workspaceSelection.WorkspaceBuildConfig
				}
				if err := cfg.Validate(); err != nil {
					return xerrors.Errorf("validate config for runner %d: %w", i, err)
				}

				runnerClient, err := loadtestutil.DupClientCopyingHeaders(client, BypassHeader)
				if err != nil {
					return xerrors.Errorf("duplicate client for runner %d: %w", i, err)
				}
				var runner harness.Runnable = chat.NewRunner(runnerClient, cfg)
				if tracingEnabled {
					runner = &runnableTraceWrapper{
						tracer:   tracer,
						runner:   runner,
						spanName: "ChatRun",
					}
				}
				th.AddRun("chat", fmt.Sprintf("chat-%d", i), runner)
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Starting chat scale test...")
			done := make(chan error, 1)
			go func() {
				done <- th.Run(testCtx)
			}()

			readyDone := make(chan struct{})
			go func() {
				readyWG.Wait()
				close(readyDone)
			}()
			select {
			case <-readyDone:
				_, _ = fmt.Fprintf(inv.Stderr, "All %d runners ready, starting chat storm...\n", count)
			case <-time.After(5 * time.Minute):
				return xerrors.Errorf("timed out waiting for runners to become ready")
			case <-testCtx.Done():
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
					_, _ = fmt.Fprintf(inv.Stderr, "All %d runners finished the initial turn, waiting %s before follow-up turns...\n", count, followUpStartDelay)
				case <-time.After(10 * time.Minute):
					return xerrors.Errorf("timed out waiting for runners to finish the initial turn")
				case <-testCtx.Done():
					return testCtx.Err()
				}
				if followUpStartDelay > 0 {
					select {
					case <-testCtx.Done():
						return testCtx.Err()
					case <-time.After(followUpStartDelay):
					}
				}
				releasedAt := time.Now().UTC()
				followUpPhaseReleasedAt = &releasedAt
				close(startFollowUpChan)
			}

			if err := <-done; err != nil {
				return xerrors.Errorf("run harness: %w", err)
			}
			loadCompletedAt := time.Now().UTC()

			res := th.Results()
			summaryCfg := chat.SummaryConfig{
				RunID:              runID,
				WorkspaceID:        workspaceSelection.WorkspaceID,
				TemplateID:         workspaceSelection.TemplateID,
				TemplateName:       workspaceSelection.TemplateName,
				ModelConfigID:      modelConfigID,
				Count:              count,
				Turns:              int(turns),
				Prompt:             prompt,
				FollowUpPrompt:     followUpPrompt,
				FollowUpStartDelay: followUpStartDelay,
				LLMMockURL:         llmMockURL,
				OutputSpecs:        scaletestOutputSpecs(outputs),
			}
			if workspaceSelection.WorkspaceBuildConfig != nil {
				summaryCfg.WorkspaceMode = chat.WorkspaceModeTemplate
				summaryCfg.CreatedWorkspaceCount = countCreatedTemplateWorkspaces(res)
			} else {
				summaryCfg.WorkspaceMode = chat.WorkspaceModeSharedWorkspace
			}
			summary := chat.NewSummary(summaryCfg, res, loadStartedAt, loadCompletedAt, followUpPhaseReleasedAt)
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
				if err := o.write(res, inv.Stdout); err != nil {
					return xerrors.Errorf("write output %q to %q: %w", o.format, o.path, err)
				}
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Cleaning up (archiving chats)...")
			cleanupCtx, cleanupCancel := cleanupStrategy.toContext(ctx)
			defer cleanupCancel()
			if err := th.Cleanup(cleanupCtx); err != nil {
				return xerrors.Errorf("cleanup: %w", err)
			}

			if res.TotalFail > 0 {
				return xerrors.Errorf("scale test failed: %d/%d runs failed", res.TotalFail, res.TotalRuns)
			}
			_, _ = fmt.Fprintf(inv.Stderr, "Scale test passed: %d/%d runs succeeded\n", res.TotalPass, res.TotalRuns)
			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "count",
			Description: "Number of concurrent chats to create.",
			Default:     "10",
			Value:       serpent.Int64Of(&count),
		},
		{
			Flag:        "workspace-id",
			Description: "UUID of the pre-existing workspace to create chats against. Mutually exclusive with --template.",
			Value:       serpent.StringOf(&workspaceID),
		},
		{
			Flag:        "template",
			Description: "Template name or UUID. When set, each runner creates one workspace and waits at the start barrier before the chat storm begins.",
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

func validateChatWorkspaceSelection(workspaceID string, template string) error {
	switch {
	case workspaceID == "" && template == "":
		return xerrors.Errorf("exactly one of --workspace-id or --template is required")
	case workspaceID != "" && template != "":
		return xerrors.Errorf("--workspace-id and --template are mutually exclusive")
	default:
		return nil
	}
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

func countCreatedTemplateWorkspaces(results harness.Results) int64 {
	var count int64
	for _, run := range results.Runs {
		if run.Metrics == nil {
			continue
		}
		workspaceID, ok := run.Metrics["workspace_id"].(string)
		if !ok || workspaceID == "" || workspaceID == uuid.Nil.String() {
			continue
		}
		count++
	}
	return count
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

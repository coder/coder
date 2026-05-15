//go:build !slim

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
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

type chatCommandConfig struct {
	WorkspaceCount     int64
	ChatsPerWorkspace  int64
	WorkspaceID        string
	Template           string
	Prompt             string
	Turns              int64
	FollowUpPrompt     string
	FollowUpStartDelay time.Duration
	LLMMockURL         string
}

func (c chatCommandConfig) Validate() error {
	if c.Turns < 1 {
		return xerrors.Errorf("--turns must be at least 1")
	}
	if c.FollowUpStartDelay > 0 && c.Turns < 2 {
		return xerrors.Errorf("--follow-up-start-delay requires --turns to be at least 2")
	}

	switch {
	case c.WorkspaceID == "" && c.Template == "":
		return xerrors.Errorf("exactly one of --workspace-id or --template is required")
	case c.WorkspaceID != "" && c.Template != "":
		return xerrors.Errorf("--workspace-id and --template are mutually exclusive")
	}
	if c.ChatsPerWorkspace < 1 {
		return xerrors.Errorf("--chats-per-workspace must be at least 1")
	}
	if c.Template != "" {
		if c.WorkspaceCount < 1 {
			return xerrors.Errorf("--workspace-count must be at least 1 when --template is set")
		}
		return nil
	}
	if c.WorkspaceCount != 0 {
		return xerrors.Errorf("--workspace-count may only be used with --template")
	}
	return nil
}

type chatWorkspaceSelection struct {
	WorkspaceID          *uuid.UUID
	TemplateName         string
	WorkspaceBuildConfig *workspacebuild.Config
}

type chatRunnerBarriers struct {
	readyWaitGroup         *sync.WaitGroup
	startChan              chan struct{}
	followUpReadyWaitGroup *sync.WaitGroup
	startFollowUpChan      chan struct{}
}

func newChatRunnerBarriers(cfg chatCommandConfig) chatRunnerBarriers {
	barriers := chatRunnerBarriers{
		readyWaitGroup: &sync.WaitGroup{},
		startChan:      make(chan struct{}),
	}
	if cfg.FollowUpStartDelay > 0 && cfg.Turns > 1 {
		barriers.followUpReadyWaitGroup = &sync.WaitGroup{}
		barriers.startFollowUpChan = make(chan struct{})
	}
	return barriers
}

func (b *chatRunnerBarriers) addRunner() {
	b.readyWaitGroup.Add(1)
	if b.followUpReadyWaitGroup != nil {
		b.followUpReadyWaitGroup.Add(1)
	}
}

func prepareChatWorkspaceIDs(testCtx context.Context, inv *serpent.Invocation, client *codersdk.Client, selection chatWorkspaceSelection, cfg chatCommandConfig, timeoutStrategy *timeoutFlags, cleanupStrategy *scaletestStrategyFlags) ([]uuid.UUID, *harness.TestHarness, error) {
	if selection.WorkspaceBuildConfig == nil {
		return []uuid.UUID{*selection.WorkspaceID}, nil, nil
	}
	return precreateTemplateWorkspaces(testCtx, inv, client, selection, cfg, timeoutStrategy, cleanupStrategy)
}

func precreateTemplateWorkspaces(testCtx context.Context, inv *serpent.Invocation, client *codersdk.Client, selection chatWorkspaceSelection, cfg chatCommandConfig, timeoutStrategy *timeoutFlags, cleanupStrategy *scaletestStrategyFlags) ([]uuid.UUID, *harness.TestHarness, error) {
	_, _ = fmt.Fprintf(inv.Stderr, "Creating %d workspaces from template %q before starting chats...\n", cfg.WorkspaceCount, selection.TemplateName)
	workspaceHarness := harness.NewTestHarness(
		timeoutStrategy.wrapStrategy(harness.ConcurrentExecutionStrategy{}),
		cleanupStrategy.toStrategy(),
	)

	workspaceRunners := make([]*workspacebuild.Runner, 0, int(cfg.WorkspaceCount))
	for i := int64(0); i < cfg.WorkspaceCount; i++ {
		runnerClient, err := loadtestutil.DupClientCopyingHeaders(client, BypassHeader)
		if err != nil {
			return nil, nil, xerrors.Errorf("duplicate client for workspace runner %d: %w", i, err)
		}
		runner := workspacebuild.NewRunner(runnerClient, *selection.WorkspaceBuildConfig)
		workspaceRunners = append(workspaceRunners, runner)
		workspaceHarness.AddRun("workspace", fmt.Sprintf("workspace-%d", i), runner)
	}

	if err := workspaceHarness.Run(testCtx); err != nil {
		return nil, workspaceHarness, xerrors.Errorf("create template workspaces: %w", err)
	}

	workspaceResults := workspaceHarness.Results()
	if workspaceResults.TotalFail > 0 {
		return nil, workspaceHarness, xerrors.Errorf("workspace provisioning failed: %d/%d workspace builds failed", workspaceResults.TotalFail, workspaceResults.TotalRuns)
	}

	workspaceIDs := make([]uuid.UUID, 0, len(workspaceRunners))
	for i, runner := range workspaceRunners {
		createdWorkspaceID := runner.WorkspaceID()
		if createdWorkspaceID == uuid.Nil {
			return nil, workspaceHarness, xerrors.Errorf("workspace runner %d did not record a created workspace ID", i)
		}
		workspaceIDs = append(workspaceIDs, createdWorkspaceID)
	}
	if int64(len(workspaceIDs)) != cfg.WorkspaceCount {
		return nil, workspaceHarness, xerrors.Errorf("workspace provisioning completed with %d workspaces, expected %d", len(workspaceIDs), cfg.WorkspaceCount)
	}
	_, _ = fmt.Fprintf(inv.Stderr, "Created %d workspaces from template %q\n", len(workspaceIDs), selection.TemplateName)
	return workspaceIDs, workspaceHarness, nil
}

func buildChatHarness(client *codersdk.Client, tracer trace.Tracer, timeoutStrategy *timeoutFlags, cleanupStrategy *scaletestStrategyFlags, workspaceIDs []uuid.UUID, cfg chatCommandConfig, runID string, modelConfigID *uuid.UUID, metrics *chat.Metrics, metricLabels []string) (*harness.TestHarness, chatRunnerBarriers, error) {
	barriers := newChatRunnerBarriers(cfg)
	chatHarness := harness.NewTestHarness(
		timeoutStrategy.wrapStrategy(harness.ConcurrentExecutionStrategy{}),
		cleanupStrategy.toStrategy(),
	)

	for workspaceIndex, targetWorkspaceID := range workspaceIDs {
		for chatIndex := int64(0); chatIndex < cfg.ChatsPerWorkspace; chatIndex++ {
			if err := addChatRunner(chatHarness, client, tracer, workspaceIndex, chatIndex, targetWorkspaceID, cfg, runID, modelConfigID, metrics, metricLabels, &barriers); err != nil {
				return nil, barriers, err
			}
		}
	}

	return chatHarness, barriers, nil
}

func addChatRunner(chatHarness *harness.TestHarness, client *codersdk.Client, tracer trace.Tracer, workspaceIndex int, chatIndex int64, targetWorkspaceID uuid.UUID, commandConfig chatCommandConfig, runID string, modelConfigID *uuid.UUID, metrics *chat.Metrics, metricLabels []string, barriers *chatRunnerBarriers) error {
	barriers.addRunner()
	cfg := chat.Config{
		RunID:                  runID,
		WorkspaceID:            targetWorkspaceID,
		Prompt:                 commandConfig.Prompt,
		ModelConfigID:          modelConfigID,
		Turns:                  int(commandConfig.Turns),
		FollowUpPrompt:         commandConfig.FollowUpPrompt,
		FollowUpStartDelay:     commandConfig.FollowUpStartDelay,
		ReadyWaitGroup:         barriers.readyWaitGroup,
		StartChan:              barriers.startChan,
		FollowUpReadyWaitGroup: barriers.followUpReadyWaitGroup,
		StartFollowUpChan:      barriers.startFollowUpChan,
		Metrics:                metrics,
		MetricLabelValues:      slices.Clone(metricLabels),
	}
	if err := cfg.Validate(); err != nil {
		return xerrors.Errorf("validate config for workspace %d chat %d: %w", workspaceIndex, chatIndex, err)
	}

	runnerClient, err := loadtestutil.DupClientCopyingHeaders(client, BypassHeader)
	if err != nil {
		return xerrors.Errorf("duplicate client for workspace %d chat %d: %w", workspaceIndex, chatIndex, err)
	}
	var runner harness.Runnable = chat.NewRunner(runnerClient, cfg)
	if tracer != nil {
		runner = &runnableTraceWrapper{
			tracer:   tracer,
			runner:   runner,
			spanName: fmt.Sprintf("chat/workspace-%d-chat-%d", workspaceIndex, chatIndex),
		}
	}
	chatHarness.AddRun("chat", fmt.Sprintf("workspace-%d-chat-%d", workspaceIndex, chatIndex), runner)
	return nil
}

func runChatHarness(testCtx context.Context, stderr io.Writer, chatHarness *harness.TestHarness, cfg chatCommandConfig, barriers chatRunnerBarriers, workspaceCount int) (harness.Results, error) {
	totalChats := int64(workspaceCount) * cfg.ChatsPerWorkspace
	_, _ = fmt.Fprintf(stderr, "Starting chat scale test with %d chats across %d workspaces...\n", totalChats, workspaceCount)
	done := make(chan error, 1)
	go func() {
		done <- chatHarness.Run(testCtx)
	}()

	waitForChatHarness := func() error {
		return <-done
	}
	drainChatHarnessAfterCancel := func() (harness.Results, error) {
		if err := waitForChatHarness(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			_, _ = fmt.Fprintf(stderr, "chat harness exited after context cancellation: %+v\n", err)
		}
		return harness.Results{}, testCtx.Err()
	}

	if err := waitForWaitGroup(testCtx, barriers.readyWaitGroup); err != nil {
		return drainChatHarnessAfterCancel()
	}
	_, _ = fmt.Fprintf(stderr, "All %d chat runners ready across %d workspaces, starting chat storm...\n", totalChats, workspaceCount)
	close(barriers.startChan)

	if barriers.followUpReadyWaitGroup != nil {
		if err := waitForWaitGroup(testCtx, barriers.followUpReadyWaitGroup); err != nil {
			return drainChatHarnessAfterCancel()
		}
		_, _ = fmt.Fprintf(stderr, "All %d chat runners finished the initial turn, waiting %s before follow-up turns...\n", totalChats, cfg.FollowUpStartDelay)
		if err := waitForDurationOrContext(testCtx, cfg.FollowUpStartDelay); err != nil {
			return drainChatHarnessAfterCancel()
		}
		close(barriers.startFollowUpChan)
	}

	if err := waitForChatHarness(); err != nil {
		return harness.Results{}, xerrors.Errorf("run harness: %w", err)
	}

	return chatHarness.Results(), nil
}

func writeChatOutputs(outputs []scaleTestOutput, stdout io.Writer, results harness.Results) error {
	for _, o := range outputs {
		if err := o.write(results, stdout); err != nil {
			return xerrors.Errorf("write output %q to %q: %w", o.format, o.path, err)
		}
	}
	return nil
}

func waitForWaitGroup(ctx context.Context, waitGroup *sync.WaitGroup) error {
	done := make(chan struct{})
	go func() {
		waitGroup.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func waitForDurationOrContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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
		Long:  "Creates chats-per-workspace concurrent chats against each selected workspace. Use --workspace-id to target one existing workspace or --template with --workspace-count to pre-create workspaces before the chat storm begins. --chats-per-workspace must be at least 1.",
		Handler: func(inv *serpent.Invocation) error {
			cfg := chatCommandConfig{
				WorkspaceCount:     workspaceCount,
				ChatsPerWorkspace:  chatsPerWorkspace,
				WorkspaceID:        workspaceID,
				Template:           template,
				Prompt:             prompt,
				Turns:              turns,
				FollowUpPrompt:     followUpPrompt,
				FollowUpStartDelay: followUpStartDelay,
				LLMMockURL:         llmMockURL,
			}
			return r.runChatScaleTest(inv, cfg, parameterFlags, tracingFlags, prometheusFlags, timeoutStrategy, cleanupStrategy, output)
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

func (r *RootCmd) runChatScaleTest(inv *serpent.Invocation, cfg chatCommandConfig, parameterFlags workspaceParameterFlags, tracingFlags *scaletestTracingFlags, prometheusFlags *scaletestPrometheusFlags, timeoutStrategy *timeoutFlags, cleanupStrategy *scaletestStrategyFlags, output *scaletestOutputFlags) (runErr error) {
	baseCtx := inv.Context()
	ctx, stop := inv.SignalNotifyContext(baseCtx, StopSignals...)
	defer stop()

	outputs, err := output.parse()
	if err != nil {
		return xerrors.Errorf("could not parse --output flags: %w", err)
	}
	if err := cfg.Validate(); err != nil {
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

	workspaceSelection, err := resolveChatWorkspaceSelection(testCtx, inv, client, me.OrganizationIDs, cfg.WorkspaceID, cfg.Template, parameterFlags)
	if err != nil {
		return err
	}

	experimentalClient := codersdk.NewExperimentalClient(client)
	var modelConfigID *uuid.UUID
	if cfg.LLMMockURL != "" {
		modelConfigID, err = chat.EnsureScaletestModelConfig(testCtx, experimentalClient, inv.Stderr, cfg.LLMMockURL)
		if err != nil {
			return err
		}
	}

	runID := uuid.NewString()
	metricLabelValues := chat.MetricLabelValues(runID)
	reg := prometheus.NewRegistry()
	metrics := chat.NewMetrics(reg, chat.MetricLabelNames()...)

	logger := slog.Make(sloghuman.Sink(inv.Stderr)).Leveled(slog.LevelDebug)
	prometheusSrvClose := ServeHandler(baseCtx, logger, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}), prometheusFlags.Address, "prometheus")
	defer prometheusSrvClose()

	tracerProvider, closeTracing, tracingEnabled, err := tracingFlags.provider(baseCtx)
	if err != nil {
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
	}()

	var tracer trace.Tracer
	if tracingEnabled {
		tracer = tracerProvider.Tracer(scaletestTracerName)
	}

	var (
		workspaceHarness *harness.TestHarness
		chatHarness      *harness.TestHarness
	)
	defer func() {
		cleanupErr := cleanupChatScaleTestResources(cleanupStrategy, inv.Stderr, chatHarness, workspaceHarness)
		if cleanupErr != nil {
			runErr = errors.Join(runErr, cleanupErr)
		}
	}()

	workspaceIDs, workspaceHarness, err := prepareChatWorkspaceIDs(testCtx, inv, client, workspaceSelection, cfg, timeoutStrategy, cleanupStrategy)
	if err != nil {
		return err
	}

	chatHarness, barriers, err := buildChatHarness(client, tracer, timeoutStrategy, cleanupStrategy, workspaceIDs, cfg, runID, modelConfigID, metrics, metricLabelValues)
	if err != nil {
		return err
	}

	results, err := runChatHarness(testCtx, inv.Stderr, chatHarness, cfg, barriers, len(workspaceIDs))
	if err != nil {
		return err
	}
	if err := writeChatOutputs(outputs, inv.Stdout, results); err != nil {
		return err
	}
	if results.TotalFail > 0 {
		return xerrors.Errorf("scale test failed: %d/%d runs failed", results.TotalFail, results.TotalRuns)
	}

	_, _ = fmt.Fprintf(inv.Stderr, "Scale test passed: %d/%d runs succeeded\n", results.TotalPass, results.TotalRuns)
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
		return chatWorkspaceSelection{WorkspaceID: &sharedWorkspaceID}, nil
	}

	tpl, err := parseTemplate(ctx, client, organizationIDs, template)
	if err != nil {
		return chatWorkspaceSelection{}, xerrors.Errorf("parse template: %w", err)
	}

	workspaceCfg, err := prepareChatTemplateWorkspaceConfig(inv, client, tpl, parameterFlags)
	if err != nil {
		return chatWorkspaceSelection{}, err
	}

	return chatWorkspaceSelection{
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
		NewWorkspaceName:     "scaletest-chat",
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
	return slices.ContainsFunc(workspace.LatestBuild.Resources, func(resource codersdk.WorkspaceResource) bool {
		return len(resource.Agents) > 0
	})
}

func cleanupChatScaleTestResources(cleanupStrategy *scaletestStrategyFlags, stderr io.Writer, chatHarness *harness.TestHarness, workspaceHarness *harness.TestHarness) error {
	if chatHarness == nil && workspaceHarness == nil {
		return nil
	}

	cleanupCtx, cleanupCancel := cleanupStrategy.toContext(context.Background())
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

package chatd

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/agentselect"
	"github.com/coder/coder/v2/coderd/x/chatd/chatadvisor"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatopenai"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatsanitize"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	skillspkg "github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// turnEnvironment is everything one generation step needs: the chat
// snapshot, model call, prompt, toolset, and compaction inputs.
type turnEnvironment struct {
	Chat     database.Chat
	Messages []database.ChatMessage

	Model              chatprovider.Model
	Prompt             []fantasy.Message
	Tools              []fantasy.AgentTool
	ActiveTools        []string
	AllowInactiveTools map[string]bool
	ProviderTools      []chatloop.ProviderTool
	ModelBuildOptions  modelBuildOptions

	// ResolvedProvider is the configured provider identity used to label
	// user-facing errors. See chatloop.GenerateAssistantOptions.ErrorProvider.
	ResolvedProvider    string
	ReasoningProvenance reasoningProvenance
	// ReasoningRejected reports whether the provider rejected the
	// encrypted reasoning replayed into this step's model call.
	ReasoningRejected *atomic.Bool

	StageModel chatloop.StageModel

	ModelConfigID        uuid.UUID
	CallTemplate         fantasy.Call
	ContextLimitFallback int64

	DynamicToolNames   map[string]bool
	StopAfterTools     map[string]struct{}
	ExclusiveToolNames map[string]bool
	BuiltinToolNames   map[string]bool
	ToolNameToConfigID map[string]uuid.UUID

	MaxSteps   int
	Compaction *generationCompaction

	Debug *generationDebug

	cleanup func()
}

// Close releases the MCP connections and workspace state the step opened.
func (p turnEnvironment) Close() {
	if p.cleanup != nil {
		p.cleanup()
	}
}

// stepReasoningProvenance returns the provenance to stamp on the step's
// reasoning. A rejection starts the provider's next epoch, so later steps
// leave out the reasoning the rejected call replayed.
func (p turnEnvironment) stepReasoningProvenance() reasoningProvenance {
	provenance := p.ReasoningProvenance
	if p.ReasoningRejected != nil && p.ReasoningRejected.Load() {
		provenance.Epoch++
	}
	return provenance
}

// effectiveMCPServerConfigs loads the chat's stored selection plus
// owner-readable Force On configs at generation time, so stored lists
// predating enforcement cannot dodge the policy (Cure53 CDM-02-010).
// Explore chats keep their immutable spawn-time snapshot instead.
func (p *Server) effectiveMCPServerConfigs(
	ctx context.Context,
	chat database.Chat,
) ([]database.MCPServerConfig, error) {
	var configs []database.MCPServerConfig
	if len(chat.MCPServerIDs) > 0 {
		var err error
		configs, err = enabledMCPServerConfigsForChatOrg(ctx, p.db, chat.OrganizationID, chat.MCPServerIDs)
		if err != nil {
			return nil, err
		}
	}
	if isExploreSubagentMode(chat.Mode) {
		return configs, nil
	}
	forced, err := forcedMCPServerConfigsForOwner(ctx, p.db, chat.OrganizationID, chat.OwnerID)
	if err != nil {
		// Fail closed: running the turn without the forced set would
		// silently bypass a security policy.
		return nil, err
	}
	seen := make(map[uuid.UUID]struct{}, len(configs))
	for _, cfg := range configs {
		seen[cfg.ID] = struct{}{}
	}
	for _, cfg := range forced {
		if _, ok := seen[cfg.ID]; !ok {
			configs = append(configs, cfg)
		}
	}
	return configs, nil
}

func (p *Server) buildTurnEnvironment(
	ctx context.Context,
	input generationPrepareInput,
) (env turnEnvironment, err error) {
	chat := input.Chat
	logger := p.logger.With(
		slog.F("chat_id", chat.ID),
		slog.F("owner_id", chat.OwnerID),
	)

	prepStart := p.clock.Now()
	defer func() {
		if prepDuration := p.clock.Since(prepStart); prepDuration >= slowPrepareThreshold {
			logger.Warn(ctx, "slow generation preparation",
				slog.F("duration", prepDuration),
			)
		}
	}()

	var (
		promptRows       []database.ChatMessage
		mcpConfigs       []database.MCPServerConfig
		mcpTokens        []database.MCPServerUserToken
		inlineMCPServers []mcpclient.Server
		// mcpLoadFailures are servers that never reached the connect
		// step. They join the connect summaries so the debug panel shows
		// them next to connection failures.
		mcpLoadFailures []mcpclient.ConnectSummary
	)

	usesMCPServers := turnUsesMCPServers(chat)
	var g errgroup.Group
	g.Go(func() error {
		var err error
		promptRows, err = p.db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("get chat messages for prompt: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		mcpConfigs, err = p.effectiveMCPServerConfigs(ctx, chat)
		return err
	})
	if p.inlineMCPServersEnabled() {
		g.Go(func() error {
			var err error
			inlineMCPServers, mcpLoadFailures, err = p.loadInlineMCPServers(ctx, chat)
			return err
		})
	}
	var internalMCPServers []mcpclient.InProcessServer
	if usesMCPServers && len(p.internalMCPServers) > 0 {
		g.Go(func() error {
			var err error
			internalMCPServers, err = p.internalMCPServersForChat(ctx, chat)
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return turnEnvironment{}, err
	}

	apiKeyID, err := p.ensureSyntheticAPIKeyID(ctx, chat.OwnerID)
	if err != nil {
		return turnEnvironment{}, xerrors.Errorf("ensure synthetic API key: %w", err)
	}
	modelOpts := modelBuildOptions{ActiveAPIKeyID: apiKeyID}

	requestedEffort := chatRequestedEffort(chat)
	resolved, err := p.resolveModelCall(ctx, modelCallSpec{
		purpose:         "standard_turn",
		chat:            chat,
		requestedEffort: requestedEffort,
		buildOptions:    modelOpts,
	})
	if err != nil {
		return turnEnvironment{}, err
	}
	// The chat config keeps driving compaction, sanitization, and debug
	// attribution even when computer use swaps the resolved call below.
	modelConfig := resolved.dbConfig
	providerIdentity := modelConfigProviderIdentity(modelConfig, chatprovider.NormalizeProvider(string(resolved.route.Provider.Type)))

	// Computer-use turns swap in a specialized model, so the substitution
	// must happen before anything model-sensitive runs: file-part
	// classification, history sanitization, and provider option preparation
	// must all agree with the client actually used for the turn.
	isComputerUse := chat.Mode.Valid && chat.Mode.ChatMode == database.ChatModeComputerUse
	var computerUseProvider codersdk.ChatComputerUseProvider
	if isComputerUse {
		var cuModelProvider, cuModelName string
		computerUseProvider, cuModelProvider, cuModelName, err = p.computerUseProviderAndModelFromConfig(ctx)
		if err != nil {
			return turnEnvironment{}, xerrors.Errorf("resolve computer use provider and model: %w", err)
		}
		cuResolved, cuErr := p.resolveModelCall(ctx, modelCallSpec{
			purpose: "computer_use",
			chat:    chat,
			fixedModel: &fixedModelCall{
				providerType: cuModelProvider,
				modelName:    cuModelName,
				callConfig:   resolved.callConfig,
			},
			requestedEffort: requestedEffort,
			buildOptions:    modelOpts,
		})
		if cuErr != nil {
			return turnEnvironment{}, xerrors.Errorf(
				"resolve computer use model for provider %q model %q: %w",
				computerUseProvider,
				cuModelName,
				cuErr,
			)
		}
		resolved = cuResolved
	}
	model := resolved.model
	callConfig := resolved.callConfig
	reasoningSource := reasoningProvenance{
		ProviderIdentity: providerIdentity,
		Model:            model.ModelID(),
		Credential:       reasoningCredential(resolved.route.ProviderAuth),
	}
	modelRoute := resolved.route

	currentPlanMode := chat.PlanMode
	isPlanModeTurn := currentPlanMode.Valid && currentPlanMode.ChatPlanMode == database.ChatPlanModePlan
	isExploreSubagent := isExploreSubagentMode(chat.Mode)
	isRootChat := !chat.ParentChatID.Valid

	mcpConnectConfigs, approvedPlanMCPConfigIDs := filterExternalMCPConfigsForTurn(
		mcpConfigs,
		currentPlanMode,
		chat.ParentChatID,
	)
	// Root plan-mode chats allow inline servers selected by the caller
	// and internal servers selected by coderd.
	inlineMCPConnectServers, approvedInlineMCPServerIDs := filterMCPServersForTurn(
		inlineMCPServers,
		currentPlanMode,
		chat.ParentChatID,
		func(srv mcpclient.Server) (uuid.UUID, bool) { return srv.ID, true },
	)
	internalMCPConnectServers, approvedInternalMCPServerIDs := filterMCPServersForTurn(
		internalMCPServers,
		currentPlanMode,
		chat.ParentChatID,
		func(srv mcpclient.InProcessServer) (uuid.UUID, bool) { return srv.ID, true },
	)
	// All sets are nil outside plan mode, so this never writes to a nil
	// map. Every tool source below is filtered against the union.
	maps.Copy(approvedPlanMCPConfigIDs, approvedInlineMCPServerIDs)
	maps.Copy(approvedPlanMCPConfigIDs, approvedInternalMCPServerIDs)
	if !usesMCPServers {
		mcpConnectConfigs = nil
		inlineMCPConnectServers = nil
		approvedPlanMCPConfigIDs = map[uuid.UUID]struct{}{}
	}

	planModeInstructions, err := p.loadPlanModeInstructions(ctx, currentPlanMode)
	if err != nil {
		return turnEnvironment{}, err
	}

	var advisorRuntime *chatadvisor.Runtime
	if p.experiments.Enabled(codersdk.ExperimentChatAdvisor) && isRootChat && !isPlanModeTurn && !isExploreSubagent {
		advisorCfg, cfgErr := p.configCache.AdvisorConfig(ctx)
		if cfgErr != nil {
			return turnEnvironment{}, xerrors.Errorf("load advisor config: %w", cfgErr)
		}
		var advisorErr error
		advisorRuntime, advisorErr = p.newAdvisorRuntime(
			ctx,
			chat,
			advisorCfg,
			modelOpts,
			logger,
		)
		if advisorErr != nil {
			return turnEnvironment{}, advisorErr
		}
	}

	var advisorPromptSnapshot []fantasy.Message
	setAdvisorPromptSnapshot := func(msgs []fantasy.Message) {
		if advisorRuntime == nil {
			return
		}
		// The advisor reads the transcript as text. Replayed OpenAI
		// reasoning can be foreign to the advisor model, and textualized
		// tool calls would leave it without its required following item.
		advisorPromptSnapshot = dropOpenAIReasoningParts(ctx, logger, msgs)
	}

	currentChat := chat
	loadChatSnapshot := func(loadCtx context.Context, chatID uuid.UUID) (database.Chat, error) {
		return p.db.GetChatByID(loadCtx, chatID)
	}
	var chatStateMu sync.Mutex
	var workspaceMu sync.Mutex
	workspaceCtx := turnWorkspaceContext{
		server:           p,
		chatStateMu:      &chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: loadChatSnapshot,
	}
	// Once g2 starts, cleanup must wait for g2.Wait because its callbacks
	// are assigned by g2 goroutines.
	var mcpCleanup, inlineMCPCleanup, internalMCPCleanup func()
	cleanup := func() {
		if inlineMCPCleanup != nil {
			inlineMCPCleanup()
		}
		if internalMCPCleanup != nil {
			internalMCPCleanup()
		}
		if mcpCleanup != nil {
			mcpCleanup()
		}
		workspaceCtx.close()
	}
	defer func() {
		if err != nil {
			cleanup()
		}
	}()

	planPathFn := func(ctx context.Context) (string, string, error) {
		conn, err := workspaceCtx.getWorkspaceConn(ctx)
		if err != nil {
			return "", "", err
		}
		home, err := chattool.ResolveWorkspaceHome(ctx, conn)
		if err != nil {
			return "", "", err
		}
		return chattool.PlanPathForChat(home, chat.ID), home, nil
	}
	resolvePlanPathForTools := func(ctx context.Context) (string, string, error) {
		planCtx, cancel := context.WithTimeout(ctx, planPathLookupTimeout)
		defer cancel()
		return planPathFn(planCtx)
	}
	resolvePlanPathBlock := func(resolveCtx context.Context) string {
		if chat.ParentChatID.Valid {
			return ""
		}

		planCtx, cancel := context.WithTimeout(resolveCtx, planPathLookupTimeout)
		defer cancel()

		if _, _, err := workspaceCtx.workspaceAgentIDForConn(planCtx); err != nil {
			logger.Debug(resolveCtx, "plan path instruction: agent not reachable",
				slog.Error(err),
				slog.F("chat_id", chat.ID),
			)
			return ""
		}

		planPath, home, err := planPathFn(planCtx)
		if err != nil {
			logger.Debug(resolveCtx, "plan path instruction: failed to resolve plan path",
				slog.Error(err),
				slog.F("chat_id", chat.ID),
			)
			return ""
		}
		return formatPlanPathBlock(planPath, home)
	}

	var (
		prompt              []fantasy.Message
		instruction         string
		mcpTools            []fantasy.AgentTool
		mcpSummaries        []mcpclient.ConnectSummary
		inlineMCPTools      []fantasy.AgentTool
		inlineMCPSummaries  []mcpclient.ConnectSummary
		workspaceMCPTools   []fantasy.AgentTool
		workspaceSkills     []chattool.SkillMeta
		personalSkills      []skillspkg.Skill
		orgSkills           []skillspkg.Skill
		orgSkillReader      rbac.Subject
		projectSkills       []skillspkg.Skill
		usableProjectID     uuid.NullUUID
		resolvedUserPrompt  string
		projectInstructions string
		planPathBlock       string
	)

	// Drop provider-executed tool history produced by a different provider,
	// and OpenAI reasoning state produced by a different provider, model,
	// or credential, or before the latest rejection of that source's
	// encrypted reasoning, before building the prompt. A provider
	// that shares another's wire format (e.g. Bedrock and Anthropic) can
	// still reject the other's provider-specific blocks, so a mid-chat
	// provider switch must not replay them.
	//
	// The pending-user segment (trailing user rows the assistant has not
	// answered yet) is handled separately from here through prompt
	// assembly: it is excluded from the compaction summarizer input and
	// replayed verbatim after the compaction boundary. It must be located
	// on the unsanitized rows because sanitization can drop an assistant
	// row that separates an already-answered user message from the
	// pending tail; locating it afterwards would pull that answered
	// message into the tail. Sanitizing only the head is equivalent to
	// sanitizing everything, since the tail is user-only and the
	// sanitizer only rewrites assistant rows.
	pendingRowsStart := pendingUserSegmentStart(promptRows)
	reasoningSource.Epoch = currentReasoningEpoch(promptRows[:pendingRowsStart], reasoningSource)
	sanitizedHead, err := p.sanitizeForeignProviderStateRows(ctx, logger, promptRows[:pendingRowsStart], chat.OwnerID, reasoningSource)
	if err != nil {
		return turnEnvironment{}, err
	}
	promptRows = append(sanitizedHead[:len(sanitizedHead):len(sanitizedHead)], promptRows[pendingRowsStart:]...)
	pendingRowsStart = len(sanitizedHead)

	if chat.WorkspaceID.Valid {
		// Resolve the workspace agent so the chat row's AgentID and
		// BuildID bindings are up to date before the chatworker
		// decision helper inspects them. ensureWorkspaceAgent does a
		// DB lookup and lazily calls persistBuildAgentBinding when
		// the bound agent has changed, so this is a cheap metadata
		// refresh, not a workspace dial. It must not insert chat
		// history; only metadata is mutated here.
		agent, agentErr := workspaceCtx.getWorkspaceAgent(ctx)
		if agentErr != nil {
			if !isNoUsableWorkspaceAgent(agentErr) {
				return turnEnvironment{}, xerrors.Errorf("get workspace agent: %w", agentErr)
			}
			logger.Debug(ctx, "workspace agent lookup failed, instruction header stays undecorated", slog.Error(agentErr))
		}

		// API-created chats bind their agent lazily here, after
		// hydrateChatContextOnCreate ran with no agent. Pin the chat to the
		// bound agent's pushed snapshot now if it is still unpinned, so the
		// first turn reads workspace context instead of waiting for the
		// agent's next push. Idempotent and snapshot-gated; runs before the
		// pinned context is read below.
		p.ensureChatContextPinnedOnFirstTurn(ctx, workspaceCtx.currentChatSnapshot())

		var resolveErr error
		instruction, workspaceSkills, resolveErr = p.resolveTurnWorkspaceContext(ctx, chat, agent)
		if resolveErr != nil {
			return turnEnvironment{}, resolveErr
		}
	}

	// Build the debug context before the connect phase so its
	// outcomes can be recorded with run-creation context even when a
	// later preparation step fails.
	triggerMessageID, historyTipMessageID, triggerLabel := deriveChatDebugSeed(promptRows)
	debugSvc := p.existingDebugService()
	var debug *generationDebug
	if resolved.debugEnabled {
		if debugSvc == nil {
			return turnEnvironment{}, xerrors.New("chat debug service missing after enablement check")
		}
		debug = &generationDebug{
			Enabled:             true,
			Service:             debugSvc,
			Provider:            resolved.resolvedProvider,
			Model:               resolved.resolvedModel,
			TriggerMessageID:    triggerMessageID,
			HistoryTipMessageID: historyTipMessageID,
			TriggerLabel:        triggerLabel,
			ModelConfig:         modelConfig,
		}
	}

	var pendingPrompt []fantasy.Message
	var g2 errgroup.Group
	g2.Go(func() error {
		var err error
		// Key the file-part acceptance on model.Provider() (the fantasy
		// transport identity), not the configured provider, because
		// aibridge routing rewrites the provider (e.g. Bedrock to the
		// Anthropic transport). The conversion that actually drops or
		// accepts a file part is the one for model.Provider().
		acceptsFilePart := model.AcceptsFilePartMediaType
		providerType := string(modelRoute.Provider.Type)
		prompt, err = chatprompt.ConvertMessagesWithFiles(ctx, promptRows[:pendingRowsStart], p.chatFileResolver(providerType), logger, acceptsFilePart, chat.WorkspaceID)
		if err != nil {
			return xerrors.Errorf("build chat prompt: %w", err)
		}
		prompt = replaceUnsupportedToolMedia(ctx, logger, prompt, model, providerType)
		if pendingRowsStart < len(promptRows) {
			pendingPrompt, err = chatprompt.ConvertMessagesWithFiles(ctx, promptRows[pendingRowsStart:], p.chatFileResolver(providerType), logger, acceptsFilePart, chat.WorkspaceID)
			if err != nil {
				return xerrors.Errorf("build pending chat prompt tail: %w", err)
			}
		}
		return nil
	})
	g2.Go(func() error {
		var err error
		personalSkills, err = p.fetchPersonalSkillMetadata(ctx, chat.OwnerID)
		return err
	})
	g2.Go(func() error {
		var err error
		orgSkillReader, err = p.chatOwnerSubject(ctx, chat.OwnerID)
		if err != nil {
			return err
		}
		orgSkills, err = p.fetchOrganizationSkillMetadata(ctx, orgSkillReader, chat.OrganizationID)
		return err
	})
	g2.Go(func() error {
		var err error
		resolvedUserPrompt, err = p.resolveUserPrompt(ctx, chat.OwnerID)
		return err
	})
	// Project instructions and skills share one access check, so the chat
	// owner gets both or neither. Both fail open: the turn continues without
	// them.
	g2.Go(func() error {
		usableProjectID = p.resolveUsableChatProjectID(ctx, logger, chat)
		if !usableProjectID.Valid {
			return nil
		}
		projectInstructions = p.loadProjectInstructions(ctx, logger, usableProjectID.UUID)
		var err error
		projectSkills, err = p.fetchProjectSkillMetadata(ctx, usableProjectID.UUID)
		if err != nil {
			logger.Warn(ctx, "failed to load chat project skills",
				slog.F("project_id", usableProjectID.UUID),
				slog.Error(err),
			)
		}
		return nil
	})
	if len(mcpConnectConfigs) > 0 {
		g2.Go(func() error {
			var err error
			mcpTokens, err = p.db.GetMCPServerUserTokensByUserID(ctx, chat.OwnerID)
			if err != nil {
				return xerrors.Errorf("get MCP server user tokens: %w", err)
			}
			mcpTokens, err = p.refreshExpiredMCPTokens(ctx, logger, mcpConnectConfigs, mcpTokens)
			if err != nil {
				return err
			}
			mcpServers := make([]mcpclient.Server, 0, len(mcpConnectConfigs))
			var invalidConfigs []mcpclient.ConnectSummary
			for _, cfg := range mcpConnectConfigs {
				if !cfg.Enabled {
					continue
				}
				srv, err := mcpclient.ServerFromConfig(cfg)
				if err != nil {
					logger.Warn(ctx, "skipping MCP server with invalid config",
						slog.F("server_slug", cfg.Slug), slog.Error(err))
					invalidConfigs = append(invalidConfigs, mcpclient.ConnectSummary{
						ConfigID: cfg.ID,
						Slug:     cfg.Slug,
						Outcome:  mcpclient.ConnectOutcomeError,
						Error:    "invalid server config",
					})
					continue
				}
				mcpServers = append(mcpServers, srv)
			}
			connectCtx, connectSpan := p.stages.Start(ctx, chatloop.StageMCPConnect)
			mcpTools, mcpSummaries, mcpCleanup = mcpclient.ConnectAll(
				connectCtx,
				logger,
				mcpServers,
				mcpTokens,
				chat.OwnerID,
				p.oidcTokenSource,
				chatprovider.CoderHeaders(chat),
				p.mcpHTTPClient,
			)
			mcpSummaries = append(mcpSummaries, invalidConfigs...)
			connected, failed, connectErr := mcpConnectOutcome(mcpSummaries)
			connectSpan.SetAttributes(
				attribute.Int(chatloop.AttrMCPServersConnected, connected),
				attribute.Int(chatloop.AttrMCPServersFailed, failed),
			)
			connectSpan.End(connectErr)
			return nil
		})
	}
	if len(inlineMCPConnectServers) > 0 {
		g2.Go(func() error {
			inlineMCPTools, inlineMCPSummaries, inlineMCPCleanup = mcpclient.ConnectInline(
				ctx,
				logger,
				inlineMCPConnectServers,
				chatprovider.CoderHeaders(chat),
				p.mcpHTTPClient,
			)
			return nil
		})
	}
	var (
		internalMCPTools     []fantasy.AgentTool
		internalMCPSummaries []mcpclient.ConnectSummary
	)
	if len(internalMCPConnectServers) > 0 {
		g2.Go(func() error {
			internalMCPTools, internalMCPSummaries, internalMCPCleanup = mcpclient.ConnectInProcess(
				ctx,
				logger,
				internalMCPConnectServers,
			)
			return nil
		})
	}
	if chat.WorkspaceID.Valid && !isPlanModeTurn && !isExploreSubagent {
		g2.Go(func() error {
			var err error
			workspaceMCPTools, err = p.pinnedWorkspaceMCPTools(ctx, chat, workspaceCtx.getWorkspaceConn)
			return err
		})
	}
	// Resolve the per-chat plan path block in the parallel phase. It dials
	// the workspace agent to read the home directory, so running it here lets
	// the cold dial overlap with the rest of turn preparation instead of
	// blocking system prompt assembly on a sequential dial. Best-effort:
	// resolvePlanPathBlock logs and returns an empty block on failure.
	if chat.WorkspaceID.Valid && !chat.ParentChatID.Valid {
		g2.Go(func() error {
			planPathBlock = resolvePlanPathBlock(ctx)
			return nil
		})
	}
	g2Err := g2.Wait()
	mcpSummaries = append(mcpSummaries, inlineMCPSummaries...)
	mcpSummaries = append(mcpSummaries, internalMCPSummaries...)
	mcpSummaries = append(mcpSummaries, mcpLoadFailures...)
	// Record connect outcomes before acting on any preparation error:
	// ConnectAll has already run, so a failure below (or in g2 itself)
	// would otherwise discard this attempt's outcomes.
	if debug != nil && input.RecordMCPConnectSummaries != nil && len(mcpSummaries) > 0 {
		input.RecordMCPConnectSummaries(ctx, chat, debug, mcpSummaries)
	}
	if g2Err != nil {
		return turnEnvironment{}, g2Err
	}

	prompt, sanitizeStats := chatsanitize.SanitizeAnthropicProviderToolHistory(model.Provider(), prompt)
	chatsanitize.LogAnthropicProviderToolSanitization(
		ctx,
		logger,
		"persisted_history_replay",
		model.Provider(),
		model.ModelID(),
		sanitizeStats,
	)

	subagentInstruction := ""
	if !isRootChat {
		subagentInstruction = defaultSubagentInstruction
	}
	resolvedSkillsFor := func(workspaceSkills []chattool.SkillMeta) []skillspkg.ResolvedSkill {
		return mergeTurnSkills(personalSkills, orgSkills, projectSkills, workspaceSkills)
	}
	resolveSkillAlias := func(alias string) (skillspkg.ResolvedSkill, error) {
		return skillspkg.Lookup(resolvedSkillsFor(workspaceSkills), alias)
	}
	initialResolvedSkills := resolvedSkillsFor(workspaceSkills)
	memoryStore, memoryProjectName, hasMemory := p.resolveProjectMemory(ctx, chat)
	memoryIndex := ""
	if hasMemory {
		memoryIndex = chattool.FormatMemoryGuidance(memoryProjectName)
	}

	prompt = buildSystemPrompt(
		prompt,
		subagentInstruction,
		instruction,
		initialResolvedSkills,
		memoryIndex,
		projectInstructions,
		resolvedUserPrompt,
		systemPromptBehaviorContext{
			planMode:             currentPlanMode,
			chatMode:             chat.Mode,
			planModeInstructions: planModeInstructions,
			isRootChat:           isRootChat,
		},
	)
	if advisorRuntime != nil {
		prompt = chatprompt.InsertSystem(prompt, chatadvisor.ParentGuidanceBlock)
	}
	prompt = renderPlanPathPrompt(prompt, planPathBlock)
	compactionPromptMessages := prompt
	pendingUserRows := promptRows[pendingRowsStart:]
	if len(pendingUserRows) == 0 || len(pendingPrompt) == 0 {
		pendingUserRows = nil
	}
	// Full slice expression: appending the tail must not share the
	// head's backing array with the summarizer input.
	prompt = append(prompt[:len(prompt):len(prompt)], pendingPrompt...)
	if pendingUserRows == nil {
		compactionPromptMessages = prompt
	}
	setAdvisorPromptSnapshot(prompt)

	storeChatAttachment := p.newStoreChatAttachmentFunc(&workspaceCtx)
	tools := []fantasy.AgentTool{
		chattool.ReadFile(chattool.ReadFileOptions{GetWorkspaceConn: workspaceCtx.getWorkspaceConn}),
		chattool.WriteFile(chattool.WriteFileOptions{
			GetWorkspaceConn: workspaceCtx.getWorkspaceConn,
			ResolvePlanPath:  resolvePlanPathForTools,
			IsPlanTurn:       isPlanModeTurn,
		}),
		chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: workspaceCtx.getWorkspaceConn,
			ResolvePlanPath:  resolvePlanPathForTools,
			IsPlanTurn:       isPlanModeTurn,
		}),
		chattool.AttachFile(chattool.AttachFileOptions{
			GetWorkspaceConn: workspaceCtx.getWorkspaceConn,
			StoreFile:        storeChatAttachment,
		}),
		chattool.Execute(chattool.ExecuteOptions{
			GetWorkspaceConn:    workspaceCtx.getWorkspaceConn,
			AgentBrowserSession: chat.ID.String(),
		}),
		chattool.ProcessOutput(chattool.ProcessToolOptions{GetWorkspaceConn: workspaceCtx.getWorkspaceConn}),
		chattool.ProcessList(chattool.ProcessToolOptions{GetWorkspaceConn: workspaceCtx.getWorkspaceConn}),
		chattool.ProcessSignal(chattool.ProcessToolOptions{GetWorkspaceConn: workspaceCtx.getWorkspaceConn}),
	}
	if isPlanModeTurn && isRootChat {
		tools = append(tools, chattool.NewAskUserQuestionTool())
	}
	if isRootChat {
		tools, err = p.appendRootChatTools(ctx, tools, rootChatToolsOptions{
			chat:            chat,
			modelConfigID:   modelConfig.ID,
			workspaceCtx:    &workspaceCtx,
			workspaceMu:     &workspaceMu,
			resolvePlanPath: resolvePlanPathForTools,
			storeFile:       storeChatAttachment,
			isPlanModeTurn:  isPlanModeTurn,
		})
		if err != nil {
			return turnEnvironment{}, err
		}
	}

	skillOpts := chattool.ReadSkillOptions{
		GetWorkspaceConn: workspaceCtx.getWorkspaceConn,
		GetSkills: func() []chattool.SkillMeta {
			return workspaceSkills
		},
		ResolveAlias: resolveSkillAlias,
		LoadStoredSkillBody: func(ctx context.Context, skill skillspkg.Skill) (skillspkg.ParsedSkill, error) {
			switch skill.Source {
			case skillspkg.SourceOrganization:
				return p.loadOrganizationSkillBody(ctx, orgSkillReader, chat.OrganizationID, skill.Name, skillLoaderModel)
			case skillspkg.SourceProject:
				return p.loadProjectSkillBody(ctx, usableProjectID.UUID, skill.Name, skillLoaderModel)
			default:
				return p.loadPersonalSkillBody(ctx, chat.OwnerID, skill.Name, skillLoaderModel)
			}
		},
	}
	appendCurrentSkillTools := func(current []fantasy.AgentTool) ([]fantasy.AgentTool, bool) {
		if len(personalSkills) == 0 && len(orgSkills) == 0 && len(projectSkills) == 0 && len(workspaceSkills) == 0 {
			return current, false
		}
		updated := current
		changed := false
		appendTool := func(tool fantasy.AgentTool) {
			name := tool.Info().Name
			if slices.ContainsFunc(current, func(existing fantasy.AgentTool) bool {
				return existing.Info().Name == name
			}) {
				return
			}
			if !changed {
				updated = slices.Clone(current)
				changed = true
			}
			updated = append(updated, tool)
		}
		appendTool(chattool.ReadSkill(skillOpts))
		if len(workspaceSkills) > 0 {
			appendTool(chattool.ReadSkillFile(skillOpts))
		}
		return updated, changed
	}
	tools, _ = appendCurrentSkillTools(tools)
	if hasMemory {
		tools = append(tools, chattool.ReadMemory(memoryStore), chattool.SaveMemory(memoryStore, memoryProjectName), chattool.DeleteMemory(memoryStore, memoryProjectName), chattool.ConsolidateMemory(memoryStore, memoryProjectName))
	}
	if advisorRuntime != nil {
		tools = append(tools, chatadvisor.Tool(chatadvisor.ToolOptions{
			Runtime: advisorRuntime,
			GetConversationSnapshot: func() []fantasy.Message {
				return stripAdvisorGuidanceBlock(slices.Clone(advisorPromptSnapshot))
			},
		}))
	}

	var exclusiveToolNames map[string]bool
	if advisorRuntime != nil {
		exclusiveToolNames = map[string]bool{chatadvisor.ToolName: true}
	}

	builtinToolNames := make(map[string]bool, len(tools))
	for _, t := range tools {
		builtinToolNames[t.Info().Name] = true
	}

	// Append sources in trust order so less trusted tools cannot replace
	// trusted tools: built-in, internal, admin, workspace, then inline.
	// Apply the plan-mode filter after all sources join.
	tools = mcpclient.AppendTools(ctx, logger, tools, internalMCPTools)
	// Exclude rejected tools from deferral so a name collision cannot
	// hide the winning tool through the rejected tool's deferral policy.
	numTools := len(tools)
	tools = mcpclient.AppendTools(ctx, logger, tools, mcpTools)
	mcpTools = tools[numTools:]
	if !isExploreSubagent {
		numTools = len(tools)
		tools = mcpclient.AppendTools(ctx, logger, tools, workspaceMCPTools)
		workspaceMCPTools = tools[numTools:]
	}
	mcpConfigByID := make(map[uuid.UUID]database.MCPServerConfig, len(mcpConnectConfigs))
	for _, config := range mcpConnectConfigs {
		mcpConfigByID[config.ID] = config
	}
	deferredCandidates := collectDeferredMCPCandidates(deferredMCPCandidateInput{
		mcpTools:              mcpTools,
		workspaceMCPTools:     workspaceMCPTools,
		mcpConfigByID:         mcpConfigByID,
		planMode:              currentPlanMode,
		parentChatID:          chat.ParentChatID,
		approvedMCPConfigIDs:  approvedPlanMCPConfigIDs,
		includeWorkspaceTools: !isExploreSubagent,
	})
	tools = mcpclient.AppendTools(ctx, logger, tools, inlineMCPTools)
	tools = filterToolsForTurn(tools, currentPlanMode, chat.ParentChatID, approvedPlanMCPConfigIDs)
	// chatloop reads an empty active tool list as every tool, so Explore
	// children also drop the tools their allowlist blocks.
	if isExploreSubagent && !isRootChat {
		tools = slices.DeleteFunc(tools, func(tool fantasy.AgentTool) bool {
			return !exploreToolAllowed(tool)
		})
	}

	var dynamicToolNames map[string]bool
	if p.disableCallerSuppliedTools {
		if chat.DynamicTools.Valid {
			logger.Debug(ctx, "skipping dynamic tools: caller-supplied tools are disabled on this deployment")
		}
	} else {
		tools, dynamicToolNames, err = appendDynamicTools(ctx, logger, tools, chat.DynamicTools, currentPlanMode, chat.Mode)
		if err != nil {
			return turnEnvironment{}, err
		}
	}

	var providerTools []chatloop.ProviderTool
	if !isPlanModeTurn && callConfig.ProviderOptions != nil {
		providerTools = buildProviderTools(callConfig.ProviderOptions)
		if isExploreSubagent {
			if !chat.ParentChatID.Valid {
				providerTools = nil
			} else {
				providerTools = slices.DeleteFunc(providerTools, func(tool chatloop.ProviderTool) bool {
					return tool.Definition.GetName() != "web_search"
				})
			}
		}
	}

	if isComputerUse {
		providerTools, err = appendComputerUseProviderTool(providerTools, computerUseProviderToolOptions{
			provider:         computerUseProvider,
			isPlanModeTurn:   isPlanModeTurn,
			isComputerUse:    isComputerUse,
			getWorkspaceConn: workspaceCtx.getWorkspaceConn,
			storeFile:        storeChatAttachment,
			clock:            p.clock,
			logger:           p.logger.Named("computer_use"),
		})
		if err != nil {
			return turnEnvironment{}, xerrors.Errorf("register computer use provider tool for provider %q: %w", computerUseProvider, err)
		}
	} else {
		providerTools, err = appendComputerUseProviderTool(providerTools, computerUseProviderToolOptions{
			isPlanModeTurn: isPlanModeTurn,
			isComputerUse:  false,
		})
		if err != nil {
			return turnEnvironment{}, err
		}
	}

	activeToolNames := activeToolNamesForTurn(tools, currentPlanMode, chat.ParentChatID, approvedPlanMCPConfigIDs)
	if isExploreSubagent {
		activeToolNames = allowedExploreToolNames(tools)
	}
	var allowInactiveTools map[string]bool
	// The owner is the subject: only the owner posts turns and descendant
	// chats inherit it. Preparation runs for every step, so the first step
	// with MCP candidates decides and later steps of the same turn reuse
	// that decision; otherwise a rule change mid-turn would reject
	// find_tools calls the model already issued. Without candidates
	// find_tools is never offered, so skip the read.
	toolSearchEnabled := false
	if len(deferredCandidates) > 0 {
		toolSearchEnabled = input.TurnExperiments.mcpToolSearchEnabled(stopNudgeKey(input.Messages), func() bool {
			return p.experimentEvaluator.Enabled(ctx, chat.OwnerID, codersdk.ExperimentMCPToolSearch)
		})
	}
	if decideMCPToolSearch(mcpToolSearchInput{
		experimentEnabled: toolSearchEnabled,
		candidates:        deferredCandidates,
		dynamicToolNames:  dynamicToolNames,
	}) {
		activationTokenBudget := float64(modelConfig.ContextLimit) / mcpToolSearchBudgetDivisor
		findTools := chattool.FindTools(chattool.FindToolsOptions{
			Entries:            deferredMCPToolEntries(deferredCandidates),
			SchemaTokenBudget:  activationTokenBudget,
			CatalogTokenBudget: activationTokenBudget,
			// Calls total is counted in executeLocalTools, which also
			// sees calls rejected before the tool runs; OnCall covers
			// only calls that reach the handler or its decode.
			OnCall: func(callCtx context.Context, call chattool.FindToolsCall) {
				if call.Rejection == "" {
					p.metrics.FindToolsMatchCount.Observe(float64(call.MatchCount))
					p.metrics.FindToolsActivationsTotal.Add(float64(len(call.Activated)))
					if call.MatchCount == 0 {
						p.metrics.FindToolsEmptyTotal.Inc()
					}
				}
				// Queries and names are model output that can echo
				// prompt content, so standard logs carry only
				// aggregate fields; raw values are visible through
				// the opt-in chat debug logging path.
				logger.Info(callCtx, "deferred MCP tool search",
					slog.F("query_count", len(call.Queries)),
					slog.F("name_count", len(call.Names)),
					slog.F("match_count", call.MatchCount),
					slog.F("activated_count", len(call.Activated)),
					slog.F("total_deferred", call.TotalDeferred),
					slog.F("rejection", call.Rejection),
				)
			},
		})
		tools, activeToolNames, allowInactiveTools = configureDeferredMCPToolSearch(
			tools,
			activeToolNames,
			deferredCandidates,
			findTools,
			deriveDeferredMCPActivations(promptRows, deferredCandidates, activationTokenBudget),
		)
		builtinToolNames[chattool.FindToolsName] = true
	}

	toolDefinitions := chatloop.BuildToolDefinitions(tools, activeToolNames, providerTools)
	toolNameToConfigID := make(map[string]uuid.UUID)
	for _, t := range tools {
		if mcpTool, ok := t.(mcpclient.MCPToolIdentifier); ok {
			toolNameToConfigID[t.Info().Name] = mcpTool.MCPServerConfigID()
		}
	}

	compactionToolCallID := "chat_summarized_" + uuid.NewString()
	effectiveThreshold := modelConfig.CompressionThreshold
	thresholdOverride, ok, err := p.resolveUserCompactionThreshold(ctx, chat.OwnerID, modelConfig.ID)
	if err != nil {
		return turnEnvironment{}, err
	}
	if ok {
		effectiveThreshold = thresholdOverride
	}
	chatTrigger := compactionTrigger{
		thresholdPercent: effectiveThreshold,
		contextLimit:     modelConfig.ContextLimit,
	}
	resolvedCompactionOverride, err := p.resolveModelOverride(ctx, modelOverrideSpec{
		context:        compactionOverrideContext,
		ownerID:        chat.OwnerID,
		organizationID: chat.OrganizationID,
	})
	if err != nil {
		return turnEnvironment{}, err
	}
	binding := chatTrigger
	bindingSource := compactionTriggerSourceChat
	var compactionOverride *resolvedModelOverride
	var otherTrigger compactionTrigger
	if resolvedCompactionOverride.Set {
		overrideTrigger := compactionTrigger{
			thresholdPercent: compactionOverrideWindowPercent,
			contextLimit:     resolvedCompactionOverride.Config.ContextLimit,
		}
		compactionOverride = &resolvedCompactionOverride
		otherTrigger = overrideTrigger
		bindingSource = bindingCompactionTriggerSource(chatTrigger, overrideTrigger)
		if bindingSource == compactionTriggerSourceOrganization {
			binding = overrideTrigger
			otherTrigger = chatTrigger
		}
	}
	compactionStepUsage := latestPromptUsage(promptRows)
	compactionNeeded := shouldCompactPromptUsage(compactionStepUsage, binding.contextLimit, binding.thresholdPercent)
	// The options carry the chat model; generateCompaction swaps in the
	// override client when one is configured.
	compactionOptions := chatloop.GenerateCompactionOptions{
		Model:                model.LanguageModel(),
		Messages:             compactionPromptMessages,
		ThresholdPercent:     binding.thresholdPercent,
		ContextLimit:         binding.contextLimit,
		ContextLimitFallback: binding.contextLimit,
		ChatContextLimit:     modelConfig.ContextLimit,
		ToolCallID:           compactionToolCallID,
		ToolName:             "chat_summarized",
		DebugSvc:             debugSvc,
		ChatID:               chat.ID,
		HistoryTipMessageID:  historyTipMessageID,
		ResolvedProvider:     resolved.resolvedProvider,
		ResolvedModel:        resolved.resolvedModel,
		ModelConfigID:        modelConfig.ID,
		StepUsage:            compactionStepUsage,
		SummaryCall:          compactionSummaryCall(resolved),
		ToolDefinitions:      toolDefinitions,
	}

	// workspaceCtx.currentChatSnapshot may carry a freshly persisted
	// AgentID/BuildID binding from the getWorkspaceAgent call above.
	// Return that snapshot so downstream consumers see the up-to-date
	// metadata.
	refreshedChat := workspaceCtx.currentChatSnapshot()
	if refreshedChat.ID == uuid.Nil {
		refreshedChat = chat
	}

	return turnEnvironment{
		Chat:                 refreshedChat,
		Messages:             input.Messages,
		Model:                model,
		Prompt:               prompt,
		Tools:                tools,
		ActiveTools:          activeToolNames,
		AllowInactiveTools:   allowInactiveTools,
		ProviderTools:        providerTools,
		ModelBuildOptions:    modelOpts,
		ResolvedProvider:     resolved.resolvedProvider,
		ReasoningProvenance:  reasoningSource,
		ReasoningRejected:    resolved.reasoningRejected,
		StageModel:           resolved.stageModel(),
		ModelConfigID:        modelConfig.ID,
		CallTemplate:         resolved.newCall(),
		ContextLimitFallback: modelConfig.ContextLimit,
		DynamicToolNames:     dynamicToolNames,
		StopAfterTools:       stopAfterBehaviorTools(currentPlanMode, chat.Mode, chat.ParentChatID),
		ExclusiveToolNames:   exclusiveToolNames,
		BuiltinToolNames:     builtinToolNames,
		ToolNameToConfigID:   toolNameToConfigID,
		MaxSteps:             p.chatLimits.MaxStepsPerTurn,
		Compaction: &generationCompaction{
			Override:        compactionOverride,
			TriggerSource:   bindingSource,
			OtherTrigger:    otherTrigger,
			ChatModelConfig: modelConfig,
			Required:        compactionNeeded,
			Options:         compactionOptions,
			PendingUserRows: pendingUserRows,
		},
		Debug:   debug,
		cleanup: cleanup,
	}, nil
}

// mcpConnectOutcome returns an error only when no server connected.
func mcpConnectOutcome(summaries []mcpclient.ConnectSummary) (connected, failed int, err error) {
	var failures []string
	for _, summary := range summaries {
		switch summary.Outcome {
		case mcpclient.ConnectOutcomeConnected, mcpclient.ConnectOutcomeNoTools:
			connected++
		default:
			failed++
			reason := cmp.Or(summary.Error, string(summary.Outcome))
			failures = append(failures, summary.Slug+": "+reason)
		}
	}
	if failed > 0 && connected == 0 {
		return connected, failed, xerrors.Errorf("no MCP server connected: %s", strings.Join(failures, "; "))
	}
	return connected, failed, nil
}

func latestPromptUsage(messages []database.ChatMessage) fantasy.Usage {
	for i := len(messages) - 1; i >= 0; i-- {
		usage := usageFromMessage(messages[i])
		if usage != (fantasy.Usage{}) {
			return usage
		}
	}
	return fantasy.Usage{}
}

// compactionOverrideWindowPercent leaves the rest of the override's window as
// headroom for the summary prompt and summary. It is a target margin, not a
// guarantee: the trigger reads the previous step's prompt usage.
const compactionOverrideWindowPercent = int32(80)

type compactionTrigger struct {
	thresholdPercent int32
	contextLimit     int64
}

func (t compactionTrigger) enabled() bool {
	return t.thresholdPercent >= 0 && t.thresholdPercent < chatloop.CompactionDisabledThresholdPercent && t.contextLimit > 0
}

func (t compactionTrigger) point() float64 {
	return float64(t.contextLimit) * float64(t.thresholdPercent) / 100
}

type compactionTriggerSource string

const (
	compactionTriggerSourceChat         compactionTriggerSource = "chat"
	compactionTriggerSourceOrganization compactionTriggerSource = "organization"
)

// bindingCompactionTriggerSource returns the trigger that fires first on the
// same prompt usage. A disabled override yields chat, a disabled chat trigger
// yields organization, and otherwise the lower token point binds, with ties to
// chat.
func bindingCompactionTriggerSource(chat, override compactionTrigger) compactionTriggerSource {
	switch {
	case !override.enabled():
		return compactionTriggerSourceChat
	case !chat.enabled():
		return compactionTriggerSourceOrganization
	case override.point() < chat.point():
		return compactionTriggerSourceOrganization
	default:
		return compactionTriggerSourceChat
	}
}

func shouldCompactPromptUsage(usage fantasy.Usage, contextLimit int64, thresholdPercent int32) bool {
	if !(compactionTrigger{thresholdPercent: thresholdPercent, contextLimit: contextLimit}).enabled() {
		return false
	}
	contextTokens := contextTokensFromUsage(usage)
	if contextTokens <= 0 {
		return false
	}
	usagePercent := (float64(contextTokens) / float64(contextLimit)) * 100
	return usagePercent >= float64(thresholdPercent)
}

func contextTokensFromUsage(usage fantasy.Usage) int64 {
	total := int64(0)
	hasContextTokens := false
	if usage.InputTokens > 0 {
		total += usage.InputTokens
		hasContextTokens = true
	}
	if usage.CacheReadTokens > 0 {
		total += usage.CacheReadTokens
		hasContextTokens = true
	}
	if usage.CacheCreationTokens > 0 {
		total += usage.CacheCreationTokens
		hasContextTokens = true
	}
	if !hasContextTokens && usage.TotalTokens > 0 {
		total = usage.TotalTokens
	}
	return total
}

func (p *Server) afterInterruptionOutcome(
	ctx context.Context,
	outcome interruptionOutcome,
) error {
	chat := outcome.Chat
	logger := p.logger.With(slog.F("chat_id", chat.ID), slog.F("owner_id", chat.OwnerID))

	if outcome.Kind == runnerActionKindFinishInterruption && !chat.ParentChatID.Valid {
		p.clearLastTurnSummaryAsync(context.WithoutCancel(ctx), chat, logger)
	}
	return nil
}

func (p *Server) afterGenerationOutcome(
	ctx context.Context,
	outcome generationOutcome,
) error {
	chat := outcome.Chat
	logger := p.logger.With(slog.F("chat_id", chat.ID), slog.F("owner_id", chat.OwnerID))

	switch outcome.Kind {
	case runnerActionKindFinishTurn:
		finalizeCtx := context.WithoutCancel(ctx)
		runResult := p.deriveFinalTurnRunResult(finalizeCtx, chat, logger)
		p.maybeFinalizeTurnStatusLabelAndPush(finalizeCtx, chat, chat.Status, "", runResult, logger)
	case runnerActionKindFinishError:
		p.maybeFinalizeTurnStatusLabelAndPush(context.WithoutCancel(ctx), chat, chat.Status, outcome.LastError, runChatResult{}, logger)
	case runnerActionKindEnterRequiresAction:
		p.maybeFinalizeTurnStatusLabelAndPush(context.WithoutCancel(ctx), chat, chat.Status, "", runChatResult{}, logger)
	}
	return nil
}

// deriveFinalTurnRunResult rebuilds the inputs needed to generate the
// end-of-turn status label directly from persisted state.
func (p *Server) deriveFinalTurnRunResult(
	ctx context.Context,
	chat database.Chat,
	logger slog.Logger,
) runChatResult {
	// generateFinalTurnStatusLabel only produces a model-generated label for
	// the Waiting status, so skip the model resolution and history read
	// otherwise.
	if chat.Status != database.ChatStatusWaiting {
		return runChatResult{}
	}

	promptRows, err := p.db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	if err != nil {
		logger.Warn(ctx, "derive final turn status label: load prompt rows", slog.Error(err))
		return runChatResult{}
	}
	triggerMessageID, historyTipMessageID, _ := deriveChatDebugSeed(promptRows)
	finalAssistantText := latestAssistantText(promptRows)
	if finalAssistantText == "" {
		return runChatResult{}
	}

	apiKeyID, err := p.ensureSyntheticAPIKeyID(ctx, chat.OwnerID)
	if err != nil {
		logger.Warn(ctx, "derive final turn status label: ensure synthetic API key", slog.Error(err))
		return runChatResult{FinalAssistantText: finalAssistantText, TriggerMessageID: triggerMessageID, HistoryTipMessageID: historyTipMessageID}
	}
	modelOpts := modelBuildOptions{ActiveAPIKeyID: apiKeyID}
	resolved, err := p.resolveQuickgenModel(ctx, "turn_status_label", chat, modelOpts)
	if err != nil {
		// Preserve the text and IDs for the generic-label fallback.
		logger.Warn(ctx, "derive final turn status label: resolve model", slog.Error(err))
		return runChatResult{
			FinalAssistantText:  finalAssistantText,
			TriggerMessageID:    triggerMessageID,
			HistoryTipMessageID: historyTipMessageID,
		}
	}

	return runChatResult{
		FinalAssistantText:  finalAssistantText,
		StatusLabelCall:     &resolved,
		TriggerMessageID:    triggerMessageID,
		HistoryTipMessageID: historyTipMessageID,
	}
}

// latestAssistantText returns the trimmed text of the most recent assistant
// message. It mirrors the FinalAssistantText that buildCommitStepMessages
// produced from the freshly generated step, making persisted history the
// single source of truth for the turn status label input.
func latestAssistantText(messages []database.ChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != database.ChatMessageRoleAssistant {
			continue
		}
		parts, err := chatprompt.ParseContent(messages[i])
		if err != nil {
			return ""
		}
		return strings.TrimSpace(textFromParts(parts))
	}
	return ""
}

// ACLs are deliberately not re-checked: revocation blocks new selection but
// leaves already-selected servers usable, like template ACLs for running
// workspaces. Disabling or deleting the config cuts off existing chats.
func enabledMCPServerConfigsForChatOrg(
	ctx context.Context,
	db database.Store,
	organizationID uuid.UUID,
	ids []uuid.UUID,
) ([]database.MCPServerConfig, error) {
	configs, err := db.GetEnabledMCPServerConfigsByOrganizationAndIDs(ctx, database.GetEnabledMCPServerConfigsByOrganizationAndIDsParams{
		OrganizationID: organizationID,
		IDs:            ids,
	})
	if err != nil {
		return nil, xerrors.Errorf("get enabled MCP server configs for organization: %w", err)
	}
	return configs, nil
}

type turnWorkspaceContext struct {
	server           *Server
	chatStateMu      *sync.Mutex
	currentChat      *database.Chat
	loadChatSnapshot func(context.Context, uuid.UUID) (database.Chat, error)

	mu                sync.Mutex
	agent             database.WorkspaceAgent
	agentLoaded       bool
	conn              workspacesdk.AgentConn
	releaseConn       func()
	cachedWorkspaceID uuid.NullUUID
}

func (c *turnWorkspaceContext) close() {
	c.clearCachedWorkspaceState()
}

func (c *turnWorkspaceContext) clearCachedWorkspaceState() {
	c.mu.Lock()
	releaseConn := c.releaseConn
	c.agent = database.WorkspaceAgent{}
	c.agentLoaded = false
	c.conn = nil
	c.releaseConn = nil
	c.cachedWorkspaceID = uuid.NullUUID{}
	c.mu.Unlock()

	if releaseConn != nil {
		releaseConn()
	}
}

func (c *turnWorkspaceContext) setCurrentChat(chat database.Chat) {
	c.chatStateMu.Lock()
	*c.currentChat = chat
	c.chatStateMu.Unlock()
}

func (c *turnWorkspaceContext) currentChatSnapshot() database.Chat {
	c.chatStateMu.Lock()
	chatSnapshot := *c.currentChat
	c.chatStateMu.Unlock()
	return chatSnapshot
}

func (c *turnWorkspaceContext) selectWorkspace(chat database.Chat) {
	c.setCurrentChat(chat)
	c.clearCachedWorkspaceState()
}

func (c *turnWorkspaceContext) currentWorkspaceMatches(expected uuid.NullUUID) (database.Chat, bool) {
	chatSnapshot := c.currentChatSnapshot()
	return chatSnapshot, nullUUIDEqual(chatSnapshot.WorkspaceID, expected)
}

func (c *turnWorkspaceContext) trackWorkspaceUsage(ctx context.Context, chatSnapshot database.Chat) {
	if c.server == nil || !chatSnapshot.WorkspaceID.Valid {
		return
	}
	logger := c.server.logger.With(
		slog.F("chat_id", chatSnapshot.ID),
		slog.F("owner_id", chatSnapshot.OwnerID),
	)
	c.server.trackWorkspaceUsage(ctx, chatSnapshot.ID, chatSnapshot.WorkspaceID, logger)
}

func nullUUIDEqual(left, right uuid.NullUUID) bool {
	if left.Valid != right.Valid {
		return false
	}
	if !left.Valid {
		return true
	}
	return left.UUID == right.UUID
}

func (c *turnWorkspaceContext) persistBuildAgentBinding(
	ctx context.Context,
	chatSnapshot database.Chat,
	buildID uuid.UUID,
	agentID uuid.UUID,
) (database.Chat, error) {
	// If the chat was rebound to a different agent (e.g. a workspace rebuild
	// produced a new agent), re-pin its context to the new agent so it stops
	// injecting the previous agent's resources. Workspace lifecycle tools clear
	// the agent binding while preserving the pin, so a missing prior agent also
	// requires a re-pin when pinned context exists. The binding and the re-pin
	// commit together: a committed binding without its re-pin would never be
	// retried, because the next turn sees no agent change. The pinned context
	// fields on updatedChat are background state, reloaded on the next
	// snapshot fetch.
	hasStaleUnboundContext := !chatSnapshot.AgentID.Valid && chatSnapshot.ContextAggregateHash != nil
	needsRepin := hasStaleUnboundContext || (chatSnapshot.AgentID.Valid && chatSnapshot.AgentID.UUID != agentID)
	var updatedChat database.Chat
	err := database.ReadModifyUpdate(c.server.db, func(tx database.Store) error {
		var err error
		updatedChat, err = tx.UpdateChatBuildAgentBinding(
			ctx,
			database.UpdateChatBuildAgentBindingParams{
				ID: chatSnapshot.ID,
				BuildID: uuid.NullUUID{
					UUID:  buildID,
					Valid: true,
				},
				AgentID: uuid.NullUUID{
					UUID:  agentID,
					Valid: true,
				},
			},
		)
		if err != nil {
			return xerrors.Errorf("update chat build/agent binding: %w", err)
		}
		if !needsRepin {
			return nil
		}
		//nolint:gocritic // Chatd re-pins chats it does not own as the daemon subject.
		if err := repinChatContext(dbauthz.AsChatd(ctx), tx, chatSnapshot.ID, uuid.NullUUID{UUID: agentID, Valid: true}); err != nil {
			return xerrors.Errorf("re-pin chat context after agent rebind: %w", err)
		}
		return nil
	})
	if err != nil {
		return chatSnapshot, err
	}

	c.setCurrentChat(updatedChat)
	return updatedChat, nil
}

func (c *turnWorkspaceContext) getWorkspaceAgent(ctx context.Context) (database.WorkspaceAgent, error) {
	_, agent, err := c.ensureWorkspaceAgent(ctx)
	return agent, err
}

func (c *turnWorkspaceContext) ensureWorkspaceAgent(
	ctx context.Context,
) (database.Chat, database.WorkspaceAgent, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.agentLoaded {
		chatSnapshot := c.currentChatSnapshot()
		if nullUUIDEqual(c.cachedWorkspaceID, chatSnapshot.WorkspaceID) {
			return chatSnapshot, c.agent, nil
		}
		c.agent = database.WorkspaceAgent{}
		c.agentLoaded = false
	}

	return c.loadWorkspaceAgentLocked(ctx)
}

func (c *turnWorkspaceContext) loadWorkspaceAgentLocked(
	ctx context.Context,
) (database.Chat, database.WorkspaceAgent, error) {
	chatSnapshot := c.currentChatSnapshot()

	for attempt := 0; attempt < 2; attempt++ {
		if !chatSnapshot.WorkspaceID.Valid {
			refreshedChat, refreshErr := refreshChatWorkspaceSnapshot(
				ctx,
				chatSnapshot,
				c.loadChatSnapshot,
			)
			if refreshErr != nil {
				return chatSnapshot, database.WorkspaceAgent{}, refreshErr
			}
			if refreshedChat.WorkspaceID.Valid {
				c.setCurrentChat(refreshedChat)
				chatSnapshot = refreshedChat
			}
		}

		if !chatSnapshot.WorkspaceID.Valid {
			return chatSnapshot, database.WorkspaceAgent{}, errChatHasNoWorkspace
		}

		// A soft-deleted workspace keeps its agent rows, so the bound agent
		// would otherwise be dialed and fail as merely unreachable.
		ws, err := c.server.db.GetWorkspaceByID(ctx, chatSnapshot.WorkspaceID.UUID)
		if xerrors.Is(err, sql.ErrNoRows) {
			return chatSnapshot, database.WorkspaceAgent{}, xerrors.Errorf("load workspace: %w", errChatWorkspaceDeleted)
		}
		if err != nil {
			return chatSnapshot, database.WorkspaceAgent{}, xerrors.Errorf(
				"load workspace: %w. %s", err, chattool.WorkspaceUnavailableHint,
			)
		}
		if ws.Deleted {
			// A concurrent create_workspace may have rebound the chat
			// while the row was read; resolve the replacement instead.
			latestChat, workspaceMatches := c.currentWorkspaceMatches(chatSnapshot.WorkspaceID)
			if !workspaceMatches {
				chatSnapshot = latestChat
				continue
			}
			return chatSnapshot, database.WorkspaceAgent{}, errChatWorkspaceDeleted
		}

		if chatSnapshot.AgentID.Valid {
			agent, err := c.server.db.GetWorkspaceAgentByID(ctx, chatSnapshot.AgentID.UUID)
			if err == nil {
				latestChat, workspaceMatches := c.currentWorkspaceMatches(chatSnapshot.WorkspaceID)
				if !workspaceMatches {
					chatSnapshot = latestChat
					continue
				}
				c.agent = agent
				c.agentLoaded = true
				c.cachedWorkspaceID = chatSnapshot.WorkspaceID
				return chatSnapshot, c.agent, nil
			}
			if !xerrors.Is(err, sql.ErrNoRows) {
				return chatSnapshot, database.WorkspaceAgent{}, xerrors.Errorf("get bound workspace agent: %w", err)
			}
		}

		agents, err := c.server.db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(
			ctx,
			chatSnapshot.WorkspaceID.UUID,
		)
		if err != nil {
			return chatSnapshot, database.WorkspaceAgent{}, xerrors.Errorf(
				"get workspace agents in latest build: %w",
				err,
			)
		}
		if len(agents) == 0 {
			return chatSnapshot, database.WorkspaceAgent{}, errChatHasNoWorkspaceAgent
		}
		selected, err := agentselect.FindChatAgent(agents)
		if err != nil {
			return chatSnapshot, database.WorkspaceAgent{}, xerrors.Errorf(
				"find chat agent: %w",
				err,
			)
		}

		build, err := c.server.db.GetLatestWorkspaceBuildByWorkspaceID(ctx, chatSnapshot.WorkspaceID.UUID)
		if err != nil {
			return chatSnapshot, database.WorkspaceAgent{}, xerrors.Errorf("get latest workspace build: %w", err)
		}

		updatedChat, err := c.persistBuildAgentBinding(
			ctx,
			chatSnapshot,
			build.ID,
			selected.ID,
		)
		if err != nil {
			return chatSnapshot, database.WorkspaceAgent{}, err
		}

		chatSnapshot = updatedChat
		latestChat, workspaceMatches := c.currentWorkspaceMatches(chatSnapshot.WorkspaceID)
		if !workspaceMatches {
			chatSnapshot = latestChat
			continue
		}
		c.agent = selected
		c.agentLoaded = true
		c.cachedWorkspaceID = chatSnapshot.WorkspaceID
		return chatSnapshot, c.agent, nil
	}

	return chatSnapshot, database.WorkspaceAgent{}, xerrors.New(
		"chat workspace changed while resolving agent",
	)
}

// isNoUsableWorkspaceAgent reports whether err from ensureWorkspaceAgent means
// the chat has no agent to use, as opposed to a failed lookup.
func isNoUsableWorkspaceAgent(err error) bool {
	return xerrors.Is(err, errChatHasNoWorkspace) ||
		xerrors.Is(err, errChatWorkspaceDeleted) ||
		xerrors.Is(err, errChatHasNoWorkspaceAgent) ||
		xerrors.Is(err, agentselect.ErrNoEligibleAgents) ||
		xerrors.Is(err, agentselect.ErrMultipleChatAgents)
}

func (c *turnWorkspaceContext) latestWorkspaceAgentID(
	ctx context.Context,
	workspaceID uuid.UUID,
) (uuid.UUID, error) {
	agents, err := c.server.db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(
		ctx,
		workspaceID,
	)
	if err != nil {
		return uuid.Nil, xerrors.Errorf(
			"get workspace agents in latest build: %w",
			err,
		)
	}
	if len(agents) == 0 {
		return uuid.Nil, errChatHasNoWorkspaceAgent
	}
	selected, err := agentselect.FindChatAgent(agents)
	if err != nil {
		return uuid.Nil, xerrors.Errorf(
			"find chat agent: %w",
			err,
		)
	}
	return selected.ID, nil
}

func (c *turnWorkspaceContext) workspaceAgentIDForConn(
	ctx context.Context,
) (database.Chat, uuid.UUID, error) {
	for attempt := 0; attempt < 2; attempt++ {
		chatSnapshot := c.currentChatSnapshot()
		if !chatSnapshot.WorkspaceID.Valid || !chatSnapshot.AgentID.Valid {
			updatedChat, agent, err := c.ensureWorkspaceAgent(ctx)
			if err != nil {
				return updatedChat, uuid.Nil, err
			}
			return updatedChat, agent.ID, nil
		}

		currentAgentID, err := c.latestWorkspaceAgentID(
			ctx,
			chatSnapshot.WorkspaceID.UUID,
		)
		if err != nil {
			if xerrors.Is(err, errChatHasNoWorkspaceAgent) {
				c.clearCachedWorkspaceState()
			}
			return chatSnapshot, uuid.Nil, err
		}

		latestChat, workspaceMatches := c.currentWorkspaceMatches(
			chatSnapshot.WorkspaceID,
		)
		if !workspaceMatches {
			continue
		}
		return latestChat, currentAgentID, nil
	}

	chatSnapshot := c.currentChatSnapshot()
	return chatSnapshot, uuid.Nil, xerrors.New(
		"chat workspace changed while resolving agent",
	)
}

// getWorkspaceConnLocked returns the cached connection when it still matches
// the current workspace. When the workspace changed, it clears the stale
// cached state and returns the release func for the caller to run after
// unlocking.
func (c *turnWorkspaceContext) getWorkspaceConnLocked() (workspacesdk.AgentConn, func()) {
	if c.conn == nil {
		return nil, nil
	}

	chatSnapshot := c.currentChatSnapshot()
	if nullUUIDEqual(c.cachedWorkspaceID, chatSnapshot.WorkspaceID) {
		return c.conn, nil
	}

	agentRelease := c.releaseConn
	c.agent = database.WorkspaceAgent{}
	c.agentLoaded = false
	c.conn = nil
	c.releaseConn = nil
	c.cachedWorkspaceID = uuid.NullUUID{}
	return nil, agentRelease
}

// isAgentUnreachable reports whether the given agent row's
// status is disconnected or timed out. It uses timestamp
// arithmetic on the row. The "connecting" state is allowed
// through because it is normal after a fresh workspace build.
func isAgentUnreachable(now time.Time, agent database.WorkspaceAgent, inactiveTimeout time.Duration) bool {
	status := agent.Status(now, inactiveTimeout)
	return status.Status == database.WorkspaceAgentStatusDisconnected ||
		status.Status == database.WorkspaceAgentStatusTimeout
}

func agentDisconnectedFor(now time.Time, agent database.WorkspaceAgent, inactiveTimeout time.Duration) (time.Duration, bool) {
	status := agent.Status(now, inactiveTimeout)
	if status.Status != database.WorkspaceAgentStatusDisconnected || status.DisconnectedAt == nil {
		return 0, false
	}

	disconnectedFor := now.Sub(*status.DisconnectedAt)
	if disconnectedFor < 0 {
		disconnectedFor = 0
	}
	return disconnectedFor, true
}

func (c *turnWorkspaceContext) latestWorkspaceAgentRecoveryError(
	ctx context.Context,
	workspaceID uuid.UUID,
) error {
	agentID, err := c.latestWorkspaceAgentID(ctx, workspaceID)
	if err != nil {
		if xerrors.Is(err, errChatHasNoWorkspaceAgent) {
			return err
		}
		c.server.logger.Warn(ctx, "failed to resolve latest agent for timeout classification", slog.Error(err))
		return errChatDialTimeout
	}

	agent, err := c.server.db.GetWorkspaceAgentByID(ctx, agentID)
	if err != nil {
		c.server.logger.Warn(ctx, "failed to load latest agent for timeout classification",
			slog.F("agent_id", agentID),
			slog.Error(err),
		)
		return errChatDialTimeout
	}

	now := c.server.clock.Now()
	status := agent.Status(now, c.server.agentInactiveDisconnectTimeout)
	recoveryErr := errChatDialTimeout
	if status.Status == database.WorkspaceAgentStatusTimeout {
		recoveryErr = errChatAgentNeverConnected
	} else if status.Status == database.WorkspaceAgentStatusDisconnected && status.DisconnectedAt != nil {
		disconnectedFor := now.Sub(*status.DisconnectedAt)
		if disconnectedFor < 0 {
			disconnectedFor = 0
		}
		if disconnectedFor >= agentDisconnectedRecoveryThreshold {
			recoveryErr = errChatAgentDisconnected
		}
	}
	return c.externalAgentError(ctx, agent, recoveryErr)
}

func (c *turnWorkspaceContext) externalAgentError(
	ctx context.Context,
	agent database.WorkspaceAgent,
	fallback error,
) error {
	isExternal, err := chattool.IsExternalWorkspaceAgent(ctx, c.server.db, agent)
	if err != nil || !isExternal {
		return fallback
	}
	return newChatExternalAgentUnavailableError(agent)
}

func (c *turnWorkspaceContext) externalAgentPreflightError(
	ctx context.Context,
	chatSnapshot database.Chat,
	agent database.WorkspaceAgent,
) error {
	// Mirror the cache-hit gate: only short-circuit on clearly offline
	// states (Disconnected/Timeout). Connecting is allowed through so
	// an external agent the user just started can still connect inside
	// the normal dial window.
	if !isAgentUnreachable(c.server.clock.Now(), agent, c.server.agentInactiveDisconnectTimeout) {
		return nil
	}

	isExternal, err := chattool.IsExternalWorkspaceAgent(ctx, c.server.db, agent)
	if err != nil || !isExternal || !chatSnapshot.WorkspaceID.Valid {
		return nil
	}

	// Stale agent bindings rely on dialWithLazyValidation to discover
	// replacement agents, so only skip the dial when this agent is still
	// the latest selected chat agent for the workspace.
	latestAgentID, err := c.latestWorkspaceAgentID(ctx, chatSnapshot.WorkspaceID.UUID)
	if err != nil || latestAgentID != agent.ID {
		return nil
	}
	return newChatExternalAgentUnavailableError(agent)
}

func (c *turnWorkspaceContext) getWorkspaceConn(ctx context.Context) (workspacesdk.AgentConn, error) {
	if c.server.agentConnFn == nil {
		return nil, xerrors.New("workspace agent connector is not configured")
	}

	for attempt := 0; attempt < 2; attempt++ {
		c.mu.Lock()
		currentConn, staleRelease := c.getWorkspaceConnLocked()
		// Capture agentID in the same lock section as
		// currentConn to prevent a TOCTOU race with
		// concurrent clearCachedWorkspaceState calls.
		agentID := c.agent.ID
		c.mu.Unlock()

		// Status check on cache hit: re-fetch the agent
		// row so we see the latest heartbeat rather than
		// a potentially stale cached copy.
		if currentConn != nil {
			chatSnapshot := c.currentChatSnapshot()
			if agentID != uuid.Nil {
				freshAgent, err := c.server.db.GetWorkspaceAgentByID(ctx, agentID)
				if err != nil {
					c.server.logger.Warn(ctx, "failed to re-fetch agent for status check",
						slog.F("agent_id", agentID),
						slog.Error(err),
					)
					// On DB error the check re-runs on the
					// next tool call.
				} else if _, disconnected := agentDisconnectedFor(
					c.server.clock.Now(),
					freshAgent,
					c.server.agentInactiveDisconnectTimeout,
				); disconnected {
					c.clearCachedWorkspaceState()
					continue
				}
			}
			c.trackWorkspaceUsage(ctx, chatSnapshot)
			return currentConn, nil
		}
		if staleRelease != nil {
			staleRelease()
		}

		chatSnapshot, agent, err := c.ensureWorkspaceAgent(ctx)
		if err != nil {
			return nil, err
		}
		if err := c.externalAgentPreflightError(ctx, chatSnapshot, agent); err != nil {
			return nil, err
		}

		// Wrap the dial in a timeout to bound the time spent
		// waiting for an unreachable agent. The timeout scopes
		// only dialWithLazyValidation, not ensureWorkspaceAgent
		// or the post-dial binding steps.
		dialCtx, dialCancelCause := context.WithCancelCause(ctx)
		dialTimer := c.server.clock.AfterFunc(
			c.server.dialTimeout,
			func() { dialCancelCause(errChatDialTimeout) },
			"chatd",
			dialTimeoutTimerTag,
		)
		dialCancel := func() {
			dialTimer.Stop()
			dialCancelCause(nil)
		}
		dialResult, err := dialWithLazyValidation(
			dialCtx,
			c.server.clock,
			agent.ID,
			chatSnapshot.WorkspaceID.UUID,
			DialFunc(c.server.agentConnFn),
			func(ctx context.Context, workspaceID uuid.UUID) (uuid.UUID, error) {
				return c.latestWorkspaceAgentID(ctx, workspaceID)
			},
			workspaceDialValidationDelay,
		)
		dialCancel()
		if err != nil {
			if xerrors.Is(err, errChatHasNoWorkspaceAgent) {
				c.clearCachedWorkspaceState()
				return nil, err
			}
			// Surface the dial timeout sentinel only when the
			// parent context is still alive. If the parent was
			// canceled, its error must propagate unchanged so
			// the runner can recognize cancellation.
			if ctx.Err() == nil && errors.Is(context.Cause(dialCtx), errChatDialTimeout) {
				c.clearCachedWorkspaceState()
				return nil, c.latestWorkspaceAgentRecoveryError(ctx, chatSnapshot.WorkspaceID.UUID)
			}
			return nil, err
		}
		agentConn := dialResult.Conn
		agentRelease := dialResult.Release
		if dialResult.WasSwitched {
			build, err := c.server.db.GetLatestWorkspaceBuildByWorkspaceID(ctx, chatSnapshot.WorkspaceID.UUID)
			if err != nil {
				if agentRelease != nil {
					agentRelease()
				}
				return nil, xerrors.Errorf("get latest workspace build: %w", err)
			}

			switchedAgent, err := c.server.db.GetWorkspaceAgentByID(ctx, dialResult.AgentID)
			if err != nil {
				if agentRelease != nil {
					agentRelease()
				}
				return nil, xerrors.Errorf("get workspace agent by id: %w", err)
			}

			updatedChat, err := c.persistBuildAgentBinding(
				ctx,
				chatSnapshot,
				build.ID,
				switchedAgent.ID,
			)
			if err != nil {
				if agentRelease != nil {
					agentRelease()
				}
				return nil, err
			}
			chatSnapshot = updatedChat

			c.mu.Lock()
			c.agent = switchedAgent
			c.agentLoaded = true
			c.cachedWorkspaceID = chatSnapshot.WorkspaceID
			c.mu.Unlock()
		}

		if _, workspaceMatches := c.currentWorkspaceMatches(chatSnapshot.WorkspaceID); !workspaceMatches {
			if agentRelease != nil {
				agentRelease()
			}
			c.clearCachedWorkspaceState()
			continue
		}

		c.mu.Lock()
		if c.conn == nil {
			c.conn = agentConn
			c.releaseConn = agentRelease
			c.cachedWorkspaceID = chatSnapshot.WorkspaceID

			var ancestorIDs []string
			if chatSnapshot.ParentChatID.Valid {
				ancestorIDs = append(ancestorIDs, chatSnapshot.ParentChatID.UUID.String())
			}
			ancestorJSON, marshalErr := json.Marshal(ancestorIDs)
			if marshalErr != nil {
				ancestorJSON = []byte("[]")
			}
			agentConn.SetExtraHeaders(http.Header{
				workspacesdk.CoderChatIDHeader:          {chatSnapshot.ID.String()},
				workspacesdk.CoderAncestorChatIDsHeader: {string(ancestorJSON)},
			})

			c.mu.Unlock()
			c.server.logger.Debug(ctx, "set chat headers on agent conn",
				slog.F("chat_id", chatSnapshot.ID),
				slog.F("ancestor_chat_ids", ancestorIDs),
				slog.F("workspace_id", chatSnapshot.WorkspaceID.UUID),
				slog.F("agent_id", dialResult.AgentID),
			)
			c.trackWorkspaceUsage(ctx, chatSnapshot)
			return agentConn, nil
		}
		currentConn = c.conn
		c.mu.Unlock()

		if agentRelease != nil {
			agentRelease()
		}
		c.trackWorkspaceUsage(ctx, chatSnapshot)
		return currentConn, nil
	}

	return nil, xerrors.New("chat workspace changed while connecting")
}

func isExploreSubagentMode(mode database.NullChatMode) bool {
	return mode.Valid && mode.ChatMode == database.ChatModeExplore
}

func isPlanModeSubagentTurn(mode database.NullChatPlanMode, parentChatID uuid.NullUUID) bool {
	return mode.Valid && mode.ChatPlanMode == database.ChatPlanModePlan && parentChatID.Valid
}

func turnUsesMCPServers(chat database.Chat) bool {
	isExploreRootChat := isExploreSubagentMode(chat.Mode) && !chat.ParentChatID.Valid
	return !isPlanModeSubagentTurn(chat.PlanMode, chat.ParentChatID) && !isExploreRootChat
}

// Plan-mode subagents have a narrower trust boundary and cannot use MCP
// servers. Root plan-mode chats require policy approval. Outside plan mode,
// all servers are visible and the approved-ID set is nil.
func filterMCPServersForTurn[T any](
	servers []T,
	mode database.NullChatPlanMode,
	parentChatID uuid.NullUUID,
	policy func(T) (id uuid.UUID, allowInPlanMode bool),
) ([]T, map[uuid.UUID]struct{}) {
	if isPlanModeSubagentTurn(mode, parentChatID) {
		return nil, map[uuid.UUID]struct{}{}
	}
	if !mode.Valid || mode.ChatPlanMode != database.ChatPlanModePlan {
		return servers, nil
	}
	approved := map[uuid.UUID]struct{}{}
	filtered := make([]T, 0, len(servers))
	for _, srv := range servers {
		id, allowInPlanMode := policy(srv)
		if !allowInPlanMode {
			continue
		}
		filtered = append(filtered, srv)
		approved[id] = struct{}{}
	}
	return filtered, approved
}

// filterExternalMCPConfigsForTurn returns the external MCP server configs
// visible on the current turn. Explore children snapshot this filtered set at
// spawn time so later model overrides cannot widen the external-tool boundary.
func filterExternalMCPConfigsForTurn(
	configs []database.MCPServerConfig,
	mode database.NullChatPlanMode,
	parentChatID uuid.NullUUID,
) ([]database.MCPServerConfig, map[uuid.UUID]struct{}) {
	return filterMCPServersForTurn(configs, mode, parentChatID, func(cfg database.MCPServerConfig) (uuid.UUID, bool) {
		return cfg.ID, cfg.AllowInPlanMode
	})
}

func builtinPlanToolAllowed(name string, isRootChat bool) bool {
	switch name {
	case "read_file", "execute", "process_output", "read_skill", "read_skill_file":
		return true
	case "write_file", "edit_files", "list_templates", "read_template",
		"create_workspace", "start_workspace", "stop_workspace", "propose_plan", "spawn_agent",
		"spawn_explore_agent", "wait_agent", "list_agents", "list_subagent_models",
		"ask_user_question", "attach_file",
		chattool.ReadMemoryToolName, chattool.SaveMemoryToolName, chattool.DeleteMemoryToolName, chattool.ConsolidateMemoryToolName:
		return isRootChat
	case "process_list", "process_signal", "message_agent", "interrupt_agent", "close_agent",
		"spawn_computer_use_agent":
		return false
	default:
		return false
	}
}

func toolAllowedForTurn(
	tool fantasy.AgentTool,
	mode database.NullChatPlanMode,
	parentChatID uuid.NullUUID,
	approvedMCPConfigIDs map[uuid.UUID]struct{},
) bool {
	if !mode.Valid || mode.ChatPlanMode != database.ChatPlanModePlan {
		return true
	}
	if builtinPlanToolAllowed(tool.Info().Name, !parentChatID.Valid) {
		return true
	}
	mcpTool, ok := tool.(mcpclient.MCPToolIdentifier)
	if !ok {
		return false
	}
	_, approved := approvedMCPConfigIDs[mcpTool.MCPServerConfigID()]
	return approved
}

func filterToolsForTurn(
	allTools []fantasy.AgentTool,
	mode database.NullChatPlanMode,
	parentChatID uuid.NullUUID,
	approvedMCPConfigIDs map[uuid.UUID]struct{},
) []fantasy.AgentTool {
	if !mode.Valid || mode.ChatPlanMode != database.ChatPlanModePlan {
		return allTools
	}

	filtered := make([]fantasy.AgentTool, 0, len(allTools))
	for _, tool := range allTools {
		if toolAllowedForTurn(tool, mode, parentChatID, approvedMCPConfigIDs) {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

func activeToolNamesForTurn(
	allTools []fantasy.AgentTool,
	mode database.NullChatPlanMode,
	parentChatID uuid.NullUUID,
	approvedMCPConfigIDs map[uuid.UUID]struct{},
) []string {
	toolNames := make([]string, 0, len(allTools))
	for _, tool := range allTools {
		if toolAllowedForTurn(tool, mode, parentChatID, approvedMCPConfigIDs) {
			toolNames = append(toolNames, tool.Info().Name)
		}
	}
	return toolNames
}

func exploreToolAllowed(tool fantasy.AgentTool) bool {
	switch tool.Info().Name {
	case "read_file", "execute", "process_output", "read_skill", "read_skill_file":
		return true
	}
	// Org tools use the spawn-time snapshot; inline tools require
	// allow_in_subagents. Internal servers decide access in ServerForChat.
	// WorkspaceMCPTool lacks MCPToolIdentifier and remains excluded.
	_, ok := tool.(mcpclient.MCPToolIdentifier)
	return ok
}

func allowedExploreToolNames(allTools []fantasy.AgentTool) []string {
	toolNames := make([]string, 0, len(allTools))
	for _, tool := range allTools {
		if exploreToolAllowed(tool) {
			toolNames = append(toolNames, tool.Info().Name)
		}
	}
	return toolNames
}

func stopAfterPlanTools(
	planMode database.NullChatPlanMode,
	parentChatID uuid.NullUUID,
) map[string]struct{} {
	if !planMode.Valid || planMode.ChatPlanMode != database.ChatPlanModePlan {
		return nil
	}
	stopTools := map[string]struct{}{
		"propose_plan": {},
	}
	if !parentChatID.Valid {
		stopTools["ask_user_question"] = struct{}{}
	}
	return stopTools
}

func stopAfterBehaviorTools(
	planMode database.NullChatPlanMode,
	chatMode database.NullChatMode,
	parentChatID uuid.NullUUID,
) map[string]struct{} {
	if isExploreSubagentMode(chatMode) {
		return nil
	}
	return stopAfterPlanTools(planMode, parentChatID)
}

type systemPromptBehaviorContext struct {
	planMode             database.NullChatPlanMode
	chatMode             database.NullChatMode
	planModeInstructions string
	isRootChat           bool
}

func workspaceSkillsForResolution(workspaceSkills []chattool.SkillMeta) []skillspkg.Skill {
	if len(workspaceSkills) == 0 {
		return nil
	}
	resolved := make([]skillspkg.Skill, 0, len(workspaceSkills))
	for _, skill := range workspaceSkills {
		resolved = append(resolved, skillspkg.Skill{
			Name:        skill.Name,
			Description: skill.Description,
			Source:      skillspkg.SourceWorkspace,
		})
	}
	return resolved
}

func mergeTurnSkills(
	personalSkills []skillspkg.Skill,
	organizationSkills []skillspkg.Skill,
	projectSkills []skillspkg.Skill,
	workspaceSkills []chattool.SkillMeta,
) []skillspkg.ResolvedSkill {
	return skillspkg.MergeSkills(
		skillspkg.SourceSkills{Source: skillspkg.SourcePersonal, Skills: personalSkills},
		skillspkg.SourceSkills{Source: skillspkg.SourceOrganization, Skills: organizationSkills},
		skillspkg.SourceSkills{Source: skillspkg.SourceProject, Skills: projectSkills},
		skillspkg.SourceSkills{Source: skillspkg.SourceWorkspace, Skills: workspaceSkillsForResolution(workspaceSkills)},
	)
}

// buildSystemPrompt applies system-level prompt injections in a fixed
// order: subagent instruction, chat instruction, skill index, memory index,
// project instructions, user prompt, then mode overlay prompts. Project
// instructions precede the user prompt so personal instructions keep the
// last word.
func buildSystemPrompt(
	prompt []fantasy.Message,
	subagentInstruction string,
	instruction string,
	resolvedSkills []skillspkg.ResolvedSkill,
	memoryIndex string,
	projectInstructions string,
	userPrompt string,
	behaviorContext systemPromptBehaviorContext,
) []fantasy.Message {
	if subagentInstruction != "" {
		prompt = chatprompt.InsertSystem(prompt, subagentInstruction)
	}
	if instruction != "" {
		prompt = chatprompt.InsertSystem(prompt, instruction)
	}
	if skillIndex := chattool.FormatResolvedSkillIndex(resolvedSkills); skillIndex != "" {
		prompt = chatprompt.InsertSystem(prompt, skillIndex)
	}
	if memoryIndex != "" {
		prompt = chatprompt.InsertSystem(prompt, memoryIndex)
	}
	if projectInstructions != "" {
		prompt = chatprompt.InsertSystem(prompt, projectInstructions)
	}
	if userPrompt != "" {
		prompt = chatprompt.InsertSystem(prompt, userPrompt)
	}
	if isExploreSubagentMode(behaviorContext.chatMode) {
		prompt = chatprompt.InsertSystem(prompt, ExploreSubagentOverlayPrompt)
		return prompt
	}
	isPlanModeTurn := behaviorContext.planMode.Valid && behaviorContext.planMode.ChatPlanMode == database.ChatPlanModePlan
	if isPlanModeTurn {
		if behaviorContext.isRootChat {
			prompt = chatprompt.InsertSystem(prompt, PlanningOverlayPrompt())
			if behaviorContext.planModeInstructions != "" {
				prompt = chatprompt.InsertSystem(prompt, behaviorContext.planModeInstructions)
			}
		} else {
			prompt = chatprompt.InsertSystem(prompt, PlanningSubagentOverlayPrompt)
		}
	}
	return prompt
}

type rootChatToolsOptions struct {
	chat            database.Chat
	modelConfigID   uuid.UUID
	workspaceCtx    *turnWorkspaceContext
	workspaceMu     *sync.Mutex
	resolvePlanPath func(context.Context) (string, string, error)
	storeFile       chattool.StoreFileFunc
	isPlanModeTurn  bool
}

func (p *Server) loadPlanModeInstructions(
	ctx context.Context,
	mode database.NullChatPlanMode,
) (string, error) {
	if !mode.Valid || mode.ChatPlanMode != database.ChatPlanModePlan {
		return "", nil
	}

	// Plan-mode instructions live in deployment config, but chat workers do
	// not carry a deployment-config actor during background execution.
	//nolint:gocritic // Required to read deployment config during background chat processing.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	fetched, err := p.db.GetChatPlanModeInstructions(systemCtx)
	if err != nil {
		return "", xerrors.Errorf("get plan mode instructions: %w", err)
	}
	return fetched, nil
}

func userSkillContext(ctx context.Context, userID uuid.UUID) context.Context {
	actor := rbac.Subject{
		Type:  rbac.SubjectTypeUser,
		ID:    userID.String(),
		Roles: rbac.RoleIdentifiers{rbac.RoleMember()},
		Scope: rbac.ScopeAll,
	}.WithCachedASTValue()
	// Chat turns run asynchronously after admission, so the original request
	// actor may no longer be available when a worker loads personal skills.
	// We synthesize the chat owner as a member instead of reusing that actor.
	// Hardcoding RoleMember is safe because dbauthz enforces
	// ResourceUserSkill.WithOwner(userID), so this actor cannot read any other
	// user's skills regardless of role. Org scoping is not needed because
	// personal skills are user-scoped, not org-scoped.
	//nolint:gocritic // The synthetic actor is intentional for the reasons above.
	return dbauthz.As(ctx, actor)
}

// skillLoader says who asked for a stored skill body. Skills that are not
// model-invocable stay hidden from the agent but load for users who invoke
// them explicitly.
type skillLoader int

const (
	// skillLoaderModel loads a skill the agent asked for with read_skill.
	skillLoaderModel skillLoader = iota
	// skillLoaderUser loads a skill the user invoked in a message.
	skillLoaderUser
)

// hides reports whether the loader must treat a skill as missing.
func (l skillLoader) hides(enabled, modelInvocable bool) bool {
	return !enabled || (l == skillLoaderModel && !modelInvocable)
}

func (p *Server) fetchPersonalSkillMetadata(
	ctx context.Context,
	userID uuid.UUID,
) ([]skillspkg.Skill, error) {
	rows, err := p.db.ListUserSkillMetadataByUserID(userSkillContext(ctx, userID), userID)
	if err != nil {
		return nil, xerrors.Errorf("list personal skill metadata: %w", err)
	}

	personalSkills := make([]skillspkg.Skill, 0, len(rows))
	for _, row := range rows {
		if skillLoaderModel.hides(row.Enabled, row.ModelInvocable) {
			continue
		}
		personalSkills = append(personalSkills, skillspkg.Skill{
			Name:        row.Name,
			Description: row.Description,
			Source:      skillspkg.SourcePersonal,
		})
	}
	return personalSkills, nil
}

func (p *Server) loadPersonalSkillBody(
	ctx context.Context,
	userID uuid.UUID,
	name string,
	loader skillLoader,
) (skillspkg.ParsedSkill, error) {
	row, err := p.db.GetUserSkillByUserIDAndName(
		userSkillContext(ctx, userID),
		database.GetUserSkillByUserIDAndNameParams{
			UserID: userID,
			Name:   name,
		},
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return skillspkg.ParsedSkill{}, skillspkg.ErrSkillNotFound
		}
		p.logger.Error(ctx, "load personal skill body failed",
			slog.F("user_id", userID),
			slog.F("name", name),
			slog.Error(err),
		)
		return skillspkg.ParsedSkill{}, xerrors.Errorf("load personal skill body: %w", err)
	}
	if loader.hides(row.Enabled, row.ModelInvocable) {
		return skillspkg.ParsedSkill{}, skillspkg.ErrSkillNotFound
	}

	parsed, err := skillspkg.ParsePersonalSkillMarkdown([]byte(row.Content))
	if err != nil {
		p.logger.Error(ctx, "parse personal skill body failed",
			slog.F("user_id", userID),
			slog.F("name", name),
			slog.Error(err),
		)
		return skillspkg.ParsedSkill{}, xerrors.Errorf("parse personal skill body: %w", err)
	}
	return parsed, nil
}

// chatOwnerSubject returns the chat owner's real RBAC subject, with their
// roles and groups, so organization skill reads honor per-skill ACLs.
// userSkillContext has no organization membership, so no grant matches it.
func (p *Server) chatOwnerSubject(ctx context.Context, ownerID uuid.UUID) (rbac.Subject, error) {
	ownerCtx, err := p.callerModelConfigContext(ctx, ownerID)
	if err != nil {
		return rbac.Subject{}, xerrors.Errorf("load chat owner subject: %w", err)
	}
	actor, ok := dbauthz.ActorFromContext(ownerCtx)
	if !ok || actor.Type != rbac.SubjectTypeUser || actor.ID != ownerID.String() {
		return rbac.Subject{}, xerrors.New("chat owner subject is unavailable")
	}
	return actor, nil
}

func (p *Server) fetchOrganizationSkillMetadata(
	ctx context.Context,
	owner rbac.Subject,
	organizationID uuid.UUID,
) ([]skillspkg.Skill, error) {
	//nolint:gocritic // Org skill ACLs are evaluated against the chat owner.
	rows, err := p.db.ListOrganizationSkillMetadataByOrganizationID(dbauthz.As(ctx, owner), organizationID)
	if err != nil {
		return nil, xerrors.Errorf("list organization skill metadata: %w", err)
	}

	organizationSkills := make([]skillspkg.Skill, 0, len(rows))
	for _, row := range rows {
		if skillLoaderModel.hides(row.Enabled, row.ModelInvocable) {
			continue
		}
		organizationSkills = append(organizationSkills, skillspkg.Skill{
			Name:        row.Name,
			Description: row.Description,
			Source:      skillspkg.SourceOrganization,
		})
	}
	return organizationSkills, nil
}

func (p *Server) loadOrganizationSkillBody(
	ctx context.Context,
	owner rbac.Subject,
	organizationID uuid.UUID,
	name string,
	loader skillLoader,
) (skillspkg.ParsedSkill, error) {
	//nolint:gocritic // Org skill ACLs are evaluated against the chat owner.
	row, err := p.db.GetOrganizationSkillByOrganizationIDAndName(
		dbauthz.As(ctx, owner),
		database.GetOrganizationSkillByOrganizationIDAndNameParams{
			OrganizationID: organizationID,
			Name:           name,
		},
	)
	if err != nil {
		// A skill that is no longer shared with the owner is not authorized.
		if errors.Is(err, sql.ErrNoRows) || dbauthz.IsNotAuthorizedError(err) {
			return skillspkg.ParsedSkill{}, skillspkg.ErrSkillNotFound
		}
		p.logger.Error(ctx, "load organization skill body failed",
			slog.F("organization_id", organizationID),
			slog.F("name", name),
			slog.Error(err),
		)
		return skillspkg.ParsedSkill{}, xerrors.Errorf("load organization skill body: %w", err)
	}
	if loader.hides(row.Enabled, row.ModelInvocable) {
		return skillspkg.ParsedSkill{}, skillspkg.ErrSkillNotFound
	}

	parsed, err := skillspkg.ParsePersonalSkillMarkdown([]byte(row.Content))
	if err != nil {
		p.logger.Error(ctx, "parse organization skill body failed",
			slog.F("organization_id", organizationID),
			slog.F("name", name),
			slog.Error(err),
		)
		return skillspkg.ParsedSkill{}, xerrors.Errorf("parse organization skill body: %w", err)
	}
	parsed.Source = skillspkg.SourceOrganization
	return parsed, nil
}

// fetchProjectSkillMetadata lists the enabled, model-invocable skills of a
// chat project. It reads as chatd, so callers must first check that the chat
// owner can use the project with usableChatProject, as for project
// instructions and memory.
func (p *Server) fetchProjectSkillMetadata(
	ctx context.Context,
	projectID uuid.UUID,
) ([]skillspkg.Skill, error) {
	rows, err := p.db.ListProjectSkillMetadataByProjectID(ctx, projectID)
	if err != nil {
		return nil, xerrors.Errorf("list project skill metadata: %w", err)
	}

	projectSkills := make([]skillspkg.Skill, 0, len(rows))
	for _, row := range rows {
		if skillLoaderModel.hides(row.Enabled, row.ModelInvocable) {
			continue
		}
		projectSkills = append(projectSkills, skillspkg.Skill{
			Name:        row.Name,
			Description: row.Description,
			Source:      skillspkg.SourceProject,
		})
	}
	return projectSkills, nil
}

// loadProjectSkillBody loads the current body of a chat project skill. Like
// fetchProjectSkillMetadata, it reads as chatd and relies on the caller's
// project access check. It returns skillspkg.ErrSkillNotFound when the skill
// or project is gone, or the skill is disabled or hidden from loader.
func (p *Server) loadProjectSkillBody(
	ctx context.Context,
	projectID uuid.UUID,
	name string,
	loader skillLoader,
) (skillspkg.ParsedSkill, error) {
	row, err := p.db.GetProjectSkillByProjectIDAndName(ctx, database.GetProjectSkillByProjectIDAndNameParams{
		ProjectID: projectID,
		Name:      name,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return skillspkg.ParsedSkill{}, skillspkg.ErrSkillNotFound
		}
		p.logger.Error(ctx, "load project skill body failed",
			slog.F("project_id", projectID),
			slog.F("name", name),
			slog.Error(err),
		)
		return skillspkg.ParsedSkill{}, xerrors.Errorf("load project skill body: %w", err)
	}
	if loader.hides(row.Enabled, row.ModelInvocable) {
		return skillspkg.ParsedSkill{}, skillspkg.ErrSkillNotFound
	}

	parsed, err := skillspkg.ParsePersonalSkillMarkdown([]byte(row.Content))
	if err != nil {
		p.logger.Error(ctx, "parse project skill body failed",
			slog.F("project_id", projectID),
			slog.F("name", name),
			slog.Error(err),
		)
		return skillspkg.ParsedSkill{}, xerrors.Errorf("parse project skill body: %w", err)
	}
	parsed.Source = skillspkg.SourceProject
	return parsed, nil
}

func (p *Server) appendRootChatTools(
	ctx context.Context,
	tools []fantasy.AgentTool,
	opts rootChatToolsOptions,
) ([]fantasy.AgentTool, error) {
	onChatUpdated := func(updatedChat database.Chat) {
		opts.workspaceCtx.selectWorkspace(updatedChat)
		// Notify the frontend immediately so it can start streaming
		// build logs before the tool completes.
		p.publishChatPubsubEvent(updatedChat, codersdk.ChatWatchEventKindStatusChange, nil)
	}

	tools = append(tools,
		chattool.ListTemplates(p.db, opts.chat.OrganizationID, chattool.ListTemplatesOptions{
			OwnerID: opts.chat.OwnerID,
			Logger:  p.logger,
			Clock:   p.clock,
		}),
		chattool.ReadTemplate(p.db, opts.chat.OrganizationID, chattool.ReadTemplateOptions{
			OwnerID:          opts.chat.OwnerID,
			RenderParameters: p.renderTemplateParametersFn,
			Logger:           p.logger,
		}),
		chattool.CreateWorkspace(p.db, opts.chat.OrganizationID, opts.chat.ID, chattool.CreateWorkspaceOptions{
			OwnerID:                        opts.chat.OwnerID,
			CreateFn:                       p.createWorkspaceFn,
			AgentConnFn:                    chattool.AgentConnFunc(p.agentConnFn),
			AgentInactiveDisconnectTimeout: p.agentInactiveDisconnectTimeout,
			WorkspaceMu:                    opts.workspaceMu,
			OnChatUpdated:                  onChatUpdated,
			Logger:                         p.logger,
		}),
		chattool.StartWorkspace(p.db, opts.chat.ID, chattool.StartWorkspaceOptions{
			OwnerID:       opts.chat.OwnerID,
			StartFn:       p.startWorkspaceFn,
			AgentConnFn:   chattool.AgentConnFunc(p.agentConnFn),
			WorkspaceMu:   opts.workspaceMu,
			OnChatUpdated: onChatUpdated,
			Logger:        p.logger,
		}),
		chattool.StopWorkspace(p.db, opts.chat.ID, chattool.StopWorkspaceOptions{
			OwnerID:       opts.chat.OwnerID,
			StopFn:        p.stopWorkspaceFn,
			WorkspaceMu:   opts.workspaceMu,
			OnChatUpdated: onChatUpdated,
			Logger:        p.logger,
		}),
	)
	if opts.isPlanModeTurn {
		tools = append(tools, chattool.ProposePlan(chattool.ProposePlanOptions{
			GetWorkspaceConn: opts.workspaceCtx.getWorkspaceConn,
			ResolvePlanPath:  opts.resolvePlanPath,
			IsPlanTurn:       opts.isPlanModeTurn,
			StoreFile:        opts.storeFile,
		}))
	}

	subagentTools, err := p.subagentTools(ctx, func() database.Chat {
		return opts.chat
	}, opts.modelConfigID)
	if err != nil {
		return nil, err
	}
	tools = append(tools, p.acpTools(ctx, opts)...)
	return append(tools, subagentTools...), nil
}

func appendDynamicTools(
	ctx context.Context,
	logger slog.Logger,
	tools []fantasy.AgentTool,
	raw pqtype.NullRawMessage,
	planMode database.NullChatPlanMode,
	chatMode database.NullChatMode,
) ([]fantasy.AgentTool, map[string]bool, error) {
	if isExploreSubagentMode(chatMode) || (planMode.Valid && planMode.ChatPlanMode == database.ChatPlanModePlan) {
		return tools, nil, nil
	}

	dynamicToolNames, err := parseDynamicToolNames(raw)
	if err != nil {
		return nil, nil, xerrors.Errorf("parse dynamic tool names: %w", err)
	}
	if len(dynamicToolNames) == 0 {
		return tools, dynamicToolNames, nil
	}

	var dynamicToolDefs []codersdk.DynamicTool
	if raw.Valid {
		if err := json.Unmarshal(raw.RawMessage, &dynamicToolDefs); err != nil {
			return nil, nil, xerrors.Errorf("unmarshal dynamic tools: %w", err)
		}
	}

	for _, t := range tools {
		info := t.Info()
		if dynamicToolNames[info.Name] {
			logger.Warn(ctx, "dynamic tool name collides with built-in tool, built-in takes precedence",
				slog.F("tool_name", info.Name))
			delete(dynamicToolNames, info.Name)
		}
	}

	var filteredDefs []codersdk.DynamicTool
	for _, dt := range dynamicToolDefs {
		if dynamicToolNames[dt.Name] {
			filteredDefs = append(filteredDefs, dt)
		}
	}

	return append(tools, dynamicToolsFromSDK(logger, filteredDefs)...), dynamicToolNames, nil
}

// buildProviderTools creates provider-native tool definitions
// (like web search) based on the model configuration. These
// tools are executed server-side by the LLM provider.
func buildProviderTools(options *codersdk.ChatModelProviderOptions) []chatloop.ProviderTool {
	var tools []chatloop.ProviderTool

	if options == nil {
		return nil
	}

	if options.Anthropic != nil && options.Anthropic.WebSearchEnabled != nil && *options.Anthropic.WebSearchEnabled {
		tools = append(tools, chatloop.ProviderTool{
			Definition: anthropic.WebSearchTool(&anthropic.WebSearchToolOptions{
				AllowedDomains: options.Anthropic.AllowedDomains,
				BlockedDomains: options.Anthropic.BlockedDomains,
			}),
		})
	}

	if tool, ok := chatopenai.WebSearchTool(options.OpenAI); ok {
		tools = append(tools, chatloop.ProviderTool{
			Definition: tool,
		})
	}

	if options.Google != nil && options.Google.WebSearchEnabled != nil && *options.Google.WebSearchEnabled {
		tools = append(tools, chatloop.ProviderTool{
			Definition: fantasy.ProviderDefinedTool{
				ID:   "web_search",
				Name: "web_search",
			},
		})
	}

	return tools
}

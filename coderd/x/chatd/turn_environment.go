package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
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

type turnEnvironment interface {
	Turn() *turnState
	ModelConfig() *turnModelConfig
	Prompt() []fantasy.Message
	Toolset() *turnToolset
	CompactionConfig() *generationCompaction
	Close()
}

type turnState struct {
	chat     database.Chat
	messages []database.ChatMessage
	maxSteps int
	debug    *generationDebug
}

type turnModelConfig struct {
	model                chatprovider.Model
	buildOptions         modelBuildOptions
	resolvedProvider     string
	configID             uuid.UUID
	callTemplate         fantasy.Call
	contextLimitFallback int64
}

type turnToolset struct {
	tools              []fantasy.AgentTool
	activeTools        []string
	allowInactiveTools map[string]bool
	providerTools      []chatloop.ProviderTool
	dynamicToolNames   map[string]bool
	stopAfterTools     map[string]struct{}
	exclusiveToolNames map[string]bool
	builtinToolNames   map[string]bool
	toolNameToConfigID map[string]uuid.UUID
}

func (t turnToolset) IsExclusive(name string) bool    { return t.exclusiveToolNames[name] }
func (t turnToolset) IsDynamic(name string) bool      { return t.dynamicToolNames[name] }
func (t turnToolset) IsBuiltin(name string) bool      { return t.builtinToolNames[name] }
func (t turnToolset) AllowsInactive(name string) bool { return t.allowInactiveTools[name] }
func (t turnToolset) StopsAfter(name string) bool     { _, ok := t.stopAfterTools[name]; return ok }
func (t turnToolset) ConfigID(name string) (uuid.UUID, bool) {
	id, ok := t.toolNameToConfigID[name]
	return id, ok
}

type turnEnvironmentState struct {
	turn       turnState
	model      turnModelConfig
	prompt     []fantasy.Message
	toolset    turnToolset
	compaction *generationCompaction
	cleanup    func()
}

func (e *turnEnvironmentState) Turn() *turnState                        { return &e.turn }
func (e *turnEnvironmentState) ModelConfig() *turnModelConfig           { return &e.model }
func (e *turnEnvironmentState) Prompt() []fantasy.Message               { return e.prompt }
func (e *turnEnvironmentState) Toolset() *turnToolset                   { return &e.toolset }
func (e *turnEnvironmentState) CompactionConfig() *generationCompaction { return e.compaction }
func (e *turnEnvironmentState) Close() {
	if e.cleanup != nil {
		e.cleanup()
	}
}

// effectiveMCPServerConfigs loads the chat's stored selection plus
// owner-readable Force On configs at generation time, so stored lists
// predating enforcement cannot dodge the policy (Cure53 CDM-02-010).
// Explore chats keep their immutable spawn-time snapshot instead.
func (server *Server) effectiveMCPServerConfigs(
	ctx context.Context,
	logger slog.Logger,
	chat database.Chat,
) ([]database.MCPServerConfig, error) {
	var configs []database.MCPServerConfig
	if len(chat.MCPServerIDs) > 0 {
		var err error
		configs, err = enabledMCPServerConfigsForChatOrg(ctx, server.db, chat.OrganizationID, chat.MCPServerIDs)
		if err != nil {
			// Best-effort for the user-selected set, matching prior
			// behavior: a load failure degrades the turn rather than
			// failing it.
			logger.Warn(ctx, "failed to load MCP server configs", slog.Error(err))
			configs = nil
		}
	}
	if isExploreSubagentMode(chat.Mode) {
		return configs, nil
	}
	forced, err := forcedMCPServerConfigsForOwner(ctx, server.db, chat.OrganizationID, chat.OwnerID)
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

func buildTurnEnvironment(
	ctx context.Context,
	server *Server,
	input generationPrepareInput,
) (turnEnvironment, error) {
	builder := turnEnvironmentBuilder{server: server}
	chat := input.Chat
	logger := server.logger.With(
		slog.F("chat_id", chat.ID),
		slog.F("owner_id", chat.OwnerID),
	)

	prepStart := server.clock.Now()
	defer func() {
		if prepDuration := server.clock.Since(prepStart); prepDuration >= slowPrepareThreshold {
			logger.Warn(ctx, "slow generation preparation",
				slog.F("duration", prepDuration),
			)
		}
	}()

	var (
		promptRows []database.ChatMessage
		mcpConfigs []database.MCPServerConfig
		mcpTokens  []database.MCPServerUserToken
	)

	var g errgroup.Group
	g.Go(func() error {
		var err error
		promptRows, err = server.db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("get chat messages for prompt: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		mcpConfigs, err = server.effectiveMCPServerConfigs(ctx, logger, chat)
		return err
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	apiKeyID, err := server.ensureSyntheticAPIKeyID(ctx, chat.OwnerID)
	if err != nil {
		return nil, xerrors.Errorf("ensure synthetic API key: %w", err)
	}
	modelOpts := modelBuildOptions{ActiveAPIKeyID: apiKeyID}

	requestedEffort := chatRequestedEffort(chat)
	resolved, err := server.resolveModelCall(ctx, modelCallSpec{
		purpose:         "standard_turn",
		chat:            chat,
		requestedEffort: requestedEffort,
		buildOptions:    modelOpts,
	})
	if err != nil {
		return nil, err
	}
	// The chat config keeps driving compaction, sanitization, and debug
	// attribution even when computer use swaps the resolved call below.
	modelConfig := resolved.dbConfig

	// Computer-use turns swap in a specialized model, so the substitution
	// must happen before anything model-sensitive runs: file-part
	// classification, history sanitization, and provider option preparation
	// must all agree with the client actually used for the turn.
	isComputerUse := chat.Mode.Valid && chat.Mode.ChatMode == database.ChatModeComputerUse
	var computerUseProvider codersdk.ChatComputerUseProvider
	if isComputerUse {
		var cuModelProvider, cuModelName string
		computerUseProvider, cuModelProvider, cuModelName, err = server.computerUseProviderAndModelFromConfig(ctx)
		if err != nil {
			return nil, xerrors.Errorf("resolve computer use provider and model: %w", err)
		}
		cuResolved, cuErr := server.resolveModelCall(ctx, modelCallSpec{
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
			return nil, xerrors.Errorf(
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
	if isExploreSubagent && isRootChat {
		mcpConnectConfigs = nil
		approvedPlanMCPConfigIDs = map[uuid.UUID]struct{}{}
	}

	planModeInstructions := builder.loadPlanModeInstructions(ctx, currentPlanMode, logger)
	advisorCfg := server.loadAdvisorConfig(ctx, logger)
	// Force Enabled from the experiment; the stored DB value is ignored.
	advisorCfg.Enabled = server.experiments.Enabled(codersdk.ExperimentChatAdvisor)

	var advisorRuntime *chatadvisor.Runtime
	if advisorCfg.Enabled && isRootChat && !isPlanModeTurn && !isExploreSubagent {
		var advisorErr error
		advisorRuntime, advisorErr = server.newAdvisorRuntime(
			ctx,
			chat,
			advisorCfg,
			modelOpts,
			logger,
		)
		if advisorErr != nil {
			return nil, advisorErr
		}
	}

	var advisorPromptSnapshot []fantasy.Message
	setAdvisorPromptSnapshot := func(msgs []fantasy.Message) {
		if advisorRuntime == nil {
			return
		}
		advisorPromptSnapshot = slices.Clone(msgs)
	}

	currentChat := chat
	loadChatSnapshot := func(loadCtx context.Context, chatID uuid.UUID) (database.Chat, error) {
		return server.db.GetChatByID(loadCtx, chatID)
	}
	var chatStateMu sync.Mutex
	var workspaceMu sync.Mutex
	workspaceCtx := turnWorkspaceContext{
		server:           server,
		chatStateMu:      &chatStateMu,
		currentChat:      &currentChat,
		loadChatSnapshot: loadChatSnapshot,
	}
	cleanup := func() {
		workspaceCtx.close()
	}

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
		prompt             []fantasy.Message
		instruction        string
		mcpTools           []fantasy.AgentTool
		mcpSummaries       []mcpclient.ConnectSummary
		mcpCleanup         func()
		workspaceMCPTools  []fantasy.AgentTool
		workspaceSkills    []chattool.SkillMeta
		personalSkills     []skillspkg.Skill
		resolvedUserPrompt string
		planPathBlock      string
	)

	// Drop provider-executed tool history produced by a different provider
	// before building the prompt. A provider that shares another's wire format
	// (e.g. Bedrock and Anthropic) can still reject the other's
	// provider-executed blocks, so a mid-chat provider switch must not replay
	// them.
	promptRows = server.sanitizeForeignProviderExecutedToolRows(ctx, logger, promptRows, chat.OwnerID, modelConfig.ID)

	if chat.WorkspaceID.Valid {
		// Resolve the workspace agent so the chat row's AgentID and
		// BuildID bindings are up to date before the chatworker
		// decision helper inspects them. ensureWorkspaceAgent does a
		// DB lookup and lazily calls persistBuildAgentBinding when
		// the bound agent has changed, so this is a cheap metadata
		// refresh, not a workspace dial. It must not insert chat
		// history; only metadata is mutated here.
		agent, _ := workspaceCtx.getWorkspaceAgent(ctx)

		// API-created chats bind their agent lazily here, after
		// hydrateChatContextOnCreate ran with no agent. Pin the chat to the
		// bound agent's pushed snapshot now if it is still unpinned, so the
		// first turn reads workspace context instead of waiting for the
		// agent's next push. Idempotent and snapshot-gated; runs before the
		// pinned context is read below.
		server.ensureChatContextPinnedOnFirstTurn(ctx, workspaceCtx.currentChatSnapshot())

		var resolveErr error
		instruction, workspaceSkills, resolveErr = server.resolveTurnWorkspaceContext(ctx, chat, agent)
		if resolveErr != nil {
			cleanup()
			return nil, resolveErr
		}
	}

	// Build the debug context before the connect phase so its
	// outcomes can be recorded with run-creation context even when a
	// later preparation step fails.
	triggerMessageID, historyTipMessageID, triggerLabel := deriveChatDebugSeed(promptRows)
	debugSvc := server.existingDebugService()
	var debug *generationDebug
	if resolved.debugEnabled {
		if debugSvc == nil {
			cleanup()
			return nil, xerrors.New("chat debug service missing after enablement check")
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
		prompt, err = chatprompt.ConvertMessagesWithFiles(ctx, promptRows, server.chatFileResolver(providerType), logger, acceptsFilePart)
		if err != nil {
			return xerrors.Errorf("build chat prompt: %w", err)
		}
		return nil
	})
	g2.Go(func() error {
		personalSkills = builder.fetchPersonalSkillMetadata(ctx, chat.OwnerID, logger)
		return nil
	})
	g2.Go(func() error {
		resolvedUserPrompt = server.resolveUserPrompt(ctx, chat.OwnerID)
		return nil
	})
	if len(mcpConnectConfigs) > 0 {
		g2.Go(func() error {
			var tokenErr error
			mcpTokens, tokenErr = server.db.GetMCPServerUserTokensByUserID(ctx, chat.OwnerID)
			if tokenErr != nil {
				logger.Warn(ctx, "failed to load MCP user tokens", slog.Error(tokenErr))
			}
			mcpTokens = server.refreshExpiredMCPTokens(ctx, logger, mcpConnectConfigs, mcpTokens)
			mcpTools, mcpSummaries, mcpCleanup = mcpclient.ConnectAll(
				ctx,
				logger,
				mcpConnectConfigs,
				mcpTokens,
				chat.OwnerID,
				server.oidcTokenSource,
				chatprovider.CoderHeaders(chat),
			)
			return nil
		})
	}
	if chat.WorkspaceID.Valid && !isPlanModeTurn && !isExploreSubagent {
		g2.Go(func() error {
			workspaceMCPTools = server.resolveWorkspaceMCPTools(ctx, logger, chat, &workspaceCtx)
			return nil
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
	// Record connect outcomes before acting on any preparation error:
	// ConnectAll has already run, so a failure below (or in g2 itself)
	// would otherwise discard this attempt's outcomes.
	if debug != nil && input.RecordMCPConnectSummaries != nil && len(mcpSummaries) > 0 {
		input.RecordMCPConnectSummaries(ctx, chat, debug, mcpSummaries)
	}
	if g2Err != nil {
		cleanup()
		return nil, g2Err
	}

	if mcpCleanup != nil {
		previousCleanup := cleanup
		cleanup = func() {
			mcpCleanup()
			previousCleanup()
		}
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
		return mergeTurnSkills(personalSkills, workspaceSkills)
	}
	resolveSkillAlias := func(alias string) (skillspkg.ResolvedSkill, error) {
		return skillspkg.Lookup(resolvedSkillsFor(workspaceSkills), alias)
	}
	initialResolvedSkills := resolvedSkillsFor(workspaceSkills)

	prompt = buildSystemPrompt(
		prompt,
		subagentInstruction,
		instruction,
		initialResolvedSkills,
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
	setAdvisorPromptSnapshot(prompt)

	storeChatAttachment := server.newStoreChatAttachmentFunc(&workspaceCtx)
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
		tools = builder.appendRootChatTools(ctx, tools, rootChatToolsOptions{
			chat:            chat,
			modelConfigID:   modelConfig.ID,
			workspaceCtx:    &workspaceCtx,
			workspaceMu:     &workspaceMu,
			resolvePlanPath: resolvePlanPathForTools,
			storeFile:       storeChatAttachment,
			isPlanModeTurn:  isPlanModeTurn,
		})
	}

	skillOpts := chattool.ReadSkillOptions{
		GetWorkspaceConn: workspaceCtx.getWorkspaceConn,
		GetSkills: func() []chattool.SkillMeta {
			return workspaceSkills
		},
		ResolveAlias: resolveSkillAlias,
		LoadPersonalSkillBody: func(ctx context.Context, name string) (skillspkg.ParsedSkill, error) {
			return builder.loadPersonalSkillBody(ctx, chat.OwnerID, name)
		},
	}
	appendCurrentSkillTools := func(current []fantasy.AgentTool) ([]fantasy.AgentTool, bool) {
		if len(personalSkills) == 0 && len(workspaceSkills) == 0 {
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
	tools = append(tools, mcpTools...)
	if !isExploreSubagent {
		tools = append(tools, workspaceMCPTools...)
	}
	tools = filterToolsForTurn(tools, currentPlanMode, chat.ParentChatID, approvedPlanMCPConfigIDs)

	tools, dynamicToolNames, err := appendDynamicTools(ctx, logger, tools, chat.DynamicTools, currentPlanMode, chat.Mode)
	if err != nil {
		cleanup()
		return nil, err
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
			clock:            server.clock,
			logger:           server.logger.Named("computer_use"),
		})
		if err != nil {
			cleanup()
			return nil, xerrors.Errorf("register computer use provider tool for provider %q: %w", computerUseProvider, err)
		}
	} else {
		providerTools, err = appendComputerUseProviderTool(providerTools, computerUseProviderToolOptions{
			isPlanModeTurn: isPlanModeTurn,
			isComputerUse:  false,
		})
		if err != nil {
			cleanup()
			return nil, err
		}
	}

	activeToolNames := activeToolNamesForTurn(tools, currentPlanMode, chat.ParentChatID, approvedPlanMCPConfigIDs)
	if isExploreSubagent {
		activeToolNames = allowedExploreToolNames(tools)
	}
	var allowInactiveTools map[string]bool
	if decideMCPToolSearch(mcpToolSearchInput{
		experimentEnabled: server.experiments.Enabled(codersdk.ExperimentMCPToolSearch),
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
					server.metrics.FindToolsMatchCount.Observe(float64(call.MatchCount))
					server.metrics.FindToolsActivationsTotal.Add(float64(len(call.Activated)))
					if call.MatchCount == 0 {
						server.metrics.FindToolsEmptyTotal.Inc()
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

	toolNameToConfigID := make(map[string]uuid.UUID)
	for _, t := range tools {
		if mcpTool, ok := t.(mcpclient.MCPToolIdentifier); ok {
			toolNameToConfigID[t.Info().Name] = mcpTool.MCPServerConfigID()
		}
	}

	compactionToolCallID := "chat_summarized_" + uuid.NewString()
	effectiveThreshold := modelConfig.CompressionThreshold
	if override, ok := server.resolveUserCompactionThreshold(ctx, chat.OwnerID, modelConfig.ID); ok {
		effectiveThreshold = override
	}
	// The compaction trigger uses the stricter of the chat and override
	// models' context limits: the history must also fit the summarizer's
	// window.
	compactionContextLimit := modelConfig.ContextLimit
	compactionOverride, err := server.resolveCompactionOverrideConfig(ctx, chat)
	if err != nil {
		cleanup()
		return nil, err
	}
	if compactionOverride != nil {
		if overrideLimit := compactionOverride.Config.ContextLimit; overrideLimit > 0 &&
			(compactionContextLimit <= 0 || overrideLimit < compactionContextLimit) {
			compactionContextLimit = overrideLimit
		}
	}
	compactionStepUsage := latestPromptUsage(promptRows)
	compactionNeeded := shouldCompactPromptUsage(compactionStepUsage, compactionContextLimit, effectiveThreshold)
	// The options carry the chat model; generateCompaction swaps in the
	// override client when one is configured.
	compactionOptions := chatloop.GenerateCompactionOptions{
		Model:                model.LanguageModel(),
		Messages:             prompt,
		ThresholdPercent:     effectiveThreshold,
		ContextLimit:         compactionContextLimit,
		ContextLimitFallback: compactionContextLimit,
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
	}

	// workspaceCtx.currentChatSnapshot may carry a freshly persisted
	// AgentID/BuildID binding from the getWorkspaceAgent call above.
	// Return that snapshot so downstream consumers see the up-to-date
	// metadata.
	refreshedChat := workspaceCtx.currentChatSnapshot()
	if refreshedChat.ID == uuid.Nil {
		refreshedChat = chat
	}

	return &turnEnvironmentState{
		turn: turnState{
			chat: refreshedChat, messages: input.Messages,
			maxSteps: maxChatSteps, debug: debug,
		},
		model: turnModelConfig{
			model: model, buildOptions: modelOpts,
			resolvedProvider: resolved.resolvedProvider,
			configID:         modelConfig.ID, callTemplate: resolved.newCall(),
			contextLimitFallback: modelConfig.ContextLimit,
		},
		prompt: prompt,
		toolset: turnToolset{
			tools: tools, activeTools: activeToolNames,
			allowInactiveTools: allowInactiveTools, providerTools: providerTools,
			dynamicToolNames:   dynamicToolNames,
			stopAfterTools:     stopAfterBehaviorTools(currentPlanMode, chat.Mode, chat.ParentChatID),
			exclusiveToolNames: exclusiveToolNames, builtinToolNames: builtinToolNames,
			toolNameToConfigID: toolNameToConfigID,
		},
		compaction: &generationCompaction{
			Override: compactionOverride, ChatModelConfig: modelConfig,
			Required: compactionNeeded, Options: compactionOptions,
		},
		cleanup: cleanup,
	}, nil
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

func shouldCompactPromptUsage(usage fantasy.Usage, contextLimit int64, thresholdPercent int32) bool {
	if thresholdPercent >= 100 || contextLimit <= 0 {
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

func (server *Server) afterInterruptionOutcome(
	ctx context.Context,
	outcome interruptionOutcome,
) error {
	chat := outcome.Chat
	logger := server.logger.With(slog.F("chat_id", chat.ID), slog.F("owner_id", chat.OwnerID))

	if outcome.Kind == runnerActionKindFinishInterruption && !chat.ParentChatID.Valid {
		server.clearLastTurnSummaryAsync(context.WithoutCancel(ctx), chat, logger)
	}
	return nil
}

func (server *Server) afterGenerationOutcome(
	ctx context.Context,
	outcome generationOutcome,
) error {
	chat := outcome.Chat
	logger := server.logger.With(slog.F("chat_id", chat.ID), slog.F("owner_id", chat.OwnerID))

	switch outcome.Kind {
	case runnerActionKindFinishTurn:
		finalizeCtx := context.WithoutCancel(ctx)
		runResult := server.deriveFinalTurnRunResult(finalizeCtx, chat, logger)
		server.maybeFinalizeTurnStatusLabelAndPush(finalizeCtx, chat, chat.Status, "", runResult, logger)
	case runnerActionKindFinishError:
		server.maybeFinalizeTurnStatusLabelAndPush(context.WithoutCancel(ctx), chat, chat.Status, outcome.LastError, runChatResult{}, logger)
	case runnerActionKindEnterRequiresAction:
		server.maybeFinalizeTurnStatusLabelAndPush(context.WithoutCancel(ctx), chat, chat.Status, "", runChatResult{}, logger)
	}
	return nil
}

// deriveFinalTurnRunResult rebuilds the inputs needed to generate the
// end-of-turn status label directly from persisted state.
func (server *Server) deriveFinalTurnRunResult(
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

	promptRows, err := server.db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	if err != nil {
		logger.Warn(ctx, "derive final turn status label: load prompt rows", slog.Error(err))
		return runChatResult{}
	}
	triggerMessageID, historyTipMessageID, _ := deriveChatDebugSeed(promptRows)
	finalAssistantText := latestAssistantText(promptRows)
	if finalAssistantText == "" {
		return runChatResult{}
	}

	apiKeyID, err := server.ensureSyntheticAPIKeyID(ctx, chat.OwnerID)
	if err != nil {
		logger.Warn(ctx, "derive final turn status label: ensure synthetic API key", slog.Error(err))
		return runChatResult{FinalAssistantText: finalAssistantText, TriggerMessageID: triggerMessageID, HistoryTipMessageID: historyTipMessageID}
	}
	modelOpts := modelBuildOptions{ActiveAPIKeyID: apiKeyID}
	resolved, err := server.resolveModelCall(ctx, modelCallSpec{
		purpose:      "turn_status_label",
		chat:         chat,
		buildOptions: modelOpts,
	})
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
	updatedChat, err := c.server.db.UpdateChatBuildAgentBinding(
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
		return chatSnapshot, xerrors.Errorf(
			"update chat build/agent binding: %w", err,
		)
	}

	// If the chat was rebound to a different agent (e.g. a workspace rebuild
	// produced a new agent), re-pin its context to the new agent so it stops
	// injecting the previous agent's resources. Workspace lifecycle tools clear
	// the agent binding while preserving the pin, so a missing prior agent also
	// requires a re-pin when pinned context exists. Best-effort: a context error
	// must never fail the binding. The pinned context fields on updatedChat are
	// background state, reloaded on the next snapshot fetch.
	hasStaleUnboundContext := !chatSnapshot.AgentID.Valid && chatSnapshot.ContextAggregateHash != nil
	if hasStaleUnboundContext || (chatSnapshot.AgentID.Valid && chatSnapshot.AgentID.UUID != agentID) {
		//nolint:gocritic // Chatd re-pins chats it does not own as the daemon subject.
		repinCtx := dbauthz.AsChatd(ctx)
		if repinErr := database.ReadModifyUpdate(c.server.db, func(tx database.Store) error {
			return repinChatContext(repinCtx, tx, chatSnapshot.ID, uuid.NullUUID{UUID: agentID, Valid: true})
		}); repinErr != nil {
			c.server.logger.Warn(ctx, "re-pin chat context after agent rebind",
				slog.F("chat_id", chatSnapshot.ID),
				slog.F("agent_id", agentID),
				slog.Error(repinErr))
		}
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
			return chatSnapshot, database.WorkspaceAgent{}, xerrors.New("no workspace is associated with this chat. Use the create_workspace tool to create one")
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
				c.server.logger.Warn(ctx, "agent binding lookup failed, re-resolving",
					slog.F("agent_id", chatSnapshot.AgentID.UUID),
					slog.Error(err),
				)
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
			// canceled (e.g. ErrInterrupted), its error must
			// propagate unchanged so the chatloop can detect it.
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

func allToolNames(allTools []fantasy.AgentTool) []string {
	toolNames := make([]string, 0, len(allTools))
	for _, tool := range allTools {
		toolNames = append(toolNames, tool.Info().Name)
	}
	return toolNames
}

func isExploreSubagentMode(mode database.NullChatMode) bool {
	return mode.Valid && mode.ChatMode == database.ChatModeExplore
}

// filterExternalMCPConfigsForTurn returns the external MCP server configs
// visible on the current turn. Explore children snapshot this filtered set at
// spawn time so later model overrides cannot widen the external-tool boundary.
func filterExternalMCPConfigsForTurn(
	configs []database.MCPServerConfig,
	mode database.NullChatPlanMode,
	parentChatID uuid.NullUUID,
) ([]database.MCPServerConfig, map[uuid.UUID]struct{}) {
	if !mode.Valid || mode.ChatPlanMode != database.ChatPlanModePlan {
		return configs, nil
	}
	if parentChatID.Valid {
		// Plan-mode subagents do not receive external MCP tools because
		// their trust boundary is narrower than the root chat's.
		return nil, map[uuid.UUID]struct{}{}
	}

	filtered := make([]database.MCPServerConfig, 0, len(configs))
	approvedIDs := make(map[uuid.UUID]struct{})
	for _, cfg := range configs {
		if !cfg.AllowInPlanMode {
			continue
		}
		filtered = append(filtered, cfg)
		approvedIDs[cfg.ID] = struct{}{}
	}
	return filtered, approvedIDs
}

func builtinPlanToolAllowed(name string, isRootChat bool) bool {
	switch name {
	case "read_file", "execute", "process_output", "read_skill", "read_skill_file":
		return true
	case "write_file", "edit_files", "list_templates", "read_template",
		"create_workspace", "start_workspace", "stop_workspace", "propose_plan", "spawn_agent",
		"spawn_explore_agent", "wait_agent", "list_agents", "list_subagent_models",
		"ask_user_question", "attach_file":
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

// activeToolNamesForTurn extends the built-in plan allowlist with approved
// external MCP tools for root plan-mode chats.
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

func allowedExploreToolNames(allTools []fantasy.AgentTool) []string {
	builtinExplorePolicy := map[string]bool{
		"read_file":            true,
		"write_file":           false,
		"edit_files":           false,
		"execute":              true,
		"process_output":       true,
		"process_list":         false,
		"process_signal":       false,
		"list_templates":       false,
		"read_template":        false,
		"create_workspace":     false,
		"start_workspace":      false,
		"stop_workspace":       false,
		"propose_plan":         false,
		"spawn_agent":          false,
		"wait_agent":           false,
		"message_agent":        false,
		"interrupt_agent":      false,
		"close_agent":          false,
		"list_agents":          false,
		"list_subagent_models": false,
		"read_skill":           true,
		"read_skill_file":      true,
		"ask_user_question":    false,
	}

	toolNames := make([]string, 0, len(allTools))
	for _, tool := range allTools {
		name := tool.Info().Name
		if builtinExplorePolicy[name] {
			toolNames = append(toolNames, name)
			continue
		}
		// External MCP tools pass through here. They were snapshot-filtered
		// at spawn time on chat.MCPServerIDs. WorkspaceMCPTool does not
		// implement MCPToolIdentifier, so workspace tools are excluded
		// here too, in addition to the structural exclusion in runChat
		// tool assembly.
		if _, ok := tool.(mcpclient.MCPToolIdentifier); ok {
			toolNames = append(toolNames, name)
		}
	}
	return toolNames
}

// allowedBehaviorToolNames runs only on non-plan turns because
// appendDynamicTools returns early for plan mode. Within that boundary,
// Explore mode wins over the default behavior that allows all tools.
func allowedBehaviorToolNames(
	allTools []fantasy.AgentTool,
	chatMode database.NullChatMode,
) []string {
	if isExploreSubagentMode(chatMode) {
		return allowedExploreToolNames(allTools)
	}
	return allToolNames(allTools)
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
	workspaceSkills []chattool.SkillMeta,
) []skillspkg.ResolvedSkill {
	return skillspkg.MergeSkills(
		personalSkills,
		workspaceSkillsForResolution(workspaceSkills),
	)
}

// buildSystemPrompt applies system-level prompt injections in a fixed
// order: subagent instruction, chat instruction, skill index, user prompt,
// then mode overlay prompts.
func buildSystemPrompt(
	prompt []fantasy.Message,
	subagentInstruction string,
	instruction string,
	resolvedSkills []skillspkg.ResolvedSkill,
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

func removeSkillIndexMessages(prompt []fantasy.Message) []fantasy.Message {
	out := make([]fantasy.Message, 0, len(prompt))
	removed := false
	for _, message := range prompt {
		if isSkillIndexMessage(message) {
			removed = true
			continue
		}
		out = append(out, message)
	}
	if !removed {
		return prompt
	}
	return out
}

func isSkillIndexMessage(message fantasy.Message) bool {
	if message.Role != fantasy.MessageRoleSystem || len(message.Content) != 1 {
		return false
	}
	textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](message.Content[0])
	if !ok {
		return false
	}
	text := strings.TrimSpace(textPart.Text)
	return strings.HasPrefix(text, chattool.AvailableSkillsOpenTag+"\n") && strings.HasSuffix(text, chattool.AvailableSkillsCloseTag)
}

type turnEnvironmentBuilder struct {
	server *Server
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

func (builder turnEnvironmentBuilder) loadPlanModeInstructions(
	ctx context.Context,
	mode database.NullChatPlanMode,
	logger slog.Logger,
) string {
	if !mode.Valid || mode.ChatPlanMode != database.ChatPlanModePlan {
		return ""
	}

	// Plan-mode instructions live in deployment config, but chat workers do
	// not carry a deployment-config actor during background execution.
	//nolint:gocritic // Required to read deployment config during background chat processing.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	fetched, err := builder.server.db.GetChatPlanModeInstructions(systemCtx)
	if err != nil {
		logger.Warn(ctx,
			"failed to fetch plan mode instructions",
			slog.Error(err),
		)
		return ""
	}

	return fetched
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

func (builder turnEnvironmentBuilder) fetchPersonalSkillMetadata(
	ctx context.Context,
	userID uuid.UUID,
	logger slog.Logger,
) []skillspkg.Skill {
	rows, err := builder.server.db.ListUserSkillMetadataByUserID(userSkillContext(ctx, userID), userID)
	// See package coderd/x/skills (doc.go) for why metadata fetch failures
	// intentionally degrade to an empty personal-skill list instead of
	// failing the chat turn.
	if err != nil {
		logger.Warn(ctx, "failed to load personal skill metadata",
			slog.F("owner_id", userID),
			slog.Error(err),
		)
		return nil
	}

	personalSkills := make([]skillspkg.Skill, 0, len(rows))
	for _, row := range rows {
		personalSkills = append(personalSkills, skillspkg.Skill{
			Name:        row.Name,
			Description: row.Description,
			Source:      skillspkg.SourcePersonal,
		})
	}
	return personalSkills
}

func (builder turnEnvironmentBuilder) loadPersonalSkillBody(
	ctx context.Context,
	userID uuid.UUID,
	name string,
) (skillspkg.ParsedSkill, error) {
	row, err := builder.server.db.GetUserSkillByUserIDAndName(
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
		builder.server.logger.Error(ctx, "load personal skill body failed",
			slog.F("user_id", userID),
			slog.F("name", name),
			slog.Error(err),
		)
		return skillspkg.ParsedSkill{}, xerrors.Errorf("load personal skill body: %w", err)
	}

	parsed, err := skillspkg.ParsePersonalSkillMarkdown([]byte(row.Content))
	if err != nil {
		builder.server.logger.Error(ctx, "parse personal skill body failed",
			slog.F("user_id", userID),
			slog.F("name", name),
			slog.Error(err),
		)
		return skillspkg.ParsedSkill{}, xerrors.Errorf("parse personal skill body: %w", err)
	}
	return parsed, nil
}

func (builder turnEnvironmentBuilder) appendRootChatTools(
	ctx context.Context,
	tools []fantasy.AgentTool,
	opts rootChatToolsOptions,
) []fantasy.AgentTool {
	onChatUpdated := func(updatedChat database.Chat) {
		opts.workspaceCtx.selectWorkspace(updatedChat)
		// Notify the frontend immediately so it can start streaming
		// build logs before the tool completes.
		builder.server.publishChatPubsubEvent(updatedChat, codersdk.ChatWatchEventKindStatusChange, nil)
	}

	tools = append(tools,
		chattool.ListTemplates(builder.server.db, opts.chat.OrganizationID, chattool.ListTemplatesOptions{
			OwnerID: opts.chat.OwnerID,
			Logger:  builder.server.logger,
			Clock:   builder.server.clock,
		}),
		chattool.ReadTemplate(builder.server.db, opts.chat.OrganizationID, chattool.ReadTemplateOptions{
			OwnerID: opts.chat.OwnerID,
		}),
		chattool.CreateWorkspace(builder.server.db, opts.chat.OrganizationID, opts.chat.ID, chattool.CreateWorkspaceOptions{
			OwnerID:                        opts.chat.OwnerID,
			CreateFn:                       builder.server.createWorkspaceFn,
			AgentConnFn:                    chattool.AgentConnFunc(builder.server.agentConnFn),
			AgentInactiveDisconnectTimeout: builder.server.agentInactiveDisconnectTimeout,
			WorkspaceMu:                    opts.workspaceMu,
			OnChatUpdated:                  onChatUpdated,
			Logger:                         builder.server.logger,
		}),
		chattool.StartWorkspace(builder.server.db, opts.chat.ID, chattool.StartWorkspaceOptions{
			OwnerID:       opts.chat.OwnerID,
			StartFn:       builder.server.startWorkspaceFn,
			AgentConnFn:   chattool.AgentConnFunc(builder.server.agentConnFn),
			WorkspaceMu:   opts.workspaceMu,
			OnChatUpdated: onChatUpdated,
			Logger:        builder.server.logger,
		}),
		chattool.StopWorkspace(builder.server.db, opts.chat.ID, chattool.StopWorkspaceOptions{
			OwnerID:       opts.chat.OwnerID,
			StopFn:        builder.server.stopWorkspaceFn,
			WorkspaceMu:   opts.workspaceMu,
			OnChatUpdated: onChatUpdated,
			Logger:        builder.server.logger,
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

	return append(tools, builder.server.subagentTools(ctx, func() database.Chat {
		return opts.chat
	}, opts.modelConfigID)...)
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

	activeToolNames := make(map[string]struct{}, len(tools))
	for _, name := range allowedBehaviorToolNames(tools, chatMode) {
		activeToolNames[name] = struct{}{}
	}
	for _, t := range tools {
		info := t.Info()
		if _, active := activeToolNames[info.Name]; !active {
			continue
		}
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

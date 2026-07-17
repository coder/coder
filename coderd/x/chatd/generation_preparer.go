package chatd

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatadvisor"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatsanitize"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	skillspkg "github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/codersdk"
)

func (server *Server) prepareGeneration(
	ctx context.Context,
	input generationPrepareInput,
) (generationPrepared, error) {
	chat := input.Chat
	logger := server.logger.With(
		slog.F("chat_id", chat.ID),
		slog.F("owner_id", chat.OwnerID),
	)

	var (
		model            fantasy.LanguageModel
		modelConfig      database.ChatModelConfig
		modelRoute       aiGatewayModelRoute
		modelOpts        modelBuildOptions
		callConfig       codersdk.ChatModelCallConfig
		promptRows       []database.ChatMessage
		mcpConfigs       []database.MCPServerConfig
		mcpTokens        []database.MCPServerUserToken
		debugEnabled     bool
		resolvedProvider string
		debugModel       string
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
	if len(chat.MCPServerIDs) > 0 {
		g.Go(func() error {
			var err error
			mcpConfigs, err = server.db.GetMCPServerConfigsByIDs(ctx, chat.MCPServerIDs)
			if err != nil {
				logger.Warn(ctx, "failed to load MCP server configs", slog.Error(err))
			}
			return nil
		})
		g.Go(func() error {
			var err error
			mcpTokens, err = server.db.GetMCPServerUserTokensByUserID(ctx, chat.OwnerID)
			if err != nil {
				logger.Warn(ctx, "failed to load MCP user tokens", slog.Error(err))
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return generationPrepared{}, err
	}

	modelOpts = modelBuildOptionsFromMessages(promptRows)
	ctx = withActiveTurnAPIKeyID(ctx, modelOpts)

	var err error
	model, modelConfig, modelRoute, debugEnabled, resolvedProvider, debugModel, err = server.resolveChatModel(ctx, chat, modelOpts)
	if err != nil {
		return generationPrepared{}, err
	}
	if len(modelConfig.Options) > 0 {
		if err := json.Unmarshal(modelConfig.Options, &callConfig); err != nil {
			return generationPrepared{}, xerrors.Errorf("parse model call config: %w", err)
		}
	}

	if callConfig.MaxOutputTokens == nil {
		maxOutputTokens := int64(32_000)
		callConfig.MaxOutputTokens = &maxOutputTokens
	}

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

	planModeInstructions := server.loadPlanModeInstructions(ctx, currentPlanMode, logger)
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
			model,
			callConfig,
			modelOpts,
			logger,
		)
		if advisorErr != nil {
			return generationPrepared{}, advisorErr
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
	promptRows = server.sanitizeForeignProviderExecutedToolRows(ctx, logger, promptRows, modelConfig.ID)

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
			return generationPrepared{}, resolveErr
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
		acceptsFilePart := func(mediaType string) bool {
			return chatprovider.AcceptsFilePartMediaType(model.Provider(), model.Model(), mediaType)
		}
		providerType := string(modelRoute.Provider.Type)
		prompt, err = chatprompt.ConvertMessagesWithFiles(ctx, promptRows, server.chatFileResolver(providerType), logger, acceptsFilePart)
		if err != nil {
			return xerrors.Errorf("build chat prompt: %w", err)
		}
		return nil
	})
	g2.Go(func() error {
		personalSkills = server.fetchPersonalSkillMetadata(ctx, chat.OwnerID, logger)
		return nil
	})
	g2.Go(func() error {
		resolvedUserPrompt = server.resolveUserPrompt(ctx, chat.OwnerID)
		return nil
	})
	if len(mcpConnectConfigs) > 0 {
		g2.Go(func() error {
			mcpTokens = server.refreshExpiredMCPTokens(ctx, logger, mcpConnectConfigs, mcpTokens)
			mcpTools, mcpCleanup = mcpclient.ConnectAll(
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
	if err := g2.Wait(); err != nil {
		cleanup()
		return generationPrepared{}, err
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
		model.Model(),
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
		chattool.Execute(chattool.ExecuteOptions{GetWorkspaceConn: workspaceCtx.getWorkspaceConn}),
		chattool.ProcessOutput(chattool.ProcessToolOptions{GetWorkspaceConn: workspaceCtx.getWorkspaceConn}),
		chattool.ProcessList(chattool.ProcessToolOptions{GetWorkspaceConn: workspaceCtx.getWorkspaceConn}),
		chattool.ProcessSignal(chattool.ProcessToolOptions{GetWorkspaceConn: workspaceCtx.getWorkspaceConn}),
	}
	if isPlanModeTurn && isRootChat {
		tools = append(tools, chattool.NewAskUserQuestionTool())
	}
	if isRootChat {
		tools = server.appendRootChatTools(ctx, tools, rootChatToolsOptions{
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
			return server.loadPersonalSkillBody(ctx, chat.OwnerID, name)
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

	tools = append(tools, mcpTools...)
	if !isExploreSubagent {
		tools = append(tools, workspaceMCPTools...)
	}
	tools = filterToolsForTurn(tools, currentPlanMode, chat.ParentChatID, approvedPlanMCPConfigIDs)

	tools, dynamicToolNames, err := appendDynamicTools(ctx, logger, tools, chat.DynamicTools, currentPlanMode, chat.Mode)
	if err != nil {
		cleanup()
		return generationPrepared{}, err
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

	isComputerUse := chat.Mode.Valid && chat.Mode.ChatMode == database.ChatModeComputerUse
	if isComputerUse {
		computerUseProvider, computerUseModelProvider, computerUseModelName, err := server.computerUseProviderAndModelFromConfig(ctx)
		if err != nil {
			cleanup()
			return generationPrepared{}, xerrors.Errorf("resolve computer use provider and model: %w", err)
		}
		computerUseRoute, keyErr := server.resolveModelRouteForProviderType(ctx, chat.OwnerID, computerUseModelProvider)
		if keyErr != nil {
			cleanup()
			return generationPrepared{}, xerrors.Errorf("resolve computer use provider route: %w", keyErr)
		}
		modelRoute = computerUseRoute
		cuModel, cuDebugEnabled, cuResolvedProvider, cuResolvedModel, cuErr := server.resolveComputerUseModel(
			ctx,
			chat,
			computerUseRoute,
			computerUseProvider,
			computerUseModelProvider,
			computerUseModelName,
			modelOpts,
		)
		if cuErr != nil {
			cleanup()
			return generationPrepared{}, cuErr
		}
		model = cuModel
		debugEnabled = cuDebugEnabled
		resolvedProvider = cuResolvedProvider
		debugModel = cuResolvedModel
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
			return generationPrepared{}, xerrors.Errorf("register computer use provider tool for provider %q: %w", computerUseProvider, err)
		}
	} else {
		providerTools, err = appendComputerUseProviderTool(providerTools, computerUseProviderToolOptions{
			isPlanModeTurn: isPlanModeTurn,
			isComputerUse:  false,
		})
		if err != nil {
			cleanup()
			return generationPrepared{}, err
		}
	}

	var requestedEffort *string
	if chat.LastReasoningEffort.Valid {
		requestedEffort = new(string(chat.LastReasoningEffort.ChatReasoningEffort))
	}
	reasoningEffort := chatprovider.ResolveReasoningEffort(
		requestedEffort,
		callConfig.ReasoningEffort,
	)
	providerOptions := chatprovider.ProviderOptionsFromChatModelConfig(model, callConfig.ProviderOptions)
	providerOptions = chatprovider.ApplyReasoningEffort(model, providerOptions, reasoningEffort)

	activeToolNames := activeToolNamesForTurn(tools, currentPlanMode, chat.ParentChatID, approvedPlanMCPConfigIDs)
	if isExploreSubagent {
		activeToolNames = allowedExploreToolNames(tools)
	}

	toolNameToConfigID := make(map[string]uuid.UUID)
	for _, t := range tools {
		if mcpTool, ok := t.(mcpclient.MCPToolIdentifier); ok {
			toolNameToConfigID[t.Info().Name] = mcpTool.MCPServerConfigID()
		}
	}

	triggerMessageID, historyTipMessageID, triggerLabel := deriveChatDebugSeed(promptRows)
	debugSvc := server.existingDebugService()
	var debug *generationDebug
	if debugEnabled {
		if debugSvc == nil {
			cleanup()
			return generationPrepared{}, xerrors.New("chat debug service missing after enablement check")
		}
		debug = &generationDebug{
			Enabled:             true,
			Service:             debugSvc,
			Provider:            resolvedProvider,
			Model:               debugModel,
			TriggerMessageID:    triggerMessageID,
			HistoryTipMessageID: historyTipMessageID,
			TriggerLabel:        triggerLabel,
			ModelConfig:         modelConfig,
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
		return generationPrepared{}, err
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
		Model:                model,
		Messages:             prompt,
		ThresholdPercent:     effectiveThreshold,
		ContextLimit:         compactionContextLimit,
		ContextLimitFallback: compactionContextLimit,
		ToolCallID:           compactionToolCallID,
		ToolName:             "chat_summarized",
		DebugSvc:             debugSvc,
		ChatID:               chat.ID,
		HistoryTipMessageID:  historyTipMessageID,
		ResolvedProvider:     resolvedProvider,
		ResolvedModel:        debugModel,
		ModelConfigID:        modelConfig.ID,
		StepUsage:            compactionStepUsage,
	}

	// workspaceCtx.currentChatSnapshot may carry a freshly persisted
	// AgentID/BuildID binding from the getWorkspaceAgent call above.
	// Return that snapshot so downstream consumers see the up-to-date
	// metadata.
	refreshedChat := workspaceCtx.currentChatSnapshot()
	if refreshedChat.ID == uuid.Nil {
		refreshedChat = chat
	}

	return generationPrepared{
		Chat:                 refreshedChat,
		Messages:             input.Messages,
		Model:                model,
		Prompt:               prompt,
		Tools:                tools,
		ActiveTools:          activeToolNames,
		ProviderTools:        providerTools,
		ModelRoute:           modelRoute,
		ModelBuildOptions:    modelOpts,
		ResolvedProvider:     resolvedProvider,
		ModelConfigID:        modelConfig.ID,
		ModelConfig:          callConfig,
		ProviderOptions:      providerOptions,
		ContextLimitFallback: modelConfig.ContextLimit,
		DynamicToolNames:     dynamicToolNames,
		StopAfterTools:       stopAfterBehaviorTools(currentPlanMode, chat.Mode, chat.ParentChatID),
		ExclusiveToolNames:   exclusiveToolNames,
		BuiltinToolNames:     builtinToolNames,
		ToolNameToConfigID:   toolNameToConfigID,
		MaxSteps:             maxChatSteps,
		Compaction: &generationCompaction{
			Override:        compactionOverride,
			ChatModelConfig: modelConfig,
			Required:        compactionNeeded,
			Options:         compactionOptions,
		},
		Cleanup: cleanup,
		Debug:   debug,
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

	if outcome.Kind == runnerActionKindFinishInterruption {
		server.maybeClearLastTurnSummaryAsync(context.WithoutCancel(ctx), chat, logger)
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

	// resolvedProvider/resolvedModel describe the model the fallback handle was
	// built from; they only feed the status-label fallback candidate's labels.
	modelOpts := modelBuildOptionsFromMessages(promptRows)
	ctx = withActiveTurnAPIKeyID(ctx, modelOpts)
	model, _, modelRoute, _, resolvedProvider, resolvedModel, err := server.resolveChatModel(ctx, chat, modelOpts)
	if err != nil {
		// Return what we have; generateFinalTurnStatusLabel falls back to a
		// generic label when StatusLabelModel is nil.
		logger.Warn(ctx, "derive final turn status label: resolve model", slog.Error(err))
		return runChatResult{
			FinalAssistantText:  finalAssistantText,
			TriggerMessageID:    triggerMessageID,
			HistoryTipMessageID: historyTipMessageID,
		}
	}

	return runChatResult{
		FinalAssistantText:  finalAssistantText,
		StatusLabelModel:    model,
		FallbackProvider:    resolvedProvider,
		FallbackRoute:       modelRoute,
		FallbackModel:       resolvedModel,
		ModelBuildOptions:   modelOpts,
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

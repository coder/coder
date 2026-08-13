package chatd

import (
	"context"
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
		return generationPrepared{}, err
	}

	apiKeyID, err := server.ensureSyntheticAPIKeyID(ctx, chat.OwnerID)
	if err != nil {
		return generationPrepared{}, xerrors.Errorf("ensure synthetic API key: %w", err)
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
		return generationPrepared{}, err
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
			return generationPrepared{}, xerrors.Errorf("resolve computer use provider and model: %w", err)
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
			return generationPrepared{}, xerrors.Errorf(
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
			resolved,
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
		acceptsFilePart := model.AcceptsFilePartMediaType
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
			var tokenErr error
			mcpTokens, tokenErr = server.db.GetMCPServerUserTokensByUserID(ctx, chat.OwnerID)
			if tokenErr != nil {
				logger.Warn(ctx, "failed to load MCP user tokens", slog.Error(tokenErr))
			}
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

	triggerMessageID, historyTipMessageID, triggerLabel := deriveChatDebugSeed(promptRows)
	debugSvc := server.existingDebugService()
	var debug *generationDebug
	if resolved.debugEnabled {
		if debugSvc == nil {
			cleanup()
			return generationPrepared{}, xerrors.New("chat debug service missing after enablement check")
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

	return generationPrepared{
		Chat:                 refreshedChat,
		Messages:             input.Messages,
		Model:                model,
		Prompt:               prompt,
		Tools:                tools,
		ActiveTools:          activeToolNames,
		AllowInactiveTools:   allowInactiveTools,
		ProviderTools:        providerTools,
		ModelRoute:           modelRoute,
		ModelBuildOptions:    modelOpts,
		ResolvedProvider:     resolved.resolvedProvider,
		ModelConfigID:        modelConfig.ID,
		CallTemplate:         resolved.newCall(),
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

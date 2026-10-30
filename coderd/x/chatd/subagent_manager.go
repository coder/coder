package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
)

// subagentManager owns child chat lifecycle for one running parent turn.
type subagentManager struct {
	server               *Server
	currentChat          func() database.Chat
	currentChatSnapshot  database.Chat
	currentModelConfigID uuid.UUID
}

type subagentManagerError struct {
	err  error
	chat *database.Chat
}

func (e *subagentManagerError) Error() string { return e.err.Error() }
func (e *subagentManagerError) Unwrap() error { return e.err }

// Tool outputs keep fields in sorted JSON key order so the payload text
// the model sees stays stable.
type subagentAwaitResult struct {
	ChatID             string  `json:"chat_id"`
	LastError          string  `json:"last_error,omitempty"`
	LastErrorDetail    string  `json:"last_error_detail,omitempty"`
	LastErrorKind      string  `json:"last_error_kind,omitempty"`
	LastErrorRetryable *bool   `json:"last_error_retryable,omitempty"`
	RecordingFileID    string  `json:"recording_file_id,omitempty"`
	Report             *string `json:"report,omitempty"`
	Status             string  `json:"status"`
	ThumbnailFileID    string  `json:"thumbnail_file_id,omitempty"`
	TimedOut           bool    `json:"timed_out,omitempty"`
	Title              string  `json:"title"`
	Type               string  `json:"type"`
}

type subagentListEntry struct {
	ChatID    string `json:"chat_id"`
	CreatedAt string `json:"created_at"`
	Status    string `json:"status"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	UpdatedAt string `json:"updated_at"`
}

type subagentListResult struct {
	Agents   []subagentListEntry `json:"agents"`
	HasMore  bool                `json:"has_more"`
	Offset   int                 `json:"offset"`
	Returned int                 `json:"returned"`
	Total    int                 `json:"total"`
}

func newSubagentManager(server *Server, currentChat func() database.Chat, currentModelConfigID uuid.UUID) *subagentManager {
	manager := &subagentManager{server: server, currentChat: currentChat, currentModelConfigID: currentModelConfigID}
	if currentChat != nil {
		manager.currentChatSnapshot = currentChat()
	}
	return manager
}

func (m *subagentManager) Spawn(ctx context.Context, args spawnAgentArgs) (database.Chat, error) {
	if m.currentChat == nil {
		return database.Chat{}, xerrors.New("subagent callbacks are not configured")
	}
	parent, err := m.server.loadSubagentSpawnParentChat(ctx, m.currentChat)
	if err != nil {
		return database.Chat{}, err
	}
	definition, err := resolveSubagentDefinition(ctx, m.server, parent, args.Type)
	if err != nil {
		return database.Chat{}, err
	}
	if definition.id == subagentTypeComputerUse && (strings.TrimSpace(args.ModelConfigID) != "" || strings.TrimSpace(args.ReasoningEffort) != "") {
		return database.Chat{}, xerrors.New("model_config_id and reasoning_effort are not supported for type \"" + subagentTypeComputerUse + "\"")
	}
	explicitModelConfigID, explicitReasoningEffort, err := m.server.resolveExplicitSpawnOverrides(ctx, parent.OwnerID, parent.OrganizationID, args)
	if err != nil {
		return database.Chat{}, err
	}
	turnParent := m.currentChatSnapshot
	if turnParent.ID == uuid.Nil {
		turnParent = parent
	}
	options, err := definition.buildOptions(ctx, m.server, parent, turnParent, m.currentModelConfigID, explicitModelConfigID, args.Prompt)
	if err != nil {
		return database.Chat{}, err
	}
	if explicitReasoningEffort != nil {
		options.reasoningEffortOverride = explicitReasoningEffort
	}
	return m.server.createChildSubagentChatWithOptions(ctx, parent, args.Prompt, args.Title, options)
}

func (m *subagentManager) ListModels(ctx context.Context) ([]map[string]any, error) {
	if m.currentChat == nil {
		return nil, xerrors.New("subagent callbacks are not configured")
	}
	parent, err := m.server.loadSubagentSpawnParentChat(ctx, m.currentChat)
	if err != nil {
		return nil, err
	}
	models, err := m.server.listSpawnableModelConfigs(ctx, parent.OwnerID, parent.OrganizationID)
	if err != nil {
		m.server.logger.Warn(ctx, "failed to list spawnable model configs", slog.F("chat_id", parent.ID), slog.Error(err))
		return nil, xerrors.New("internal error listing model configs")
	}
	return models, nil
}

func (m *subagentManager) Await(ctx context.Context, args waitAgentArgs) (subagentAwaitResult, error) {
	if m.currentChat == nil {
		return subagentAwaitResult{}, xerrors.New("subagent callbacks are not configured")
	}
	targetChatID, err := parseSubagentToolChatID(args.ChatID)
	if err != nil {
		return subagentAwaitResult{}, err
	}
	parent := m.currentChat()
	targetChatInfo := m.targetChatInfo(ctx, targetChatID, "recording")
	isDescendant, err := isSubagentDescendant(ctx, m.server.db, parent.ID, targetChatID)
	if err != nil {
		return subagentAwaitResult{}, subagentToolError(xerrors.Errorf("failed to verify subagent relationship: %v", err), targetChatInfo)
	}
	if !isDescendant {
		return subagentAwaitResult{}, subagentToolError(ErrSubagentNotDescendant, targetChatInfo)
	}

	// Recording a computer_use child is best-effort and never blocks the wait.
	var recordingID string
	var agentConn workspacesdk.AgentConn
	if targetChatInfo != nil && targetChatInfo.Mode.Valid && targetChatInfo.Mode.ChatMode == database.ChatModeComputerUse && targetChatInfo.AgentID.Valid && m.server.agentConnFn != nil {
		conn, closeFn, connErr := m.server.agentConnFn(ctx, targetChatInfo.AgentID.UUID)
		if connErr == nil {
			agentConn = conn
			defer closeFn()
			recordingID = targetChatID.String()
			if startErr := conn.StartDesktopRecording(ctx, workspacesdk.StartDesktopRecordingRequest{RecordingID: recordingID}); startErr != nil {
				m.server.logger.Warn(ctx, "failed to start desktop recording", slog.Error(startErr))
				recordingID = ""
			}
		} else {
			m.server.logger.Warn(ctx, "failed to get agent conn for recording", slog.Error(connErr))
		}
	}

	timeout := defaultSubagentWaitTimeout
	if args.TimeoutSeconds != nil {
		timeout = time.Duration(*args.TimeoutSeconds) * time.Second
	}
	// On timeout or error, leave the recording running on the agent so
	// the next wait_agent call continues it. Only success stops and
	// stores it.
	targetChat, report, awaitErr := m.server.awaitSubagentCompletion(ctx, parent.ID, targetChatID, timeout)
	if xerrors.Is(awaitErr, ErrSubagentWaitTimeout) {
		// The agent may have completed between the last poll and the
		// timer firing, so re-check with a fresh read before reporting
		// a timeout.
		checkedChat, checkedReport, done, checkErr := m.server.checkSubagentCompletion(ctx, targetChatID)
		if checkErr != nil {
			return subagentAwaitResult{}, subagentToolError(checkErr, targetChatInfo)
		}
		if !done {
			return subagentAwaitResult{ChatID: targetChatID.String(), Status: string(checkedChat.Status), TimedOut: true, Title: checkedChat.Title, Type: subagentTypeFromChat(checkedChat)}, nil
		}
		targetChat, report, awaitErr = handleSubagentDone(checkedChat, checkedReport)
	}
	if awaitErr != nil {
		if statusErr, ok := errors.AsType[*subagentStatusError](awaitErr); ok {
			return subagentAwaitStatusErrorResult(statusErr), nil
		}
		return subagentAwaitResult{}, subagentToolError(awaitErr, targetChatInfo)
	}
	return m.server.waitAgentSuccessResult(ctx, recordingID, agentConn, parent, targetChat, report), nil
}

func (m *subagentManager) Send(ctx context.Context, args messageAgentArgs) (database.Chat, bool, error) {
	if m.currentChat == nil {
		return database.Chat{}, false, xerrors.New("subagent callbacks are not configured")
	}
	targetChatID, err := parseSubagentToolChatID(args.ChatID)
	if err != nil {
		return database.Chat{}, false, err
	}
	targetChatInfo := m.targetChatInfo(ctx, targetChatID, "message")
	message := strings.TrimSpace(args.Message)
	if message == "" {
		return database.Chat{}, false, subagentToolError(xerrors.New("message is required"), targetChatInfo)
	}
	parent := m.currentChat()
	isDescendant, err := isSubagentDescendant(ctx, m.server.db, parent.ID, targetChatID)
	if err != nil {
		return database.Chat{}, false, subagentToolError(err, targetChatInfo)
	}
	if !isDescendant {
		return database.Chat{}, false, subagentToolError(ErrSubagentNotDescendant, targetChatInfo)
	}
	targetChat, err := m.server.db.GetChatByID(ctx, targetChatID)
	if err != nil {
		return database.Chat{}, false, subagentToolError(xerrors.Errorf("get target chat: %w", err), targetChatInfo)
	}
	busyBehavior := SendMessageBusyBehaviorQueue
	if args.Interrupt {
		busyBehavior = SendMessageBusyBehaviorInterrupt
	}
	sendResult, err := m.server.SendMessage(ctx, SendMessageOptions{
		ChatID:       targetChatID,
		CreatedBy:    targetChat.OwnerID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText(message)},
		BusyBehavior: busyBehavior,
	})
	if err != nil {
		return database.Chat{}, false, subagentToolError(err, targetChatInfo)
	}
	interrupted := args.Interrupt && targetChatInfo != nil && targetChatInfo.Status == database.ChatStatusRunning
	return sendResult.Chat, interrupted, nil
}

func (m *subagentManager) Interrupt(ctx context.Context, args interruptAgentArgs) (database.Chat, bool, error) {
	if m.currentChat == nil {
		return database.Chat{}, false, xerrors.New("subagent callbacks are not configured")
	}
	targetChatID, err := parseSubagentToolChatID(args.ChatID)
	if err != nil {
		return database.Chat{}, false, err
	}
	targetChatInfo := m.targetChatInfo(ctx, targetChatID, "interrupt")
	parent := m.currentChat()
	isDescendant, err := isSubagentDescendant(ctx, m.server.db, parent.ID, targetChatID)
	if err != nil {
		return database.Chat{}, false, subagentToolError(err, targetChatInfo)
	}
	if !isDescendant {
		return database.Chat{}, false, subagentToolError(ErrSubagentNotDescendant, targetChatInfo)
	}
	targetChat, err := m.server.db.GetChatByID(ctx, targetChatID)
	if err != nil {
		return database.Chat{}, false, subagentToolError(xerrors.Errorf("get target chat: %w", err), targetChatInfo)
	}
	// Idle and paused chats have no turn to interrupt.
	if targetChat.Status == database.ChatStatusWaiting || targetChat.Status == database.ChatStatusPaused {
		return targetChat, false, nil
	}
	updatedChat, err := m.server.InterruptChat(ctx, targetChat)
	if err != nil {
		// Idle and archived chats fail the chatstate.Interrupt
		// precondition; surface that instead of reporting success.
		return database.Chat{}, false, subagentToolError(xerrors.Errorf("interrupt subagent chat: %w", err), targetChatInfo)
	}
	// Active runs land in interrupting and requires-action chats in
	// running; workers finalize either transition.
	return updatedChat, true, nil
}

func (m *subagentManager) List(ctx context.Context, args listAgentsArgs) (subagentListResult, error) {
	if m.currentChat == nil {
		return subagentListResult{}, xerrors.New("subagent callbacks are not configured")
	}
	parent := m.currentChat()
	if parent.ParentChatID.Valid {
		return subagentListResult{}, xerrors.New("list_agents is only available on root chats")
	}
	rows, err := m.server.db.GetChildChatsByParentIDs(ctx, database.GetChildChatsByParentIDsParams{
		ParentIds: []uuid.UUID{parent.ID},
		// Exclude archived children. An invalid NullBool would include
		// archived rows.
		Archived: sql.NullBool{Bool: false, Valid: true},
	})
	if err != nil {
		return subagentListResult{}, xerrors.Errorf("list child agents: %w", err)
	}
	slices.SortStableFunc(rows, func(a, b database.GetChildChatsByParentIDsRow) int {
		if c := b.Chat.UpdatedAt.Compare(a.Chat.UpdatedAt); c != 0 {
			return c
		}
		return strings.Compare(b.Chat.ID.String(), a.Chat.ID.String())
	})
	limit := defaultListAgentsLimit
	if args.Limit != nil {
		limit = min(max(*args.Limit, 1), maxListAgentsLimit)
	}
	offset := 0
	if args.Offset != nil && *args.Offset > 0 {
		offset = *args.Offset
	}
	total := len(rows)
	start := min(offset, total)
	end := min(start+limit, total)
	agents := make([]subagentListEntry, 0, end-start)
	for _, row := range rows[start:end] {
		chat := row.Chat
		agents = append(agents, subagentListEntry{ChatID: chat.ID.String(), CreatedAt: chat.CreatedAt.Format(time.RFC3339), Status: string(chat.Status), Title: chat.Title, Type: subagentTypeFromChat(chat), UpdatedAt: chat.UpdatedAt.Format(time.RFC3339)})
	}
	return subagentListResult{Agents: agents, HasMore: end < total, Offset: offset, Returned: len(agents), Total: total}, nil
}

func (m *subagentManager) targetChatInfo(ctx context.Context, chatID uuid.UUID, operation string) *database.Chat {
	chat, err := m.server.db.GetChatByID(ctx, chatID)
	if err == nil {
		return &chat
	}
	if !xerrors.Is(err, sql.ErrNoRows) {
		m.server.logger.Warn(ctx, "unexpected error looking up chat for "+operation, slog.F("chat_id", chatID), slog.Error(err))
	}
	return nil
}

func subagentToolError(err error, chat *database.Chat) error {
	return &subagentManagerError{err: err, chat: chat}
}

func subagentAwaitStatusErrorResult(statusErr *subagentStatusError) subagentAwaitResult {
	chat := statusErr.chat
	decoded, lastError := subagentLastError(chat.LastError)
	if lastError == "" {
		lastError = statusErr.reason
	}
	result := subagentAwaitResult{ChatID: chat.ID.String(), LastError: lastError, Report: &statusErr.report, Status: string(chat.Status), Title: chat.Title, Type: subagentTypeFromChat(chat)}
	if decoded != nil {
		kind := decoded.Kind
		if kind == "" {
			kind = codersdk.ChatErrorKindGeneric
		}
		retryable := decoded.Retryable
		result.LastErrorKind = string(kind)
		result.LastErrorRetryable = &retryable
		result.LastErrorDetail = decoded.Detail
	}
	return result
}

func (p *Server) loadSubagentSpawnParentChat(
	ctx context.Context,
	currentChat func() database.Chat,
) (database.Chat, error) {
	parent := currentChat()
	if err := validateSubagentSpawnParent(parent); err != nil {
		return database.Chat{}, err
	}
	reloadedParent, err := p.db.GetChatByID(ctx, parent.ID)
	if err != nil {
		p.logger.Warn(ctx, "failed to load parent chat for spawn_agent",
			slog.F("chat_id", parent.ID),
			slog.Error(err),
		)
		return database.Chat{}, xerrors.New("failed to load parent chat")
	}
	parent = reloadedParent
	if err := validateSubagentSpawnParent(parent); err != nil {
		return database.Chat{}, err
	}
	// Fail before prompt hooks run for a child that cannot be created.
	// Spawn parents are always roots, so this is the family's archived
	// flag. The locked check in chatstate.CreateChat stays authoritative.
	if parent.Archived {
		return database.Chat{}, errSpawnUnderArchivedParent(chatstate.ErrChatFamilyArchived)
	}
	return parent, nil
}

// errSpawnUnderArchivedParent wraps err, which must match
// chatstate.ErrChatFamilyArchived, into the tool error spawn_agent
// returns to the model.
func errSpawnUnderArchivedParent(err error) error {
	return xerrors.Errorf("cannot create a child agent because the parent chat is archived: %w", err)
}

func parseSubagentToolChatID(raw string) (uuid.UUID, error) {
	chatID, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, xerrors.New("chat_id must be a valid UUID")
	}
	return chatID, nil
}

// childSubagentChatOptions carries per-child overrides for subagent chat
// creation. modelConfigIDOverride, reasoningEffortOverride, and
// planModeOverride apply to any subagent. inheritedMCPServerIDs is an
// Explore-only snapshot of the spawning parent turn's effective external MCP
// entitlement. resolveExploreToolSnapshot computes and persists it on the
// child chat. Non-Explore children ignore this field.
type childSubagentChatOptions struct {
	chatMode                database.NullChatMode
	systemPrompt            string
	modelConfigIDOverride   *uuid.UUID
	reasoningEffortOverride *string
	planModeOverride        *database.NullChatPlanMode
	inheritedMCPServerIDs   []uuid.UUID
}

// resolveExploreToolSnapshot computes the child chat's inherited MCP
// server snapshot from the spawning parent turn.
//
// The MCP set is filtered in two stages. First,
// filterExternalMCPConfigsForTurn applies the parent turn's plan-mode
// policy to the parent's MCP configs, producing visibleConfigs. Second,
// if the parent is itself an Explore child, the visible set is narrowed to
// the parent's persisted MCPServerIDs so an Explore chain cannot
// re-escalate beyond the original grant. Non-Explore parents pass
// through the second stage unchanged.
func (p *Server) resolveExploreToolSnapshot(
	ctx context.Context,
	parent database.Chat,
) ([]uuid.UUID, error) {
	inheritedMCPServerIDs := []uuid.UUID{}
	if len(parent.MCPServerIDs) > 0 {
		configs, err := enabledMCPServerConfigsForChatOrg(ctx, p.db, parent.OrganizationID, parent.MCPServerIDs)
		if err != nil {
			return nil, xerrors.Errorf("get parent MCP server configs for chat %s: %w", parent.ID, err)
		}

		visibleConfigs, _ := filterExternalMCPConfigsForTurn(
			configs,
			parent.PlanMode,
			parent.ParentChatID,
		)
		// Empty means the parent is not Explore, so all plan-filtered
		// configs remain eligible. Populated means the parent is
		// Explore, so only its persisted snapshot can pass.
		allowedParentIDs := map[uuid.UUID]struct{}{}
		if isExploreSubagentMode(parent.Mode) {
			for _, id := range parent.MCPServerIDs {
				allowedParentIDs[id] = struct{}{}
			}
		}
		for _, cfg := range visibleConfigs {
			if len(allowedParentIDs) > 0 {
				if _, ok := allowedParentIDs[cfg.ID]; !ok {
					continue
				}
			}
			inheritedMCPServerIDs = append(inheritedMCPServerIDs, cfg.ID)
		}
	}

	return inheritedMCPServerIDs, nil
}

func (p *Server) createChildSubagentChatWithOptions(
	ctx context.Context,
	parent database.Chat,
	prompt string,
	title string,
	opts childSubagentChatOptions,
) (database.Chat, error) {
	if parent.ParentChatID.Valid {
		return database.Chat{}, xerrors.New("delegated chats cannot create child subagents")
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return database.Chat{}, xerrors.New("prompt is required")
	}

	title = strings.TrimSpace(title)

	rootChatID := parent.ID
	if parent.RootChatID.Valid {
		rootChatID = parent.RootChatID.UUID
	}

	modelConfigID := parent.LastModelConfigID
	if opts.modelConfigIDOverride != nil {
		modelConfigID = *opts.modelConfigIDOverride
	}
	if modelConfigID == uuid.Nil {
		return database.Chat{}, xerrors.New("model config is required")
	}
	childPlanMode := parent.PlanMode
	if opts.planModeOverride != nil {
		childPlanMode = *opts.planModeOverride
	}

	mcpServerIDs := parent.MCPServerIDs
	if isExploreSubagentMode(opts.chatMode) {
		mcpServerIDs = slices.Clone(opts.inheritedMCPServerIDs)
	}
	if mcpServerIDs == nil {
		mcpServerIDs = []uuid.UUID{}
	}

	labelsJSON, err := json.Marshal(database.StringMap{})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("marshal labels: %w", err)
	}
	childSystemPrompt := codersdk.SanitizePromptText(opts.systemPrompt)
	// Resolve the deployment prompt before opening the transaction so
	// child chat creation does not hold one DB connection while waiting
	// for another pool checkout.
	deploymentPrompt, err := p.resolveDeploymentSystemPrompt(ctx)
	if err != nil {
		return database.Chat{}, err
	}
	// Delegated chats cannot call list_agents or message_agent, so
	// strip the root-only orchestration guidance from their prompt.
	deploymentPrompt = strings.Replace(deploymentPrompt, subagentOrchestrationPromptBlock, "", 1)
	organizationPrompt, err := p.resolveOrganizationSystemPrompt(ctx, parent.OrganizationID)
	if err != nil {
		return database.Chat{}, err
	}

	// Review before persistence so spawned chats cannot bypass prompt policy.
	childChatID := uuid.New()
	var promptResult *chathooks.Result
	if p.hooks.Enabled() {
		mintedTurnID := uuid.New()
		promptMessage, err := chathooks.UserPromptMessage([]codersdk.ChatMessagePart{codersdk.ChatMessageText(prompt)})
		if err != nil {
			return database.Chat{}, err
		}
		promptResult, err = p.hooks.Trigger(ctx, chathooks.Chat{
			ID:           childChatID,
			OwnerID:      parent.OwnerID,
			WorkspaceID:  parent.WorkspaceID,
			ParentChatID: uuid.NullUUID{UUID: parent.ID, Valid: true},
			RootChatID:   uuid.NullUUID{UUID: rootChatID, Valid: true},
			TurnID:       &mintedTurnID,
		}, promptMessage, agenthooks.EventUserPromptSubmit, dispatch.CapacityClassGeneration)
		if err != nil {
			return database.Chat{}, chathooks.UserPromptDenial(err)
		}
		override, overridden, overrideErr := chathooks.UserPromptOverride(promptResult)
		if overrideErr != nil {
			return database.Chat{}, overrideErr
		}
		if overridden {
			// The overridden prompt also feeds the fallback title below.
			prompt = override
		}
	}
	titleSource := database.ChatTitleSourceUser
	if title == "" {
		title = subagentFallbackChatTitle(prompt)
		titleSource = database.ChatTitleSourceFallback
	}

	workspaceAwareness := workspaceDetachedNoCreateAwareness
	if parent.WorkspaceID.Valid {
		workspaceAwareness = workspaceAttachedAwareness
	}
	workspaceAwarenessContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText(workspaceAwareness),
	})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("marshal workspace awareness: %w", err)
	}
	childUserParts := []codersdk.ChatMessagePart{codersdk.ChatMessageText(prompt)}
	childUserParts = append(childUserParts, chathooks.UserPromptParts(promptResult)...)
	userContent, err := chatprompt.MarshalParts(childUserParts)
	if err != nil {
		return database.Chat{}, xerrors.Errorf("marshal initial user content: %w", err)
	}

	initialMessages := make([]chatstate.Message, 0, 5)
	if deploymentPrompt != "" {
		deploymentContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText(deploymentPrompt),
		})
		if err != nil {
			return database.Chat{}, xerrors.Errorf("marshal deployment system prompt: %w", err)
		}
		initialMessages = append(initialMessages, systemMessage(deploymentContent, modelConfigID))
	}
	if organizationPrompt != "" {
		organizationContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText(organizationPrompt),
		})
		if err != nil {
			return database.Chat{}, xerrors.Errorf("marshal organization system prompt: %w", err)
		}
		initialMessages = append(initialMessages, systemMessage(organizationContent, modelConfigID))
	}
	if childSystemPrompt != "" {
		childSystemPromptContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText(childSystemPrompt),
		})
		if err != nil {
			return database.Chat{}, xerrors.Errorf("marshal child system prompt: %w", err)
		}
		initialMessages = append(initialMessages, systemMessage(childSystemPromptContent, modelConfigID))
	}
	initialMessages = append(initialMessages, systemMessage(workspaceAwarenessContent, modelConfigID))

	// The child shares the parent's workspace and agent, so it inherits
	// workspace context the same way a top-level chat does: pinned from the
	// agent's latest snapshot (see hydrateChatContextOnCreate below). The
	// parent's context is not copied into child history.
	initialMessages = append(initialMessages, userMessage(userContent, modelConfigID, parent.OwnerID, opts.reasoningEffortOverride))

	publisher := p.pubsub
	if publisher == nil {
		publisher = dbpubsub.NewInMemory()
	}
	result, err := chatstate.CreateChatWithID(ctx, p.db, publisher, childChatID, chatstate.CreateChatInput{
		OrganizationID:    parent.OrganizationID,
		OwnerID:           parent.OwnerID,
		WorkspaceID:       parent.WorkspaceID,
		BuildID:           parent.BuildID,
		AgentID:           parent.AgentID,
		ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: rootChatID, Valid: true},
		LastModelConfigID: modelConfigID,
		Title:             title,
		TitleSource:       titleSource,
		Mode:              opts.chatMode,
		PlanMode:          childPlanMode,
		MCPServerIDs:      mcpServerIDs,
		Labels: pqtype.NullRawMessage{
			RawMessage: labelsJSON,
			Valid:      true,
		},
		DynamicTools:    pqtype.NullRawMessage{},
		ClientType:      parent.ClientType,
		InitialMessages: initialMessages,
		InitialStatus:   database.ChatStatusRunning,
	})
	if err != nil {
		if errors.Is(err, chatstate.ErrChatFamilyArchived) {
			return database.Chat{}, errSpawnUnderArchivedParent(err)
		}
		return database.Chat{}, xerrors.Errorf("create child chat: %w", err)
	}

	child := result.Chat

	// Pin the child to its agent's latest context snapshot, mirroring the
	// top-level create path. The child shares the parent's workspace agent,
	// so this reproduces the parent's workspace context without copying it
	// through chat history.
	p.hydrateChatContextOnCreate(ctx, child)

	p.publishChatPubsubEvent(child, codersdk.ChatWatchEventKindCreated, nil)
	return child, nil
}

func (p *Server) awaitSubagentCompletion(
	ctx context.Context,
	parentChatID uuid.UUID,
	targetChatID uuid.UUID,
	timeout time.Duration,
) (database.Chat, string, error) {
	isDescendant, err := isSubagentDescendant(ctx, p.db, parentChatID, targetChatID)
	if err != nil {
		return database.Chat{}, "", err
	}
	if !isDescendant {
		return database.Chat{}, "", ErrSubagentNotDescendant
	}

	// Check immediately before entering the poll loop.
	targetChat, report, done, checkErr := p.checkSubagentCompletion(ctx, targetChatID)
	if checkErr != nil {
		return database.Chat{}, "", checkErr
	}
	if done {
		return handleSubagentDone(targetChat, report)
	}

	if timeout <= 0 {
		timeout = defaultSubagentWaitTimeout
	}
	timer := p.clock.NewTimer(timeout, "chatd", "subagent_await")
	defer timer.Stop()

	// Subscribe for fast status notifications and use a less
	// aggressive fallback poll. If subscription fails, fall back to
	// the original 200ms polling.
	pollInterval := subagentAwaitFallbackPoll
	ch := make(chan struct{}, 1)
	notifyCh := (<-chan struct{})(ch)
	cancel, subErr := p.pubsub.SubscribeWithErr(
		coderdpubsub.ChatStateUpdateChannel(targetChatID),
		func(_ context.Context, _ []byte, _ error) {
			// Non-blocking send so we never stall the
			// pubsub dispatch goroutine.
			select {
			case ch <- struct{}{}:
			default:
			}
		},
	)
	if subErr == nil {
		defer cancel()
	} else {
		// Subscription failed; fall back to fast polling.
		pollInterval = subagentAwaitPollInterval
		notifyCh = nil
	}

	ticker := p.clock.NewTicker(pollInterval, "chatd", "subagent_poll")
	defer ticker.Stop()

	for {
		select {
		case <-notifyCh:
		case <-ticker.C:
		case <-timer.C:
			return database.Chat{}, "", ErrSubagentWaitTimeout
		case <-ctx.Done():
			return database.Chat{}, "", ctx.Err()
		}

		targetChat, report, done, checkErr = p.checkSubagentCompletion(ctx, targetChatID)
		if checkErr != nil {
			return database.Chat{}, "", checkErr
		}
		if done {
			return handleSubagentDone(targetChat, report)
		}
	}
}

// handleSubagentDone translates a completed subagent check into the
// appropriate return value. An error-status chat is returned as a typed
// subagentStatusError that carries the chat and report so the
// wait_agent handler can surface a structured, recoverable-aware payload.
func handleSubagentDone(
	chat database.Chat,
	report string,
) (database.Chat, string, error) {
	if chat.Status == database.ChatStatusError {
		reason := strings.TrimSpace(report)
		if reason == "" {
			reason = "agent reached error status"
		}
		return database.Chat{}, "", &subagentStatusError{
			chat:   chat,
			report: report,
			reason: reason,
		}
	}
	return chat, report, nil
}

// subagentGenericErrorMessage matches the normalized fallback that
// chaterror and db2sdk emit for unclassifiable failures. It carries no
// actionable information, so a provider detail replaces it entirely.
const subagentGenericErrorMessage = "The chat request failed unexpectedly."

// subagentLastError decodes a chat's last_error payload and builds the
// message surfaced to the parent model, preferring the actionable
// provider detail. The content mirrors what the chat UI renders from
// the same payload. An unrecognized payload yields an empty message so
// the caller falls back to its own status reason instead of exposing
// raw stored bytes.
func subagentLastError(raw pqtype.NullRawMessage) (*codersdk.ChatError, string) {
	if !raw.Valid {
		return nil, ""
	}
	var payload codersdk.ChatError
	if err := json.Unmarshal(raw.RawMessage, &payload); err != nil {
		return nil, ""
	}
	switch {
	case payload.Message == "" && payload.Detail == "":
		return nil, ""
	case payload.Detail == "":
		return &payload, payload.Message
	case payload.Message == "" || payload.Message == subagentGenericErrorMessage:
		return &payload, payload.Detail
	default:
		return &payload, payload.Message + " (" + payload.Detail + ")"
	}
}

// waitAgentSuccessResult stops and stores the recording (if active) and
// builds the normal completion payload for a wait_agent call.
func (p *Server) waitAgentSuccessResult(
	ctx context.Context,
	recordingID string,
	agentConn workspacesdk.AgentConn,
	parent database.Chat,
	targetChat database.Chat,
	report string,
) subagentAwaitResult {
	var recResult recordingResult
	if recordingID != "" && agentConn != nil {
		// Use a fresh context for cleanup so a canceled
		// parent context does not prevent recording storage.
		stopCtx, stopCancel := context.WithTimeout(context.WithoutCancel(ctx), subagentRecordingStopTimeout)
		defer stopCancel()
		recResult = p.stopAndStoreRecording(stopCtx, agentConn,
			recordingID, parent.ID, parent.OwnerID, parent.WorkspaceID)
	}
	return subagentAwaitResult{
		ChatID:          targetChat.ID.String(),
		RecordingFileID: recResult.recordingFileID,
		Report:          &report,
		Status:          string(targetChat.Status),
		ThumbnailFileID: recResult.thumbnailFileID,
		Title:           targetChat.Title,
		Type:            subagentTypeFromChat(targetChat),
	}
}

func (p *Server) checkSubagentCompletion(
	ctx context.Context,
	chatID uuid.UUID,
) (database.Chat, string, bool, error) {
	chat, err := p.db.GetChatByID(ctx, chatID)
	if err != nil {
		return database.Chat{}, "", false, xerrors.Errorf("get chat: %w", err)
	}

	// interrupting is transient: the worker transitions it to
	// waiting (no queued messages) or running (queued messages).
	// Treat it as not-done so the agent settles before
	// classification, avoiding stale partial output. Paused is not
	// done either: the child resumes when the queued edit ends.
	if chat.Status == database.ChatStatusRunning ||
		chat.Status == database.ChatStatusInterrupting ||
		chat.Status == database.ChatStatusPaused {
		return chat, "", false, nil
	}

	report, err := latestSubagentAssistantMessage(ctx, p.db, chatID)
	if err != nil {
		return database.Chat{}, "", false, err
	}

	return chat, report, true, nil
}

func latestSubagentAssistantMessage(
	ctx context.Context,
	store database.Store,
	chatID uuid.UUID,
) (string, error) {
	messages, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chatID,
		AfterID: 0,
	})
	if err != nil {
		return "", xerrors.Errorf("get chat messages: %w", err)
	}

	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if message.Role != database.ChatMessageRoleAssistant ||
			message.Visibility == database.ChatMessageVisibilityModel {
			continue
		}

		content, parseErr := chatprompt.ParseContent(message)
		if parseErr != nil {
			continue
		}
		text := strings.TrimSpace(contentBlocksToText(content))
		if text == "" {
			continue
		}
		return text, nil
	}

	return "", nil
}

// isSubagentDescendant reports whether targetChatID is a descendant
// of ancestorChatID by walking up the parent chain from the target.
// This is O(depth) DB queries instead of O(nodes) BFS.
func isSubagentDescendant(
	ctx context.Context,
	store database.Store,
	ancestorChatID uuid.UUID,
	targetChatID uuid.UUID,
) (bool, error) {
	if ancestorChatID == targetChatID {
		return false, nil
	}

	currentID := targetChatID
	visited := map[uuid.UUID]struct{}{} // cycle protection
	for {
		if _, seen := visited[currentID]; seen {
			return false, nil
		}
		visited[currentID] = struct{}{}

		chat, err := store.GetChatByID(ctx, currentID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return false, nil // chain broken; not a confirmed descendant
			}
			return false, xerrors.Errorf("get chat %s: %w", currentID, err)
		}
		if !chat.ParentChatID.Valid {
			return false, nil // reached root without finding ancestor
		}
		if chat.ParentChatID.UUID == ancestorChatID {
			return true, nil
		}
		currentID = chat.ParentChatID.UUID
	}
}

func subagentFallbackChatTitle(message string) string {
	const maxWords = 6
	const maxRunes = 80

	words := strings.Fields(message)
	if len(words) == 0 {
		return chatprompt.DefaultChatTitle
	}

	truncated := false
	if len(words) > maxWords {
		words = words[:maxWords]
		truncated = true
	}

	title := strings.Join(words, " ")
	if truncated {
		title += "..."
	}

	return subagentTruncateRunes(title, maxRunes)
}

func subagentTruncateRunes(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}

	return string(runes[:maxRunes])
}

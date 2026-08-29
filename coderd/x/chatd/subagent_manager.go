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

// SubagentManager owns child chat lifecycle for one running parent turn.
type SubagentManager struct {
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

// SubagentAwaitResult is the typed outcome of waiting for a child.
type SubagentAwaitResult struct {
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

// SubagentListEntry describes one child chat.
type SubagentListEntry struct {
	CreatedAt string `json:"created_at"`
	ChatID    string `json:"chat_id"`
	Status    string `json:"status"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	UpdatedAt string `json:"updated_at"`
}

// SubagentListResult is a page of child chats.
type SubagentListResult struct {
	Agents   []SubagentListEntry `json:"agents"`
	HasMore  bool                `json:"has_more"`
	Offset   int                 `json:"offset"`
	Returned int                 `json:"returned"`
	Total    int                 `json:"total"`
}

func newSubagentManager(server *Server, currentChat func() database.Chat, currentModelConfigID uuid.UUID) *SubagentManager {
	manager := &SubagentManager{server: server, currentChat: currentChat, currentModelConfigID: currentModelConfigID}
	if currentChat != nil {
		manager.currentChatSnapshot = currentChat()
	}
	return manager
}

// Spawn creates a child chat from a delegated task.
func (m *SubagentManager) Spawn(ctx context.Context, args spawnAgentArgs) (database.Chat, error) {
	if m.currentChat == nil {
		return database.Chat{}, xerrors.New("subagent callbacks are not configured")
	}
	parent, err := m.loadSubagentSpawnParentChat(ctx)
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
	options, err := definition.buildOptions(ctx, m, parent, turnParent, m.currentModelConfigID, explicitModelConfigID, args.Prompt)
	if err != nil {
		return database.Chat{}, err
	}
	if explicitReasoningEffort != nil {
		options.reasoningEffortOverride = explicitReasoningEffort
	}
	return m.createChildSubagentChatWithOptions(ctx, parent, args.Prompt, args.Title, options)
}

// ListModels returns model configurations available to the child.
func (m *SubagentManager) ListModels(ctx context.Context) ([]map[string]any, error) {
	if m.currentChat == nil {
		return nil, xerrors.New("subagent callbacks are not configured")
	}
	parent, err := m.loadSubagentSpawnParentChat(ctx)
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

// Await waits for a child and extracts its latest report.
func (m *SubagentManager) Await(ctx context.Context, args waitAgentArgs) (SubagentAwaitResult, error) {
	if m.currentChat == nil {
		return SubagentAwaitResult{}, xerrors.New("subagent callbacks are not configured")
	}
	targetChatID, err := parseSubagentToolChatID(args.ChatID)
	if err != nil {
		return SubagentAwaitResult{}, err
	}
	parent := m.currentChat()
	targetChatInfo := m.targetChatInfo(ctx, targetChatID, "recording")
	isDescendant, err := isSubagentDescendant(ctx, m.server.db, parent.ID, targetChatID)
	if err != nil {
		return SubagentAwaitResult{}, subagentToolError(xerrors.Errorf("failed to verify subagent relationship: %v", err), targetChatInfo)
	}
	if !isDescendant {
		return SubagentAwaitResult{}, subagentToolError(ErrSubagentNotDescendant, targetChatInfo)
	}

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
	targetChat, report, awaitErr := m.awaitSubagentCompletion(ctx, parent.ID, targetChatID, timeout)
	if xerrors.Is(awaitErr, ErrSubagentWaitTimeout) {
		checkedChat, checkedReport, done, checkErr := m.checkSubagentCompletion(ctx, targetChatID)
		if checkErr != nil {
			return SubagentAwaitResult{}, subagentToolError(checkErr, targetChatInfo)
		}
		if !done {
			return SubagentAwaitResult{ChatID: targetChatID.String(), Status: string(checkedChat.Status), TimedOut: true, Title: checkedChat.Title, Type: subagentTypeFromChat(checkedChat)}, nil
		}
		targetChat, report, awaitErr = handleSubagentDone(checkedChat, checkedReport)
	}
	if awaitErr != nil {
		if statusErr, ok := errors.AsType[*subagentStatusError](awaitErr); ok {
			return subagentAwaitStatusErrorResult(statusErr), nil
		}
		return SubagentAwaitResult{}, subagentToolError(awaitErr, targetChatInfo)
	}
	return m.waitAgentSuccessResult(ctx, recordingID, agentConn, parent, targetChat, report), nil
}

// Send queues a message for a child.
func (m *SubagentManager) Send(ctx context.Context, args messageAgentArgs) (database.Chat, bool, error) {
	if m.currentChat == nil {
		return database.Chat{}, false, xerrors.New("subagent callbacks are not configured")
	}
	targetChatID, err := parseSubagentToolChatID(args.ChatID)
	if err != nil {
		return database.Chat{}, false, err
	}
	targetChatInfo := m.targetChatInfo(ctx, targetChatID, "message")
	busyBehavior := SendMessageBusyBehaviorQueue
	if args.Interrupt {
		busyBehavior = SendMessageBusyBehaviorInterrupt
	}
	chat, err := m.sendSubagentMessage(ctx, m.currentChat().ID, targetChatID, args.Message, busyBehavior)
	if err != nil {
		return database.Chat{}, false, subagentToolError(err, targetChatInfo)
	}
	interrupted := args.Interrupt && targetChatInfo != nil && targetChatInfo.Status == database.ChatStatusRunning
	return chat, interrupted, nil
}

// Interrupt stops a child run.
func (m *SubagentManager) Interrupt(ctx context.Context, args interruptAgentArgs) (database.Chat, bool, error) {
	if m.currentChat == nil {
		return database.Chat{}, false, xerrors.New("subagent callbacks are not configured")
	}
	targetChatID, err := parseSubagentToolChatID(args.ChatID)
	if err != nil {
		return database.Chat{}, false, err
	}
	targetChatInfo := m.targetChatInfo(ctx, targetChatID, "interrupt")
	chat, interrupted, err := m.interruptSubagent(ctx, m.currentChat().ID, targetChatID)
	if err != nil {
		return database.Chat{}, false, subagentToolError(err, targetChatInfo)
	}
	return chat, interrupted, nil
}

// List returns the parent chat children.
func (m *SubagentManager) List(ctx context.Context, args listAgentsArgs) (SubagentListResult, error) {
	if m.currentChat == nil {
		return SubagentListResult{}, xerrors.New("subagent callbacks are not configured")
	}
	parent := m.currentChat()
	if parent.ParentChatID.Valid {
		return SubagentListResult{}, xerrors.New("list_agents is only available on root chats")
	}
	rows, err := m.server.db.GetChildChatsByParentIDs(ctx, database.GetChildChatsByParentIDsParams{ParentIds: []uuid.UUID{parent.ID}, Archived: sql.NullBool{Bool: false, Valid: true}})
	if err != nil {
		return SubagentListResult{}, xerrors.Errorf("list child agents: %w", err)
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
	agents := make([]SubagentListEntry, 0, end-start)
	for _, row := range rows[start:end] {
		chat := row.Chat
		agents = append(agents, SubagentListEntry{CreatedAt: chat.CreatedAt.Format(time.RFC3339), ChatID: chat.ID.String(), Status: string(chat.Status), Title: chat.Title, Type: subagentTypeFromChat(chat), UpdatedAt: chat.UpdatedAt.Format(time.RFC3339)})
	}
	return SubagentListResult{Agents: agents, HasMore: end < total, Offset: offset, Returned: len(agents), Total: total}, nil
}

func (m *SubagentManager) targetChatInfo(ctx context.Context, chatID uuid.UUID, operation string) *database.Chat {
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

func subagentAwaitStatusErrorResult(statusErr *subagentStatusError) SubagentAwaitResult {
	chat := statusErr.chat
	decoded, lastError := subagentLastError(chat.LastError)
	if lastError == "" {
		lastError = statusErr.reason
	}
	result := SubagentAwaitResult{ChatID: chat.ID.String(), LastError: lastError, Report: &statusErr.report, Status: string(chat.Status), Title: chat.Title, Type: subagentTypeFromChat(chat)}
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

func (m *SubagentManager) loadSubagentSpawnParentChat(ctx context.Context) (database.Chat, error) {
	parent := m.currentChat()
	if err := validateSubagentSpawnParent(parent); err != nil {
		return database.Chat{}, err
	}
	reloadedParent, err := m.server.db.GetChatByID(ctx, parent.ID)
	if err != nil {
		m.server.logger.Warn(ctx, "failed to load parent chat for spawn_agent",
			slog.F("chat_id", parent.ID),
			slog.Error(err),
		)
		return database.Chat{}, xerrors.New("failed to load parent chat")
	}
	parent = reloadedParent
	if err := validateSubagentSpawnParent(parent); err != nil {
		return database.Chat{}, err
	}
	return parent, nil
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
func (m *SubagentManager) resolveExploreToolSnapshot(
	ctx context.Context,
	parent database.Chat,
) ([]uuid.UUID, error) {
	inheritedMCPServerIDs := []uuid.UUID{}
	if len(parent.MCPServerIDs) > 0 {
		configs, err := enabledMCPServerConfigsForChatOrg(ctx, m.server.db, parent.OrganizationID, parent.MCPServerIDs)
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

func (m *SubagentManager) createChildSubagentChatWithOptions(
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
	deploymentPrompt := m.server.resolveDeploymentSystemPrompt(ctx)
	// Delegated chats cannot call list_agents or message_agent, so
	// strip the root-only orchestration guidance from their prompt.
	deploymentPrompt = strings.Replace(deploymentPrompt, subagentOrchestrationPromptBlock, "", 1)

	// Review before persistence so spawned chats cannot bypass prompt policy.
	childChatID := uuid.New()
	var promptResult *chathooks.Result
	if m.server.hooks.Enabled() {
		mintedTurnID := uuid.New()
		promptMessage, err := chathooks.UserPromptMessage([]codersdk.ChatMessagePart{codersdk.ChatMessageText(prompt)})
		if err != nil {
			return database.Chat{}, err
		}
		promptResult, err = m.server.hooks.Trigger(ctx, chathooks.Chat{
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
	if title == "" {
		title = subagentFallbackChatTitle(prompt)
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

	initialMessages := make([]chatstate.Message, 0, 4)
	if deploymentPrompt != "" {
		deploymentContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
			codersdk.ChatMessageText(deploymentPrompt),
		})
		if err != nil {
			return database.Chat{}, xerrors.Errorf("marshal deployment system prompt: %w", err)
		}
		initialMessages = append(initialMessages, systemMessage(deploymentContent, modelConfigID))
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

	publisher := m.server.pubsub
	if publisher == nil {
		publisher = dbpubsub.NewInMemory()
	}
	result, err := chatstate.CreateChatWithID(ctx, m.server.db, publisher, childChatID, chatstate.CreateChatInput{
		OrganizationID:    parent.OrganizationID,
		OwnerID:           parent.OwnerID,
		WorkspaceID:       parent.WorkspaceID,
		BuildID:           parent.BuildID,
		AgentID:           parent.AgentID,
		ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: rootChatID, Valid: true},
		LastModelConfigID: modelConfigID,
		Title:             title,
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
	})
	if err != nil {
		return database.Chat{}, xerrors.Errorf("create child chat: %w", err)
	}

	child := result.Chat

	// Pin the child to its agent's latest context snapshot, mirroring the
	// top-level create path. The child shares the parent's workspace agent,
	// so this reproduces the parent's workspace context without copying it
	// through chat history.
	m.server.hydrateChatContextOnCreate(ctx, child)

	m.server.publishChatPubsubEvent(child, codersdk.ChatWatchEventKindCreated, nil)
	return child, nil
}

func (m *SubagentManager) sendSubagentMessage(
	ctx context.Context,
	parentChatID uuid.UUID,
	targetChatID uuid.UUID,
	message string,
	busyBehavior SendMessageBusyBehavior,
) (database.Chat, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return database.Chat{}, xerrors.New("message is required")
	}

	isDescendant, err := isSubagentDescendant(ctx, m.server.db, parentChatID, targetChatID)
	if err != nil {
		return database.Chat{}, err
	}
	if !isDescendant {
		return database.Chat{}, ErrSubagentNotDescendant
	}

	// Look up the target chat to get the owner for CreatedBy.
	targetChat, err := m.server.db.GetChatByID(ctx, targetChatID)
	if err != nil {
		return database.Chat{}, xerrors.Errorf("get target chat: %w", err)
	}

	sendResult, err := m.server.SendMessage(ctx, SendMessageOptions{
		ChatID:       targetChatID,
		CreatedBy:    targetChat.OwnerID,
		Content:      []codersdk.ChatMessagePart{codersdk.ChatMessageText(message)},
		BusyBehavior: busyBehavior,
	})
	if err != nil {
		return database.Chat{}, err
	}

	return sendResult.Chat, nil
}

func (m *SubagentManager) awaitSubagentCompletion(
	ctx context.Context,
	parentChatID uuid.UUID,
	targetChatID uuid.UUID,
	timeout time.Duration,
) (database.Chat, string, error) {
	isDescendant, err := isSubagentDescendant(ctx, m.server.db, parentChatID, targetChatID)
	if err != nil {
		return database.Chat{}, "", err
	}
	if !isDescendant {
		return database.Chat{}, "", ErrSubagentNotDescendant
	}

	// Check immediately before entering the poll loop.
	targetChat, report, done, checkErr := m.checkSubagentCompletion(ctx, targetChatID)
	if checkErr != nil {
		return database.Chat{}, "", checkErr
	}
	if done {
		return handleSubagentDone(targetChat, report)
	}

	if timeout <= 0 {
		timeout = defaultSubagentWaitTimeout
	}
	timer := m.server.clock.NewTimer(timeout, "chatd", "subagent_await")
	defer timer.Stop()

	// Subscribe for fast status notifications and use a less
	// aggressive fallback poll. If subscription fails, fall back to
	// the original 200ms polling.
	pollInterval := subagentAwaitFallbackPoll
	ch := make(chan struct{}, 1)
	notifyCh := (<-chan struct{})(ch)
	cancel, subErr := m.server.pubsub.SubscribeWithErr(
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

	ticker := m.server.clock.NewTicker(pollInterval, "chatd", "subagent_poll")
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

		targetChat, report, done, checkErr = m.checkSubagentCompletion(ctx, targetChatID)
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
func (m *SubagentManager) waitAgentSuccessResult(
	ctx context.Context,
	recordingID string,
	agentConn workspacesdk.AgentConn,
	parent database.Chat,
	targetChat database.Chat,
	report string,
) SubagentAwaitResult {
	var recResult recordingResult
	if recordingID != "" && agentConn != nil {
		// Use a fresh context for cleanup so a canceled
		// parent context does not prevent recording storage.
		stopCtx, stopCancel := context.WithTimeout(context.WithoutCancel(ctx), subagentRecordingStopTimeout)
		defer stopCancel()
		recResult = m.server.stopAndStoreRecording(stopCtx, agentConn,
			recordingID, parent.ID, parent.OwnerID, parent.WorkspaceID)
	}
	reportCopy := report
	return SubagentAwaitResult{
		ChatID:          targetChat.ID.String(),
		RecordingFileID: recResult.recordingFileID,
		Report:          &reportCopy,
		Status:          string(targetChat.Status),
		ThumbnailFileID: recResult.thumbnailFileID,
		Title:           targetChat.Title,
		Type:            subagentTypeFromChat(targetChat),
	}
}

func (m *SubagentManager) interruptSubagent(
	ctx context.Context,
	parentChatID uuid.UUID,
	targetChatID uuid.UUID,
) (database.Chat, bool, error) {
	isDescendant, err := isSubagentDescendant(ctx, m.server.db, parentChatID, targetChatID)
	if err != nil {
		return database.Chat{}, false, err
	}
	if !isDescendant {
		return database.Chat{}, false, ErrSubagentNotDescendant
	}

	targetChat, err := m.server.db.GetChatByID(ctx, targetChatID)
	if err != nil {
		return database.Chat{}, false, xerrors.Errorf("get target chat: %w", err)
	}

	if targetChat.Status == database.ChatStatusWaiting {
		return targetChat, false, nil
	}

	updatedChat, err := m.server.InterruptChat(ctx, targetChat)
	if err != nil {
		// Idle / archived chats no longer satisfy the
		// chatstate.Interrupt precondition. Surface the error
		// so the caller can decide whether the parent expected
		// the subagent to already be waiting.
		return database.Chat{}, false, xerrors.Errorf("interrupt subagent chat: %w", err)
	}
	// chatstate.Interrupt lands active runs in `interrupting`
	// and requires-action chats in `running`. Workers finalize
	// the transition; accept either non-active status as long as
	// the transition committed.
	return updatedChat, true, nil
}

func (m *SubagentManager) checkSubagentCompletion(
	ctx context.Context,
	chatID uuid.UUID,
) (database.Chat, string, bool, error) {
	chat, err := m.server.db.GetChatByID(ctx, chatID)
	if err != nil {
		return database.Chat{}, "", false, xerrors.Errorf("get chat: %w", err)
	}

	// interrupting is transient: the worker transitions it to
	// waiting (no queued messages) or running (queued messages).
	// Treat it as not-done so the agent settles before
	// classification, avoiding stale partial output.
	if chat.Status == database.ChatStatusRunning ||
		chat.Status == database.ChatStatusInterrupting {
		return chat, "", false, nil
	}

	report, err := latestSubagentAssistantMessage(ctx, m.server.db, chatID)
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
		return "New Chat"
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

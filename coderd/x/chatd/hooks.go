package chatd

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chathooks"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agenthooks"
)

// UserPromptDeniedError reports that a lifecycle hook rejected a prompt.
type UserPromptDeniedError struct {
	UserMessage string
}

func (*UserPromptDeniedError) Error() string {
	return "user prompt denied by lifecycle hook"
}

func promptText(parts []codersdk.ChatMessagePart) string {
	var prompt strings.Builder
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeText {
			_, _ = prompt.WriteString(part.Text)
		}
	}
	return prompt.String()
}

const (
	sessionStartSourceStartup = "startup"
	sessionStartSourceResume  = "resume"
	sessionStartSourceClear   = "clear"
)

func lifecycleHookEvent(
	chat database.Chat,
	turnID *uuid.UUID,
	eventType agenthooks.EventType,
	data any,
) chathooks.Event {
	var workspaceID *uuid.UUID
	if chat.WorkspaceID.Valid {
		workspaceID = &chat.WorkspaceID.UUID
	}
	var parentChatID *uuid.UUID
	if chat.ParentChatID.Valid {
		parentChatID = &chat.ParentChatID.UUID
	}
	var rootChatID *uuid.UUID
	if chat.RootChatID.Valid {
		rootChatID = &chat.RootChatID.UUID
	}
	return chathooks.Event{
		Type: eventType,
		ChatRef: agenthooks.ChatRef{
			ChatID:       chat.ID,
			OwnerID:      chat.OwnerID,
			WorkspaceID:  workspaceID,
			TurnID:       turnID,
			ParentChatID: parentChatID,
			RootChatID:   rootChatID,
		},
		Data: data,
	}
}

func (p *Server) dispatchLifecycleHook(
	ctx context.Context,
	chat database.Chat,
	turnID *uuid.UUID,
	eventType agenthooks.EventType,
	data any,
) (agenthooks.Response, error) {
	if p.hookDispatcher == nil || !p.hookDispatcher.Enabled() {
		return agenthooks.Response{}, nil
	}
	resp, _, err := p.hookDispatcher.Dispatch(ctx, lifecycleHookEvent(chat, turnID, eventType, data))
	return resp, err
}

func (p *Server) dispatchSessionStart(
	ctx context.Context,
	chat database.Chat,
	turnID *uuid.UUID,
	source string,
) (agenthooks.Response, error) {
	return p.dispatchLifecycleHook(ctx, chat, turnID, agenthooks.EventSessionStart, agenthooks.SessionStartData{Source: source})
}

func (p *Server) dispatchPreCompact(ctx context.Context, chat database.Chat, turnID *uuid.UUID) (agenthooks.Response, error) {
	return p.dispatchLifecycleHook(ctx, chat, turnID, agenthooks.EventPreCompact, agenthooks.PreCompactData{})
}

func (p *Server) dispatchPostCompact(ctx context.Context, chat database.Chat, turnID *uuid.UUID) (agenthooks.Response, error) {
	return p.dispatchLifecycleHook(ctx, chat, turnID, agenthooks.EventPostCompact, agenthooks.PostCompactData{})
}

func (p *Server) dispatchStop(ctx context.Context, chat database.Chat, turnID *uuid.UUID) (agenthooks.Response, error) {
	return p.dispatchLifecycleHook(ctx, chat, turnID, agenthooks.EventStop, agenthooks.StopData{})
}

type preToolUseResult struct {
	Step              chatloop.PersistedStep
	Responses         []agenthooks.Response
	EffectDispatchIDs []uuid.UUID
}

func (p *Server) dispatchPreToolUse(
	ctx context.Context,
	chat database.Chat,
	turnID *uuid.UUID,
	toolCall fantasy.ToolCallContent,
) (agenthooks.Response, uuid.UUID, error) {
	event := lifecycleHookEvent(chat, turnID, agenthooks.EventPreToolUse, agenthooks.PreToolUseData{
		ToolUseID: toolCall.ToolCallID,
		ToolName:  toolCall.ToolName,
		ToolInput: json.RawMessage(toolCall.Input),
	})
	return p.hookDispatcher.Dispatch(ctx, event)
}

func (p *Server) dispatchPostToolUseData(
	ctx context.Context,
	chat database.Chat,
	turnID *uuid.UUID,
	data agenthooks.PostToolUseData,
) (agenthooks.Response, error) {
	event := lifecycleHookEvent(chat, turnID, agenthooks.EventPostToolUse, data)
	resp, _, err := p.hookDispatcher.Dispatch(ctx, event)
	return resp, err
}

func (p *Server) dispatchPostToolUse(
	ctx context.Context,
	chat database.Chat,
	turnID *uuid.UUID,
	toolResult fantasy.ToolResultContent,
) (agenthooks.Response, error) {
	data := agenthooks.PostToolUseData{
		ToolUseID: toolResult.ToolCallID,
		ToolName:  toolResult.ToolName,
	}
	switch output := toolResult.Result.(type) {
	case fantasy.ToolResultOutputContentError:
		if output.Error != nil {
			data.ToolError = output.Error.Error()
		}
	case *fantasy.ToolResultOutputContentError:
		if output != nil && output.Error != nil {
			data.ToolError = output.Error.Error()
		}
	default:
		encoded, err := json.Marshal(toolResult.Result)
		if err != nil {
			return agenthooks.Response{}, xerrors.Errorf("marshal post_tool_use response: %w", err)
		}
		data.ToolResponse = encoded
	}
	return p.dispatchPostToolUseData(ctx, chat, turnID, data)
}

func (p *Server) dispatchPostToolUseResults(
	ctx context.Context,
	chat database.Chat,
	turnID *uuid.UUID,
	content []fantasy.Content,
) ([]agenthooks.Response, error) {
	if p.hookDispatcher == nil || !p.hookDispatcher.Enabled() {
		return nil, nil
	}
	responses := make([]agenthooks.Response, 0, len(content))
	// Dispatch every completed result so later side effects are audited.
	// Preserve only the first failure before an accepted end_chat.
	var firstErr error
	endChatSeen := false
	for _, block := range content {
		toolResult, ok := asToolResultContent(block)
		if !ok || toolResult.ProviderExecuted {
			continue
		}
		response, err := p.dispatchPostToolUse(ctx, chat, turnID, toolResult)
		if err != nil {
			if firstErr == nil && !endChatSeen {
				firstErr = err
			}
			continue
		}
		endChatSeen = endChatSeen || response.EndChat
		responses = append(responses, response)
	}
	return responses, firstErr
}

func (p *Server) preflightToolCalls(
	ctx context.Context,
	chat database.Chat,
	turnID *uuid.UUID,
	step chatloop.PersistedStep,
	toolCalls []fantasy.ToolCallContent,
) (preToolUseResult, error) {
	result := preToolUseResult{Step: step}
	if p.hookDispatcher == nil || !p.hookDispatcher.Enabled() {
		return result, nil
	}
	if err := rejectDuplicateToolUseIDs(toolCalls); err != nil {
		return preToolUseResult{}, err
	}

	for _, toolCall := range toolCalls {
		if toolCall.ProviderExecuted {
			continue
		}
		response, dispatchID, err := p.dispatchPreToolUse(ctx, chat, turnID, toolCall)
		if err != nil {
			return preToolUseResult{}, err
		}
		if err := applyPreToolUsePermission(&result.Step, toolCall, response); err != nil {
			return preToolUseResult{}, err
		}
		result.Responses = append(result.Responses, response)
		result.EffectDispatchIDs = append(result.EffectDispatchIDs, dispatchID)
		// Accepted end_chat effects take precedence over later dispatch failures.
		if response.EndChat {
			break
		}
	}
	return result, nil
}

// restoreToolCallOrder reorders tool results to match the assistant's
// call order because providers pair results with calls positionally.
// Entries that are not tool results for the given calls keep their slots.
func restoreToolCallOrder(content []fantasy.Content, calls []fantasy.ToolCallContent) {
	position := make(map[string]int, len(calls))
	for index, call := range calls {
		position[call.ToolCallID] = index
	}
	slots := make([]int, 0, len(content))
	results := make([]fantasy.ToolResultContent, 0, len(content))
	for index, entry := range content {
		result, ok := entry.(fantasy.ToolResultContent)
		if !ok {
			continue
		}
		if _, known := position[result.ToolCallID]; !known {
			continue
		}
		slots = append(slots, index)
		results = append(results, result)
	}
	slices.SortStableFunc(results, func(a, b fantasy.ToolResultContent) int {
		return position[a.ToolCallID] - position[b.ToolCallID]
	})
	for index, slot := range slots {
		content[slot] = results[index]
	}
}

// rejectDuplicateToolUseIDs fails closed because persisted decisions are
// keyed by tool-use ID within a turn.
func rejectDuplicateToolUseIDs(toolCalls []fantasy.ToolCallContent) error {
	seen := make(map[string]struct{}, len(toolCalls))
	for _, toolCall := range toolCalls {
		if toolCall.ProviderExecuted {
			continue
		}
		if _, ok := seen[toolCall.ToolCallID]; ok {
			return xerrors.Errorf("duplicate tool use ID %q in one step; lifecycle hook decisions cannot be attributed unambiguously", toolCall.ToolCallID)
		}
		seen[toolCall.ToolCallID] = struct{}{}
	}
	return nil
}

type preToolUseExecutionResult struct {
	Allowed           []fantasy.ToolCallContent
	Denied            []fantasy.ToolResultContent
	Responses         []agenthooks.Response
	Overrides         map[string]json.RawMessage
	EffectDispatchIDs []uuid.UUID
}

func (p *Server) preflightPendingToolCalls(
	ctx context.Context,
	chat database.Chat,
	turnID *uuid.UUID,
	toolCalls []fantasy.ToolCallContent,
	priorToolCallIDs map[string]bool,
) (preToolUseExecutionResult, error) {
	result := preToolUseExecutionResult{
		Allowed:   make([]fantasy.ToolCallContent, 0, len(toolCalls)),
		Overrides: make(map[string]json.RawMessage),
	}
	if p.hookDispatcher == nil || !p.hookDispatcher.Enabled() {
		result.Allowed = append(result.Allowed, toolCalls...)
		return result, nil
	}
	if err := rejectDuplicateToolUseIDs(toolCalls); err != nil {
		return preToolUseExecutionResult{}, err
	}

	for _, toolCall := range toolCalls {
		// Re-dispatch invalid JSON or IDs reused earlier in the turn because no
		// persisted decision can safely authorize those calls.
		row, err := database.ChatHookDispatch{}, sql.ErrNoRows
		if json.Valid([]byte(toolCall.Input)) && !priorToolCallIDs[toolCall.ToolCallID] {
			row, err = p.db.GetChatHookDispatchDecision(ctx, database.GetChatHookDispatchDecisionParams{
				ChatID:    chat.ID,
				ToolUseID: toolCall.ToolCallID,
				ToolName:  toolCall.ToolName,
				ToolInput: json.RawMessage(toolCall.Input),
				TurnID:    hookTurnID(turnID),
			})
		}
		if err == nil {
			// Replay unapplied effects from a finalized dispatch without re-dispatching.
			endChat := false
			if !row.EffectsAppliedAt.Valid {
				response := dispatchRowResponse(row)
				endChat = response.EndChat
				result.Responses = append(result.Responses, response)
				result.EffectDispatchIDs = append(result.EffectDispatchIDs, row.ID)
			}
			switch agenthooks.PermissionDecision(row.Decision.String) {
			case agenthooks.PermissionAllow:
				if row.InputOverride.Valid {
					toolCall.Input = string(row.InputOverride.RawMessage)
					result.Overrides[toolCall.ToolCallID] = row.InputOverride.RawMessage
				}
				result.Allowed = append(result.Allowed, toolCall)
			case agenthooks.PermissionDeny:
				result.Denied = append(result.Denied, deniedToolResult(toolCall, row.DecisionReason.String))
			}
			// Accepted end_chat effects take precedence over later dispatch failures.
			if endChat {
				break
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return preToolUseExecutionResult{}, xerrors.Errorf("get pre_tool_use decision: %w", err)
		}

		response, dispatchID, err := p.dispatchPreToolUse(ctx, chat, turnID, toolCall)
		if err != nil {
			return preToolUseExecutionResult{}, err
		}
		result.Responses = append(result.Responses, response)
		result.EffectDispatchIDs = append(result.EffectDispatchIDs, dispatchID)
		if response.Permission == nil {
			result.Allowed = append(result.Allowed, toolCall)
		} else {
			switch response.Permission.Decision {
			case agenthooks.PermissionAllow:
				toolCall.Input = string(response.Permission.InputOverride)
				result.Overrides[toolCall.ToolCallID] = response.Permission.InputOverride
				result.Allowed = append(result.Allowed, toolCall)
			case agenthooks.PermissionDeny:
				result.Denied = append(result.Denied, deniedToolResult(toolCall, response.Permission.Reason))
			}
		}
		// Accepted end_chat effects take precedence over later dispatch failures.
		if response.EndChat {
			break
		}
	}
	return result, nil
}

func dispatchRowResponse(row database.ChatHookDispatch) agenthooks.Response {
	response := agenthooks.Response{
		EndChat: row.EndChat.Valid && row.EndChat.Bool,
	}
	if row.ModelContext.Valid {
		response.ModelContext = row.ModelContext.String
	}
	if row.UserMessage.Valid {
		response.UserMessage = row.UserMessage.String
	}
	if row.AllowedTools.Valid {
		var allowed []string
		if err := json.Unmarshal(row.AllowedTools.RawMessage, &allowed); err == nil {
			response.AllowedTools = &allowed
		}
	}
	return response
}

// Mark effects in the same transaction so rollback leaves them replayable.
func markHookDispatchEffectsApplied(
	ctx context.Context,
	store database.Store,
	chatID uuid.UUID,
	dispatchIDs []uuid.UUID,
) error {
	if len(dispatchIDs) == 0 {
		return nil
	}
	if err := store.MarkChatHookDispatchEffectsApplied(ctx, database.MarkChatHookDispatchEffectsAppliedParams{
		ChatID:      chatID,
		DispatchIds: dispatchIDs,
	}); err != nil {
		return xerrors.Errorf("mark hook dispatch effects applied: %w", err)
	}
	return nil
}

func replacePersistedToolCallInputs(
	ctx context.Context,
	store database.Store,
	chatID uuid.UUID,
	overrides map[string]json.RawMessage,
) error {
	if len(overrides) == 0 {
		return nil
	}
	assistant, err := store.GetLastChatMessageByRole(ctx, database.GetLastChatMessageByRoleParams{
		ChatID: chatID,
		Role:   database.ChatMessageRoleAssistant,
	})
	if err != nil {
		return xerrors.Errorf("get assistant message for tool override: %w", err)
	}
	parts, err := chatprompt.ParseContent(assistant)
	if err != nil {
		return xerrors.Errorf("parse assistant message for tool override: %w", err)
	}
	for i := range parts {
		if override, ok := overrides[parts[i].ToolCallID]; ok && parts[i].Type == codersdk.ChatMessagePartTypeToolCall {
			parts[i].Args = override
		}
	}
	content, err := chatprompt.MarshalParts(parts)
	if err != nil {
		return xerrors.Errorf("marshal assistant message with tool override: %w", err)
	}
	if err := store.UpdateChatMessageContentByID(ctx, database.UpdateChatMessageContentByIDParams{
		Content: content.RawMessage,
		ID:      assistant.ID,
	}); err != nil {
		return xerrors.Errorf("update assistant message with tool override: %w", err)
	}
	return nil
}

func applyPreToolUsePermission(step *chatloop.PersistedStep, toolCall fantasy.ToolCallContent, response agenthooks.Response) error {
	if response.Permission == nil {
		return nil
	}
	switch response.Permission.Decision {
	case agenthooks.PermissionAllow:
		if !replaceToolCallInput(step.Content, toolCall.ToolCallID, string(response.Permission.InputOverride)) {
			return xerrors.Errorf("tool call %q is missing from generated step", toolCall.ToolCallID)
		}
	case agenthooks.PermissionDeny:
		step.Content = append(step.Content, deniedToolResult(toolCall, response.Permission.Reason))
	}
	return nil
}

func deniedToolResult(toolCall fantasy.ToolCallContent, reason string) fantasy.ToolResultContent {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "denied by lifecycle hook"
	}
	return fantasy.ToolResultContent{
		ToolCallID: toolCall.ToolCallID,
		ToolName:   toolCall.ToolName,
		Result: fantasy.ToolResultOutputContentError{
			Error: xerrors.New("DENIED: " + reason),
		},
	}
}

func replaceToolCallInput(content []fantasy.Content, toolCallID, input string) bool {
	for i, block := range content {
		if toolCall, ok := fantasy.AsContentType[fantasy.ToolCallContent](block); ok && toolCall.ToolCallID == toolCallID {
			toolCall.Input = input
			content[i] = toolCall
			return true
		}
		if toolCall, ok := fantasy.AsContentType[*fantasy.ToolCallContent](block); ok && toolCall != nil && toolCall.ToolCallID == toolCallID {
			updated := *toolCall
			updated.Input = input
			content[i] = updated
			return true
		}
	}
	return false
}

func sessionStartSource(messages []database.ChatMessage) string {
	for _, message := range messages {
		if message.Role == database.ChatMessageRoleAssistant {
			return sessionStartSourceResume
		}
	}
	return sessionStartSourceStartup
}

func activeTurnID(messages []database.ChatMessage) *uuid.UUID {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].TurnID.Valid {
			turnID := messages[i].TurnID.UUID
			return &turnID
		}
	}
	return nil
}

func (p *Server) dispatchUserPromptSubmit(
	ctx context.Context,
	chat database.Chat,
	turnID uuid.UUID,
	parts []codersdk.ChatMessagePart,
) (agenthooks.Response, error) {
	encodedParts, err := chatprompt.MarshalParts(parts)
	if err != nil {
		return agenthooks.Response{}, xerrors.Errorf("marshal prompt parts for hook: %w", err)
	}
	response, err := p.dispatchLifecycleHook(ctx, chat, &turnID, agenthooks.EventUserPromptSubmit, agenthooks.UserPromptSubmitData{
		Prompt: promptText(parts),
		Parts:  encodedParts.RawMessage,
	})
	if err != nil {
		return agenthooks.Response{}, err
	}
	if response.Permission != nil && response.Permission.Decision == agenthooks.PermissionDeny {
		// Preserve end_chat from a denied response.
		return response, &UserPromptDeniedError{UserMessage: response.UserMessage}
	}
	return response, nil
}

// Treat an already archived or deleted chat as satisfying accepted end_chat.
func (p *Server) endChatAfterPromptDenial(ctx context.Context, chatID uuid.UUID, prefixMessages []chatstate.Message) error {
	var (
		ended       database.Chat
		descendants []database.Chat
	)
	machine := p.newChatMachine(chatID)
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		endResult, err := tx.EndChat(chatstate.EndChatInput{PrefixMessages: prefixMessages})
		if err != nil {
			return err
		}
		descendants = endResult.EndedDescendants
		chat, err := store.GetChatByID(ctx, chatID)
		if err != nil {
			return xerrors.Errorf("reload ended chat: %w", err)
		}
		ended = chat
		return nil
	})
	if errors.Is(err, chatstate.ErrTransitionNotAllowed) || errors.Is(err, chatstate.ErrChatNotFound) {
		return nil
	}
	if err != nil {
		return xerrors.Errorf("end chat after prompt denial: %w", err)
	}
	p.publishEndChatSideEffects(ctx, ended, descendants)
	return nil
}

func (p *Server) endChatFromEditSessionStart(
	ctx context.Context,
	chat database.Chat,
	turnID *uuid.UUID,
	response agenthooks.Response,
) (EditMessageResult, error) {
	prefixMessages, err := hookPrefixMessages(response, chat.LastModelConfigID, turnID)
	if err != nil {
		return EditMessageResult{}, err
	}
	var (
		result      EditMessageResult
		descendants []database.Chat
	)
	machine := p.newChatMachine(chat.ID)
	err = machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		endResult, err := tx.EndChat(chatstate.EndChatInput{PrefixMessages: prefixMessages})
		if err != nil {
			return xerrors.Errorf("end chat from session_start: %w", err)
		}
		descendants = endResult.EndedDescendants
		refreshed, err := store.GetChatByID(ctx, chat.ID)
		if err != nil {
			return xerrors.Errorf("reload ended chat: %w", err)
		}
		result.Chat = refreshed
		result.Ended = true
		return nil
	})
	if err != nil {
		return EditMessageResult{}, err
	}
	p.publishEndChatSideEffects(ctx, result.Chat, descendants)
	return result, nil
}

func (p *Server) endChatAfterToolHookFailure(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	chatID uuid.UUID,
	suffixMessages []chatstate.Message,
) error {
	var (
		ended       database.Chat
		descendants []database.Chat
	)
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		endResult, err := tx.EndChat(chatstate.EndChatInput{PrefixMessages: suffixMessages})
		if err != nil {
			return err
		}
		descendants = endResult.EndedDescendants
		chat, err := store.GetChatByID(ctx, chatID)
		if err != nil {
			return xerrors.Errorf("reload ended chat: %w", err)
		}
		ended = chat
		return nil
	})
	if errors.Is(err, chatstate.ErrTransitionNotAllowed) || errors.Is(err, chatstate.ErrChatNotFound) {
		return nil
	}
	if err != nil {
		return xerrors.Errorf("end chat after tool hook failure: %w", err)
	}
	p.publishEndChatSideEffects(ctx, ended, descendants)
	return nil
}

func (p *Server) handleUserPromptDispatchError(ctx context.Context, chatID uuid.UUID, dispatchErr error) error {
	return p.handleAPIDispatchError(ctx, chatID, agenthooks.EventUserPromptSubmit, dispatchErr)
}

func (p *Server) handleAPIDispatchError(ctx context.Context, chatID uuid.UUID, eventType agenthooks.EventType, dispatchErr error) error {
	lastError, ok := hookDispatchErrorMessage(eventType, dispatchErr)
	if !ok {
		return dispatchErr
	}
	var failedChat database.Chat
	machine := p.newChatMachine(chatID)
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		if _, err := tx.FailIdle(chatstate.FailIdleInput{
			LastError: lastError,
			Kind:      codersdk.ChatErrorKindHookDispatchFailed,
		}); err != nil {
			return err
		}
		chat, err := store.GetChatByID(ctx, chatID)
		if err != nil {
			return xerrors.Errorf("reload chat after hook failure: %w", err)
		}
		failedChat = chat
		return nil
	})
	if errors.Is(err, chatstate.ErrTransitionNotAllowed) {
		return dispatchErr
	}
	if err != nil {
		return errors.Join(dispatchErr, xerrors.Errorf("fail idle chat after hook dispatch: %w", err))
	}
	p.publishChatPubsubEvent(failedChat, codersdk.ChatWatchEventKindStatusChange, nil)
	return dispatchErr
}

func hookDispatchErrorMessage(eventType agenthooks.EventType, dispatchErr error) (string, bool) {
	var structured *chathooks.DispatchError
	if !errors.As(dispatchErr, &structured) {
		return "", false
	}
	return fmt.Sprintf(
		"hook dispatch failed: %s: %s (dispatch %s)",
		eventType,
		structured.Class,
		structured.DispatchID,
	), true
}

func sessionStartDispatchError(dispatchErr error) error {
	return generationHookDispatchError(agenthooks.EventSessionStart, dispatchErr)
}

func generationHookDispatchError(eventType agenthooks.EventType, dispatchErr error) error {
	message, ok := hookDispatchErrorMessage(eventType, dispatchErr)
	if !ok {
		message = dispatchErr.Error()
	}
	return chaterror.WithClassification(dispatchErr, chaterror.ClassifiedError{
		Message: message,
		Kind:    codersdk.ChatErrorKindHookDispatchFailed,
	})
}

type sessionStartResult struct {
	Chat             database.Chat
	Ended            bool
	EndedDescendants []database.Chat
}

func applySessionStartResponse(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	chat database.Chat,
	turnID *uuid.UUID,
	response agenthooks.Response,
) (sessionStartResult, error) {
	if response.ModelContext == "" && response.UserMessage == "" && response.AllowedTools == nil && !response.EndChat {
		return sessionStartResult{Chat: chat}, nil
	}

	// Pre-turn-ID histories persist hook messages with a NULL turn_id.
	prefixMessages, err := hookPrefixMessages(response, chat.LastModelConfigID, turnID)
	if err != nil {
		return sessionStartResult{}, err
	}

	var result sessionStartResult
	err = machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		if _, err := loadChatForGeneration(ctx, store, input, generationAttemptNotRequired); err != nil {
			return xerrors.Errorf("load chat for session_start response: %w", err)
		}
		if err := applyHookAllowedTools(ctx, store, input.ChatID, response); err != nil {
			return err
		}
		if response.EndChat {
			endResult, err := tx.EndChat(chatstate.EndChatInput{PrefixMessages: prefixMessages})
			if err != nil {
				return xerrors.Errorf("end chat from session_start: %w", err)
			}
			result.Ended = true
			result.EndedDescendants = endResult.EndedDescendants
		} else if len(prefixMessages) > 0 {
			if _, err := tx.CommitStep(chatstate.CommitStepInput{Messages: prefixMessages}); err != nil {
				return xerrors.Errorf("insert session_start response messages: %w", err)
			}
		}
		result.Chat, err = store.GetChatByID(ctx, input.ChatID)
		if err != nil {
			return xerrors.Errorf("reload chat after session_start response: %w", err)
		}
		return nil
	})
	if err != nil {
		return sessionStartResult{}, normalizeTaskTransitionError(err, "apply session_start response")
	}
	return result, nil
}

func (s *taskStarter) finishSessionStartEnd(ctx context.Context, input chatWorkerTaskStartInput, result sessionStartResult) error {
	return s.finishEndedChat(ctx, input, result.Chat, result.EndedDescendants)
}

func hookPrefixMessages(response agenthooks.Response, modelConfigID uuid.UUID, turnID *uuid.UUID) ([]chatstate.Message, error) {
	messages := make([]chatstate.Message, 0, 2)
	if response.ModelContext != "" {
		content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(response.ModelContext)})
		if err != nil {
			return nil, xerrors.Errorf("marshal hook model context: %w", err)
		}
		messages = append(messages, chatstate.Message{
			Role:           database.ChatMessageRoleUser,
			Content:        content,
			Visibility:     database.ChatMessageVisibilityModel,
			TurnID:         hookTurnID(turnID),
			ModelConfigID:  uuid.NullUUID{UUID: modelConfigID, Valid: modelConfigID != uuid.Nil},
			ContentVersion: chatprompt.CurrentContentVersion,
		})
	}
	if response.UserMessage != "" {
		content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(response.UserMessage)})
		if err != nil {
			return nil, xerrors.Errorf("marshal hook user message: %w", err)
		}
		messages = append(messages, chatstate.Message{
			Role:           database.ChatMessageRoleSystem,
			Content:        content,
			Visibility:     database.ChatMessageVisibilityUser,
			TurnID:         hookTurnID(turnID),
			ModelConfigID:  uuid.NullUUID{UUID: modelConfigID, Valid: modelConfigID != uuid.Nil},
			ContentVersion: chatprompt.CurrentContentVersion,
		})
	}
	return messages, nil
}

func hookTurnID(turnID *uuid.UUID) uuid.NullUUID {
	if turnID == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *turnID, Valid: true}
}

func hookResponseMessages(
	responses []agenthooks.Response,
	modelConfigID uuid.UUID,
	turnID *uuid.UUID,
) ([]chatstate.Message, bool, error) {
	var messages []chatstate.Message
	endChat := false
	for _, response := range responses {
		responseMessages, err := hookPrefixMessages(response, modelConfigID, turnID)
		if err != nil {
			return nil, false, err
		}
		messages = append(messages, responseMessages...)
		endChat = endChat || response.EndChat
	}
	return messages, endChat, nil
}

func applyHookResponseMessages(
	messages stepMessagesForCommit,
	responses []agenthooks.Response,
	modelConfigID uuid.UUID,
	turnID *uuid.UUID,
) (stepMessagesForCommit, bool, error) {
	prefix, endChat, err := hookResponseMessages(responses, modelConfigID, turnID)
	if err != nil {
		return stepMessagesForCommit{}, false, err
	}
	if len(prefix) > 0 {
		messages.Messages = append(prefix, messages.Messages...)
		messages.VisibleIndexes = visibleMessageIndexes(messages.Messages)
	}
	return messages, endChat, nil
}

func appendHookResponseMessages(
	messages stepMessagesForCommit,
	responses []agenthooks.Response,
	modelConfigID uuid.UUID,
	turnID *uuid.UUID,
) (stepMessagesForCommit, bool, error) {
	suffix, endChat, err := hookResponseMessages(responses, modelConfigID, turnID)
	if err != nil {
		return stepMessagesForCommit{}, false, err
	}
	if len(suffix) > 0 {
		messages.Messages = append(messages.Messages, suffix...)
		messages.VisibleIndexes = visibleMessageIndexes(messages.Messages)
	}
	return messages, endChat, nil
}

func hookAllowedTools(response agenthooks.Response) (pqtype.NullRawMessage, error) {
	if response.AllowedTools == nil {
		return pqtype.NullRawMessage{}, nil
	}
	encoded, err := json.Marshal(response.AllowedTools)
	if err != nil {
		return pqtype.NullRawMessage{}, xerrors.Errorf("marshal hook allowed tools: %w", err)
	}
	return pqtype.NullRawMessage{RawMessage: encoded, Valid: true}, nil
}

type pendingHookAllowedToolsContextKey struct{}

type pendingHookAllowedToolsContextValue struct {
	chatID uuid.UUID
	policy pqtype.NullRawMessage
}

func withPendingHookAllowedTools(ctx context.Context, chatID uuid.UUID, policy pqtype.NullRawMessage) context.Context {
	return context.WithValue(ctx, pendingHookAllowedToolsContextKey{}, pendingHookAllowedToolsContextValue{
		chatID: chatID,
		policy: policy,
	})
}

func pendingHookAllowedToolsFromContext(ctx context.Context, chatID uuid.UUID) (pqtype.NullRawMessage, bool) {
	value, ok := ctx.Value(pendingHookAllowedToolsContextKey{}).(pendingHookAllowedToolsContextValue)
	if !ok || value.chatID != chatID {
		return pqtype.NullRawMessage{}, false
	}
	return value.policy, true
}

func narrowHookAllowedToolsResponses(current pqtype.NullRawMessage, responses []agenthooks.Response) (pqtype.NullRawMessage, bool, error) {
	narrowed := current
	applied := false
	for _, response := range responses {
		incoming, err := hookAllowedTools(response)
		if err != nil {
			return pqtype.NullRawMessage{}, false, err
		}
		if !incoming.Valid {
			continue
		}
		narrowed, err = chatstate.NarrowHookAllowedTools(narrowed, incoming)
		if err != nil {
			return pqtype.NullRawMessage{}, false, err
		}
		applied = true
	}
	return narrowed, applied, nil
}

func applyHookAllowedTools(ctx context.Context, store database.Store, chatID uuid.UUID, response agenthooks.Response) error {
	allowedTools, err := hookAllowedTools(response)
	if err != nil {
		return err
	}
	if !allowedTools.Valid {
		return nil
	}
	chat, err := store.GetChatByID(ctx, chatID)
	if err != nil {
		return xerrors.Errorf("load chat for hook allowed tools: %w", err)
	}
	narrowed, err := chatstate.NarrowHookAllowedTools(chat.HookAllowedTools, allowedTools)
	if err != nil {
		return err
	}
	if err := store.UpdateChatHookAllowedTools(ctx, database.UpdateChatHookAllowedToolsParams{
		HookAllowedTools: narrowed,
		ID:               chatID,
	}); err != nil {
		return xerrors.Errorf("update hook allowed tools: %w", err)
	}
	return nil
}

func applyHookAllowedToolsResponses(ctx context.Context, store database.Store, chatID uuid.UUID, responses []agenthooks.Response) error {
	for _, response := range responses {
		if err := applyHookAllowedTools(ctx, store, chatID, response); err != nil {
			return err
		}
	}
	return nil
}

func userPromptOverride(response agenthooks.Response) (string, bool, error) {
	if response.Permission == nil || response.Permission.Decision != agenthooks.PermissionAllow {
		return "", false, nil
	}
	var override struct {
		Prompt *string `json:"prompt"`
	}
	decoder := json.NewDecoder(bytes.NewReader(response.Permission.InputOverride))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&override); err != nil {
		return "", false, xerrors.Errorf("decode user prompt input override: %w", err)
	}
	if override.Prompt == nil {
		return "", false, xerrors.New("decode user prompt input override: prompt is required")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return "", false, xerrors.New("decode user prompt input override: trailing JSON value")
	}
	return *override.Prompt, true, nil
}

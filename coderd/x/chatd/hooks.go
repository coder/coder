package chatd

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agenthooks"
)

// UserPromptDeniedError reports a consumer denial without persisting the prompt.
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
	var workspaceID *uuid.UUID
	if chat.WorkspaceID.Valid {
		workspaceID = &chat.WorkspaceID.UUID
	}
	return p.hookDispatcher.Dispatch(ctx, chathooks.Event{
		Type:        eventType,
		ChatID:      chat.ID,
		OwnerID:     chat.OwnerID,
		WorkspaceID: workspaceID,
		TurnID:      turnID,
		Data:        data,
	})
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
	Step      chatloop.PersistedStep
	Responses []agenthooks.Response
}

func (p *Server) dispatchPreToolUse(
	ctx context.Context,
	chat database.Chat,
	turnID *uuid.UUID,
	toolCall fantasy.ToolCallContent,
) (agenthooks.Response, error) {
	var workspaceID *uuid.UUID
	if chat.WorkspaceID.Valid {
		workspaceID = &chat.WorkspaceID.UUID
	}
	toolUseID := toolCall.ToolCallID
	return p.hookDispatcher.Dispatch(ctx, chathooks.Event{
		Type:        agenthooks.EventPreToolUse,
		ChatID:      chat.ID,
		OwnerID:     chat.OwnerID,
		WorkspaceID: workspaceID,
		TurnID:      turnID,
		ToolUseID:   &toolUseID,
		Data: agenthooks.PreToolUseData{
			ToolUseID: toolUseID,
			ToolName:  toolCall.ToolName,
			ToolInput: json.RawMessage(toolCall.Input),
		},
	})
}

func (p *Server) dispatchPostToolUseData(
	ctx context.Context,
	chat database.Chat,
	turnID *uuid.UUID,
	data agenthooks.PostToolUseData,
) (agenthooks.Response, error) {
	var workspaceID *uuid.UUID
	if chat.WorkspaceID.Valid {
		workspaceID = &chat.WorkspaceID.UUID
	}
	toolUseID := data.ToolUseID
	return p.hookDispatcher.Dispatch(ctx, chathooks.Event{
		Type:        agenthooks.EventPostToolUse,
		ChatID:      chat.ID,
		OwnerID:     chat.OwnerID,
		WorkspaceID: workspaceID,
		TurnID:      turnID,
		ToolUseID:   &toolUseID,
		Data:        data,
	})
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
	for _, block := range content {
		toolResult, ok := asToolResultContent(block)
		if !ok || toolResult.ProviderExecuted {
			continue
		}
		response, err := p.dispatchPostToolUse(ctx, chat, turnID, toolResult)
		if err != nil {
			return responses, err
		}
		responses = append(responses, response)
	}
	return responses, nil
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

	for _, toolCall := range toolCalls {
		if toolCall.ProviderExecuted {
			continue
		}
		response, err := p.dispatchPreToolUse(ctx, chat, turnID, toolCall)
		if err != nil {
			return preToolUseResult{}, err
		}
		if err := applyPreToolUsePermission(&result.Step, toolCall, response); err != nil {
			return preToolUseResult{}, err
		}
		result.Responses = append(result.Responses, response)
	}
	return result, nil
}

type preToolUseExecutionResult struct {
	Allowed   []fantasy.ToolCallContent
	Denied    []fantasy.ToolResultContent
	Responses []agenthooks.Response
	Overrides map[string]json.RawMessage
}

func (p *Server) preflightPendingToolCalls(
	ctx context.Context,
	chat database.Chat,
	turnID *uuid.UUID,
	toolCalls []fantasy.ToolCallContent,
) (preToolUseExecutionResult, error) {
	result := preToolUseExecutionResult{
		Allowed:   make([]fantasy.ToolCallContent, 0, len(toolCalls)),
		Overrides: make(map[string]json.RawMessage),
	}
	if p.hookDispatcher == nil || !p.hookDispatcher.Enabled() {
		result.Allowed = append(result.Allowed, toolCalls...)
		return result, nil
	}

	for _, toolCall := range toolCalls {
		row, err := p.db.GetChatHookDispatchDecision(ctx, database.GetChatHookDispatchDecisionParams{
			ChatID:    chat.ID,
			ToolUseID: toolCall.ToolCallID,
			TurnID:    hookTurnID(turnID),
		})
		if err == nil {
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
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return preToolUseExecutionResult{}, xerrors.Errorf("get pre_tool_use decision: %w", err)
		}

		response, err := p.dispatchPreToolUse(ctx, chat, turnID, toolCall)
		if err != nil {
			return preToolUseExecutionResult{}, err
		}
		result.Responses = append(result.Responses, response)
		if response.Permission == nil {
			result.Allowed = append(result.Allowed, toolCall)
			continue
		}
		switch response.Permission.Decision {
		case agenthooks.PermissionAllow:
			toolCall.Input = string(response.Permission.InputOverride)
			result.Overrides[toolCall.ToolCallID] = response.Permission.InputOverride
			result.Allowed = append(result.Allowed, toolCall)
		case agenthooks.PermissionDeny:
			result.Denied = append(result.Denied, deniedToolResult(toolCall, response.Permission.Reason))
		}
	}
	return result, nil
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
	if p.hookDispatcher == nil || !p.hookDispatcher.Enabled() {
		return agenthooks.Response{}, nil
	}

	var workspaceID *uuid.UUID
	if chat.WorkspaceID.Valid {
		workspaceID = &chat.WorkspaceID.UUID
	}
	response, err := p.hookDispatcher.Dispatch(ctx, chathooks.Event{
		Type:        agenthooks.EventUserPromptSubmit,
		ChatID:      chat.ID,
		OwnerID:     chat.OwnerID,
		WorkspaceID: workspaceID,
		TurnID:      &turnID,
		ToolUseID:   nil,
		Data: agenthooks.UserPromptSubmitData{
			Prompt: promptText(parts),
		},
	})
	if err != nil {
		return agenthooks.Response{}, err
	}
	if response.Permission != nil && response.Permission.Decision == agenthooks.PermissionDeny {
		return agenthooks.Response{}, &UserPromptDeniedError{UserMessage: response.UserMessage}
	}
	return response, nil
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
		if _, err := tx.FailIdle(chatstate.FailIdleInput{LastError: lastError}); err != nil {
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
	Chat  database.Chat
	Ended bool
}

func applySessionStartResponse(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	chat database.Chat,
	turnID *uuid.UUID,
	response agenthooks.Response,
) (sessionStartResult, error) {
	if turnID == nil && (response.ModelContext != "" || response.UserMessage != "") {
		return sessionStartResult{}, xerrors.New("session_start response messages require an active turn")
	}
	if response.ModelContext == "" && response.UserMessage == "" && response.AllowedTools == nil && !response.EndChat {
		return sessionStartResult{Chat: chat}, nil
	}

	var prefixMessages []chatstate.Message
	var err error
	if turnID != nil {
		prefixMessages, err = hookPrefixMessages(response, chat.LastModelConfigID, turnID)
		if err != nil {
			return sessionStartResult{}, err
		}
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
			if _, err := tx.EndChat(chatstate.EndChatInput{PrefixMessages: prefixMessages}); err != nil {
				return xerrors.Errorf("end chat from session_start: %w", err)
			}
			result.Ended = true
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
	return s.finishEndedChat(ctx, input, result.Chat)
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

func applyHookResponseMessages(
	messages stepMessagesForCommit,
	responses []agenthooks.Response,
	modelConfigID uuid.UUID,
	turnID *uuid.UUID,
) (stepMessagesForCommit, bool, error) {
	var prefix []chatstate.Message
	endChat := false
	for _, response := range responses {
		if response.ModelContext != "" || response.UserMessage != "" {
			responseMessages, err := hookPrefixMessages(response, modelConfigID, turnID)
			if err != nil {
				return stepMessagesForCommit{}, false, err
			}
			prefix = append(prefix, responseMessages...)
		}
		endChat = endChat || response.EndChat
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
	var suffix []chatstate.Message
	endChat := false
	for _, response := range responses {
		if response.ModelContext != "" || response.UserMessage != "" {
			responseMessages, err := hookPrefixMessages(response, modelConfigID, turnID)
			if err != nil {
				return stepMessagesForCommit{}, false, err
			}
			suffix = append(suffix, responseMessages...)
		}
		endChat = endChat || response.EndChat
	}
	if len(suffix) > 0 {
		messages.Messages = append(messages.Messages, suffix...)
		messages.VisibleIndexes = visibleMessageIndexes(messages.Messages)
	}
	return messages, endChat, nil
}

func applyHookAllowedTools(ctx context.Context, store database.Store, chatID uuid.UUID, response agenthooks.Response) error {
	if response.AllowedTools == nil {
		return nil
	}
	encoded, err := json.Marshal(response.AllowedTools)
	if err != nil {
		return xerrors.Errorf("marshal hook allowed tools: %w", err)
	}
	if err := store.UpdateChatHookAllowedTools(ctx, database.UpdateChatHookAllowedToolsParams{
		HookAllowedTools: pqtype.NullRawMessage{RawMessage: encoded, Valid: true},
		ID:               chatID,
	}); err != nil {
		return xerrors.Errorf("update hook allowed tools: %w", err)
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

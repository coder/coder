package chatd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
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
	if !p.hookDispatcher.Enabled() {
		return agenthooks.Response{}, nil
	}
	resp, _, err := p.hookDispatcher.Dispatch(ctx, lifecycleHookEvent(chat, turnID, eventType, data))
	return resp, err
}

type preToolUseResult struct {
	Step chatloop.PersistedStep
	// Responses carries the responses whose transcript effects still
	// need to be committed with the step.
	Responses []agenthooks.Response
	// EffectToolUseIDs identifies the banked decisions whose transcript
	// effects commit with the step, for post-commit cache marking.
	EffectToolUseIDs []string
}

func (p *Server) dispatchPreToolUse(
	ctx context.Context,
	chat database.Chat,
	turnID *uuid.UUID,
	toolCall fantasy.ToolCallContent,
) (agenthooks.Response, error) {
	return p.dispatchLifecycleHook(ctx, chat, turnID, agenthooks.EventPreToolUse, agenthooks.PreToolUseData{
		ToolUseID: toolCall.ToolCallID,
		ToolName:  toolCall.ToolName,
		ToolInput: json.RawMessage(toolCall.Input),
	})
}

func (p *Server) dispatchPostToolUseData(
	ctx context.Context,
	chat database.Chat,
	turnID *uuid.UUID,
	data agenthooks.PostToolUseData,
) (agenthooks.Response, error) {
	return p.dispatchLifecycleHook(ctx, chat, turnID, agenthooks.EventPostToolUse, data)
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
	if !p.hookDispatcher.Enabled() {
		return nil, nil
	}
	responses := make([]agenthooks.Response, 0, len(content))
	// Dispatch every completed non-provider-executed tool result.
	// Preserve only the first failure.
	var firstErr error
	for _, block := range content {
		toolResult, ok := asToolResultContent(block)
		if !ok || toolResult.ProviderExecuted {
			continue
		}
		response, err := p.dispatchPostToolUse(ctx, chat, turnID, toolResult)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		responses = append(responses, response)
	}
	return responses, firstErr
}

func (p *Server) resolvePreToolUseDecision(
	ctx context.Context,
	chat database.Chat,
	turnID *uuid.UUID,
	toolCall fantasy.ToolCallContent,
	priorToolCallIDs map[string]bool,
) (agenthooks.Response, bool, error) {
	// Re-consult invalid JSON or IDs reused earlier in the turn because
	// no banked decision can safely authorize those calls.
	if json.Valid([]byte(toolCall.Input)) && !priorToolCallIDs[toolCall.ToolCallID] {
		response, effectsApplied, ok := p.hookDecisions.lookup(chat.ID, toolCall.ToolCallID, toolCall.ToolName, toolCall.Input)
		if ok {
			return response, effectsApplied, nil
		}
	}
	response, err := p.dispatchPreToolUse(ctx, chat, turnID, toolCall)
	if err != nil {
		return agenthooks.Response{}, false, err
	}
	p.bankPreToolUseDecision(chat.ID, toolCall, response)
	return response, false, nil
}

func (p *Server) preflightToolCalls(
	ctx context.Context,
	chat database.Chat,
	turnID *uuid.UUID,
	step chatloop.PersistedStep,
	toolCalls []fantasy.ToolCallContent,
	priorToolCallIDs map[string]bool,
) (preToolUseResult, error) {
	result := preToolUseResult{Step: step}
	if !p.hookDispatcher.Enabled() {
		return result, nil
	}
	if err := rejectDuplicateToolUseIDs(toolCalls); err != nil {
		return preToolUseResult{}, err
	}

	for _, toolCall := range toolCalls {
		if toolCall.ProviderExecuted {
			continue
		}
		banked, effectsApplied, err := p.resolvePreToolUseDecision(ctx, chat, turnID, toolCall, priorToolCallIDs)
		if err != nil {
			return preToolUseResult{}, err
		}
		if err := applyPreToolUsePermission(&result.Step, toolCall, banked); err != nil {
			return preToolUseResult{}, err
		}
		if !effectsApplied {
			result.Responses = append(result.Responses, transcriptHookResponse(banked))
			result.EffectToolUseIDs = append(result.EffectToolUseIDs, toolCall.ToolCallID)
		}
	}
	return result, nil
}

// bankPreToolUseDecision caches a successful decision for same-process
// replay. Invalid JSON input is never banked because replay identity
// requires the exact input.
func (p *Server) bankPreToolUseDecision(chatID uuid.UUID, toolCall fantasy.ToolCallContent, response agenthooks.Response) {
	if !json.Valid([]byte(toolCall.Input)) {
		return
	}
	p.hookDecisions.put(chatID, toolCall.ToolCallID, toolCall.ToolName, toolCall.Input, response)
}

// transcriptHookResponse returns the response with denial model context
// cleared: a denied call's model_context is folded into the synthetic
// tool result, so persisting it again as a row would duplicate it.
func transcriptHookResponse(response agenthooks.Response) agenthooks.Response {
	if response.Permission != nil && response.Permission.Decision == agenthooks.PermissionDeny {
		response.ModelContext = ""
	}
	return response
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

// rejectDuplicateToolUseIDs fails closed because banked replay decisions
// are keyed by tool-use ID within a turn.
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
	Allowed   []fantasy.ToolCallContent
	Denied    []fantasy.ToolResultContent
	Responses []agenthooks.Response
	Overrides map[string]json.RawMessage
	// EffectToolUseIDs identifies the banked decisions whose transcript
	// effects commit with this step, for post-commit cache marking.
	EffectToolUseIDs []string
}

func (p *Server) preflightPendingToolCalls(
	ctx context.Context,
	chat database.Chat,
	turnID *uuid.UUID,
	toolCalls []fantasy.ToolCallContent,
	priorToolCallIDs map[string]bool,
) (preToolUseExecutionResult, error) {
	if !p.hookDispatcher.Enabled() {
		return preToolUseExecutionResult{Allowed: toolCalls}, nil
	}
	result := preToolUseExecutionResult{
		Allowed: make([]fantasy.ToolCallContent, 0, len(toolCalls)),
	}
	if err := rejectDuplicateToolUseIDs(toolCalls); err != nil {
		return preToolUseExecutionResult{}, err
	}

	for _, toolCall := range toolCalls {
		banked, effectsApplied, err := p.resolvePreToolUseDecision(ctx, chat, turnID, toolCall, priorToolCallIDs)
		if err != nil {
			return preToolUseExecutionResult{}, err
		}
		if !effectsApplied {
			result.Responses = append(result.Responses, transcriptHookResponse(banked))
			result.EffectToolUseIDs = append(result.EffectToolUseIDs, toolCall.ToolCallID)
		}
		if banked.Permission == nil {
			result.Allowed = append(result.Allowed, toolCall)
			continue
		}
		switch banked.Permission.Decision {
		case agenthooks.PermissionAllow:
			if len(banked.Permission.InputOverride) > 0 {
				toolCall.Input = string(banked.Permission.InputOverride)
				if result.Overrides == nil {
					result.Overrides = make(map[string]json.RawMessage)
				}
				result.Overrides[toolCall.ToolCallID] = banked.Permission.InputOverride
			}
			result.Allowed = append(result.Allowed, toolCall)
		case agenthooks.PermissionDeny:
			result.Denied = append(result.Denied, deniedToolResult(toolCall, banked.Permission.Reason, banked.ModelContext))
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
	changed := false
	for i := range parts {
		if override, ok := overrides[parts[i].ToolCallID]; ok && parts[i].Type == codersdk.ChatMessagePartTypeToolCall && !bytes.Equal(parts[i].Args, override) {
			parts[i].Args = override
			changed = true
		}
	}
	if !changed {
		return nil
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
		step.Content = append(step.Content, deniedToolResult(toolCall, response.Permission.Reason, response.ModelContext))
	}
	return nil
}

// deniedToolResult synthesizes the denial as a tool result so the model
// can replan within the same turn. The consumer's model_context rides in
// the same result instead of a separate transcript row.
func deniedToolResult(toolCall fantasy.ToolCallContent, reason, modelContext string) fantasy.ToolResultContent {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "denied by lifecycle hook"
	}
	message := "DENIED: " + reason
	if modelContext = strings.TrimSpace(modelContext); modelContext != "" {
		message += "\n\n" + modelContext
	}
	return fantasy.ToolResultContent{
		ToolCallID: toolCall.ToolCallID,
		ToolName:   toolCall.ToolName,
		Result: fantasy.ToolResultOutputContentError{
			Error: xerrors.New(message),
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

// UserPromptDeniedError reports that a lifecycle hook rejected a prompt.
type UserPromptDeniedError struct {
	UserMessage string
}

// Error includes UserMessage so callers that only surface the error
// string, such as subagent tool responses, still expose the hook's
// reason. The HTTP handlers unwrap the typed error instead.
func (e *UserPromptDeniedError) Error() string {
	if e.UserMessage == "" {
		return "user prompt denied by lifecycle hook"
	}
	return "user prompt denied by lifecycle hook: " + e.UserMessage
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
		Prompt: textFromParts(parts),
		Parts:  encodedParts.RawMessage,
	})
	if err != nil {
		return agenthooks.Response{}, err
	}
	if response.Permission != nil && response.Permission.Decision == agenthooks.PermissionDeny {
		return response, &UserPromptDeniedError{UserMessage: response.UserMessage}
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
	Chat database.Chat
}

func applySessionStartResponse(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	chat database.Chat,
	response agenthooks.Response,
) (sessionStartResult, error) {
	if response.ModelContext == "" && response.UserMessage == "" {
		return sessionStartResult{Chat: chat}, nil
	}

	eventMessages, err := hookEventMessages(response, chat.LastModelConfigID)
	if err != nil {
		return sessionStartResult{}, err
	}

	var result sessionStartResult
	err = machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		if _, err := loadChatForGeneration(ctx, store, input, generationAttemptNotRequired); err != nil {
			return xerrors.Errorf("load chat for session_start response: %w", err)
		}
		if len(eventMessages) > 0 {
			if _, err := tx.CommitStep(chatstate.CommitStepInput{Messages: eventMessages}); err != nil {
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

// hookEventMessages converts a turn-time hook response into ordinary
// transcript rows: model context becomes a user-role, model-visible row
// and the user message becomes a system-role, user-visible notice row.
func hookEventMessages(response agenthooks.Response, modelConfigID uuid.UUID) ([]chatstate.Message, error) {
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
			ModelConfigID:  uuid.NullUUID{UUID: modelConfigID, Valid: modelConfigID != uuid.Nil},
			ContentVersion: chatprompt.CurrentContentVersion,
		})
	}
	return messages, nil
}

func hookEventMessagesForResponses(
	responses []agenthooks.Response,
	modelConfigID uuid.UUID,
) ([]chatstate.Message, error) {
	var messages []chatstate.Message
	for _, response := range responses {
		responseMessages, err := hookEventMessages(response, modelConfigID)
		if err != nil {
			return nil, err
		}
		messages = append(messages, responseMessages...)
	}
	return messages, nil
}

// applyHookResponseMessages inserts hook event rows before the step's
// own rows so injected model context precedes the assistant content it
// steers; providers require tool results to directly follow the
// assistant tool calls.
func applyHookResponseMessages(
	messages stepMessagesForCommit,
	responses []agenthooks.Response,
	modelConfigID uuid.UUID,
) (stepMessagesForCommit, error) {
	rows, err := hookEventMessagesForResponses(responses, modelConfigID)
	if err != nil {
		return stepMessagesForCommit{}, err
	}
	if len(rows) > 0 {
		messages.Messages = append(rows, messages.Messages...)
		messages.VisibleIndexes = visibleMessageIndexes(messages.Messages)
	}
	return messages, nil
}

func appendHookResponseMessages(
	messages stepMessagesForCommit,
	responses []agenthooks.Response,
	modelConfigID uuid.UUID,
) (stepMessagesForCommit, error) {
	suffix, err := hookEventMessagesForResponses(responses, modelConfigID)
	if err != nil {
		return stepMessagesForCommit{}, err
	}
	if len(suffix) > 0 {
		messages.Messages = append(messages.Messages, suffix...)
		messages.VisibleIndexes = visibleMessageIndexes(messages.Messages)
	}
	return messages, nil
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

// userPromptHookParts converts a user_prompt_submit response into the
// typed parts carried inside the submitted message: hook-context is
// model-only steering, hook-notice is a client-only notice.
func userPromptHookParts(response agenthooks.Response) []codersdk.ChatMessagePart {
	parts := make([]codersdk.ChatMessagePart, 0, 2)
	if response.ModelContext != "" {
		parts = append(parts, codersdk.ChatMessagePart{
			Type: codersdk.ChatMessagePartTypeHookContext,
			Text: response.ModelContext,
		})
	}
	if response.UserMessage != "" {
		parts = append(parts, codersdk.ChatMessagePart{
			Type: codersdk.ChatMessagePartTypeHookNotice,
			Text: response.UserMessage,
		})
	}
	return parts
}

// composeUserPromptContent applies a user_prompt_submit response to the
// submitted parts. The merge order is fixed: override-or-original user
// parts first, then hook-context, then hook-notice. The composite
// content then flows through the ordinary send, queue, and edit paths.
func composeUserPromptContent(parts []codersdk.ChatMessagePart, response agenthooks.Response) ([]codersdk.ChatMessagePart, bool, error) {
	override, overridden, err := userPromptOverride(response)
	if err != nil {
		return nil, false, err
	}
	userParts := parts
	if overridden {
		userParts = []codersdk.ChatMessagePart{codersdk.ChatMessageText(override)}
	}
	hookParts := userPromptHookParts(response)
	if len(hookParts) == 0 {
		return userParts, overridden, nil
	}
	combined := make([]codersdk.ChatMessagePart, 0, len(userParts)+len(hookParts))
	combined = append(combined, userParts...)
	combined = append(combined, hookParts...)
	return combined, overridden, nil
}

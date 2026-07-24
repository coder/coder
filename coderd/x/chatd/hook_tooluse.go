package chatd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/hooks"
	"github.com/coder/coder/v2/codersdk"
)

// rejectDuplicateToolUseIDs fails closed because hook consumers key
// decisions by tool-use ID; a duplicated ID in one step makes decisions
// unattributable.
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

// preToolUseExecutionResult preserves hook results in tool-call order for
// transcript injection.
type preToolUseExecutionResult struct {
	Allowed   []fantasy.ToolCallContent
	Denied    []fantasy.ToolResultContent
	Results   []*hookResult
	Overrides map[string]json.RawMessage
}

func (t *hookTrigger) preflightPendingToolCalls(
	ctx context.Context,
	chat hookChat,
	toolCalls []fantasy.ToolCallContent,
) (preToolUseExecutionResult, error) {
	if !t.enabled() {
		return preToolUseExecutionResult{Allowed: toolCalls}, nil
	}
	result := preToolUseExecutionResult{
		Allowed: make([]fantasy.ToolCallContent, 0, len(toolCalls)),
	}
	if err := rejectDuplicateToolUseIDs(toolCalls); err != nil {
		return preToolUseExecutionResult{}, err
	}

	for _, toolCall := range toolCalls {
		callResult, err := t.trigger(ctx, chat, hookMessage{
			ToolUseID: toolCall.ToolCallID,
			ToolName:  toolCall.ToolName,
			ToolInput: json.RawMessage(toolCall.Input),
		}, hooks.EventPreToolUse)
		if err != nil {
			var denied *hookDeniedError
			if !errors.As(err, &denied) {
				return preToolUseExecutionResult{}, err
			}
			// The denial's model context folds into the synthetic tool
			// result; only the user notice needs a transcript row.
			result.Results = append(result.Results, &hookResult{UserMessage: denied.UserMessage})
			result.Denied = append(result.Denied, deniedToolResult(toolCall, denied.Reason, denied.ModelContext))
			continue
		}
		result.Results = append(result.Results, callResult)
		if len(callResult.InputOverride) > 0 {
			toolCall.Input = string(callResult.InputOverride)
			if result.Overrides == nil {
				result.Overrides = make(map[string]json.RawMessage)
			}
			result.Overrides[toolCall.ToolCallID] = callResult.InputOverride
		}
		result.Allowed = append(result.Allowed, toolCall)
	}
	return result, nil
}

func postToolUseMessage(toolResult fantasy.ToolResultContent) (hookMessage, error) {
	msg := hookMessage{
		ToolUseID: toolResult.ToolCallID,
		ToolName:  toolResult.ToolName,
	}
	switch output := toolResult.Result.(type) {
	case fantasy.ToolResultOutputContentError:
		if output.Error != nil {
			msg.ToolError = output.Error.Error()
		}
	case *fantasy.ToolResultOutputContentError:
		if output != nil && output.Error != nil {
			msg.ToolError = output.Error.Error()
		}
	default:
		encoded, err := json.Marshal(toolResult.Result)
		if err != nil {
			return hookMessage{}, xerrors.Errorf("marshal post_tool_use response: %w", err)
		}
		msg.ToolResponse = encoded
	}
	return msg, nil
}

func (t *hookTrigger) postToolUseResults(
	ctx context.Context,
	chat hookChat,
	content []fantasy.Content,
) ([]*hookResult, error) {
	if !t.enabled() {
		return nil, nil
	}
	results := make([]*hookResult, 0, len(content))
	var firstErr error
	for _, block := range content {
		toolResult, ok := asToolResultContent(block)
		if !ok || toolResult.ProviderExecuted {
			continue
		}
		msg, err := postToolUseMessage(toolResult)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		result, err := t.trigger(ctx, chat, msg, hooks.EventPostToolUse)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		results = append(results, result)
	}
	return results, firstErr
}

func replacePersistedToolCallInputs(
	ctx context.Context,
	tx *chatstate.Tx,
	chatID uuid.UUID,
	overrides map[string]json.RawMessage,
) error {
	if len(overrides) == 0 {
		return nil
	}
	assistant, err := tx.Store().GetLastChatMessageByRole(ctx, database.GetLastChatMessageByRoleParams{
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
	if err := tx.UpdateMessageContent(assistant.ID, content.RawMessage); err != nil {
		return xerrors.Errorf("update assistant message with tool override: %w", err)
	}
	return nil
}

type dynamicPostToolUseState struct {
	chat          database.Chat
	modelConfigID uuid.UUID
	toolNames     map[string]string
}

func loadDynamicPostToolUseState(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	opts SubmitToolResultsOptions,
) (dynamicPostToolUseState, error) {
	var state dynamicPostToolUseState
	err := machine.ReadLock(ctx, func(store database.Store) error {
		chat, err := store.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		if chat.Archived {
			return ErrChatArchived
		}
		if chat.Status != database.ChatStatusRequiresAction {
			return &ToolResultStatusConflictError{ActualStatus: chat.Status}
		}
		messages, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  opts.ChatID,
			AfterID: 0,
		})
		if err != nil {
			return xerrors.Errorf("load chat messages: %w", err)
		}
		_, pending, err := unresolvedToolCallsFromHistory(messages, dynamicToolNamesFromChat(chat))
		if err != nil {
			return xerrors.Errorf("load pending dynamic tool calls: %w", err)
		}
		toolNames := make(map[string]string, len(pending))
		for _, call := range pending {
			toolNames[call.ToolCallID] = call.ToolName
		}
		if err := validateSubmittedToolResults(opts.Results, toolNames); err != nil {
			return err
		}
		modelConfigID := opts.ModelConfigID
		if modelConfigID == uuid.Nil {
			modelConfigID = chat.LastModelConfigID
		}
		state = dynamicPostToolUseState{
			chat:          chat,
			modelConfigID: modelConfigID,
			toolNames:     toolNames,
		}
		return nil
	})
	return state, err
}

func validateSubmittedToolResults(results []codersdk.ToolResult, toolNames map[string]string) error {
	submitted := make(map[string]struct{}, len(results))
	for _, result := range results {
		if _, ok := submitted[result.ToolCallID]; ok {
			return &ToolResultValidationError{
				Message: "Duplicate tool_call_id in results.",
				Detail:  fmt.Sprintf("Duplicate tool call ID %q.", result.ToolCallID),
			}
		}
		if !json.Valid(result.Output) {
			return &ToolResultValidationError{
				Message: "Tool result output must be valid JSON.",
				Detail:  fmt.Sprintf("Output for tool call %q is not valid JSON.", result.ToolCallID),
			}
		}
		if _, ok := toolNames[result.ToolCallID]; !ok {
			return &ToolResultValidationError{
				Message: "Unexpected tool result.",
				Detail:  fmt.Sprintf("No pending tool call with ID %q.", result.ToolCallID),
			}
		}
		submitted[result.ToolCallID] = struct{}{}
	}
	for toolCallID := range toolNames {
		if _, ok := submitted[toolCallID]; !ok {
			return &ToolResultValidationError{
				Message: "Missing tool result.",
				Detail:  fmt.Sprintf("Missing result for tool call %q.", toolCallID),
			}
		}
	}
	return nil
}

func dynamicPostToolUseMessage(result codersdk.ToolResult, toolName string) hookMessage {
	msg := hookMessage{
		ToolUseID: result.ToolCallID,
		ToolName:  toolName,
	}
	if result.IsError {
		if err := json.Unmarshal(result.Output, &msg.ToolError); err != nil {
			msg.ToolError = string(result.Output)
		}
	} else {
		msg.ToolResponse = append(json.RawMessage(nil), result.Output...)
	}
	return msg
}

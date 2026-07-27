package chathooks

import (
	"context"
	"encoding/json"
	"errors"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
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

// PreToolUseExecutionResult preserves hook results in tool-call order for
// transcript injection.
type PreToolUseExecutionResult struct {
	Allowed   []fantasy.ToolCallContent
	Denied    []fantasy.ToolResultContent
	Results   []*Result
	Overrides map[string]json.RawMessage
}

func (t *Trigger) PreflightPendingToolCalls(
	ctx context.Context,
	chat Chat,
	toolCalls []fantasy.ToolCallContent,
) (PreToolUseExecutionResult, error) {
	if !t.Enabled() {
		return PreToolUseExecutionResult{Allowed: toolCalls}, nil
	}
	result := PreToolUseExecutionResult{
		Allowed: make([]fantasy.ToolCallContent, 0, len(toolCalls)),
	}
	if err := rejectDuplicateToolUseIDs(toolCalls); err != nil {
		return PreToolUseExecutionResult{}, err
	}

	for _, toolCall := range toolCalls {
		callResult, err := t.Trigger(ctx, chat, Message{
			ToolUseID: toolCall.ToolCallID,
			ToolName:  toolCall.ToolName,
			ToolInput: json.RawMessage(toolCall.Input),
		}, agenthooks.EventPreToolUse)
		if err != nil {
			denied, ok := errors.AsType[*deniedError](err)
			if !ok {
				return PreToolUseExecutionResult{}, err
			}
			// The synthetic tool result is client-visible, so the
			// denial's model context becomes a model-only transcript
			// row instead of riding in the result.
			result.Results = append(result.Results, &Result{
				ModelContext: denied.ModelContext,
				UserMessage:  denied.UserMessage,
			})
			result.Denied = append(result.Denied, deniedToolResult(toolCall, denied.Reason))
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

func postToolUseMessage(toolResult fantasy.ToolResultContent) (Message, error) {
	msg := Message{
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
			return Message{}, xerrors.Errorf("marshal post_tool_use response: %w", err)
		}
		msg.ToolResponse = encoded
	}
	return msg, nil
}

func (t *Trigger) PostToolUseResults(
	ctx context.Context,
	chat Chat,
	content []fantasy.Content,
) ([]*Result, error) {
	if !t.Enabled() {
		return nil, nil
	}
	results := make([]*Result, 0, len(content))
	var firstErr error
	for _, block := range content {
		toolResult, ok := asToolResultContent(block)
		if !ok || toolResult.ProviderExecuted {
			continue
		}
		// A hook dispatch failure means admission was refused, so the tool
		// never ran and there is no use to post-process.
		if dispatchFailureFromResult(toolResult) != nil {
			continue
		}
		msg, err := postToolUseMessage(toolResult)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		result, err := t.Trigger(ctx, chat, msg, agenthooks.EventPostToolUse)
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

// ApplyAdmittedToolCalls rewrites the step's tool-call inputs to the values
// pre_tool_use admitted and appends the synthetic denial results, so the step
// is persisted with the input the tool runs with.
func ApplyAdmittedToolCalls(content []fantasy.Content, preflight PreToolUseExecutionResult) []fantasy.Content {
	admitted := make(map[string]fantasy.ToolCallContent, len(preflight.Allowed))
	for _, toolCall := range preflight.Allowed {
		admitted[toolCall.ToolCallID] = toolCall
	}
	rewritten := make([]fantasy.Content, 0, len(content)+len(preflight.Denied))
	for _, block := range content {
		toolCall, ok := asToolCallContent(block)
		if !ok {
			rewritten = append(rewritten, block)
			continue
		}
		if replacement, found := admitted[toolCall.ToolCallID]; found {
			toolCall.Input = replacement.Input
		}
		rewritten = append(rewritten, toolCall)
	}
	for _, denied := range preflight.Denied {
		rewritten = append(rewritten, denied)
	}
	return rewritten
}

func asToolCallContent(block fantasy.Content) (fantasy.ToolCallContent, bool) {
	if tc, ok := fantasy.AsContentType[fantasy.ToolCallContent](block); ok {
		return tc, true
	}
	if tc, ok := fantasy.AsContentType[*fantasy.ToolCallContent](block); ok && tc != nil {
		return *tc, true
	}
	return fantasy.ToolCallContent{}, false
}

// PendingToolCalls lists the tool calls a step leaves for Coder to run.
// Provider-executed calls run inside the provider, so hooks never see them.
func PendingToolCalls(content []fantasy.Content) []fantasy.ToolCallContent {
	toolCalls := make([]fantasy.ToolCallContent, 0, len(content))
	for _, block := range content {
		toolCall, ok := asToolCallContent(block)
		if !ok || toolCall.ProviderExecuted {
			continue
		}
		toolCalls = append(toolCalls, toolCall)
	}
	return toolCalls
}

func DynamicPostToolUseMessage(result codersdk.ToolResult, toolName string) Message {
	msg := Message{
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

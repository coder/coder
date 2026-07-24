package chatd

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
)

// hookEventMessages converts a turn-time hook result into ordinary
// transcript rows: model context becomes a user-role, model-visible row
// and the user message becomes a system-role, user-visible notice row.
func hookEventMessages(result *hookResult, modelConfigID uuid.UUID) ([]chatstate.Message, error) {
	messages := make([]chatstate.Message, 0, 2)
	if result.modelContext() != "" {
		content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(result.ModelContext)})
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
	if result.userMessage() != "" {
		content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(result.UserMessage)})
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

func hookEventMessagesForResults(
	results []*hookResult,
	modelConfigID uuid.UUID,
) ([]chatstate.Message, error) {
	var messages []chatstate.Message
	for _, result := range results {
		resultMessages, err := hookEventMessages(result, modelConfigID)
		if err != nil {
			return nil, err
		}
		messages = append(messages, resultMessages...)
	}
	return messages, nil
}

// applyHookResultMessages inserts hook event rows before the step's
// own rows so injected model context precedes the assistant content it
// steers; providers require tool results to directly follow the
// assistant tool calls.
func applyHookResultMessages(
	messages stepMessagesForCommit,
	results []*hookResult,
	modelConfigID uuid.UUID,
) (stepMessagesForCommit, error) {
	return insertHookResultMessages(messages, results, modelConfigID, hookRowsBeforeStep)
}

func appendHookResultMessages(
	messages stepMessagesForCommit,
	results []*hookResult,
	modelConfigID uuid.UUID,
) (stepMessagesForCommit, error) {
	return insertHookResultMessages(messages, results, modelConfigID, hookRowsAfterStep)
}

type hookRowPlacement int

const (
	hookRowsBeforeStep hookRowPlacement = iota
	hookRowsAfterStep
)

func insertHookResultMessages(
	messages stepMessagesForCommit,
	results []*hookResult,
	modelConfigID uuid.UUID,
	placement hookRowPlacement,
) (stepMessagesForCommit, error) {
	rows, err := hookEventMessagesForResults(results, modelConfigID)
	if err != nil {
		return stepMessagesForCommit{}, err
	}
	if len(rows) > 0 {
		if placement == hookRowsBeforeStep {
			messages.Messages = append(rows, messages.Messages...)
		} else {
			messages.Messages = append(messages.Messages, rows...)
		}
		messages.VisibleIndexes = visibleMessageIndexes(messages.Messages)
	}
	return messages, nil
}

// deniedToolResult synthesizes the denial as a tool result so the model
// can replan within the same turn. The consumer's model_context rides in
// the same result instead of a separate transcript row. The text must
// distinguish a policy denial from a genuine tool failure, or the model
// retries the call and misreports the denial as an infrastructure error.
func deniedToolResult(toolCall fantasy.ToolCallContent, reason, modelContext string) fantasy.ToolResultContent {
	message := "Tool call denied by the deployment's lifecycle hook policy."
	if reason = strings.TrimSpace(reason); reason != "" {
		message += " Reason: " + reason + "."
	}
	message += " This is an administrative policy decision, not a tool or" +
		" workspace failure; retrying the same call will be denied again." +
		" Explain the denial to the user and adjust your approach."
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

// restoreToolCallOrder reorders known tool results to match the assistant's
// call order while preserving slots for unrelated entries.
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

func userPromptOverride(result *hookResult) (string, bool, error) {
	if result == nil || len(result.InputOverride) == 0 {
		return "", false, nil
	}
	var override struct {
		Prompt *string `json:"prompt"`
	}
	decoder := json.NewDecoder(bytes.NewReader(result.InputOverride))
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

func userPromptHookParts(result *hookResult) []codersdk.ChatMessagePart {
	parts := make([]codersdk.ChatMessagePart, 0, 2)
	if result.modelContext() != "" {
		parts = append(parts, codersdk.ChatMessagePart{
			Type: codersdk.ChatMessagePartTypeHookContext,
			Text: result.ModelContext,
		})
	}
	if result.userMessage() != "" {
		parts = append(parts, codersdk.ChatMessagePart{
			Type: codersdk.ChatMessagePartTypeHookNotice,
			Text: result.UserMessage,
		})
	}
	return parts
}

// composeUserPromptContent applies a user_prompt_submit result to the
// submitted parts. The merge order is fixed: override-or-original user
// parts first, then hook-context, then hook-notice. The composite
// content then flows through the ordinary send, queue, and edit paths.
func composeUserPromptContent(parts []codersdk.ChatMessagePart, result *hookResult) ([]codersdk.ChatMessagePart, bool, error) {
	override, overridden, err := userPromptOverride(result)
	if err != nil {
		return nil, false, err
	}
	userParts := parts
	if overridden {
		userParts = []codersdk.ChatMessagePart{codersdk.ChatMessageText(override)}
	}
	hookParts := userPromptHookParts(result)
	if len(hookParts) == 0 {
		return userParts, overridden, nil
	}
	combined := make([]codersdk.ChatMessagePart, 0, len(userParts)+len(hookParts))
	combined = append(combined, userParts...)
	combined = append(combined, hookParts...)
	return combined, overridden, nil
}

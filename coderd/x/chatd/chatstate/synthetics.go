package chatstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

// synthesizePendingToolCancellations builds [Message] inserts that
// satisfy every outstanding tool call on the chat's last assistant
// message with a synthetic cancellation tool-result message.
//
// The caller controls whether to limit to dynamic-tool calls (true) or
// close every outstanding tool call regardless of source (false). The
// dynamic-only variant is used by requires-action interrupts; the
// all-tool variant is used by any transition that needs to insert a
// new user message into history.
//
// The synthetic results use the supplied chat's last_model_config_id.
// Returns (nil, nil) when there is nothing to synthesize.
func synthesizePendingToolCancellations(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
	reason string,
	dynamicOnly bool,
) ([]Message, error) {
	calls, err := outstandingToolCalls(ctx, store, chat, dynamicOnly)
	if err != nil {
		return nil, err
	}
	var out []Message
	for _, call := range calls {
		result := codersdk.ChatMessageToolResult(call.ToolCallID, call.ToolName, json.RawMessage(fmt.Sprintf("%q", reason)), true, false)
		raw, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{result})
		if err != nil {
			return nil, xerrors.Errorf("marshal synthetic tool result: %w", err)
		}
		out = append(out, Message{
			Role:           database.ChatMessageRoleTool,
			Content:        raw,
			Visibility:     database.ChatMessageVisibilityBoth,
			ContentVersion: chatprompt.CurrentContentVersion,
			ModelConfigID:  uuid.NullUUID{UUID: chat.LastModelConfigID, Valid: true},
		})
	}
	return out, nil
}

// pendingDynamicToolCallIDs returns the dynamic tool-call IDs on the
// chat's last assistant message that do not yet have a matching
// tool-result message in active history. The returned map is keyed by
// tool-call ID and valued by tool name so callers can build matching
// result messages without re-parsing the assistant content.
func pendingDynamicToolCallIDs(ctx context.Context, store database.Store, chat database.Chat) (map[string]string, error) {
	calls, err := outstandingToolCalls(ctx, store, chat, true)
	if err != nil {
		return nil, err
	}
	pending := make(map[string]string, len(calls))
	for _, call := range calls {
		pending[call.ToolCallID] = call.ToolName
	}
	return pending, nil
}

// outstandingToolCalls returns, in assistant-part order, the tool calls
// on the chat's last assistant message that have no matching tool
// result in that message or in a tool message after it.
//
//nolint:revive // dynamicOnly is a domain flag, not a control flag.
func outstandingToolCalls(ctx context.Context, store database.Store, chat database.Chat, dynamicOnly bool) ([]codersdk.ChatMessagePart, error) {
	var dynamicToolNames map[string]bool
	if dynamicOnly {
		var err error
		dynamicToolNames, err = parseDynamicToolNamesFromRaw(chat.DynamicTools)
		if err != nil {
			return nil, xerrors.Errorf("parse dynamic tool names: %w", err)
		}
		if len(dynamicToolNames) == 0 {
			return nil, nil
		}
	}
	parts, answered, err := lastAssistantTurn(ctx, store, chat.ID)
	if err != nil {
		return nil, err
	}
	var out []codersdk.ChatMessagePart
	for _, p := range parts {
		// Provider-executed tool calls are answered server-side by the
		// LLM provider. A synthetic client tool-result for them is
		// invalid replay history: Anthropic rejects a plain tool_result
		// block that references a server_tool_use ID.
		if p.Type != codersdk.ChatMessagePartTypeToolCall || p.ProviderExecuted || answered[p.ToolCallID] {
			continue
		}
		if dynamicOnly && !dynamicToolNames[p.ToolName] {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// lastAssistantTurn returns the parts of the chat's last assistant
// message and the IDs of the tool calls answered in that message or by
// a tool message after it. Provider-executed tool results (e.g.
// web_search) are persisted inside the assistant message itself.
func lastAssistantTurn(ctx context.Context, store database.Store, chatID uuid.UUID) ([]codersdk.ChatMessagePart, map[string]bool, error) {
	lastAssistant, err := store.GetLastChatMessageByRole(ctx, database.GetLastChatMessageByRoleParams{
		ChatID: chatID,
		Role:   database.ChatMessageRoleAssistant,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, xerrors.Errorf("get last assistant message: %w", err)
	}
	parts, err := chatprompt.ParseContent(lastAssistant)
	if err != nil {
		return nil, nil, xerrors.Errorf("parse assistant message: %w", err)
	}
	afterMsgs, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chatID,
		AfterID: lastAssistant.ID,
	})
	if err != nil {
		return nil, nil, xerrors.Errorf("get messages after assistant: %w", err)
	}
	answered := make(map[string]bool)
	markAnswered := func(msgParts []codersdk.ChatMessagePart) {
		for _, p := range msgParts {
			if p.Type == codersdk.ChatMessagePartTypeToolResult {
				answered[p.ToolCallID] = true
			}
		}
	}
	markAnswered(parts)
	for _, msg := range afterMsgs {
		if msg.Role != database.ChatMessageRoleTool {
			continue
		}
		messageParts, err := chatprompt.ParseContent(msg)
		if err != nil {
			// An unparsable historical message must not fail the scan;
			// its tool results are treated as unknown.
			continue
		}
		markAnswered(messageParts)
	}
	return parts, answered, nil
}

// parseDynamicToolNamesFromRaw is a private mirror of
// chatd.parseDynamicToolNames so chatstate does not pull a runtime
// dependency on the chatd package. It accepts a nullable raw JSON
// blob and returns a name set.
func parseDynamicToolNamesFromRaw(raw pqtype.NullRawMessage) (map[string]bool, error) {
	if !raw.Valid || len(raw.RawMessage) == 0 {
		return map[string]bool{}, nil
	}
	var tools []codersdk.DynamicTool
	if err := json.Unmarshal(raw.RawMessage, &tools); err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(tools))
	for _, t := range tools {
		out[t.Name] = true
	}
	return out, nil
}

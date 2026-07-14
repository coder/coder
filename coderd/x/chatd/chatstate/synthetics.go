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
// "Outstanding" means a tool call present on the last assistant
// message that does not yet have a matching tool-result message in
// the active history after it. The caller controls whether to limit
// to dynamic-tool calls (true) or close every outstanding tool call
// regardless of source (false). The dynamic-only variant is used by
// requires-action interrupts; the all-tool variant is used by any
// transition that needs to insert a new user message into history.
//
// The synthetic results use the supplied chat's last_model_config_id.
// Returns (nil, nil) when there is nothing to synthesize.
//
//nolint:revive // dynamicOnly is a domain flag, not a control flag.
func synthesizePendingToolCancellations(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
	reason string,
	dynamicOnly bool,
) ([]Message, error) {
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

	lastAssistant, err := store.GetLastChatMessageByRole(ctx, database.GetLastChatMessageByRoleParams{
		ChatID: chat.ID,
		Role:   database.ChatMessageRoleAssistant,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, xerrors.Errorf("get last assistant message: %w", err)
	}
	assistantParts, err := chatprompt.ParseContent(lastAssistant)
	if err != nil {
		return nil, xerrors.Errorf("parse assistant message: %w", err)
	}
	afterMsgs, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: lastAssistant.ID,
	})
	if err != nil {
		return nil, xerrors.Errorf("get messages after assistant: %w", err)
	}
	handled := make(map[string]bool)
	// Provider-executed tool results (e.g. web_search) are persisted
	// inside the assistant message itself, not as tool-role messages
	// after it. Count them as handled so their calls are not treated
	// as outstanding.
	for _, p := range assistantParts {
		if p.Type == codersdk.ChatMessagePartTypeToolResult {
			handled[p.ToolCallID] = true
		}
	}
	for _, msg := range afterMsgs {
		if msg.Role != database.ChatMessageRoleTool {
			continue
		}
		parts, err := chatprompt.ParseContent(msg)
		if err != nil {
			// Don't fail the whole cancellation just because one
			// historical message is unparsable; treat its tool
			// results as unknown.
			continue
		}
		for _, p := range parts {
			if p.Type == codersdk.ChatMessagePartTypeToolResult {
				handled[p.ToolCallID] = true
			}
		}
	}
	out := make([]Message, 0)
	for _, part := range assistantParts {
		if part.Type != codersdk.ChatMessagePartTypeToolCall {
			continue
		}
		// Provider-executed tool calls are handled server-side by the
		// LLM provider. A synthetic client tool-result for them is
		// invalid replay history: Anthropic rejects a plain tool_result
		// block that references a server_tool_use ID.
		if part.ProviderExecuted {
			continue
		}
		if dynamicOnly && !dynamicToolNames[part.ToolName] {
			continue
		}
		if handled[part.ToolCallID] {
			continue
		}
		resultPart := codersdk.ChatMessagePart{
			Type:       codersdk.ChatMessagePartTypeToolResult,
			ToolCallID: part.ToolCallID,
			ToolName:   part.ToolName,
			Result:     json.RawMessage(fmt.Sprintf("%q", reason)),
			IsError:    true,
		}
		raw, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{resultPart})
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
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// pendingDynamicToolCallIDs returns the dynamic tool-call IDs on the
// chat's last assistant message that do not yet have a matching
// tool-result message in active history. The returned map is keyed by
// tool-call ID and valued by tool name so callers can build matching
// result messages without re-parsing the assistant content.
func pendingDynamicToolCallIDs(ctx context.Context, store database.Store, chat database.Chat) (map[string]string, error) {
	dynamic, err := parseDynamicToolNamesFromRaw(chat.DynamicTools)
	if err != nil {
		return nil, err
	}
	if len(dynamic) == 0 {
		return map[string]string{}, nil
	}
	return outstandingToolCallIDs(ctx, store, chat, func(toolName string) bool {
		return dynamic[toolName]
	})
}

// pendingAllToolCallIDs returns the tool-call IDs of every outstanding
// tool call on the chat's last assistant message, regardless of
// whether the tool is dynamic. The returned map is keyed by tool-call
// ID and valued by tool name. Callers that must guarantee a valid
// LLM message history (e.g. before promoting a user message into
// active history, or after committing an interruption's partial
// messages) should use this variant so non-dynamic tool calls do not
// silently bypass the check.
func pendingAllToolCallIDs(ctx context.Context, store database.Store, chat database.Chat) (map[string]string, error) {
	return outstandingToolCallIDs(ctx, store, chat, func(string) bool { return true })
}

// outstandingToolCallIDs walks the chat's last assistant message and
// returns the subset of its tool calls that have no matching
// tool-result message in the active history after it. The accept
// callback can be used to restrict the walk to a subset of tools
// (e.g. dynamic-only).
func outstandingToolCallIDs(ctx context.Context, store database.Store, chat database.Chat, accept func(toolName string) bool) (map[string]string, error) {
	lastAssistant, err := store.GetLastChatMessageByRole(ctx, database.GetLastChatMessageByRoleParams{
		ChatID: chat.ID,
		Role:   database.ChatMessageRoleAssistant,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return map[string]string{}, nil
		}
		return nil, xerrors.Errorf("get last assistant: %w", err)
	}
	parts, err := chatprompt.ParseContent(lastAssistant)
	if err != nil {
		return nil, xerrors.Errorf("parse assistant: %w", err)
	}
	afterMsgs, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: lastAssistant.ID,
	})
	if err != nil {
		return nil, xerrors.Errorf("get messages after assistant: %w", err)
	}
	handled := make(map[string]bool)
	// Provider-executed tool results are persisted inside the
	// assistant message itself; count them as handled.
	for _, p := range parts {
		if p.Type == codersdk.ChatMessagePartTypeToolResult {
			handled[p.ToolCallID] = true
		}
	}
	for _, msg := range afterMsgs {
		if msg.Role != database.ChatMessageRoleTool {
			continue
		}
		messageParts, err := chatprompt.ParseContent(msg)
		if err != nil {
			continue
		}
		for _, p := range messageParts {
			if p.Type == codersdk.ChatMessagePartTypeToolResult {
				handled[p.ToolCallID] = true
			}
		}
	}
	out := make(map[string]string)
	for _, p := range parts {
		if p.Type != codersdk.ChatMessagePartTypeToolCall {
			continue
		}
		// Provider-executed tool calls are answered server-side by the
		// LLM provider and must never be reported as outstanding.
		if p.ProviderExecuted {
			continue
		}
		if !accept(p.ToolName) {
			continue
		}
		if handled[p.ToolCallID] {
			continue
		}
		out[p.ToolCallID] = p.ToolName
	}
	return out, nil
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

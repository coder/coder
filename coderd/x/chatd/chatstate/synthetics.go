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
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
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
	pending := make([]codersdk.ChatMessagePart, 0)
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
		pending = append(pending, part)
	}
	if len(pending) == 0 {
		return nil, nil
	}
	toolCallIDs := make([]string, 0, len(pending))
	for _, part := range pending {
		toolCallIDs = append(toolCallIDs, part.ToolCallID)
	}
	mappedByCallID, err := mapCanceledExecutions(ctx, store, chat.ID, lastAssistant.ID, toolCallIDs, true)
	if err != nil {
		return nil, err
	}
	out := make([]Message, 0, len(pending))
	for _, part := range pending {
		resultPart := codersdk.ChatMessagePart{
			Type:       codersdk.ChatMessagePartTypeToolResult,
			ToolCallID: part.ToolCallID,
			ToolName:   part.ToolName,
			Result:     json.RawMessage(fmt.Sprintf("%q", reason)),
			IsError:    true,
		}
		if payload, spared, err := SparedBackgroundResult(mappedByCallID[part.ToolCallID]); err != nil {
			return nil, err
		} else if spared {
			resultPart.Result = payload
			resultPart.IsError = false
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
	return out, nil
}

// mapCanceledExecutions maps the ledger rows of tool calls a
// cancellation transition is resolving, mirroring the interrupt
// commit: the mapping and the result-committed stamp run in the same
// transaction as the chat commit. Rows the mapping routes to
// cancel_requested are picked up by the worker's periodic execution
// sweep, which kills their processes; without this every
// non-interrupt cancellation would orphan a live process and leave
// an uncommitted ledger row no purge can reap. spareBackground
// selects whether a background process with a recorded handle stays
// alive as detached; callers that commit no synthetic result must
// pass false, because a spared handle would have no carrier. Returns
// the mapped rows keyed by tool-call ID so spared background
// processes keep their handle in the committed result.
func mapCanceledExecutions(
	ctx context.Context,
	store database.Store,
	chatID uuid.UUID,
	assistantMessageID int64,
	toolCallIDs []string,
	spareBackground bool,
) (map[string]database.ChatToolCallExecution, error) {
	now := dbtime.Now()
	mapped, err := store.MarkChatToolCallExecutionsInterrupted(ctx, database.MarkChatToolCallExecutionsInterruptedParams{
		ChatID:             chatID,
		AssistantMessageID: assistantMessageID,
		ToolCallIds:        toolCallIDs,
		SpareBackground:    spareBackground,
		UpdatedAt:          now,
	})
	if err != nil {
		return nil, xerrors.Errorf("mark tool call executions interrupted: %w", err)
	}
	if err := store.MarkChatToolCallExecutionsResultCommitted(ctx, database.MarkChatToolCallExecutionsResultCommittedParams{
		ChatID:             chatID,
		AssistantMessageID: assistantMessageID,
		ToolCallIds:        toolCallIDs,
		ResultCommittedAt:  now,
	}); err != nil {
		return nil, xerrors.Errorf("mark tool call executions result committed: %w", err)
	}
	out := make(map[string]database.ChatToolCallExecution, len(mapped))
	for _, row := range mapped {
		out[row.ToolCallID] = row
	}
	return out, nil
}

// SparedBackgroundResult returns the synthetic tool result payload
// for a mapped ledger row spared as a detached background process,
// and reports whether the row is such a spare. It is the single
// spare-detection predicate for every cancellation path, so the
// spared-row status cannot drift between consumers.
func SparedBackgroundResult(row database.ChatToolCallExecution) (json.RawMessage, bool, error) {
	if row.Status != database.ChatToolCallExecutionStatusDetached || !row.ProcessID.Valid {
		return nil, false, nil
	}
	payload, err := chattool.SparedBackgroundCancellationResult(row.ProcessID.String)
	if err != nil {
		return nil, false, err
	}
	return payload, true, nil
}

// mapExecutionsForHistoryDelete maps the execution ledger rows whose
// processes an edit would otherwise orphan. EditMessage calls it
// before soft-deleting the history suffix starting at
// fromMessageID: after the delete the assistant message is invisible
// to the pending scan, so mapping afterwards would orphan a live
// process behind an uncommitted row no sweep can claim.
//
// Two kinds of rows lose their process carrier to the delete:
//
//   - Outstanding calls on the last assistant message. The deleted
//     turn gets no synthetic history results, so nothing can carry a
//     background handle back to the user and backgrounds are not
//     spared: every dispatched row becomes cancel_requested.
//   - Process-bearing rows (running or detached) anchored in the
//     deleted suffix: background starts, timed-out foregrounds,
//     interrupt-spared backgrounds, and rows whose best-effort
//     detach write failed. Their committed results were the only
//     carriers of their process handles and are deleted with the
//     suffix, so they are routed to cancel_requested as well, with
//     result_committed_at re-stamped so the sweep's give-up clock
//     starts at the edit.
//
// Both arms leave a fresh give-up anchor, through complementary
// mechanisms: arm 1's MarkChatToolCallExecutionsResultCommitted
// stamps only NULL result_committed_at, which every outstanding row
// has, while arm 2 re-stamps unconditionally because its rows
// committed results earlier. Consolidating the arms or dropping arm
// 1's stamp would reopen the zero-kill-budget orphan for an
// outstanding row claimed longer ago than the sweep's give-up bound.
//
// The sweep kills every cancel_requested row's process.
func mapExecutionsForHistoryDelete(ctx context.Context, store database.Store, chat database.Chat, fromMessageID int64) error {
	assistantMessageID, pending, err := pendingAllToolCallIDs(ctx, store, chat)
	if err != nil {
		return err
	}
	if len(pending) > 0 {
		toolCallIDs := make([]string, 0, len(pending))
		for id := range pending {
			toolCallIDs = append(toolCallIDs, id)
		}
		if _, err := mapCanceledExecutions(ctx, store, chat.ID, assistantMessageID, toolCallIDs, false); err != nil {
			return err
		}
	}
	_, err = store.MarkChatToolCallExecutionsCancelRequestedForHistoryDelete(ctx, database.MarkChatToolCallExecutionsCancelRequestedForHistoryDeleteParams{
		ChatID:        chat.ID,
		FromMessageID: fromMessageID,
		UpdatedAt:     dbtime.Now(),
	})
	if err != nil {
		return xerrors.Errorf("mark executions cancel requested for history delete: %w", err)
	}
	return nil
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
	_, pending, err := outstandingToolCallIDs(ctx, store, chat, func(toolName string) bool {
		return dynamic[toolName]
	})
	return pending, err
}

// pendingAllToolCallIDs returns the tool-call IDs of every outstanding
// tool call on the chat's last assistant message, regardless of
// whether the tool is dynamic, along with that assistant message's ID.
// The returned map is keyed by tool-call ID and valued by tool name.
// Callers that must guarantee a valid LLM message history (e.g. before
// promoting a user message into active history, or after committing an
// interruption's partial messages) should use this variant so
// non-dynamic tool calls do not silently bypass the check.
func pendingAllToolCallIDs(ctx context.Context, store database.Store, chat database.Chat) (int64, map[string]string, error) {
	return outstandingToolCallIDs(ctx, store, chat, func(string) bool { return true })
}

// outstandingToolCallIDs walks the chat's last assistant message and
// returns its ID plus the subset of its tool calls that have no
// matching tool-result message in the active history after it. The
// accept callback can be used to restrict the walk to a subset of
// tools (e.g. dynamic-only).
func outstandingToolCallIDs(ctx context.Context, store database.Store, chat database.Chat, accept func(toolName string) bool) (int64, map[string]string, error) {
	lastAssistant, err := store.GetLastChatMessageByRole(ctx, database.GetLastChatMessageByRoleParams{
		ChatID: chat.ID,
		Role:   database.ChatMessageRoleAssistant,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, map[string]string{}, nil
		}
		return 0, nil, xerrors.Errorf("get last assistant: %w", err)
	}
	parts, err := chatprompt.ParseContent(lastAssistant)
	if err != nil {
		return 0, nil, xerrors.Errorf("parse assistant: %w", err)
	}
	afterMsgs, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID:  chat.ID,
		AfterID: lastAssistant.ID,
	})
	if err != nil {
		return 0, nil, xerrors.Errorf("get messages after assistant: %w", err)
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
	return lastAssistant.ID, out, nil
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

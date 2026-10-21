package chatstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// ErrStructuredOutputBusy rejects a structured output request for a chat
// that cannot start its turn at once.
var ErrStructuredOutputBusy = xerrors.New("structured output requires an idle chat")

// RejectedCallLimit is the number of rejected finalizer calls that fails a
// structured output request without a validated value.
const RejectedCallLimit = 3

// isPrompt reports whether msg is a client-visible user message, the kind
// of message that starts a turn. Hook context, compaction copies and other
// model-only rows are not prompts.
func isPrompt(msg Message) bool {
	return msg.Role == database.ChatMessageRoleUser &&
		(msg.Visibility == database.ChatMessageVisibilityUser || msg.Visibility == database.ChatMessageVisibilityBoth)
}

// insertStructuredOutput inserts the pending result of the last user
// message in inserted when request is set.
func insertStructuredOutput(ctx context.Context, store database.Store, chatID uuid.UUID, inserted []database.ChatMessage, request json.RawMessage) error {
	if request == nil {
		return nil
	}
	for i := len(inserted) - 1; i >= 0; i-- {
		if inserted[i].Role != database.ChatMessageRoleUser {
			continue
		}
		if _, err := store.InsertChatStructuredOutput(ctx, database.InsertChatStructuredOutputParams{
			MessageID: inserted[i].ID, ChatID: chatID, Request: request,
		}); err != nil {
			return xerrors.Errorf("insert structured output: %w", err)
		}
		return nil
	}
	return xerrors.New("structured output request has no user message")
}

// closeEndedStructuredOutput closes the chat's pending structured output
// result when the transition ended its turn: the chat is waiting or in
// error, or a newer user message started a turn. A validated value
// succeeds, also when the turn then failed. Otherwise the code says how
// the turn ended. Every transition passes here, so no exit path closes a
// result itself. It queries only when the turn may have ended.
func (tx *Tx) closeEndedStructuredOutput(chat database.Chat) error {
	ended := chat.Status == database.ChatStatusWaiting || chat.Status == database.ChatStatusError
	if !ended && !tx.userPrompted {
		return nil
	}
	pending, err := tx.store.GetPendingChatStructuredOutput(tx.ctx, tx.chatID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return xerrors.Errorf("get pending structured output: %w", err)
	}
	if !ended {
		newer, err := tx.store.HasChatUserMessageAfter(tx.ctx, database.HasChatUserMessageAfterParams{ChatID: tx.chatID, AfterID: pending.MessageID})
		if err != nil {
			return xerrors.Errorf("check newer user message: %w", err)
		}
		if !newer {
			return nil
		}
	}
	status, code := database.ChatStructuredOutputStatusSucceeded, codersdk.ChatStructuredOutputErrorCode("")
	switch {
	case pending.Value.Valid:
	case chat.Status == database.ChatStatusError:
		status, code = database.ChatStructuredOutputStatusFailed, codersdk.ChatStructuredOutputErrorCodeGenerationFailed
	case tx.turnFinished:
		status, code = database.ChatStructuredOutputStatusFailed, codersdk.ChatStructuredOutputErrorCodeNotProduced
	default:
		status, code = database.ChatStructuredOutputStatusFailed, codersdk.ChatStructuredOutputErrorCodeInterrupted
	}
	return UpdateStructuredOutput(tx.ctx, tx.store, pending, status, code)
}

// UpdateStructuredOutput writes status and code to a pending result,
// keeping its value and rejected call count. A result that is no longer
// pending is left unchanged.
func UpdateStructuredOutput(ctx context.Context, store database.Store, pending database.ChatStructuredOutput, status database.ChatStructuredOutputStatus, code codersdk.ChatStructuredOutputErrorCode) error {
	_, err := store.UpdatePendingChatStructuredOutput(ctx, database.UpdatePendingChatStructuredOutputParams{
		Status:        status,
		ErrorCode:     sql.NullString{String: string(code), Valid: code != ""},
		Value:         pending.Value,
		RejectedCalls: pending.RejectedCalls,
		ChatID:        pending.ChatID,
		MessageID:     pending.MessageID,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return xerrors.Errorf("update structured output: %w", err)
	}
	return nil
}

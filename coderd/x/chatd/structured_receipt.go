package chatd

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/codersdk"
)

// Fixed receipt messages for requests closed without an output.
const (
	interruptedStructuredOutputMessage = "The turn was interrupted before it produced the structured output."
	reconciledStructuredOutputMessage  = "The chat was reconciled from an invalid state before it produced the structured output."
)

// activeRequestReceipt returns the receipt that closes the open structured
// output request of history's latest user turn with out's status and error,
// or none when that turn has no request or it is already closed. History
// that cannot be reconstructed gets no receipt and only a warning, so the
// caller's transition still completes. Run it inside the ChatMachine.Update
// that commits the transition, with history read under the same lock.
func activeRequestReceipt(ctx context.Context, logger slog.Logger, store database.Store, chatID uuid.UUID, history []database.ChatMessage, out codersdk.ChatStructuredOutput) ([]chatstate.Message, error) {
	rows := make([]chatstructured.Row, 0, len(history))
	for _, msg := range history {
		parts, err := chatprompt.ParseContent(msg)
		if err != nil {
			logger.Warn(ctx, "skip closing structured output request: unreadable message",
				slog.F("chat_id", chatID), slog.F("message_id", msg.ID))
			return nil, nil
		}
		rows = append(rows, chatstructured.Row{
			ID: msg.ID, Role: codersdk.ChatMessageRole(msg.Role), Visibility: chatstructured.Visibility(msg.Visibility), Parts: parts,
		})
	}
	state, err := chatstructured.ActiveRequest(rows)
	if err != nil {
		logger.Warn(ctx, "skip closing structured output request: inconsistent history", slog.F("chat_id", chatID))
		return nil, nil
	}
	if !state.Active || state.Closed {
		return nil, nil
	}
	out.RequestID = state.Request.RequestID
	return structuredReceiptMessages(ctx, store, chatID, state.RequestRowID, out)
}

// structuredReceiptMessages returns the terminal receipt message for out, or
// none when the history after afterID (the request row) already holds a
// receipt for the same request. Run it inside the ChatMachine.Update that
// commits the terminal transition: the check and the insert then share the
// chat lock, and a failed fence rolls both back.
func structuredReceiptMessages(ctx context.Context, store database.Store, chatID uuid.UUID, afterID int64, out codersdk.ChatStructuredOutput) ([]chatstate.Message, error) {
	history, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chatID, AfterID: afterID})
	if err != nil {
		return nil, xerrors.Errorf("get history for structured output receipt: %w", err)
	}
	for _, msg := range history {
		// A malformed receipt is never this request's receipt.
		if existing, err := chatstate.ReceiptRowOutcome(msg); err == nil && existing.RequestID == out.RequestID {
			return nil, nil
		}
	}
	parts, err := chatstructured.ReceiptParts(out)
	if err != nil {
		return nil, xerrors.Errorf("build structured output receipt: %w", err)
	}
	content, err := chatprompt.MarshalParts(parts)
	if err != nil {
		return nil, xerrors.Errorf("marshal structured output receipt: %w", err)
	}
	return []chatstate.Message{{
		Role:           database.ChatMessageRoleAssistant,
		Visibility:     database.ChatMessageVisibilityUser,
		ContentVersion: chatprompt.ContentVersionV1,
		Content:        content,
	}}, nil
}

package chatd

import (
	"bytes"
	"cmp"
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
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
	interruptedStructuredOutputMessage  = "The turn was interrupted before it produced the structured output."
	reconciledStructuredOutputMessage   = "The chat was reconciled from an invalid state before it produced the structured output."
	queueDeletedStructuredOutputMessage = "The queued message was deleted before it produced the structured output."
	supersededStructuredOutputMessage   = "An edit discarded the message before it produced the structured output."
)

// activeRequestReceipt returns the receipt that closes the open structured
// output request of history's latest user turn with out's status and error,
// or none when that turn has no request or it is already closed. History
// that cannot be reconstructed gets no receipt and only a warning, so the
// caller's transition still completes. Run it inside the ChatMachine.Update
// that commits the transition, with history read under the same lock.
func activeRequestReceipt(ctx context.Context, logger slog.Logger, store database.Store, chatID uuid.UUID, history []database.ChatMessage, out codersdk.ChatStructuredOutput) ([]chatstate.Message, error) {
	state, open := openStructuredRequest(ctx, logger, chatID, history)
	if !open {
		return nil, nil
	}
	out.RequestID = state.Request.RequestID
	return structuredReceiptMessages(ctx, store, chatID, state.RequestRowID, out)
}

// openStructuredRequest reconstructs the structured output request of
// history's latest user turn and reports whether it is open. History that
// cannot be reconstructed logs a warning and counts as no open request.
func openStructuredRequest(ctx context.Context, logger slog.Logger, chatID uuid.UUID, history []database.ChatMessage) (chatstructured.ActiveRequestState, bool) {
	// Only user rows carry requests; ordinary chats skip parsing entirely.
	if !slices.ContainsFunc(history, func(msg database.ChatMessage) bool {
		return msg.Role == database.ChatMessageRoleUser &&
			bytes.Contains(msg.Content.RawMessage, []byte(codersdk.ChatMessagePartTypeStructuredOutputRequest))
	}) {
		return chatstructured.ActiveRequestState{}, false
	}
	rows := make([]chatstructured.Row, 0, len(history))
	for _, msg := range history {
		parts, err := chatprompt.ParseContent(msg)
		if err != nil {
			logger.Warn(ctx, "ignore structured output request: unreadable message",
				slog.F("chat_id", chatID), slog.F("message_id", msg.ID))
			return chatstructured.ActiveRequestState{}, false
		}
		rows = append(rows, chatstructured.Row{
			ID: msg.ID, Role: codersdk.ChatMessageRole(msg.Role), Visibility: chatstructured.Visibility(msg.Visibility), Parts: parts,
		})
	}
	state, err := chatstructured.ActiveRequest(rows)
	if err != nil {
		logger.Warn(ctx, "ignore structured output request: inconsistent history", slog.F("chat_id", chatID))
		return chatstructured.ActiveRequestState{}, false
	}
	return state, state.Active && !state.Closed
}

// queueDeletedRequestReceipts returns the receipt that cancels the
// structured output request of a queued row being deleted, or none for an
// ordinary row or one already closed.
func queueDeletedRequestReceipts(ctx context.Context, logger slog.Logger, store database.Store, chatID uuid.UUID, queued database.ChatQueuedMessage) ([]chatstate.Message, error) {
	id, ok := structuredRequestID(ctx, logger, chatID, queuedRow(queued))
	if !ok {
		return nil, nil
	}
	return structuredReceiptMessages(ctx, store, chatID, 0, canceledStructuredOutput(id, codersdk.ChatStructuredOutputErrorCodeQueueDeleted, queueDeletedStructuredOutputMessage))
}

// supersededRequestReceipts returns receipts that cancel every pending
// structured output request an edit discards: requests in discarded user
// rows in history order, then in queued rows in queue order. Requests
// already closed by a receipt among the discarded rows get none.
func supersededRequestReceipts(ctx context.Context, logger slog.Logger, chatID uuid.UUID, discarded []database.ChatMessage, queued []database.ChatQueuedMessage) ([]chatstate.Message, error) {
	closed := make(map[uuid.UUID]bool)
	rows := make([]database.ChatMessage, 0, len(discarded)+len(queued))
	for _, msg := range discarded {
		if out, err := chatstate.ReceiptRowOutcome(msg); err == nil {
			closed[out.RequestID] = true
		} else if msg.Role == database.ChatMessageRoleUser {
			rows = append(rows, msg)
		}
	}
	for _, q := range queued {
		rows = append(rows, queuedRow(q))
	}
	var receipts []chatstate.Message
	for _, row := range rows {
		id, ok := structuredRequestID(ctx, logger, chatID, row)
		if !ok || closed[id] {
			continue
		}
		closed[id] = true
		receipt, err := receiptMessage(canceledStructuredOutput(id, codersdk.ChatStructuredOutputErrorCodeSuperseded, supersededStructuredOutputMessage))
		if err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	return receipts, nil
}

// structuredRequestID returns the request ID of msg's structured output
// request part. Unreadable content, an undecodable part or more than one
// request part is corrupt: it logs a warning and reports no request.
func structuredRequestID(ctx context.Context, logger slog.Logger, chatID uuid.UUID, msg database.ChatMessage) (uuid.UUID, bool) {
	parts, err := chatprompt.ParseContent(msg)
	var ids []uuid.UUID
	for _, part := range parts {
		if part.Type != codersdk.ChatMessagePartTypeStructuredOutputRequest {
			continue
		}
		req, decodeErr := chatstructured.DecodeRequestPart(part)
		err = cmp.Or(err, decodeErr)
		ids = append(ids, req.RequestID)
	}
	if err != nil || len(ids) > 1 {
		logger.Warn(ctx, "skip structured output receipt: inconsistent request metadata", slog.F("chat_id", chatID))
		return uuid.Nil, false
	}
	if len(ids) == 0 {
		return uuid.Nil, false
	}
	return ids[0], true
}

// queuedRow views a queued message as the user row it would become.
func queuedRow(q database.ChatQueuedMessage) database.ChatMessage {
	return database.ChatMessage{
		Role: database.ChatMessageRoleUser, Visibility: database.ChatMessageVisibilityBoth,
		Content: pqtype.NullRawMessage{RawMessage: q.Content, Valid: q.Content != nil}, ContentVersion: chatprompt.CurrentContentVersion,
	}
}

func canceledStructuredOutput(id uuid.UUID, code codersdk.ChatStructuredOutputErrorCode, message string) codersdk.ChatStructuredOutput {
	return codersdk.ChatStructuredOutput{
		RequestID: id, Status: codersdk.ChatStructuredOutputStatusCanceled,
		Error: &codersdk.ChatStructuredOutputError{Code: code, Message: message},
	}
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
	receipt, err := receiptMessage(out)
	if err != nil {
		return nil, err
	}
	return []chatstate.Message{receipt}, nil
}

// receiptMessage builds the receipt row for out.
func receiptMessage(out codersdk.ChatStructuredOutput) (chatstate.Message, error) {
	parts, err := chatstructured.ReceiptParts(out)
	if err != nil {
		return chatstate.Message{}, xerrors.Errorf("build structured output receipt: %w", err)
	}
	content, err := chatprompt.MarshalParts(parts)
	if err != nil {
		return chatstate.Message{}, xerrors.Errorf("marshal structured output receipt: %w", err)
	}
	return chatstate.Message{
		Role:           database.ChatMessageRoleAssistant,
		Visibility:     database.ChatMessageVisibilityUser,
		ContentVersion: chatprompt.ContentVersionV1,
		Content:        content,
	}, nil
}

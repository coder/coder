package chatd

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/codersdk"
)

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

package chatd

import (
	"context"

	"github.com/sqlc-dev/pqtype"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// WaitUntilIdleForTest waits for background chat work tracked by the server to
// finish without shutting the server down. Tests use this to assert final
// database state only after asynchronous chat processing has completed.
// Close waits for the same tracked work, but also stops the server.
func WaitUntilIdleForTest(server *Server) {
	server.drainInflight()
}

// FinishActiveChatForTest exposes the unexported cleanup TX so tests
// can drive the post-run state machine deterministically. Returns the
// resulting chat, the promoted message (if any), the synthetic
// tool-result rows the cleanup TX inserted (if any), and the cleanup
// error. The lastError string is encoded into a structured payload
// the same way runChat does, so callers do not need to know about
// the structured-error wrapper.
func FinishActiveChatForTest(
	ctx context.Context,
	server *Server,
	chat database.Chat,
	status database.ChatStatus,
	lastError string,
) (database.Chat, *database.ChatMessage, []database.ChatMessage, error) {
	logger := server.logger.With(slog.F("chat_id", chat.ID))
	var encoded pqtype.NullRawMessage
	if lastError != "" {
		var err error
		encoded, err = encodeChatLastErrorPayload(&codersdk.ChatError{
			Message: lastError,
		})
		if err != nil {
			return database.Chat{}, nil, nil, err
		}
	}
	result, err := server.finishActiveChat(ctx, logger, chat, status, encoded)
	if err != nil {
		return database.Chat{}, nil, nil, err
	}
	return result.updatedChat, result.promotedMessage, result.syntheticToolResults, nil
}

// RecoverStaleChatsForTest exposes the unexported stale-recovery loop
// so tests can assert the recovery state machine without waiting for
// the periodic ticker.
func RecoverStaleChatsForTest(ctx context.Context, server *Server) {
	server.recoverStaleChats(ctx)
}

// InsertSyntheticToolResultsTxForTest exposes the unexported helper
// so tests can verify the dedup path against pre-existing tool
// results.
func InsertSyntheticToolResultsTxForTest(
	ctx context.Context,
	store database.Store,
	chat database.Chat,
	reason string,
) ([]database.ChatMessage, error) {
	return insertSyntheticToolResultsTx(ctx, store, chat, reason)
}

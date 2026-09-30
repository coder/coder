package chatd

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// TestStreamLoopLatePartAfterRetry checks that parts of a failed
// generation attempt are dropped once the stream has announced a retry
// for it. The frontend clears its preview on the retry event
// (useChatStore.ts), so a late part of the failed attempt would clear the
// retry banner and show a preview that starts mid-message.
func TestStreamLoopLatePartAfterRetry(t *testing.T) {
	t.Parallel()

	// startFailedAttempt streams part 1 of attempt 1 and then records a
	// retry for that attempt, returning the loop and the last snapshot.
	startFailedAttempt := func(t *testing.T) (*streamLoop, database.Chat) {
		t.Helper()

		chatID := uuid.New()
		worker := uuid.NullUUID{UUID: uuid.New(), Valid: true}
		loop := newStreamLoop(database.Chat{ID: chatID}, nil, slogtest.Make(t, nil), 0)

		// Attempt 1 of history version 1 is running on the worker.
		events := loop.applyDBSnapshot(streamDBSnapshot{chat: database.Chat{
			ID:                chatID,
			Status:            database.ChatStatusRunning,
			SnapshotVersion:   3,
			HistoryVersion:    1,
			GenerationAttempt: 1,
			WorkerID:          worker,
		}})
		require.Contains(t, eventTypes(events), codersdk.ChatStreamEventTypePreviewReset)

		// The relay delivers the first part of attempt 1.
		_, accepted, err := loop.part(StreamPart{HistoryVersion: 1, GenerationAttempt: 1, Seq: 1, Part: codersdk.ChatMessageText("Hel")})
		require.NoError(t, err)
		require.True(t, accepted)

		// Attempt 1 fails and the worker records a retry for the same
		// attempt (RecordRetryState changes only retry_state and its
		// version).
		retry, err := json.Marshal(codersdk.ChatStreamRetry{Attempt: 1, DelayMs: 1000, Error: "rate limited", RetryingAt: time.Now()})
		require.NoError(t, err)
		chat := database.Chat{
			ID:                chatID,
			Status:            database.ChatStatusRunning,
			SnapshotVersion:   4,
			HistoryVersion:    1,
			GenerationAttempt: 1,
			RetryStateVersion: 4,
			RetryState:        pqtype.NullRawMessage{RawMessage: retry, Valid: true},
			WorkerID:          worker,
		}
		events = loop.applyDBSnapshot(streamDBSnapshot{chat: chat})
		require.Equal(t, []codersdk.ChatStreamEventType{codersdk.ChatStreamEventTypeRetry}, eventTypes(events))

		// A part produced before the failure is still buffered in the
		// relay. It must not be delivered after the retry event.
		_, accepted, err = loop.part(StreamPart{HistoryVersion: 1, GenerationAttempt: 1, Seq: 2, Part: codersdk.ChatMessageText("lo")})
		require.NoError(t, err)
		require.False(t, accepted, "part 2 of the failed attempt was delivered after the retry event, which cleared the client preview")
		return loop, chat
	}

	t.Run("NextAttemptStreams", func(t *testing.T) {
		t.Parallel()

		loop, chat := startFailedAttempt(t)

		// The retry starts attempt 2, which clears retry_state.
		chat.SnapshotVersion = 5
		chat.GenerationAttempt = 2
		chat.RetryStateVersion = 5
		chat.RetryState = pqtype.NullRawMessage{}
		events := loop.applyDBSnapshot(streamDBSnapshot{chat: chat})
		require.Equal(t, []codersdk.ChatStreamEventType{codersdk.ChatStreamEventTypePreviewReset}, eventTypes(events))

		_, accepted, err := loop.part(StreamPart{HistoryVersion: 1, GenerationAttempt: 1, Seq: 3, Part: codersdk.ChatMessageText("!")})
		require.NoError(t, err)
		require.False(t, accepted, "attempt 1 is superseded")

		_, accepted, err = loop.part(StreamPart{HistoryVersion: 1, GenerationAttempt: 2, Seq: 1, Part: codersdk.ChatMessageText("Hello")})
		require.NoError(t, err)
		require.True(t, accepted, "the new attempt starts a fresh preview")
	})

	t.Run("RetryCancelledKeepsAttemptRetired", func(t *testing.T) {
		t.Parallel()

		loop, chat := startFailedAttempt(t)

		// An interrupt during the retry backoff clears the pending retry
		// but keeps the history version and generation attempt.
		chat.SnapshotVersion = 5
		chat.Status = database.ChatStatusInterrupting
		chat.RetryStateVersion = 5
		chat.RetryState = pqtype.NullRawMessage{}
		events := loop.applyDBSnapshot(streamDBSnapshot{chat: chat})
		require.Equal(t, []codersdk.ChatStreamEventType{codersdk.ChatStreamEventTypeStatus}, eventTypes(events))

		_, accepted, err := loop.part(StreamPart{HistoryVersion: 1, GenerationAttempt: 1, Seq: 2, Part: codersdk.ChatMessageText("lo")})
		require.NoError(t, err)
		require.False(t, accepted, "canceling the retry does not revive the failed attempt")
	})
}

func eventTypes(events []codersdk.ChatStreamEvent) []codersdk.ChatStreamEventType {
	types := make([]codersdk.ChatStreamEventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

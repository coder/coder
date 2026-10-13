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
		loop := newStreamLoop(database.Chat{ID: chatID}, nil, slogtest.Make(t, nil), StreamCursor{})

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

// TestStreamLoopLatePartAfterClearedRetry covers a retry that is recorded
// and cleared again (for example by an interrupt during the backoff)
// before the stream loop syncs. The loop never sees the payload, but the
// advanced retry_state_version within the same episode still proves that
// the current attempt failed.
func TestStreamLoopLatePartAfterClearedRetry(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	worker := uuid.NullUUID{UUID: uuid.New(), Valid: true}
	loop := newStreamLoop(database.Chat{ID: chatID}, nil, slogtest.Make(t, nil), StreamCursor{})
	chat := database.Chat{
		ID:                chatID,
		Status:            database.ChatStatusRunning,
		SnapshotVersion:   3,
		HistoryVersion:    1,
		GenerationAttempt: 1,
		WorkerID:          worker,
	}
	loop.applyDBSnapshot(streamDBSnapshot{chat: chat})
	_, accepted, err := loop.part(StreamPart{HistoryVersion: 1, GenerationAttempt: 1, Seq: 1, Part: codersdk.ChatMessageText("Hel")})
	require.NoError(t, err)
	require.True(t, accepted)

	// RecordRetryState (snapshot 4) and Interrupt (snapshot 5) both commit
	// before the next sync, which only sees the cleared payload.
	chat.SnapshotVersion = 5
	chat.Status = database.ChatStatusInterrupting
	chat.RetryStateVersion = 5
	events := loop.applyDBSnapshot(streamDBSnapshot{chat: chat})
	require.Equal(t, []codersdk.ChatStreamEventType{codersdk.ChatStreamEventTypeStatus}, eventTypes(events))

	_, accepted, err = loop.part(StreamPart{HistoryVersion: 1, GenerationAttempt: 1, Seq: 2, Part: codersdk.ChatMessageText("lo")})
	require.NoError(t, err)
	require.False(t, accepted, "part 2 of the failed attempt was delivered after its retry was recorded")
}

func eventTypes(events []codersdk.ChatStreamEvent) []codersdk.ChatStreamEventType {
	types := make([]codersdk.ChatStreamEventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

// TestStreamLoopLatePartAfterError checks that parts of a generation
// attempt that failed with a terminal error are dropped once the stream
// has announced the error. The frontend clears its preview on the error
// event (useChatStore.ts), so a late part of the failed attempt would
// show a preview that starts mid-message under the error.
func TestStreamLoopLatePartAfterError(t *testing.T) {
	t.Parallel()

	// startAttempt streams part 1 of attempt 1 of history version 1 and
	// returns the loop and the last snapshot.
	startAttempt := func(t *testing.T) (*streamLoop, database.Chat) {
		t.Helper()

		chatID := uuid.New()
		worker := uuid.NullUUID{UUID: uuid.New(), Valid: true}
		loop := newStreamLoop(database.Chat{ID: chatID}, nil, slogtest.Make(t, nil), StreamCursor{})
		chat := database.Chat{
			ID:                chatID,
			Status:            database.ChatStatusRunning,
			SnapshotVersion:   3,
			HistoryVersion:    1,
			GenerationAttempt: 1,
			WorkerID:          worker,
		}
		events := loop.applyDBSnapshot(streamDBSnapshot{chat: chat})
		require.Contains(t, eventTypes(events), codersdk.ChatStreamEventTypePreviewReset)

		_, accepted, err := loop.part(StreamPart{HistoryVersion: 1, GenerationAttempt: 1, Seq: 1, Part: codersdk.ChatMessageText("Hel")})
		require.NoError(t, err)
		require.True(t, accepted)
		return loop, chat
	}

	// failAttempt applies a non-retryable failure of the current attempt.
	// FinishError changes only the status and last_error; the history
	// version and generation attempt stay.
	failAttempt := func(t *testing.T, loop *streamLoop, chat database.Chat) database.Chat {
		t.Helper()

		lastErr, err := json.Marshal(codersdk.ChatError{Message: "bad request", Kind: codersdk.ChatErrorKindGeneric})
		require.NoError(t, err)
		chat.SnapshotVersion++
		chat.Status = database.ChatStatusError
		chat.LastError = pqtype.NullRawMessage{RawMessage: lastErr, Valid: true}
		events := loop.applyDBSnapshot(streamDBSnapshot{chat: chat})
		require.Equal(t, []codersdk.ChatStreamEventType{
			codersdk.ChatStreamEventTypeStatus,
			codersdk.ChatStreamEventTypeError,
		}, eventTypes(events))
		return chat
	}

	t.Run("FailedAttemptDropped", func(t *testing.T) {
		t.Parallel()

		loop, chat := startAttempt(t)
		failAttempt(t, loop, chat)

		// A part produced before the failure is still buffered in the
		// relay. It must not be delivered after the error event.
		_, accepted, err := loop.part(StreamPart{HistoryVersion: 1, GenerationAttempt: 1, Seq: 2, Part: codersdk.ChatMessageText("lo")})
		require.NoError(t, err)
		require.False(t, accepted, "part 2 of the failed attempt was delivered after the error event, which cleared the client preview")
	})

	t.Run("NextAttemptStreams", func(t *testing.T) {
		t.Parallel()

		loop, chat := startAttempt(t)
		chat = failAttempt(t, loop, chat)

		// The chat runs again and records a new generation attempt.
		chat.SnapshotVersion++
		chat.Status = database.ChatStatusRunning
		chat.LastError = pqtype.NullRawMessage{}
		chat.GenerationAttempt = 2
		events := loop.applyDBSnapshot(streamDBSnapshot{chat: chat})
		require.Equal(t, []codersdk.ChatStreamEventType{
			codersdk.ChatStreamEventTypeStatus,
			codersdk.ChatStreamEventTypePreviewReset,
		}, eventTypes(events))

		_, accepted, err := loop.part(StreamPart{HistoryVersion: 1, GenerationAttempt: 2, Seq: 1, Part: codersdk.ChatMessageText("Hello")})
		require.NoError(t, err)
		require.True(t, accepted, "the new attempt starts a fresh preview")
	})

	t.Run("InterruptingKeepsStreaming", func(t *testing.T) {
		t.Parallel()

		loop, chat := startAttempt(t)

		// An interrupt does not end the attempt's preview: the client
		// keeps it until the worker persists the partial message.
		chat.SnapshotVersion++
		chat.Status = database.ChatStatusInterrupting
		events := loop.applyDBSnapshot(streamDBSnapshot{chat: chat})
		require.Equal(t, []codersdk.ChatStreamEventType{codersdk.ChatStreamEventTypeStatus}, eventTypes(events))

		_, accepted, err := loop.part(StreamPart{HistoryVersion: 1, GenerationAttempt: 1, Seq: 2, Part: codersdk.ChatMessageText("lo")})
		require.NoError(t, err)
		require.True(t, accepted)
	})
}

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

// TestStreamLoopLatePartAfterRetry checks that a retry recorded for a
// generation attempt sends preview_reset and that the attempt's later parts
// are dropped. A late part of the failed attempt would otherwise show a
// preview that starts mid-message.
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
		require.Equal(t, []codersdk.ChatStreamEventType{
			codersdk.ChatStreamEventTypeRetry,
			codersdk.ChatStreamEventTypePreviewReset,
			codersdk.ChatStreamEventTypeCheckpoint,
		}, eventTypes(events))

		// A part produced before the failure is still buffered in the
		// relay. It must not be delivered after the preview_reset.
		_, accepted, err = loop.part(StreamPart{HistoryVersion: 1, GenerationAttempt: 1, Seq: 2, Part: codersdk.ChatMessageText("lo")})
		require.NoError(t, err)
		require.False(t, accepted, "part 2 of the failed attempt was delivered after the preview_reset")
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
		require.Equal(t, []codersdk.ChatStreamEventType{codersdk.ChatStreamEventTypePreviewReset, codersdk.ChatStreamEventTypeCheckpoint}, eventTypes(events))

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
		require.Equal(t, []codersdk.ChatStreamEventType{codersdk.ChatStreamEventTypeStatus, codersdk.ChatStreamEventTypeCheckpoint}, eventTypes(events))

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
	require.Equal(t, []codersdk.ChatStreamEventType{
		codersdk.ChatStreamEventTypeStatus,
		codersdk.ChatStreamEventTypePreviewReset,
		codersdk.ChatStreamEventTypeCheckpoint,
	}, eventTypes(events))

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

// TestStreamLoopLatePartAfterError checks that a chat that stops running
// sends preview_reset and drops the later parts of its attempt. A late part
// would otherwise show a preview that starts mid-message under the error,
// or live output under a finished turn.
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
			codersdk.ChatStreamEventTypePreviewReset,
			codersdk.ChatStreamEventTypeCheckpoint,
		}, eventTypes(events))
		return chat
	}

	t.Run("FailedAttemptDropped", func(t *testing.T) {
		t.Parallel()

		loop, chat := startAttempt(t)
		failAttempt(t, loop, chat)

		// A part produced before the failure is still buffered in the
		// relay. It must not be delivered after the preview_reset.
		_, accepted, err := loop.part(StreamPart{HistoryVersion: 1, GenerationAttempt: 1, Seq: 2, Part: codersdk.ChatMessageText("lo")})
		require.NoError(t, err)
		require.False(t, accepted, "part 2 of the failed attempt was delivered after the preview_reset")
	})

	t.Run("TurnFinishedWithoutCommit", func(t *testing.T) {
		t.Parallel()

		// A model that returns no content finishes the turn without
		// committing a message, so the history version and generation
		// attempt stay while the status becomes waiting.
		loop, chat := startAttempt(t)
		chat.SnapshotVersion++
		chat.Status = database.ChatStatusWaiting
		events := loop.applyDBSnapshot(streamDBSnapshot{chat: chat})
		require.Equal(t, []codersdk.ChatStreamEventType{
			codersdk.ChatStreamEventTypeStatus,
			codersdk.ChatStreamEventTypePreviewReset,
			codersdk.ChatStreamEventTypeCheckpoint,
		}, eventTypes(events))

		_, accepted, err := loop.part(StreamPart{HistoryVersion: 1, GenerationAttempt: 1, Seq: 2, Part: codersdk.ChatMessageText("lo")})
		require.NoError(t, err)
		require.False(t, accepted, "a part of the finished turn's attempt was delivered after the turn ended")
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
			codersdk.ChatStreamEventTypeCheckpoint,
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
		require.Equal(t, []codersdk.ChatStreamEventType{codersdk.ChatStreamEventTypeStatus, codersdk.ChatStreamEventTypeCheckpoint}, eventTypes(events))

		_, accepted, err := loop.part(StreamPart{HistoryVersion: 1, GenerationAttempt: 1, Seq: 2, Part: codersdk.ChatMessageText("lo")})
		require.NoError(t, err)
		require.True(t, accepted)
	})
}

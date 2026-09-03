package chatd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestSessionStartTrackerRetriesIncompleteDispatch(t *testing.T) {
	t.Parallel()
	tracker := &sessionStartTracker{}
	claimed, complete, err := tracker.claim(t.Context())
	require.NoError(t, err)
	require.True(t, claimed)

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err = tracker.claim(canceled)
	require.ErrorIs(t, err, context.Canceled)
	complete(false)

	claimed, complete, err = tracker.claim(t.Context())
	require.NoError(t, err)
	require.True(t, claimed)
	complete(true)
	claimed, _, err = tracker.claim(t.Context())
	require.NoError(t, err)
	require.False(t, claimed)
}

func TestSessionStartDispatchFailureFinishesGeneration(t *testing.T) {
	t.Parallel()
	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	chat = f.acquireChat(t, chat.ID, workerID, runnerID)
	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(consumer.Close)
	dispatcher := dispatch.New(
		slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
		consumer.Client(),
		consumer.URL,
		false,
		"test-hook-secret-32-bytes-minimum!!",
		time.Second,
		"test-deployment",
		"test-version",
		prometheus.NewRegistry(),
	)
	starter := newTestTaskStarter(t, f, newTaskSideEffectRecorder())
	starter.server.hooks = chathooks.NewTrigger(dispatcher)
	ctx := testutil.Context(t, testutil.WaitLong)
	debugTurn := newRunnerDebugTurn(ctx, starter.opts.Logger)
	defer debugTurn.Finalize(ctx)
	err := starter.StartGeneration(ctx, chatWorkerTaskStartInput{
		ChatID:            chat.ID,
		WorkerID:          workerID,
		RunnerID:          runnerID,
		HistoryVersion:    chat.HistoryVersion,
		GenerationAttempt: chat.GenerationAttempt,
		Status:            database.ChatStatusRunning,
		DebugTurn:         debugTurn,
		SessionStart:      &sessionStartTracker{},
	})
	require.NoError(t, err)
	updated, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusError, updated.Status)
	var chatErr codersdk.ChatError
	require.NoError(t, json.Unmarshal(updated.LastError.RawMessage, &chatErr))
	require.Equal(t, codersdk.ChatErrorKindHookDispatchFailed, chatErr.Kind)
	require.Contains(t, chatErr.Message, "hook dispatch failed: session_start: http_error (dispatch ")
	require.False(t, chatErr.Retryable)
}

func TestApplySessionStartResponse(t *testing.T) {
	t.Parallel()
	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	chat = f.acquireChat(t, chat.ID, workerID, runnerID)
	ctx := testutil.Context(t, testutil.WaitLong)
	input := chatWorkerTaskStartInput{
		ChatID:         chat.ID,
		WorkerID:       workerID,
		RunnerID:       runnerID,
		HistoryVersion: chat.HistoryVersion,
		Status:         database.ChatStatusRunning,
	}
	_, err := applySessionStartResponse(
		ctx,
		chatstate.NewChatMachine(f.db, f.pubsub, chat.ID),
		input,
		chat,
		&chathooks.Result{
			ModelContext: "model context",
			UserMessage:  "user notice",
		},
	)
	require.NoError(t, err)

	promptRows, err := f.db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, "model context", hookMessageTextInternal(t, promptRows[len(promptRows)-1]))
	require.Equal(t, database.ChatMessageVisibilityModel, promptRows[len(promptRows)-1].Visibility)
	allRows, err := f.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chat.ID})
	require.NoError(t, err)
	userNotice := allRows[len(allRows)-1]
	require.Equal(t, database.ChatMessageRoleSystem, userNotice.Role)
	require.Equal(t, database.ChatMessageVisibilityUser, userNotice.Visibility)
	require.Equal(t, "user notice", hookMessageTextInternal(t, userNotice))
}

func TestApplySessionStartResponseNoOp(t *testing.T) {
	t.Parallel()
	f := newTaskTestFixture(t)
	chat := f.createRunningChat(t)
	f.pubsub.clear()
	result, err := applySessionStartResponse(
		testutil.Context(t, testutil.WaitLong),
		chatstate.NewChatMachine(f.db, f.pubsub, chat.ID),
		chatWorkerTaskStartInput{},
		chat,
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, chat.SnapshotVersion, result.Chat.SnapshotVersion)
	require.Empty(t, f.pubsub.events())
}

func hookMessageTextInternal(t *testing.T, message database.ChatMessage) string {
	t.Helper()
	parts, err := chatprompt.ParseContent(message)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	return parts[0].Text
}

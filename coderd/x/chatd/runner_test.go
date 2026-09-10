package chatd //nolint:testpackage // Uses unexported chatworker helpers.

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestRunner_IgnoresDuplicateStateNotifications(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	startWorker(t, testOptions(t, f, starter))
	starter.waitCall(t, taskKindGeneration, chat.ID)
	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)

	publishChatUpdate(t, f, latest)
	publishChatUpdate(t, f, latest)
	starter.assertNoCall(t)
}

func TestRunner_CancelsActiveTaskWhenHistoryChanges(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	updated := commitAssistantStep(t, f, chat.ID, "first step")
	require.Greater(t, updated.HistoryVersion, first.input.HistoryVersion)
	requireTaskCanceled(t, first)
	require.NotErrorIs(t, context.Cause(first.ctx), errTaskTimeout)
	second := starter.waitCall(t, taskKindGeneration, chat.ID)
	require.Equal(t, updated.HistoryVersion, second.input.HistoryVersion)
	require.Same(t, first.input.SessionStart, second.input.SessionStart)
}

func TestRunner_CancelsActiveTaskWhenStatusChanges(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	updated := interruptChat(t, f, chat.ID)
	require.Equal(t, database.ChatStatusInterrupting, updated.Status)
	requireTaskCanceled(t, first)
	second := starter.waitCall(t, taskKindInterrupt, chat.ID)
	require.Equal(t, updated.HistoryVersion, second.input.HistoryVersion)
}

func TestRunner_CleansUpOnOwnershipTakeover(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	acquireChat(t, f, chat.ID, uuid.New(), uuid.New())
	requireTaskCanceled(t, first)
	starter.assertNoCall(t)
}

func TestRunner_SerializesReplacementTasksForSameHistoryAndStatus(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(true)
	defer starter.releaseAll()
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	forceExecutionStateAndPublish(t, f, chat.ID, database.ChatStatusInterrupting, false)
	starter.waitCall(t, taskKindInterrupt, chat.ID)
	forceExecutionStateAndPublish(t, f, chat.ID, database.ChatStatusRunning, false)
	starter.assertNoCall(t)

	starter.release(t, 0)
	replacement := starter.waitCall(t, taskKindGeneration, chat.ID)
	require.Equal(t, first.input.HistoryVersion, replacement.input.HistoryVersion)
}

func TestRunner_AllowsReplacementForDifferentHistoryOrStatus(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(true)
	defer starter.releaseAll()
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	updated := commitAssistantStep(t, f, chat.ID, "different history")
	second := starter.waitCall(t, taskKindGeneration, chat.ID)
	require.Greater(t, second.input.HistoryVersion, first.input.HistoryVersion)
	require.Equal(t, updated.HistoryVersion, second.input.HistoryVersion)
}

func TestRunner_TaskTimeoutRetries(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
	timeoutTrap := clock.Trap().AfterFunc("chatworker", "task-timeout-generation")
	starter := newBlockingTaskStarter(false)
	opts := testOptions(t, f, starter)
	opts.Clock = clock
	opts.TaskRetryInitialBackoff = time.Minute
	opts.TaskRetryMaxBackoff = time.Minute
	startWorker(t, opts)

	timeoutTrap.MustWait(testutil.Context(t, testutil.WaitLong)).MustRelease(testutil.Context(t, testutil.WaitLong))
	timeoutTrap.Close()
	first := starter.waitCall(t, taskKindGeneration, chat.ID)
	retryTrap := clock.Trap().NewTimer("chatworker", "task-retry-generation")
	defer retryTrap.Close()

	ctx := testutil.Context(t, testutil.WaitLong)
	clock.Advance(defaultTaskTimeout).MustWait(ctx)
	retryTrap.MustWait(ctx).MustRelease(ctx)
	require.ErrorIs(t, context.Cause(first.ctx), errTaskTimeout)
	clock.Advance(time.Minute).MustWait(ctx)
	second := starter.waitCall(t, taskKindGeneration, chat.ID)
	require.Equal(t, first.input.HistoryVersion, second.input.HistoryVersion)
}

func TestWorker_RoutesDatabaseSyncStateToActiveRunner(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
	starter := newBlockingTaskStarter(false)
	opts := testOptions(t, f, starter)
	opts.Clock = clock
	opts.RunnerSyncInterval = time.Minute
	startWorker(t, opts)
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	forceExecutionState(t, f, chat.ID, database.ChatStatusInterrupting, false)
	clock.Advance(time.Minute).MustWait(testutil.Context(t, testutil.WaitLong))
	requireTaskCanceled(t, first)
	starter.waitCall(t, taskKindInterrupt, chat.ID)
}

func TestWorker_CleanupStopsRoutingAndCancelsTasks(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	startWorker(t, testOptions(t, f, starter))
	first := starter.waitCall(t, taskKindGeneration, chat.ID)

	latest := acquireChat(t, f, chat.ID, uuid.New(), uuid.New())
	requireTaskCanceled(t, first)
	publishChatUpdate(t, f, latest)
	starter.assertNoCall(t)
}

// None of these cases publishes a state update: the task stops on its own,
// a snapshot bump arrives without its notification, or the chat_messages
// trigger changes history_version without a snapshot bump. The periodic sync
// alone must be enough to start the right task.
func TestRunner_SyncRestoresRequiredWork(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		releaseTask bool
		mutate      func(t *testing.T, f *workerTestFixture, chat database.Chat) database.Chat
	}{
		{
			name:        "UnchangedRow",
			releaseTask: true,
			mutate: func(_ *testing.T, _ *workerTestFixture, chat database.Chat) database.Chat {
				return chat
			},
		},
		{
			name:        "NewerSnapshotSameWork",
			releaseTask: true,
			mutate: func(t *testing.T, f *workerTestFixture, chat database.Chat) database.Chat {
				return forceExecutionState(t, f, chat.ID, database.ChatStatusRunning, false)
			},
		},
		{
			name:        "EqualSnapshotHistoryChange",
			releaseTask: true,
			mutate: func(t *testing.T, f *workerTestFixture, chat database.Chat) database.Chat {
				return editUserMessage(t, f.db, f.sqlDB, chat.ID, "edited")
			},
		},
		{
			name: "EqualSnapshotHistoryChangeWhileActive",
			mutate: func(t *testing.T, f *workerTestFixture, chat database.Chat) database.Chat {
				return editUserMessage(t, f.db, f.sqlDB, chat.ID, "edited")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newWorkerTestFixture(t)
			chat := f.createRunningChat(t)
			clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
			starter := newBlockingTaskStarter(false)
			opts := testOptions(t, f, starter)
			opts.Clock = clock
			opts.RunnerSyncInterval = time.Minute
			startWorker(t, opts)
			first := starter.waitCall(t, taskKindGeneration, chat.ID)
			if tc.releaseTask {
				starter.release(t, 0)
			}
			expected := tc.mutate(t, f, chat)
			starter.assertNoCall(t)

			// Each advance triggers one sync. The released task may not have
			// exited when the first sync runs, so advance until a task starts.
			var second taskCall
			testutil.Eventually(ctx, t, func(ctx context.Context) bool {
				clock.Advance(time.Minute).MustWait(ctx)
				select {
				case second = <-starter.callCh:
					return true
				default:
					return false
				}
			}, testutil.IntervalFast)
			if !tc.releaseTask {
				requireTaskCanceled(t, first)
			}
			require.Equal(t, taskKindGeneration, second.kind)
			require.Equal(t, expected.HistoryVersion, second.input.HistoryVersion)
			require.Equal(t, first.input.RunnerID, second.input.RunnerID)
			require.Same(t, first.input.SessionStart, second.input.SessionStart)
		})
	}
}

// Editing a chat_messages row with plain SQL, outside any transition, changes
// the history under a running generation. The generation refuses to commit its
// stale response and exits, and nothing publishes a state update. The periodic
// sync must then start a generation from the edited history.
func TestRunner_RealGenerationRecoversHistoryFence(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
	sink := testutil.NewFakeSink(t)
	firstRequest := make(chan struct{})
	releaseResponse := make(chan struct{})
	recoveredRequest := make(chan string, 1)
	var requests atomic.Int32
	providerURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		if !req.Stream {
			return chattest.OpenAINonStreamingResponse("history fence")
		}
		if requests.Add(1) == 1 {
			close(firstRequest)
			select {
			case <-releaseResponse:
			case <-req.Context().Done():
			}
			return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("stale response must not commit")...)
		}
		select {
		case recoveredRequest <- string(req.RawBody):
		case <-req.Context().Done():
		}
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("recovered response")...)
	})
	provider := dbgen.ChatProvider(t, f.db, database.ChatProvider{
		Provider: "openai-compat", DisplayName: "history fence", BaseUrl: providerURL,
	})
	model := dbgen.ChatModelConfig(t, f.db, database.ChatModelConfig{
		AIProviderID: uuid.NullUUID{UUID: provider.ID, Valid: true}, OrganizationID: f.org.ID,
	})
	server := New(f.pubsub, Config{
		Logger:                   sink.Logger(),
		Database:                 f.db,
		ReplicaID:                uuid.New(),
		Clock:                    clock,
		Experiments:              codersdk.ExperimentsKnown,
		AIBridgeTransportFactory: aibridgeTestFactoryPointer(chattest.NewMockAIBridgeTransport(t, providerURL)),
	})
	t.Cleanup(func() { require.NoError(t, server.Close()) })
	created, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID: f.org.ID, OwnerID: f.user.ID, ModelConfigID: model.ID,
		Title: "history fence", InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)
	server.Start()

	testutil.TryReceive(ctx, t, firstRequest)
	editUserMessage(t, f.db, f.sqlDB, created.ID, "hello after out-of-band edit")
	close(releaseResponse)

	// Wait for the exit log entry; taskExitedLogMessage documents why the log is
	// the only observable.
	var exits []slog.SinkEntry
	testutil.Eventually(ctx, t, func(context.Context) bool {
		exits = sink.Entries(func(e slog.SinkEntry) bool {
			return e.Message == taskExitedLogMessage &&
				sinkFieldValue(t, e.Fields, "chat_id") == created.ID.String()
		})
		return len(exits) > 0
	}, testutil.IntervalFast)
	require.Len(t, exits, 1)
	require.Equal(t, taskExitReasonExpectedNonRetryable, sinkFieldValue(t, exits[0].Fields, "reason"))
	require.Contains(t, sinkFieldValue(t, exits[0].Fields, "error"), "chat history version mismatch")

	// Fire the server's timers one at a time until the runner sync
	// redelivers the row and the runner asks the model again.
	var recovered string
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		_, waiter := clock.AdvanceNext()
		waiter.MustWait(ctx)
		select {
		case recovered = <-recoveredRequest:
			return true
		default:
			return false
		}
	}, testutil.IntervalFast)
	require.Contains(t, recovered, "hello after out-of-band edit")

	var final database.Chat
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		final, err = f.db.GetChatByID(ctx, created.ID)
		return err == nil && final.Status == database.ChatStatusWaiting && !final.WorkerID.Valid && !final.RunnerID.Valid
	}, testutil.IntervalFast)
	require.False(t, final.LastError.Valid)
	messages, err := f.db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: created.ID})
	require.NoError(t, err)
	var users, assistants int
	for _, message := range messages {
		switch message.Role {
		case database.ChatMessageRoleUser:
			users++
			require.Contains(t, string(message.Content.RawMessage), "hello after out-of-band edit")
		case database.ChatMessageRoleAssistant:
			assistants++
			require.Contains(t, string(message.Content.RawMessage), "recovered response")
		}
		require.NotContains(t, string(message.Content.RawMessage), "stale response must not commit")
	}
	require.Equal(t, 1, users)
	require.Equal(t, 1, assistants)
	require.Equal(t, int32(2), requests.Load())
}

package chatd //nolint:testpackage // Tests unexported chat worker internals.

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestWorker_NewRequiresTaskStarterOrMessagePartBuffer(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	_, err := newChatWorker(nil, chatWorkerOptions{WorkerID: uuid.New(), Store: f.db, Pubsub: f.pubsub})
	require.ErrorContains(t, err, "task starter or message part buffer is required")
}

func TestWorker_NewRequiresWorkerID(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	opts := testOptions(t, f, newRecordingTaskStarter())
	opts.WorkerID = uuid.Nil
	_, err := newChatWorker(nil, opts)
	require.ErrorContains(t, err, "worker ID is required")
}

func TestWorker_UsesConfiguredWorkerID(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	workerID := opts.WorkerID
	worker, err := newChatWorker(nil, opts)
	require.NoError(t, err)
	require.Equal(t, workerID, worker.chatWorkerID())
	require.NoError(t, worker.Start(context.Background()))
	require.Equal(t, workerID, worker.chatWorkerID())
	require.NoError(t, worker.Close())
}

func TestWorker_AcquiresRunnableChatFromOwnershipHint(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newRecordingTaskStarter()
	worker := startWorker(t, testOptions(t, f, starter))

	call := starter.waitCall(t, TaskKindGeneration, chat.ID)
	require.Equal(t, worker.chatWorkerID(), call.input.WorkerID)
	require.Equal(t, database.ChatStatusRunning, call.input.Status)
	require.NotEqual(t, uuid.Nil, call.input.RunnerID)

	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.Equal(t, worker.chatWorkerID(), latest.WorkerID.UUID)
	require.Equal(t, call.input.RunnerID, latest.RunnerID.UUID)
	_, err = f.db.GetChatHeartbeat(testutil.Context(t, testutil.WaitShort), database.GetChatHeartbeatParams{
		ChatID:   chat.ID,
		RunnerID: call.input.RunnerID,
	})
	require.NoError(t, err)
}

func TestWorker_AcquiresRequiresActionChatFromOwnershipHint(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRequiresActionChat(t)
	starter := newRecordingTaskStarter()
	startWorker(t, testOptions(t, f, starter))

	call := starter.waitCall(t, TaskKindRequiresActionTimeout, chat.ID)
	require.Equal(t, database.ChatStatusRequiresAction, call.input.Status)
	require.True(t, call.input.RequiresActionDeadlineAt.Valid)
}

func TestWorker_SkipsFreshlyOwnedChat(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	otherWorker := uuid.New()
	otherRunner := uuid.New()
	acquireChat(t, f, chat.ID, otherWorker, otherRunner)
	starter := newRecordingTaskStarter()
	worker := startWorker(t, testOptions(t, f, starter))
	worker.Wake()

	starter.assertNoCall(t)
	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.Equal(t, otherWorker, latest.WorkerID.UUID)
	require.Equal(t, otherRunner, latest.RunnerID.UUID)
}

func TestWorker_ReacquiresStaleOwnedChat(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	deadWorker := uuid.New()
	deadRunner := uuid.New()
	acquireChat(t, f, chat.ID, deadWorker, deadRunner)
	makeHeartbeatStale(t, f, chat.ID, deadRunner)
	starter := newBlockingTaskStarter(false)
	worker := startWorker(t, testOptions(t, f, starter))

	call := starter.waitCall(t, TaskKindGeneration, chat.ID)
	require.Equal(t, worker.chatWorkerID(), call.input.WorkerID)
	require.Equal(t, database.ChatStatusRunning, call.input.Status)
	require.NotEqual(t, deadRunner, call.input.RunnerID)

	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.Equal(t, worker.chatWorkerID(), latest.WorkerID.UUID)
	require.Equal(t, call.input.RunnerID, latest.RunnerID.UUID)
	require.NotEqual(t, deadWorker, latest.WorkerID.UUID)
	require.NotEqual(t, deadRunner, latest.RunnerID.UUID)
	_, err = f.db.GetChatHeartbeat(testutil.Context(t, testutil.WaitShort), database.GetChatHeartbeatParams{
		ChatID:   chat.ID,
		RunnerID: call.input.RunnerID,
	})
	require.NoError(t, err)
}

func TestWorker_TwoWorkersRaceSingleOwner(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	firstStarter := newRecordingTaskStarter()
	secondStarter := newRecordingTaskStarter()
	first := startWorker(t, testOptions(t, f, firstStarter))
	second := startWorker(t, testOptions(t, f, secondStarter))

	call := waitAnyTaskCall(t, firstStarter, secondStarter, TaskKindGeneration, chat.ID)
	require.Contains(t, []uuid.UUID{first.chatWorkerID(), second.chatWorkerID()}, call.input.WorkerID)
	firstStarter.assertNoCall(t)
	secondStarter.assertNoCall(t)

	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.True(t, latest.WorkerID.Valid)
	require.True(t, latest.RunnerID.Valid)
	require.Equal(t, call.input.WorkerID, latest.WorkerID.UUID)
	require.Equal(t, call.input.RunnerID, latest.RunnerID.UUID)
}

func TestWorker_DrainsMultipleRunnableChatsOnWake(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	first := f.createRunningChat(t)
	second := f.createRunningChat(t)
	third := f.createRunningChat(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.AcquisitionBatchSize = 1
	startWorker(t, opts)

	want := map[uuid.UUID]bool{first.ID: true, second.ID: true, third.ID: true}
	for range 3 {
		call := starter.waitCall(t, TaskKindGeneration, uuid.Nil)
		delete(want, call.input.ChatID)
	}
	require.Empty(t, want)
}

func TestWorker_DoesNotAcquireIdleOrArchivedChats(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	waiting := f.createRunningChat(t)
	finishTurn(t, f, waiting.ID)
	errorChat := f.createRunningChat(t)
	forceExecutionStateAndPublish(t, f, errorChat.ID, database.ChatStatusError, false)
	archived := f.createRunningChat(t)
	forceExecutionStateAndPublish(t, f, archived.ID, database.ChatStatusRunning, true)
	starter := newRecordingTaskStarter()
	worker := startWorker(t, testOptions(t, f, starter))
	worker.Wake()

	starter.assertNoCall(t)
}

func TestWorker_HeartbeatLoopRefreshesActiveRunnerHeartbeat(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	clock := quartz.NewMock(t)
	heartbeatTrap := clock.Trap().NewTicker("chatworker", "heartbeat")
	defer heartbeatTrap.Close()
	starter := newBlockingTaskStarter(false)
	opts := testOptions(t, f, starter)
	opts.Clock = clock
	opts.HeartbeatInterval = time.Minute
	startWorker(t, opts)
	heartbeatTrap.MustWait(testutil.Context(t, testutil.WaitLong)).MustRelease(testutil.Context(t, testutil.WaitLong))
	call := starter.waitCall(t, TaskKindGeneration, chat.ID)
	oldHeartbeat := makeHeartbeatStale(t, f, chat.ID, call.input.RunnerID)

	clock.Advance(time.Minute).MustWait(testutil.Context(t, testutil.WaitLong))
	testutil.Eventually(testutil.Context(t, testutil.WaitLong), t, func(ctx context.Context) bool {
		heartbeat, err := f.db.GetChatHeartbeat(ctx, database.GetChatHeartbeatParams{
			ChatID:   chat.ID,
			RunnerID: call.input.RunnerID,
		})
		return err == nil && heartbeat.HeartbeatAt.After(oldHeartbeat)
	}, testutil.IntervalFast, "heartbeat should be refreshed")
}

func TestWorker_HeartbeatCleanupDeletesStaleRows(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	finishTurn(t, f, chat.ID)
	runnerID := uuid.New()
	require.NoError(t, f.db.UpsertChatHeartbeat(testutil.Context(t, testutil.WaitShort), database.UpsertChatHeartbeatParams{
		ChatID:   chat.ID,
		RunnerID: runnerID,
	}))
	makeHeartbeatStale(t, f, chat.ID, runnerID)
	clock := quartz.NewMock(t)
	cleanupTrap := clock.Trap().NewTicker("chatworker", "heartbeat-cleanup")
	defer cleanupTrap.Close()
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.Clock = clock
	opts.HeartbeatCleanupInterval = time.Minute
	startWorker(t, opts)
	cleanupTrap.MustWait(testutil.Context(t, testutil.WaitLong)).MustRelease(testutil.Context(t, testutil.WaitLong))

	clock.Advance(time.Minute).MustWait(testutil.Context(t, testutil.WaitLong))
	testutil.Eventually(testutil.Context(t, testutil.WaitLong), t, func(ctx context.Context) bool {
		_, err := f.db.GetChatHeartbeat(ctx, database.GetChatHeartbeatParams{
			ChatID:   chat.ID,
			RunnerID: runnerID,
		})
		return errors.Is(err, sql.ErrNoRows)
	}, testutil.IntervalFast)
}

func TestWorker_CloseDeletesOwnedHeartbeatsAndPublishesOwnershipHints(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	first := f.createRunningChat(t)
	second := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	pubsub := newRecordingPubsub(f.pubsub)
	opts := testOptions(t, f, starter)
	opts.Pubsub = pubsub
	worker := startWorker(t, opts)
	callsByChat := make(map[uuid.UUID]taskCall)
	for range 2 {
		call := starter.waitCall(t, TaskKindGeneration, uuid.Nil)
		callsByChat[call.input.ChatID] = call
	}
	require.Contains(t, callsByChat, first.ID)
	require.Contains(t, callsByChat, second.ID)

	require.NoError(t, worker.Close())
	for _, call := range callsByChat {
		_, err := f.db.GetChatHeartbeat(testutil.Context(t, testutil.WaitShort), database.GetChatHeartbeatParams{
			ChatID:   call.input.ChatID,
			RunnerID: call.input.RunnerID,
		})
		require.ErrorIs(t, err, sql.ErrNoRows)
	}

	messages := pubsub.ownershipMessages(t)
	seen := make(map[uuid.UUID]bool)
	for _, msg := range messages {
		seen[msg.ChatID] = true
		require.NotZero(t, msg.SnapshotVersion)
	}
	require.True(t, seen[first.ID], "expected ownership hint for first runner")
	require.True(t, seen[second.ID], "expected ownership hint for second runner")
}

func TestWorker_CloseIsIdempotentAndDoesNotBlock(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newBlockingTaskStarter(false)
	worker := startWorker(t, testOptions(t, f, starter))
	call := starter.waitCall(t, TaskKindGeneration, chat.ID)

	closed := make(chan error, 1)
	go func() {
		if err := worker.Close(); err != nil {
			closed <- err
			return
		}
		closed <- worker.Close()
	}()
	select {
	case err := <-closed:
		require.NoError(t, err)
	case <-time.After(testutil.WaitLong):
		t.Fatal("worker close did not return")
	}
	select {
	case <-call.ctx.Done():
	case <-time.After(testutil.WaitLong):
		t.Fatal("active task was not canceled")
	}
}

func waitAnyTaskCall(
	t *testing.T,
	first *recordingTaskStarter,
	second *recordingTaskStarter,
	kind TaskKind,
	chatID uuid.UUID,
) taskCall {
	t.Helper()
	deadline := time.After(testutil.WaitLong)
	for {
		select {
		case call := <-first.callCh:
			if call.kind == kind && call.input.ChatID == chatID {
				return call
			}
		case call := <-second.callCh:
			if call.kind == kind && call.input.ChatID == chatID {
				return call
			}
		case <-deadline:
			t.Fatal("timed out waiting for either worker to start task")
			return taskCall{}
		}
	}
}

func requireTaskCanceled(t *testing.T, call taskCall) {
	t.Helper()
	select {
	case <-call.ctx.Done():
		require.True(t, errors.Is(call.ctx.Err(), context.Canceled))
	case <-time.After(testutil.WaitLong):
		t.Fatal("task context was not canceled")
	}
}

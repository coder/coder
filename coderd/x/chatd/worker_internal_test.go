package chatd

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestWorker_NewRequiresTaskStarterOrMessagePartBuffer(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	_, err := newChatWorker(newUnstartedServer(t, f.pubsub, f.db), chatWorkerOptions{WorkerID: uuid.New(), Store: f.db, Pubsub: f.pubsub})
	require.ErrorContains(t, err, "task starter or message part buffer is required")
}

func TestWorker_NewRequiresServer(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	_, err := newChatWorker(nil, testOptions(t, f, newRecordingTaskStarter()))
	require.ErrorContains(t, err, "server is required")
}

func TestWorker_NewRequiresWorkerID(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	opts := testOptions(t, f, newRecordingTaskStarter())
	opts.WorkerID = uuid.Nil
	_, err := newChatWorker(newUnstartedServer(t, f.pubsub, f.db), opts)
	require.ErrorContains(t, err, "worker ID is required")
}

func TestWorker_UsesConfiguredWorkerID(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	workerID := opts.WorkerID
	worker, err := newChatWorker(newUnstartedServer(t, f.pubsub, f.db), opts)
	require.NoError(t, err)
	require.Equal(t, workerID, worker.opts.WorkerID)
	require.NoError(t, worker.Start(context.Background()))
	require.Equal(t, workerID, worker.opts.WorkerID)
	require.NoError(t, worker.Close())
}

func TestWorker_AcquiresRunnableChatFromOwnershipHint(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	starter := newRecordingTaskStarter()
	worker := startWorker(t, testOptions(t, f, starter))

	call := starter.waitCall(t, taskKindGeneration, chat.ID)
	require.Equal(t, worker.opts.WorkerID, call.input.WorkerID)
	require.Equal(t, database.ChatStatusRunning, call.input.Status)
	require.NotEqual(t, uuid.Nil, call.input.RunnerID)

	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.Equal(t, worker.opts.WorkerID, latest.WorkerID.UUID)
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

	call := starter.waitCall(t, taskKindRequiresActionTimeout, chat.ID)
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

	call := starter.waitCall(t, taskKindGeneration, chat.ID)
	require.Equal(t, worker.opts.WorkerID, call.input.WorkerID)
	require.Equal(t, database.ChatStatusRunning, call.input.Status)
	require.NotEqual(t, deadRunner, call.input.RunnerID)

	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.Equal(t, worker.opts.WorkerID, latest.WorkerID.UUID)
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

	call := waitAnyTaskCall(t, firstStarter, secondStarter, taskKindGeneration, chat.ID)
	require.Contains(t, []uuid.UUID{first.opts.WorkerID, second.opts.WorkerID}, call.input.WorkerID)
	firstStarter.assertNoCall(t)
	secondStarter.assertNoCall(t)

	latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
	require.NoError(t, err)
	require.True(t, latest.WorkerID.Valid)
	require.True(t, latest.RunnerID.Valid)
	require.Equal(t, call.input.WorkerID, latest.WorkerID.UUID)
	require.Equal(t, call.input.RunnerID, latest.RunnerID.UUID)
}

func TestWorker_AcquisitionBatchSizeLimitsSuccessfulAcquisitions(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	first := f.createRunningChat(t)
	second := f.createRunningChat(t)
	third := f.createRunningChat(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.AcquisitionBatchSize = 1
	worker := startWorker(t, opts)

	want := map[uuid.UUID]bool{first.ID: true, second.ID: true, third.ID: true}
	for range 3 {
		call := starter.waitCall(t, taskKindGeneration, uuid.Nil)
		delete(want, call.input.ChatID)
		starter.assertNoCall(t)
		worker.Wake()
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
	call := starter.waitCall(t, taskKindGeneration, chat.ID)
	// Renewal only extends a lease that is still fresh.
	setHeartbeatAge(t, f, chat.ID, call.input.RunnerID, 10*time.Second)
	aged, err := f.db.GetChatHeartbeat(testutil.Context(t, testutil.WaitShort), database.GetChatHeartbeatParams{
		ChatID:   chat.ID,
		RunnerID: call.input.RunnerID,
	})
	require.NoError(t, err)
	oldHeartbeat := aged.HeartbeatAt

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
		call := starter.waitCall(t, taskKindGeneration, uuid.Nil)
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
	call := starter.waitCall(t, taskKindGeneration, chat.ID)

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
	kind taskKind,
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

func TestWorkerRunnerTurnSpan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		staleOwner       bool
		wantAcquisitions int
	}{
		{name: "Unowned", wantAcquisitions: 1},
		{name: "StaleOwner", staleOwner: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			f := newWorkerTestFixture(t)
			chat := f.createRunningChat(t)
			if tt.staleOwner {
				deadRunner := uuid.New()
				acquireChat(t, f, chat.ID, uuid.New(), deadRunner)
				makeHeartbeatStale(t, f, chat.ID, deadRunner)
			}
			tracer, recorder := newStageTestTracer(t)
			server := newUnstartedServer(t, f.pubsub, f.db)
			server.stages = tracer
			starter := newBlockingTaskStarter(false)
			worker, err := newChatWorker(server, testOptions(t, f, starter))
			require.NoError(t, err)
			require.NoError(t, worker.Start(context.Background()))

			call := starter.waitCall(t, taskKindGeneration, chat.ID)
			require.NotNil(t, call.input.TurnSpan)
			call.input.TurnSpan.Ensure(ctx, call.input.TaskID, chat, chat.CreatedAt)
			require.NoError(t, worker.Close())

			turns := turnSpansByStart(t, recorder)
			require.Len(t, turns, 1)
			outcome, _ := spanAttribute(t, turns[0], chatloop.AttrTurnOutcome)
			require.Equal(t, chatloop.TurnOutcomeAbandoned, chatloop.TurnOutcome(outcome.AsString()))
			require.Len(t, stageSpansByStart(t, recorder, chatloop.StageAcquisition), tt.wantAcquisitions)
		})
	}
}

func TestWorkerRunnerReleasesTurnOnTaskExit(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat := f.createRunningChat(t)
	tracer, recorder := newStageTestTracer(t)
	server := newUnstartedServer(t, f.pubsub, f.db)
	server.stages = tracer
	starter := newBlockingTaskStarter(false)
	worker, err := newChatWorker(server, testOptions(t, f, starter))
	require.NoError(t, err)
	require.NoError(t, worker.Start(context.Background()))
	t.Cleanup(func() { _ = worker.Close() })

	call := starter.waitCall(t, taskKindGeneration, chat.ID)
	_, token := call.input.TurnSpan.Ensure(ctx, call.input.TaskID, chat, chat.CreatedAt)
	require.NotZero(t, token)
	call.input.TurnSpan.Complete(token)
	call.input.TurnSpan.Settle(token)
	require.Empty(t, turnSpansByStart(t, recorder))

	starter.release(t, 0)
	testutil.Eventually(ctx, t, func(context.Context) bool {
		return len(turnSpansByStart(t, recorder)) == 1
	}, testutil.IntervalFast)
	outcome, _ := spanAttribute(t, turnSpansByStart(t, recorder)[0], chatloop.AttrTurnOutcome)
	require.Equal(t, chatloop.TurnOutcomeCompleted, chatloop.TurnOutcome(outcome.AsString()))
}

// createWaitingChat creates an idle chat, which publishes no ownership
// hint and is not an acquisition candidate.
func (f *workerTestFixture) createWaitingChat(t *testing.T) database.Chat {
	t.Helper()
	res, err := chatstate.CreateChat(testutil.Context(t, testutil.WaitShort), f.db, f.pubsub, chatstate.CreateChatInput{
		OrganizationID:    f.org.ID,
		OwnerID:           f.user.ID,
		LastModelConfigID: f.model.ID,
		Title:             "test",
		ClientType:        database.ChatClientTypeApi,
		InitialStatus:     database.ChatStatusWaiting,
		InitialMessages: []chatstate.Message{
			userTextMessage(t, "hello", f.user.ID, f.model.ID, f.apiKey.ID),
		},
	})
	require.NoError(t, err)
	return res.Chat
}

// sendMessageWith makes chat runnable through a machine that acquires
// locally through acquirer, and returns the chat:update that Update
// published.
func sendMessageWith(t *testing.T, f *workerTestFixture, ps *recordingPubsub, acquirer chatstate.LocalAcquirer, chatID uuid.UUID) coderdpubsub.ChatStateUpdateMessage {
	t.Helper()
	before := len(ps.stateUpdateMessages(t, chatID))
	machine := chatstate.NewChatMachine(f.db, ps, chatID).WithLocalAcquirer(acquirer)
	err := machine.Update(testutil.Context(t, testutil.WaitShort), func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.SendMessage(chatstate.SendMessageInput{
			Message:      userTextMessage(t, "next", f.user.ID, f.model.ID, f.apiKey.ID),
			BusyBehavior: chatstate.BusyBehaviorQueue,
			MaxQueueSize: codersdk.DefaultChatMaxQueuedMessagesPerChat,
		})
		return err
	})
	require.NoError(t, err)
	published := ps.stateUpdateMessages(t, chatID)
	require.Greater(t, len(published), before)
	return published[before]
}

// closeBeforeStartAcquirer closes the worker between commit and spawn.
type closeBeforeStartAcquirer struct {
	*chatWorker
	t *testing.T
}

func (a closeBeforeStartAcquirer) StartAcquired(chatID uuid.UUID, input chatstate.AcquireInput) {
	require.NoError(a.t, a.Close())
	a.chatWorker.StartAcquired(chatID, input)
}

func TestWorker_LocalAcquire(t *testing.T) {
	t.Parallel()

	uncapped := func() *fakeAdmission {
		admission := newFakeAdmission()
		admission.uncapped = true
		return admission
	}

	t.Run("SpawnsWithoutHint", func(t *testing.T) {
		t.Parallel()
		f := newWorkerTestFixture(t)
		rec := newRecordingPubsub(f.pubsub)
		starter := newRecordingTaskStarter()
		opts := testOptions(t, f, starter)
		opts.Pubsub = rec
		admission := uncapped()
		opts.AgentCapacityLimiter = admission
		worker := startWorker(t, opts)
		ctx := testutil.Context(t, testutil.WaitShort)

		chat := f.createWaitingChat(t)
		published := sendMessageWith(t, f, rec, worker, chat.ID)
		require.NotNil(t, published.WorkerID)
		require.Equal(t, worker.opts.WorkerID, *published.WorkerID)
		call := starter.waitCall(t, taskKindGeneration, chat.ID)
		require.NotNil(t, published.RunnerID)
		require.Equal(t, *published.RunnerID, call.input.RunnerID)
		stored, err := f.db.GetChatByID(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, uuid.NullUUID{UUID: worker.opts.WorkerID, Valid: true}, stored.WorkerID)
		require.Equal(t, uuid.NullUUID{UUID: call.input.RunnerID, Valid: true}, stored.RunnerID)
		require.Equal(t, 1, admission.admitCallCount())

		created, err := chatstate.CreateChatWithID(ctx, f.db, rec, uuid.New(), chatstate.CreateChatInput{
			OrganizationID:    f.org.ID,
			OwnerID:           f.user.ID,
			LastModelConfigID: f.model.ID,
			Title:             "created",
			ClientType:        database.ChatClientTypeApi,
			InitialMessages: []chatstate.Message{
				userTextMessage(t, "hello", f.user.ID, f.model.ID, f.apiKey.ID),
			},
			LocalAcquirer: worker,
		})
		require.NoError(t, err)
		require.Equal(t, uuid.NullUUID{UUID: worker.opts.WorkerID, Valid: true}, created.Chat.WorkerID)
		call = starter.waitCall(t, taskKindGeneration, created.Chat.ID)
		require.Equal(t, created.Chat.RunnerID.UUID, call.input.RunnerID)

		require.Empty(t, rec.ownershipMessages(t))
	})

	t.Run("CappedFallsBackToHint", func(t *testing.T) {
		t.Parallel()
		f := newWorkerTestFixture(t)
		rec := newRecordingPubsub(f.pubsub)
		starter := newRecordingTaskStarter()
		worker := startWorker(t, testOptions(t, f, starter))

		chat := f.createWaitingChat(t)
		published := sendMessageWith(t, f, rec, worker, chat.ID)
		require.Nil(t, published.WorkerID)
		require.Len(t, rec.ownershipMessages(t), 1)
		call := starter.waitCall(t, taskKindGeneration, chat.ID)
		require.Equal(t, worker.opts.WorkerID, call.input.WorkerID)
	})

	t.Run("AdmissionRefusedFallsBackToHint", func(t *testing.T) {
		t.Parallel()
		f := newWorkerTestFixture(t)
		rec := newRecordingPubsub(f.pubsub)
		starter := newRecordingTaskStarter()
		opts := testOptions(t, f, starter)
		admission := uncapped()
		opts.AgentCapacityLimiter = admission
		worker := startWorker(t, opts)

		chat := f.createWaitingChat(t)
		admission.refuse(chat.ID)
		published := sendMessageWith(t, f, rec, worker, chat.ID)
		require.Nil(t, published.WorkerID)
		require.Len(t, rec.ownershipMessages(t), 1)
	})

	t.Run("WorkerNotRunning", func(t *testing.T) {
		t.Parallel()
		f := newWorkerTestFixture(t)
		opts := testOptions(t, f, nil)
		opts.AgentCapacityLimiter = uncapped()
		worker, err := newChatWorker(newUnstartedServer(t, f.pubsub, f.db), opts)
		require.NoError(t, err)
		chat := f.createWaitingChat(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		_, ok, err := worker.PrepareAcquire(ctx, f.db, chat)
		require.NoError(t, err)
		require.False(t, ok, "an unstarted worker leaves the chat to the hint")

		require.NoError(t, worker.Start(context.Background()))
		_, ok, err = worker.PrepareAcquire(ctx, f.db, chat)
		require.NoError(t, err)
		require.True(t, ok)

		require.NoError(t, worker.Close())
		_, ok, err = worker.PrepareAcquire(ctx, f.db, chat)
		require.NoError(t, err)
		require.False(t, ok, "a closed worker leaves the chat to the hint")
	})

	t.Run("SpawnFailureAbandons", func(t *testing.T) {
		t.Parallel()
		f := newWorkerTestFixture(t)
		rec := newRecordingPubsub(f.pubsub)
		starter := newRecordingTaskStarter()
		opts := testOptions(t, f, starter)
		opts.Pubsub = rec
		opts.AgentCapacityLimiter = uncapped()
		worker := startWorker(t, opts)

		chat := f.createWaitingChat(t)
		published := sendMessageWith(t, f, rec, closeBeforeStartAcquirer{chatWorker: worker, t: t}, chat.ID)
		require.NotNil(t, published.WorkerID)
		require.Equal(t, worker.opts.WorkerID, *published.WorkerID)

		latest, err := f.db.GetChatByID(testutil.Context(t, testutil.WaitShort), chat.ID)
		require.NoError(t, err)
		require.False(t, latest.WorkerID.Valid)
		require.False(t, latest.RunnerID.Valid)
		hints := rec.ownershipMessages(t)
		require.Len(t, hints, 1)
		require.Equal(t, latest.SnapshotVersion, hints[0].SnapshotVersion)
		starter.assertNoCall(t)
	})
}

package chatd

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

type fakeAdmission struct {
	mu      sync.Mutex
	refused map[uuid.UUID]bool

	admitCalls int
	admitted   []uuid.UUID
	limits     AgentCapacityLimits
	uncapped   bool
}

func newFakeAdmission() *fakeAdmission {
	return &fakeAdmission{
		refused: make(map[uuid.UUID]bool),
		limits:  AgentCapacityLimits{Root: 1, Subagent: 1},
	}
}

func (f *fakeAdmission) Limits() (AgentCapacityLimits, bool) {
	return f.limits, !f.uncapped
}

func (f *fakeAdmission) refuse(chatID uuid.UUID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refused[chatID] = true
}

func (f *fakeAdmission) allow(chatID uuid.UUID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.refused, chatID)
}

func (f *fakeAdmission) Admit(_ context.Context, _ database.Store, chat database.Chat) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.admitCalls++
	if f.refused[chat.ID] {
		return false, nil
	}
	f.admitted = append(f.admitted, chat.ID)
	return true, nil
}

func (f *fakeAdmission) admittedOrder() []uuid.UUID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uuid.UUID(nil), f.admitted...)
}

func (f *fakeAdmission) admitCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.admitCalls
}

// cappedLimits marks single-purpose admission fakes as capped so clear events
// and release nudges stay enabled.
type cappedLimits struct{}

func (cappedLimits) Limits() (AgentCapacityLimits, bool) {
	return AgentCapacityLimits{Root: 1, Subagent: 1}, true
}

func (w *chatWorker) capacityQueueContains(chatID uuid.UUID) bool {
	w.capacityMu.Lock()
	defer w.capacityMu.Unlock()
	_, ok := w.capacityQueue[chatID]
	return ok
}

func (w *chatWorker) capacityQueueLen() int {
	w.capacityMu.Lock()
	defer w.capacityMu.Unlock()
	return len(w.capacityQueue)
}

func chatOwnershipMessages(t *testing.T, ps *recordingPubsub, chatID uuid.UUID) int {
	t.Helper()
	count := 0
	for _, msg := range ps.ownershipMessages(t) {
		if msg.ChatID == chatID {
			count++
		}
	}
	return count
}

func capacityEvents(t *testing.T, ps *recordingPubsub, chatID uuid.UUID) []codersdk.ChatWatchEvent {
	t.Helper()
	events := make([]codersdk.ChatWatchEvent, 0)
	for _, event := range ps.watchEvents(t) {
		if event.Kind == codersdk.ChatWatchEventKindCapacityChange && event.Chat.ID == chatID {
			events = append(events, event)
		}
	}
	return events
}

func TestWorker_AdmissionRefusalQueuesChat(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	recording := newRecordingPubsub(f.pubsub)
	starter := newRecordingTaskStarter()
	admission := newFakeAdmission()
	opts := testOptions(t, f, starter)
	opts.Pubsub = recording
	opts.AgentCapacityLimiter = admission

	chat := f.createRunningChat(t)
	admission.refuse(chat.ID)
	startWorker(t, opts)

	// The queued event publishes after the map insert, so wait on the event.
	require.Eventually(t, func() bool {
		return len(capacityEvents(t, recording, chat.ID)) >= 1
	}, testutil.WaitLong, testutil.IntervalFast)
	starter.assertNoCall(t)

	events := capacityEvents(t, recording, chat.ID)
	require.True(t, events[len(events)-1].Chat.QueuedForCapacity)

	// The recorder wraps only worker pubsub, so an ownership hint here would
	// prove a refusal can wake workers into an immediate retry loop.
	require.Equal(t, 0, chatOwnershipMessages(t, recording, chat.ID))
}

func TestWorker_RefusalPublishesQueuedEventOnce(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	recording := newRecordingPubsub(f.pubsub)
	starter := newRecordingTaskStarter()
	admission := newFakeAdmission()
	opts := testOptions(t, f, starter)
	opts.Pubsub = recording
	opts.AgentCapacityLimiter = admission

	chat := f.createRunningChat(t)
	admission.refuse(chat.ID)
	worker := startWorker(t, opts)

	// The queued event publishes after the map insert, so wait on the event.
	require.Eventually(t, func() bool {
		return len(capacityEvents(t, recording, chat.ID)) == 1
	}, testutil.WaitLong, testutil.IntervalFast)

	admitCallsAfterQueue := admission.admitCallCount()
	worker.Wake()
	require.Eventually(t, func() bool {
		return admission.admitCallCount() > admitCallsAfterQueue
	}, testutil.WaitLong, testutil.IntervalFast)
	require.Len(t, capacityEvents(t, recording, chat.ID), 1,
		"repeated refusals of an already-queued chat must not republish the queued event")
}

func TestWorker_AdmissionAdmitPublishesClear(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	recording := newRecordingPubsub(f.pubsub)
	starter := newRecordingTaskStarter()
	admission := newFakeAdmission()
	metrics := newCapacityMetrics(prometheus.NewRegistry())
	opts := testOptions(t, f, starter)
	opts.Pubsub = recording
	opts.AgentCapacityLimiter = admission
	opts.CapacityMetrics = metrics

	chat := f.createRunningChat(t)
	admission.refuse(chat.ID)
	worker := startWorker(t, opts)

	require.Eventually(t, func() bool {
		return worker.capacityQueueContains(chat.ID)
	}, testutil.WaitLong, testutil.IntervalFast)

	admission.allow(chat.ID)
	worker.Wake()

	call := starter.waitCall(t, taskKindGeneration, chat.ID)
	require.Equal(t, chat.ID, call.input.ChatID)
	require.False(t, worker.capacityQueueContains(chat.ID))

	require.Eventually(t, func() bool {
		events := capacityEvents(t, recording, chat.ID)
		return len(events) >= 2 && !events[len(events)-1].Chat.QueuedForCapacity
	}, testutil.WaitLong, testutil.IntervalFast)
	require.Equal(t, 1, promtestutil.CollectAndCount(metrics.waitSeconds),
		"admitting a queued chat must observe its capacity wait")
}

func TestWorker_InterruptingSortsBeforeRunning(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	running := f.createRunningChat(t)
	interrupting := f.createRunningChat(t)
	interruptChat(t, f, interrupting.ID)
	requiresAction := f.createRequiresActionChat(t)

	rows, err := f.db.GetChatWorkerAcquisitionCandidates(ctx, database.GetChatWorkerAcquisitionCandidatesParams{
		StaleSeconds: 30,
		LimitCount:   10,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(rows), 3)
	require.Equal(t, interrupting.ID, rows[0].ID, "interrupting chats must sort before running chats")
	require.Equal(t, requiresAction.ID, rows[1].ID, "requires_action chats bypass admission and must sort before running chats")
	require.Equal(t, running.ID, rows[2].ID)
}

func TestWorker_MessageBumpSendsChatToQueueBack(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	older := f.createRunningChat(t)
	newer := f.createRunningChat(t)

	// updated_at is the FIFO key, so any write that bumps it (for example a
	// message sent to a capacity-queued chat) re-queues the chat at the back.
	_, err := f.sqlDB.ExecContext(ctx,
		"UPDATE chats SET updated_at = NOW() + INTERVAL '1 hour' WHERE id = $1", older.ID)
	require.NoError(t, err)

	rows, err := f.db.GetChatWorkerAcquisitionCandidates(ctx, database.GetChatWorkerAcquisitionCandidatesParams{
		StaleSeconds: 30,
		LimitCount:   10,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(rows), 2)
	require.Equal(t, newer.ID, rows[0].ID)
	require.Equal(t, older.ID, rows[1].ID)
}

type rootRefusingAdmission struct {
	cappedLimits
	mu    sync.Mutex
	calls int
}

func (a *rootRefusingAdmission) Admit(_ context.Context, _ database.Store, chat database.Chat) (bool, error) {
	a.mu.Lock()
	a.calls++
	a.mu.Unlock()
	if chat.Status != database.ChatStatusRunning {
		return true, nil
	}
	return chat.ParentChatID.Valid, nil
}

func (a *rootRefusingAdmission) callCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls
}

func TestWorker_FullPoolDoesNotStarveOtherPool(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.AgentCapacityLimiter = &rootRefusingAdmission{}

	// The root backlog hides the later subagent unless the query interleaves pools.
	roots := make([]database.Chat, 0, 2*int(opts.AcquisitionBatchSize)+5)
	for range cap(roots) {
		roots = append(roots, f.createRunningChat(t))
	}
	sub := f.createRunningSubagentChat(t, roots[0].ID)

	startWorker(t, opts)

	call := starter.waitCall(t, taskKindGeneration, sub.ID)
	require.Equal(t, sub.ID, call.input.ChatID)
}

func TestWorker_BatchSizeOneCannotHideAPool(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.AgentCapacityLimiter = &rootRefusingAdmission{}
	opts.AcquisitionBatchSize = 1

	roots := []database.Chat{f.createRunningChat(t), f.createRunningChat(t)}
	sub := f.createRunningSubagentChat(t, roots[0].ID)

	startWorker(t, opts)

	call := starter.waitCall(t, taskKindGeneration, sub.ID)
	require.Equal(t, sub.ID, call.input.ChatID)
}

func TestWorker_FullPoolSkipsRefusalsAfterFirst(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	admission := &rootRefusingAdmission{}
	opts.AgentCapacityLimiter = admission

	chats := make([]database.Chat, 5)
	for i := range chats {
		chats[i] = f.createRunningChat(t)
	}
	worker := startWorker(t, opts)

	require.Eventually(t, func() bool {
		for _, chat := range chats {
			if !worker.capacityQueueContains(chat.ID) {
				return false
			}
		}
		return true
	}, testutil.WaitLong, testutil.IntervalFast)
	require.LessOrEqual(t, admission.callCount(), 2,
		"a full pool must be skipped after one refusal, not re-refused per chat")
}

func TestWorker_AdmissionPublishesClearWithoutLocalRefusal(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	recording := newRecordingPubsub(f.pubsub)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.Pubsub = recording
	opts.AgentCapacityLimiter = newFakeAdmission()

	// Another replica may have refused this chat and published the queued
	// event, so the admitting replica must publish the clear even though
	// its own capacityQueue never held the chat.
	chat := f.createRunningChat(t)
	startWorker(t, opts)

	starter.waitCall(t, taskKindGeneration, chat.ID)
	require.Eventually(t, func() bool {
		events := capacityEvents(t, recording, chat.ID)
		return len(events) >= 1 && !events[len(events)-1].Chat.QueuedForCapacity
	}, testutil.WaitLong, testutil.IntervalFast)
}

func TestWorker_InterruptAcquisitionPublishesClear(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	recording := newRecordingPubsub(f.pubsub)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.Pubsub = recording
	opts.AgentCapacityLimiter = newFakeAdmission()

	// An interrupt can reach acquisition after a capacity wait, so it must
	// clear the queued state.
	chat := f.createRunningChat(t)
	interruptChat(t, f, chat.ID)
	startWorker(t, opts)

	starter.waitCall(t, taskKindInterrupt, chat.ID)
	require.Eventually(t, func() bool {
		events := capacityEvents(t, recording, chat.ID)
		return len(events) >= 1 && !events[len(events)-1].Chat.QueuedForCapacity
	}, testutil.WaitLong, testutil.IntervalFast)
}

func TestWorker_QueuedEventRevalidatesOwnedCandidate(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	recording := newRecordingPubsub(f.pubsub)
	opts := testOptions(t, f, newRecordingTaskStarter())
	opts.Pubsub = recording
	opts.AgentCapacityLimiter = newFakeAdmission()
	ctx := testutil.Context(t, testutil.WaitLong)

	// Another replica acquires the chat between the candidate query and the
	// queued publish; a queued event here would carry the acquisition's
	// updated_at and outlive the owner's clear on open chat pages.
	chat := f.createRunningChat(t)
	acquireChat(t, f, chat.ID, uuid.New(), uuid.New())

	worker, err := newChatWorker(newUnstartedServer(t, recording, f.db), opts)
	require.NoError(t, err)
	require.False(t, worker.enterCapacityQueue(ctx, chat.ID))
	require.False(t, worker.capacityQueueContains(chat.ID))
	require.Empty(t, capacityEvents(t, recording, chat.ID))
}

func TestWorker_QueuedEventPublishesForStaleOwnedCandidate(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	recording := newRecordingPubsub(f.pubsub)
	opts := testOptions(t, f, newRecordingTaskStarter())
	opts.Pubsub = recording
	opts.AgentCapacityLimiter = newFakeAdmission()
	ctx := testutil.Context(t, testutil.WaitLong)

	// A crashed owner leaves stale ownership, so revalidation must still
	// publish the queued event.
	chat := f.createRunningChat(t)
	runnerID := uuid.New()
	acquireChat(t, f, chat.ID, uuid.New(), runnerID)
	makeHeartbeatStale(t, f, chat.ID, runnerID)

	worker, err := newChatWorker(newUnstartedServer(t, recording, f.db), opts)
	require.NoError(t, err)
	require.True(t, worker.enterCapacityQueue(ctx, chat.ID))
	require.True(t, worker.capacityQueueContains(chat.ID))
	events := capacityEvents(t, recording, chat.ID)
	require.Len(t, events, 1)
	require.True(t, events[0].Chat.QueuedForCapacity)
}

func TestWorker_UncappedAcquisitionStillPublishesClear(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	recording := newRecordingPubsub(f.pubsub)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.Pubsub = recording
	admission := newFakeAdmission()
	admission.uncapped = true
	opts.AgentCapacityLimiter = admission

	// A license update can uncap the deployment after another replica
	// published queued=true, so the clear must not be gated on the cap.
	chat := f.createRunningChat(t)
	startWorker(t, opts)

	starter.waitCall(t, taskKindGeneration, chat.ID)
	require.Eventually(t, func() bool {
		events := capacityEvents(t, recording, chat.ID)
		return len(events) >= 1 && !events[len(events)-1].Chat.QueuedForCapacity
	}, testutil.WaitLong, testutil.IntervalFast)
}

func TestWorker_WithoutAdmissionPublishesNoCapacityEvents(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	recording := newRecordingPubsub(f.pubsub)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.Pubsub = recording
	require.Nil(t, opts.AgentCapacityLimiter, "the noop limiter applies at construction")

	chat := f.createRunningChat(t)
	startWorker(t, opts)

	call := starter.waitCall(t, taskKindGeneration, chat.ID)
	require.Equal(t, chat.ID, call.input.ChatID)
	require.Empty(t, capacityEvents(t, recording, chat.ID))
}

func TestWorker_AdmissionAdmitsInUpdatedAtOrder(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	admission := newFakeAdmission()
	opts.AgentCapacityLimiter = admission

	older := f.createRunningChat(t)
	newer := f.createRunningChat(t)
	// Back-to-back inserts can collide at timestamp resolution, which would
	// leave FIFO order to the random UUID tiebreak.
	ctx := testutil.Context(t, testutil.WaitLong)
	_, err := f.sqlDB.ExecContext(ctx,
		"UPDATE chats SET updated_at = NOW() - INTERVAL '1 hour' WHERE id = $1", older.ID)
	require.NoError(t, err)
	admission.refuse(older.ID)
	admission.refuse(newer.ID)
	worker := startWorker(t, opts)

	require.Eventually(t, func() bool {
		return worker.capacityQueueContains(older.ID) && worker.capacityQueueContains(newer.ID)
	}, testutil.WaitLong, testutil.IntervalFast)

	admission.allow(older.ID)
	admission.allow(newer.ID)
	worker.Wake()

	// Runner goroutines race task starts, so wait for both without
	// ordering and assert the worker's serial admission order instead.
	starter.waitCall(t, taskKindGeneration, uuid.Nil)
	starter.waitCall(t, taskKindGeneration, uuid.Nil)
	require.Equal(t, []uuid.UUID{older.ID, newer.ID}, admission.admittedOrder(),
		"the longer-waiting chat must admit first")
}

type runningRefusingAdmission struct{ cappedLimits }

func (runningRefusingAdmission) Admit(_ context.Context, _ database.Store, chat database.Chat) (bool, error) {
	return chat.Status != database.ChatStatusRunning, nil
}

func TestWorker_InterruptClaimsCapacityQueuedChat(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.AgentCapacityLimiter = runningRefusingAdmission{}

	chat := f.createRunningChat(t)
	worker := startWorker(t, opts)

	require.Eventually(t, func() bool {
		return worker.capacityQueueContains(chat.ID)
	}, testutil.WaitLong, testutil.IntervalFast)

	interruptChat(t, f, chat.ID)
	worker.Wake()

	call := starter.waitCall(t, taskKindInterrupt, chat.ID)
	require.Equal(t, chat.ID, call.input.ChatID)
}

func TestWorker_AdmissionPassReachesChatsBeyondRefusedBatch(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.AgentCapacityLimiter = &rootRefusingAdmission{}
	opts.AcquisitionBatchSize = 2

	chats := make([]database.Chat, 5)
	for i := range chats {
		chats[i] = f.createRunningChat(t)
	}
	worker := startWorker(t, opts)

	require.Eventually(t, func() bool {
		for _, chat := range chats {
			if !worker.capacityQueueContains(chat.ID) {
				return false
			}
		}
		return true
	}, testutil.WaitLong, testutil.IntervalFast)
}

func TestWorker_PrunesDepartedChatDespiteFullPoolBacklog(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.AgentCapacityLimiter = &rootRefusingAdmission{}
	opts.AcquisitionBatchSize = 2

	// The backlog exceeds one batch, so every later pass ends all-skipped;
	// prune must still reconcile the chat another replica acquired.
	chats := make([]database.Chat, 5)
	for i := range chats {
		chats[i] = f.createRunningChat(t)
	}
	worker := startWorker(t, opts)

	require.Eventually(t, func() bool {
		return worker.capacityQueueLen() == len(chats)
	}, testutil.WaitLong, testutil.IntervalFast)

	acquireChat(t, f, chats[len(chats)-1].ID, uuid.New(), uuid.New())
	worker.Wake()

	require.Eventually(t, func() bool {
		return !worker.capacityQueueContains(chats[len(chats)-1].ID)
	}, testutil.WaitLong, testutil.IntervalFast,
		"a chat acquired by another replica must leave the local capacity queue")
}

func TestWorker_QueuesChatArrivingBehindFullPoolBacklog(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.AgentCapacityLimiter = &rootRefusingAdmission{}
	opts.AcquisitionBatchSize = 2

	// The backlog exceeds one batch, so every later pass ends all-skipped.
	chats := make([]database.Chat, 5)
	for i := range chats {
		chats[i] = f.createRunningChat(t)
	}
	worker := startWorker(t, opts)

	require.Eventually(t, func() bool {
		return worker.capacityQueueLen() == len(chats)
	}, testutil.WaitLong, testutil.IntervalFast)

	arrival := f.createRunningChat(t)
	worker.Wake()

	require.Eventually(t, func() bool {
		return worker.capacityQueueContains(arrival.ID)
	}, testutil.WaitLong, testutil.IntervalFast,
		"a chat arriving behind a full-pool backlog must enter the local capacity queue")
}

func TestWorker_ResumeDuringAbandonGapRequiresAdmission(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.AgentCapacityLimiter = &rootRefusingAdmission{}
	ctx := testutil.Context(t, testutil.WaitLong)

	// A finished chat whose runner has not yet abandoned ownership: the
	// waiting row still carries worker_id/runner_id and a fresh heartbeat.
	chat := f.createRunningChat(t)
	acquireChat(t, f, chat.ID, uuid.New(), uuid.New())
	finishTurn(t, f, chat.ID)

	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.SendMessage(chatstate.SendMessageInput{
			Message:      userTextMessage(t, "resume", f.user.ID, f.model.ID, f.apiKey.ID),
			BusyBehavior: chatstate.BusyBehaviorQueue,
		})
		return err
	}))
	resumed, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.False(t, resumed.WorkerID.Valid,
		"resume from waiting must clear ownership so admission applies")

	worker := startWorker(t, opts)
	require.Eventually(t, func() bool {
		return worker.capacityQueueContains(chat.ID)
	}, testutil.WaitLong, testutil.IntervalFast,
		"a resumed chat must pass admission, not restart on the retained runner")
}

func TestWorker_PrunesDepartedChatsFromCapacityQueue(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	admission := newFakeAdmission()
	opts := testOptions(t, f, starter)
	opts.AgentCapacityLimiter = admission
	ctx := testutil.Context(t, testutil.WaitLong)

	chat := f.createRunningChat(t)
	admission.refuse(chat.ID)
	worker := startWorker(t, opts)

	require.Eventually(t, func() bool {
		return worker.capacityQueueContains(chat.ID)
	}, testutil.WaitLong, testutil.IntervalFast)

	_, err := f.db.ArchiveChatByID(ctx, chat.ID)
	require.NoError(t, err)
	worker.Wake()

	require.Eventually(t, func() bool {
		return worker.capacityQueueLen() == 0
	}, testutil.WaitLong, testutil.IntervalFast,
		"archived chats must leave the local capacity queue")
}

func TestWorker_RunnerPublishesCapacityReleaseNudge(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	recording := newRecordingPubsub(f.pubsub)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.Pubsub = recording
	opts.AgentCapacityLimiter = newFakeAdmission()

	chat := f.createRunningChat(t)
	startWorker(t, opts)
	starter.waitCall(t, taskKindGeneration, chat.ID)

	ctx := testutil.Context(t, testutil.WaitLong)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishError(chatstate.FinishErrorInput{
			LastError: pqtype.NullRawMessage{RawMessage: []byte(`{"message":"boom"}`), Valid: true},
		})
		return err
	}))

	// FinishError publishes no ownership hint, so any recorded hint is the
	// runner's capacity-release nudge.
	require.Eventually(t, func() bool {
		return chatOwnershipMessages(t, recording, chat.ID) >= 1
	}, testutil.WaitLong, testutil.IntervalFast)
}

func TestWorker_CapacityMetricsCountQueuedOnlyWhenPoolFull(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	metrics := newCapacityMetrics(prometheus.NewRegistry())
	opts := testOptions(t, f, newRecordingTaskStarter())
	opts.CapacityMetrics = metrics
	opts.AgentCapacityLimiter = newFakeAdmission()
	ctx := testutil.Context(t, testutil.WaitLong)

	occupied := f.createRunningChat(t)
	acquireChat(t, f, occupied.ID, uuid.New(), uuid.New())
	f.createRunningChat(t)

	worker, err := newChatWorker(newUnstartedServer(t, f.pubsub, f.db), opts)
	require.NoError(t, err)
	worker.refreshCapacityMetrics(ctx)

	require.Equal(t, float64(1), promtestutil.ToFloat64(metrics.active.WithLabelValues("root")))
	require.Equal(t, float64(1), promtestutil.ToFloat64(metrics.queued.WithLabelValues("root")),
		"an unowned running chat counts as queued when its pool is full")

	forceExecutionState(t, f, occupied.ID, database.ChatStatusWaiting, false)
	worker.refreshCapacityMetrics(ctx)
	require.Equal(t, float64(0), promtestutil.ToFloat64(metrics.queued.WithLabelValues("root")))
}

func TestGetChatQueuedForCapacity(t *testing.T) {
	t.Parallel()

	queued := func(t *testing.T, f *workerTestFixture, chatID uuid.UUID, rootCap, subagentCap int64) bool {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		got, err := f.db.GetChatQueuedForCapacity(ctx, database.GetChatQueuedForCapacityParams{
			ChatID:           chatID,
			StaleSeconds:     30,
			RootCapacity:     rootCap,
			SubagentCapacity: subagentCap,
		})
		require.NoError(t, err)
		return got
	}

	t.Run("PoolNotFull", func(t *testing.T) {
		t.Parallel()
		f := newWorkerTestFixture(t)
		chat := f.createRunningChat(t)
		require.False(t, queued(t, f, chat.ID, 1, 1))
	})

	t.Run("PoolFull", func(t *testing.T) {
		t.Parallel()
		f := newWorkerTestFixture(t)
		occupied := f.createRunningChat(t)
		acquireChat(t, f, occupied.ID, uuid.New(), uuid.New())
		chat := f.createRunningChat(t)
		require.True(t, queued(t, f, chat.ID, 1, 1))
	})

	t.Run("OwnedChatIsNotQueued", func(t *testing.T) {
		t.Parallel()
		f := newWorkerTestFixture(t)
		occupied := f.createRunningChat(t)
		acquireChat(t, f, occupied.ID, uuid.New(), uuid.New())
		require.False(t, queued(t, f, occupied.ID, 1, 1))
	})

	t.Run("NonRunningChatIsNotQueued", func(t *testing.T) {
		t.Parallel()
		f := newWorkerTestFixture(t)
		occupied := f.createRunningChat(t)
		acquireChat(t, f, occupied.ID, uuid.New(), uuid.New())
		requiresAction := f.createRequiresActionChat(t)
		require.False(t, queued(t, f, requiresAction.ID, 1, 1))
	})

	t.Run("PoolsAreIndependent", func(t *testing.T) {
		t.Parallel()
		f := newWorkerTestFixture(t)
		occupied := f.createRunningChat(t)
		acquireChat(t, f, occupied.ID, uuid.New(), uuid.New())
		sub := f.createRunningSubagentChat(t, occupied.ID)
		require.False(t, queued(t, f, sub.ID, 1, 1),
			"a full root pool must not mark subagents queued")
	})
}

func TestServer_ChatQueuedForCapacity(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	occupied := f.createRunningChat(t)
	acquireChat(t, f, occupied.ID, uuid.New(), uuid.New())
	waiting := f.createRunningChat(t)

	server := newUnstartedServer(t, f.pubsub, f.db)

	queued, err := server.ChatQueuedForCapacity(ctx, waiting)
	require.NoError(t, err)
	require.False(t, queued, "the noop limiter must mean never queued")

	uncapped := newFakeAdmission()
	uncapped.uncapped = true
	server.agentCapacityLimiter = uncapped
	queued, err = server.ChatQueuedForCapacity(ctx, waiting)
	require.NoError(t, err)
	require.False(t, queued, "uncapped deployments must never report queued")

	server.agentCapacityLimiter = newFakeAdmission()
	queued, err = server.ChatQueuedForCapacity(ctx, waiting)
	require.NoError(t, err)
	require.True(t, queued)

	queued, err = server.ChatQueuedForCapacity(ctx, occupied)
	require.NoError(t, err)
	require.False(t, queued, "owned chats are active, not queued")
}

func TestCountChatCapacityByPool(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	owned := f.createRunningChat(t)
	acquireChat(t, f, owned.ID, uuid.New(), uuid.New())
	f.createRunningChat(t)
	f.createRunningSubagentChat(t, owned.ID)

	counts, err := f.db.CountChatCapacityByPool(ctx, database.CountChatCapacityByPoolParams{StaleSeconds: 30})
	require.NoError(t, err)
	require.EqualValues(t, 1, counts.ActiveRootCount)
	require.EqualValues(t, 0, counts.ActiveSubagentCount)
	require.EqualValues(t, 1, counts.UnownedRootCount)
	require.EqualValues(t, 1, counts.UnownedSubagentCount)

	counts, err = f.db.CountChatCapacityByPool(ctx, database.CountChatCapacityByPoolParams{
		ExcludeChatID: owned.ID,
		StaleSeconds:  30,
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, counts.ActiveRootCount, "the excluded chat must not count as active")
	require.EqualValues(t, 1, counts.UnownedRootCount, "exclusion must not affect unowned counts")
}

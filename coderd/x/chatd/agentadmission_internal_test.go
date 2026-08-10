package chatd

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
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

func TestWorker_AdmissionRefusalDoesNotAcquireChat(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	recording := newRecordingPubsub(f.pubsub)
	starter := newRecordingTaskStarter()
	admission := newFakeAdmission()
	opts := testOptions(t, f, starter)
	opts.Pubsub = recording
	opts.AgentAdmission = admission

	chat := f.createRunningChat(t)
	admission.refuse(chat.ID)
	startWorker(t, opts)

	require.Eventually(t, func() bool {
		return admission.admitCallCount() > 0
	}, testutil.WaitLong, testutil.IntervalFast)
	starter.assertNoCall(t)

	// The recorder wraps only worker pubsub, so an ownership hint here would
	// prove a refusal can wake workers into an immediate retry loop.
	require.Empty(t, recording.ownershipMessages(t))
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
	_, err := f.sqlDB.ExecContext(ctx, `
		UPDATE chats
		SET updated_at = CASE id
			WHEN $1 THEN NOW() - INTERVAL '1 hour'
			WHEN $2 THEN NOW() - INTERVAL '30 minutes'
		END
		WHERE id IN ($1, $2)
	`, older.ID, newer.ID)
	require.NoError(t, err)

	machine := chatstate.NewChatMachine(f.db, f.pubsub, older.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.SendMessage(chatstate.SendMessageInput{
			Message:      userTextMessage(t, "move me", f.user.ID, f.model.ID, f.apiKey.ID),
			BusyBehavior: chatstate.BusyBehaviorQueue,
		})
		return err
	}))

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
	opts.AgentAdmission = &rootRefusingAdmission{}

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
	opts.AgentAdmission = &rootRefusingAdmission{}
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
	opts.AgentAdmission = admission
	opts.AcquisitionBatchSize = 2

	for range 5 {
		f.createRunningChat(t)
	}
	startWorker(t, opts)

	require.Eventually(t, func() bool {
		return admission.callCount() == 1
	}, testutil.WaitLong, testutil.IntervalFast)
	starter.assertNoCall(t)
	require.Equal(t, 1, admission.callCount(),
		"a full pool must be skipped after one refusal, not re-refused per chat")
}

func TestWorker_AdmissionAdmitsInUpdatedAtOrder(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	admission := newFakeAdmission()
	opts.AgentAdmission = admission

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
		return admission.admitCallCount() == 1
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

type runningRefusingAdmission struct {
	mu    sync.Mutex
	calls int
}

func (a *runningRefusingAdmission) Admit(_ context.Context, _ database.Store, chat database.Chat) (bool, error) {
	a.mu.Lock()
	a.calls++
	a.mu.Unlock()
	return chat.Status != database.ChatStatusRunning, nil
}

func (a *runningRefusingAdmission) callCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls
}

func TestWorker_InterruptClaimsCapacityQueuedChat(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	admission := &runningRefusingAdmission{}
	opts.AgentAdmission = admission

	chat := f.createRunningChat(t)
	worker := startWorker(t, opts)

	require.Eventually(t, func() bool {
		return admission.callCount() > 0
	}, testutil.WaitLong, testutil.IntervalFast)

	interruptChat(t, f, chat.ID)
	worker.Wake()

	call := starter.waitCall(t, taskKindInterrupt, chat.ID)
	require.Equal(t, chat.ID, call.input.ChatID)
}

func TestWorker_CapacityMetricsUseFreshOwnership(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	metrics := newCapacityMetrics(prometheus.NewRegistry())
	opts := testOptions(t, f, newRecordingTaskStarter())
	opts.CapacityMetrics = metrics
	opts.CapacityPolicy = newFakeAdmission()
	ctx := testutil.Context(t, testutil.WaitLong)

	occupied := f.createRunningChat(t)
	acquireChat(t, f, occupied.ID, uuid.New(), uuid.New())
	f.createRunningChat(t)

	worker, err := newChatWorker(newUnstartedServer(t, f.pubsub, f.db), opts)
	require.NoError(t, err)
	worker.refreshCapacityMetrics(ctx)

	require.Equal(t, float64(1), promtestutil.ToFloat64(metrics.active.WithLabelValues("root")))
	require.Equal(t, float64(1), promtestutil.ToFloat64(metrics.queued.WithLabelValues("root")))

	forceExecutionState(t, f, occupied.ID, database.ChatStatusWaiting, true)
	worker.refreshCapacityMetrics(ctx)
	require.Equal(t, float64(1), promtestutil.ToFloat64(metrics.active.WithLabelValues("root")))
	require.Equal(t, float64(1), promtestutil.ToFloat64(metrics.queued.WithLabelValues("root")))
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

	t.Run("IncompleteOwnershipIsQueued", func(t *testing.T) {
		t.Parallel()
		f := newWorkerTestFixture(t)
		occupied := f.createRunningChat(t)
		acquireChat(t, f, occupied.ID, uuid.New(), uuid.New())
		chat := f.createRunningChat(t)
		acquireChat(t, f, chat.ID, uuid.New(), uuid.New())
		_, err := f.sqlDB.ExecContext(testutil.Context(t, testutil.WaitLong), `UPDATE chats SET worker_id = NULL WHERE id = $1`, chat.ID)
		require.NoError(t, err)
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
	server.agentCapacityPolicy = uncapped
	queued, err = server.ChatQueuedForCapacity(ctx, waiting)
	require.NoError(t, err)
	require.False(t, queued, "uncapped deployments must never report queued")

	server.agentCapacityPolicy = newFakeAdmission()
	queued, err = server.ChatQueuedForCapacity(ctx, waiting)
	require.NoError(t, err)
	require.True(t, queued)

	queued, err = server.ChatQueuedForCapacity(ctx, occupied)
	require.NoError(t, err)
	require.False(t, queued, "owned chats are active, not queued")
}

func TestChatCapacityCountsByPool(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	owned := f.createRunningChat(t)
	acquireChat(t, f, owned.ID, uuid.New(), uuid.New())
	f.createRunningChat(t)
	f.createRunningSubagentChat(t, owned.ID)
	incomplete := f.createRunningChat(t)
	acquireChat(t, f, incomplete.ID, uuid.New(), uuid.New())
	_, err := f.sqlDB.ExecContext(ctx, `UPDATE chats SET worker_id = NULL WHERE id = $1`, incomplete.ID)
	require.NoError(t, err)

	active, err := f.db.CountChatCapacityActiveByPool(ctx, database.CountChatCapacityActiveByPoolParams{StaleSeconds: 30})
	require.NoError(t, err)
	require.EqualValues(t, 1, active.ActiveRootCount)
	require.EqualValues(t, 0, active.ActiveSubagentCount)

	queued, err := f.db.CountChatCapacityQueuedByPool(ctx, 30)
	require.NoError(t, err)
	require.EqualValues(t, 2, queued.QueuedRootCount)
	require.EqualValues(t, 1, queued.QueuedSubagentCount)

	active, err = f.db.CountChatCapacityActiveByPool(ctx, database.CountChatCapacityActiveByPoolParams{
		ExcludeChatID: owned.ID,
		StaleSeconds:  30,
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, active.ActiveRootCount, "the excluded chat must not count as active")
}

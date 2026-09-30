package chatd

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// refusalCountingLimiter wraps the production limiter and records refusals.
type refusalCountingLimiter struct {
	*agentCapacityLimiter
	mu      sync.Mutex
	refused map[uuid.UUID]int
}

func newRefusalCountingLimiter(rootCapacity int64) *refusalCountingLimiter {
	inner := newAgentCapacityLimiter(nil, 30)
	inner.rootCapacity = rootCapacity
	return &refusalCountingLimiter{agentCapacityLimiter: inner, refused: map[uuid.UUID]int{}}
}

func (l *refusalCountingLimiter) Admit(ctx context.Context, store database.Store, chat database.Chat) (bool, error) {
	admitted, err := l.agentCapacityLimiter.Admit(ctx, store, chat)
	if err == nil && !admitted {
		l.mu.Lock()
		l.refused[chat.ID]++
		l.mu.Unlock()
	}
	return admitted, err
}

func (l *refusalCountingLimiter) refusals(chatID uuid.UUID) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.refused[chatID]
}

// partitionableLimiter fails every admission while its replica is cut off
// from the database, so that replica acquires nothing.
type partitionableLimiter struct {
	*refusalCountingLimiter
	partitioned atomic.Bool
}

func (l *partitionableLimiter) Admit(ctx context.Context, store database.Store, chat database.Chat) (bool, error) {
	if l.partitioned.Load() {
		return false, xerrors.New("replica is partitioned from the database")
	}
	return l.refusalCountingLimiter.Admit(ctx, store, chat)
}

// runningFreshLeases counts chats that are running and hold a fresh lease
// for their current runner, the quantity the admission limit bounds.
func runningFreshLeases(ctx context.Context, t *testing.T, db database.Store, chatIDs ...uuid.UUID) int {
	t.Helper()
	n := 0
	for _, id := range chatIDs {
		chat, err := db.GetChatByID(ctx, id)
		require.NoError(t, err)
		if chat.Status != database.ChatStatusRunning || !chat.WorkerID.Valid || !chat.RunnerID.Valid {
			continue
		}
		stale, err := db.IsChatHeartbeatStale(ctx, database.IsChatHeartbeatStaleParams{
			ChatID:       id,
			RunnerID:     chat.RunnerID.UUID,
			StaleSeconds: 30,
		})
		require.NoError(t, err)
		if !stale {
			n++
		}
	}
	return n
}

// releaseCapacitySlot ends the chat's turn and clears its ownership, the
// way a runner frees its slot when a turn finishes.
func releaseCapacitySlot(t *testing.T, f *workerTestFixture, chatID uuid.UUID) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chatID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		if _, err := tx.FinishTurn(chatstate.FinishTurnInput{}); err != nil {
			return err
		}
		_, err := tx.Abandon(chatstate.AbandonInput{})
		return err
	}))
}

// A chat waiting for capacity is interrupted with a queued message. It must
// not be acquired until a slot frees; otherwise FinishInterruption promotes
// the message into a running turn beyond the pool capacity.
func TestAdmission_InterruptedWaitingChatStaysWithinCapacity(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	limiter := newRefusalCountingLimiter(1)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.AgentCapacityLimiter = limiter

	// Another replica holds the only root slot.
	occupant := f.createRunningChat(t)
	acquireChat(t, f, occupant.ID, uuid.New(), uuid.New())

	waiting := f.createRunningChat(t)
	worker := startWorker(t, opts)
	require.Eventually(t, func() bool {
		return limiter.refusals(waiting.ID) > 0
	}, testutil.WaitLong, testutil.IntervalFast, "the waiting chat must first be refused for capacity")

	// The user sends a message with busy_behavior=interrupt.
	interrupted := interruptChat(t, f, waiting.ID)
	require.Equal(t, database.ChatStatusInterrupting, interrupted.Status)
	refusalsBefore := limiter.refusals(waiting.ID)
	worker.Wake()
	require.Eventually(t, func() bool {
		return limiter.refusals(waiting.ID) > refusalsBefore
	}, testutil.WaitLong, testutil.IntervalFast, "the interrupting chat must be refused while the pool is full")
	stillWaiting, err := f.db.GetChatByID(ctx, waiting.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusInterrupting, stillWaiting.Status)
	require.False(t, stillWaiting.WorkerID.Valid, "a refused chat stays unowned")
	require.Equal(t, 1, runningFreshLeases(ctx, t, f.db, occupant.ID, waiting.ID))

	// Freeing the slot lets the interruption finish and the promoted
	// message run within capacity.
	releaseCapacitySlot(t, f, occupant.ID)
	worker.Wake()
	call := starter.waitCall(t, taskKindInterrupt, waiting.ID)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, waiting.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		chat, err := store.GetChatByID(ctx, waiting.ID)
		if err != nil {
			return err
		}
		require.True(t, ownedByTask(chat, call.input), "the interrupt task owns the chat")
		_, err = tx.FinishInterruption(chatstate.FinishInterruptionInput{})
		return err
	}))
	starter.waitCall(t, taskKindGeneration, waiting.ID)
	require.Equal(t, 1, runningFreshLeases(ctx, t, f.db, occupant.ID, waiting.ID))
}

// A requires_action chat whose replica died must not be taken over while
// another chat holds its slot; otherwise resolving the action returns it to
// running beyond the pool capacity.
func TestAdmission_RequiresActionTakeoverStaysWithinCapacity(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	limiter := newRefusalCountingLimiter(1)
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.AgentCapacityLimiter = limiter

	// The requires_action chat was owned by a replica that died.
	pending := f.createRequiresActionChat(t)
	deadRunner := uuid.New()
	acquireChat(t, f, pending.ID, uuid.New(), deadRunner)
	makeHeartbeatStale(t, f, pending.ID, deadRunner)

	// Another replica holds the only root slot.
	occupant := f.createRunningChat(t)
	acquireChat(t, f, occupant.ID, uuid.New(), uuid.New())

	worker := startWorker(t, opts)
	require.Eventually(t, func() bool {
		return limiter.refusals(pending.ID) > 0
	}, testutil.WaitLong, testutil.IntervalFast, "the requires_action takeover must be refused while the pool is full")
	stillPending, err := f.db.GetChatByID(ctx, pending.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusRequiresAction, stillPending.Status)
	require.Equal(t, deadRunner, stillPending.RunnerID.UUID, "a refused takeover keeps the stale owner")
	require.Equal(t, 1, runningFreshLeases(ctx, t, f.db, occupant.ID, pending.ID))

	// Freeing the slot lets a worker take the chat over, and resolving the
	// action continues within capacity.
	releaseCapacitySlot(t, f, occupant.ID)
	worker.Wake()
	call := starter.waitCall(t, taskKindRequiresActionTimeout, pending.ID)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, pending.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		chat, err := store.GetChatByID(ctx, pending.ID)
		if err != nil {
			return err
		}
		require.True(t, ownedByTask(chat, call.input), "the worker owns the chat")
		_, err = tx.Interrupt(chatstate.InterruptInput{})
		return err
	}))
	starter.waitCall(t, taskKindGeneration, pending.ID)
	require.Equal(t, 1, runningFreshLeases(ctx, t, f.db, occupant.ID, pending.ID))
}

// Replica one runs chat A and loses the database for longer than the stale
// threshold, so replica two admits chat B into the only root slot. When
// replica one returns, its heartbeat must not revive A's stale lease, and
// A's runner must stop generating.
func TestRunnerManager_HeartbeatDoesNotReviveStaleLease(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	limiterOne := &partitionableLimiter{refusalCountingLimiter: newRefusalCountingLimiter(1)}
	// Generation tasks block until canceled, like a turn in progress.
	starterOne := newBlockingTaskStarter(false)
	optsOne := testOptions(t, f, starterOne)
	optsOne.AgentCapacityLimiter = limiterOne

	// Replica one admits A and starts generating.
	chatA := f.createRunningChat(t)
	replicaOne := startWorker(t, optsOne)
	generationA := starterOne.waitCall(t, taskKindGeneration, chatA.ID)

	// B waits for capacity.
	chatB := f.createRunningChat(t)
	replicaOne.Wake()
	require.Eventually(t, func() bool {
		return limiterOne.refusals(chatB.ID) > 0
	}, testutil.WaitLong, testutil.IntervalFast, "B must first be refused for capacity")

	// A user queues a message on A, which moves A behind B in the
	// acquisition order (updated_at).
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chatA.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.SendMessage(chatstate.SendMessageInput{
			Message:      userTextMessage(t, "next", f.user.ID, f.model.ID, f.apiKey.ID),
			BusyBehavior: chatstate.BusyBehaviorQueue,
			MaxQueueSize: codersdk.DefaultChatMaxQueuedMessagesPerChat,
		})
		return err
	}))

	// Replica one loses the database for longer than the stale threshold.
	limiterOne.partitioned.Store(true)
	ownedA, err := f.db.GetChatByID(ctx, chatA.ID)
	require.NoError(t, err)
	require.True(t, ownedA.RunnerID.Valid)
	makeHeartbeatStale(t, f, chatA.ID, ownedA.RunnerID.UUID)

	// Replica two admits B, then refuses A because B holds the slot.
	limiterTwo := newRefusalCountingLimiter(1)
	starterTwo := newRecordingTaskStarter()
	optsTwo := testOptions(t, f, starterTwo)
	optsTwo.AgentCapacityLimiter = limiterTwo
	startWorker(t, optsTwo)
	starterTwo.waitCall(t, taskKindGeneration, chatB.ID)
	require.Eventually(t, func() bool {
		return limiterTwo.refusals(chatA.ID) > 0
	}, testutil.WaitLong, testutil.IntervalFast, "replica two must refuse A")
	require.Equal(t, 1, runningFreshLeases(ctx, t, f.db, chatA.ID, chatB.ID))

	// Replica one reconnects and runs one heartbeat tick.
	limiterOne.partitioned.Store(false)
	replicaOne.mu.Lock()
	managerOne := replicaOne.manager
	replicaOne.mu.Unlock()
	require.NoError(t, managerOne.heartbeatOnce(ctx))

	stillA, err := f.db.GetChatByID(ctx, chatA.ID)
	require.NoError(t, err)
	require.Equal(t, ownedA.RunnerID, stillA.RunnerID, "A keeps its original runner")
	require.Equal(t, 1, runningFreshLeases(ctx, t, f.db, chatA.ID, chatB.ID),
		"the stale lease must not be renewed while B holds the slot")
	select {
	case <-generationA.ctx.Done():
	case <-ctx.Done():
		t.Fatal("the runner with the lost lease must stop generating")
	}
	require.Eventually(t, func() bool {
		managerOne.mu.Lock()
		defer managerOne.mu.Unlock()
		_, ok := managerOne.runners[runnerKey{ChatID: chatA.ID, RunnerID: ownedA.RunnerID.UUID}]
		return !ok
	}, testutil.WaitLong, testutil.IntervalFast, "the runner with the lost lease must be cleaned up")
}

func setHeartbeatAge(t *testing.T, f *workerTestFixture, chatID uuid.UUID, runnerID uuid.UUID, age time.Duration) {
	t.Helper()
	_, err := f.sqlDB.ExecContext(
		testutil.Context(t, testutil.WaitShort),
		`UPDATE chat_heartbeats SET heartbeat_at = NOW() - ($3 * INTERVAL '1 millisecond') WHERE chat_id = $1 AND runner_id = $2`,
		chatID, runnerID, age.Milliseconds(),
	)
	require.NoError(t, err)
}

func TestRenewChatHeartbeats_OnlyRenewsFreshOwnedLeases(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	fresh := f.createRunningChat(t)
	freshRunner := uuid.New()
	acquireChat(t, f, fresh.ID, uuid.New(), freshRunner)
	setHeartbeatAge(t, f, fresh.ID, freshRunner, 10*time.Second)

	stale := f.createRunningChat(t)
	staleRunner := uuid.New()
	acquireChat(t, f, stale.ID, uuid.New(), staleRunner)
	makeHeartbeatStale(t, f, stale.ID, staleRunner)

	// The previous runner's row is still fresh, but the chat now belongs
	// to another runner.
	replaced := f.createRunningChat(t)
	previousRunner := uuid.New()
	acquireChat(t, f, replaced.ID, uuid.New(), previousRunner)
	acquireChat(t, f, replaced.ID, uuid.New(), uuid.New())

	// The heartbeat row was deleted.
	deleted := f.createRunningChat(t)
	deletedRunner := uuid.New()
	acquireChat(t, f, deleted.ID, uuid.New(), deletedRunner)
	_, err := f.sqlDB.ExecContext(ctx, `DELETE FROM chat_heartbeats WHERE chat_id = $1`, deleted.ID)
	require.NoError(t, err)

	renewed, err := f.db.RenewChatHeartbeats(ctx, database.RenewChatHeartbeatsParams{
		ChatIds:      []uuid.UUID{fresh.ID, stale.ID, replaced.ID, deleted.ID},
		RunnerIds:    []uuid.UUID{freshRunner, staleRunner, previousRunner, deletedRunner},
		StaleSeconds: 30,
	})
	require.NoError(t, err)
	require.Equal(t, []database.RenewChatHeartbeatsRow{{ChatID: fresh.ID, RunnerID: freshRunner}}, renewed)

	stillStale, err := f.db.IsChatHeartbeatStale(ctx, database.IsChatHeartbeatStaleParams{
		ChatID: stale.ID, RunnerID: staleRunner, StaleSeconds: 30,
	})
	require.NoError(t, err)
	require.True(t, stillStale, "a stale lease must not be revived")
	_, err = f.db.GetChatHeartbeat(ctx, database.GetChatHeartbeatParams{ChatID: deleted.ID, RunnerID: deletedRunner})
	require.ErrorIs(t, err, sql.ErrNoRows, "renewal must not recreate a deleted lease")
}

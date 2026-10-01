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

// Stopping a chat that waits for capacity must not wait for a slot: the
// chat has no runner, so the interruption finishes inline and the chat
// lands in waiting without ever being acquired.
func TestAdmission_StopWaitingChatNeedsNoCapacity(t *testing.T) {
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

	// The user presses stop.
	machine := chatstate.NewChatMachine(f.db, f.pubsub, waiting.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.Interrupt(chatstate.InterruptInput{})
		return err
	}))
	stopped, err := f.db.GetChatByID(ctx, waiting.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, stopped.Status, "stop must not wait for a capacity slot")
	require.False(t, stopped.WorkerID.Valid, "a stopped chat needs no worker")

	// A waiting chat is not runnable, so no worker picks it up.
	worker.Wake()
	starter.assertNoCall(t)
	stopped, err = f.db.GetChatByID(ctx, waiting.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, stopped.Status)
	require.False(t, stopped.WorkerID.Valid)
	require.Equal(t, 1, runningFreshLeases(ctx, t, f.db, occupant.ID, waiting.ID))
}

// A chat waiting for capacity is interrupted with a new message. The
// message is promoted inline, but the chat stays unowned and must not be
// admitted while the pool is full; otherwise it would run beyond the pool
// capacity.
func TestAdmission_InterruptWithMessageOnWaitingChatStaysWithinCapacity(t *testing.T) {
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
	require.Equal(t, database.ChatStatusRunning, interrupted.Status, "the message is promoted without a runner")
	require.False(t, interrupted.WorkerID.Valid)
	refusalsBefore := limiter.refusals(waiting.ID)
	worker.Wake()
	require.Eventually(t, func() bool {
		return limiter.refusals(waiting.ID) > refusalsBefore
	}, testutil.WaitLong, testutil.IntervalFast, "the promoted turn must be refused while the pool is full")
	stillWaiting, err := f.db.GetChatByID(ctx, waiting.ID)
	require.NoError(t, err)
	require.False(t, stillWaiting.WorkerID.Valid, "a refused chat stays unowned")
	require.Equal(t, 1, runningFreshLeases(ctx, t, f.db, occupant.ID, waiting.ID))

	// Freeing the slot lets the promoted turn run within capacity.
	releaseCapacitySlot(t, f, occupant.ID)
	worker.Wake()
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

// holdCapacityAdmissionLock holds the capacity admission lock in another
// transaction until the returned release function is called.
func holdCapacityAdmissionLock(t *testing.T, db database.Store) (release func()) {
	t.Helper()
	locked := make(chan struct{})
	done := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- db.InTx(func(tx database.Store) error {
			if err := tx.AcquireLock(context.Background(), database.LockIDChatCapacityAdmission); err != nil {
				return err
			}
			close(locked)
			<-done
			return nil
		}, nil)
	}()
	select {
	case <-locked:
	case err := <-errCh:
		t.Fatalf("hold capacity admission lock: %v", err)
	case <-time.After(testutil.WaitShort):
		t.Fatal("timed out acquiring the capacity admission lock")
	}
	var once sync.Once
	release = func() {
		once.Do(func() {
			close(done)
			require.NoError(t, <-errCh)
		})
	}
	t.Cleanup(release)
	return release
}

// A heartbeat tick that fails or times out, for example because a stalled
// transaction holds the capacity admission lock, says nothing about
// individual leases. It must fail the tick without cleaning up runners, and
// the next tick must renew normally.
func TestRunnerManager_FailedHeartbeatTickKeepsRunners(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name           string
		renewalTimeout time.Duration
		lockTimeout    time.Duration
		wantErr        string
	}{
		{name: "LockTimeout", renewalTimeout: testutil.WaitShort, lockTimeout: 100 * time.Millisecond, wantErr: "due to lock timeout"},
		{name: "RenewalTimeout", renewalTimeout: 200 * time.Millisecond, lockTimeout: time.Minute, wantErr: "acquire capacity admission lock"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newWorkerTestFixture(t)
			ctx := testutil.Context(t, testutil.WaitLong)

			// Generation tasks block until canceled, like a turn in progress.
			starter := newBlockingTaskStarter(false)
			opts := testOptions(t, f, starter)
			opts.HeartbeatRenewalTimeout = tc.renewalTimeout
			opts.HeartbeatLockTimeout = tc.lockTimeout
			chat := f.createRunningChat(t)
			worker := startWorker(t, opts)
			generation := starter.waitCall(t, taskKindGeneration, chat.ID)
			key := runnerKey{ChatID: chat.ID, RunnerID: generation.input.RunnerID}
			setHeartbeatAge(t, f, key.ChatID, key.RunnerID, 10*time.Second)
			before, err := f.db.GetChatHeartbeat(ctx, database.GetChatHeartbeatParams{ChatID: key.ChatID, RunnerID: key.RunnerID})
			require.NoError(t, err)

			worker.mu.Lock()
			manager := worker.manager
			worker.mu.Unlock()
			registered := func() bool {
				manager.mu.Lock()
				defer manager.mu.Unlock()
				_, ok := manager.runners[key]
				return ok
			}

			release := holdCapacityAdmissionLock(t, f.db)
			require.ErrorContains(t, manager.heartbeatOnce(ctx), tc.wantErr, "the tick must fail while the lock is held")
			require.True(t, registered(), "a failed tick must not clean up runners")
			require.NoError(t, generation.ctx.Err(), "a failed tick must not stop generation")
			unchanged, err := f.db.GetChatHeartbeat(ctx, database.GetChatHeartbeatParams{ChatID: key.ChatID, RunnerID: key.RunnerID})
			require.NoError(t, err)
			require.True(t, unchanged.HeartbeatAt.Equal(before.HeartbeatAt), "a failed tick renews nothing")

			release()
			require.NoError(t, manager.heartbeatOnce(ctx))
			require.True(t, registered())
			require.NoError(t, generation.ctx.Err())
			renewed, err := f.db.GetChatHeartbeat(ctx, database.GetChatHeartbeatParams{ChatID: key.ChatID, RunnerID: key.RunnerID})
			require.NoError(t, err)
			require.True(t, renewed.HeartbeatAt.After(before.HeartbeatAt), "the next tick renews the lease")
		})
	}
}

// admitObservingLimiter records when admissions start and finish.
type admitObservingLimiter struct {
	*agentCapacityLimiter
	started  atomic.Int64
	finished atomic.Int64
}

func (l *admitObservingLimiter) Admit(ctx context.Context, store database.Store, chat database.Chat) (bool, error) {
	l.started.Add(1)
	defer l.finished.Add(1)
	return l.agentCapacityLimiter.Admit(ctx, store, chat)
}

// A takeover reads the lease as stale before it takes the admission lock.
// A heartbeat renewal that holds the lock at that moment can still renew
// the lease, so the takeover must recheck the lease under the lock instead
// of replacing a runner whose lease was just renewed.
func TestAdmission_TakeoverRechecksLeaseUnderAdmissionLock(t *testing.T) {
	t.Parallel()
	f := newWorkerTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	chat := f.createRunningChat(t)
	liveRunner := uuid.New()
	acquireChat(t, f, chat.ID, uuid.New(), liveRunner)
	// Old enough that the takeover's 30 second threshold sees it as stale.
	setHeartbeatAge(t, f, chat.ID, liveRunner, 45*time.Second)

	// The live runner's renewal saw the lease as fresh (a 60 second window
	// stands in for a renewal statement that started just before expiry)
	// and holds the admission lock while it commits.
	renewed := make(chan struct{})
	commit := make(chan struct{})
	renewErr := make(chan error, 1)
	go func() {
		renewErr <- f.db.InTx(func(tx database.Store) error {
			if err := tx.AcquireLock(ctx, database.LockIDChatCapacityAdmission); err != nil {
				return err
			}
			rows, err := tx.RenewChatHeartbeats(ctx, database.RenewChatHeartbeatsParams{
				ChatIds:      []uuid.UUID{chat.ID},
				RunnerIds:    []uuid.UUID{liveRunner},
				StaleSeconds: 60,
			})
			if err != nil {
				return err
			}
			if len(rows) != 1 {
				return xerrors.Errorf("renewed %d leases, want 1", len(rows))
			}
			close(renewed)
			<-commit
			return nil
		}, nil)
	}()
	select {
	case <-renewed:
	case err := <-renewErr:
		t.Fatalf("renew: %v", err)
	case <-ctx.Done():
		t.Fatal("timed out renewing the lease")
	}

	limiter := &admitObservingLimiter{agentCapacityLimiter: newAgentCapacityLimiter(nil, 30)}
	starter := newRecordingTaskStarter()
	opts := testOptions(t, f, starter)
	opts.AgentCapacityLimiter = limiter
	startWorker(t, opts)
	// The worker classified the lease as stale and now waits for the lock.
	require.Eventually(t, func() bool {
		return limiter.started.Load() > 0
	}, testutil.WaitLong, testutil.IntervalFast, "the takeover must reach admission")

	close(commit)
	require.NoError(t, <-renewErr)
	require.Eventually(t, func() bool {
		return limiter.finished.Load() > 0
	}, testutil.WaitLong, testutil.IntervalFast)

	// Give the acquisition transaction time to commit if it proceeds.
	require.Never(t, func() bool {
		got, err := f.db.GetChatByID(ctx, chat.ID)
		return err != nil || got.RunnerID.UUID != liveRunner
	}, time.Second, testutil.IntervalFast, "a takeover must not replace a runner whose lease was renewed under the lock")
	starter.assertNoCall(t)
}

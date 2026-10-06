package chatstate_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/testutil"
)

// waitForChan returns true if c receives a value before ctx is done.
// Helper used in concurrency tests to avoid time.Sleep.
func waitForChan(ctx context.Context, c <-chan struct{}) bool {
	select {
	case <-c:
		return true
	case <-ctx.Done():
		return false
	}
}

// waitForWaitGroup returns true if wg completes before ctx is done.
func waitForWaitGroup(ctx context.Context, wg *sync.WaitGroup) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	return waitForChan(ctx, done)
}

// TestConcurrentUpdatesSerializeOnChatRow verifies that two
// goroutines racing to Update the same chat both succeed but their
// effects serialize on the chat row lock: snapshot_version advances
// by exactly N (one per Update) and each transition observes the
// effects of the prior one.
func TestConcurrentUpdatesSerializeOnChatRow(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitMedium)
	created := createTestChat(t, f)
	before := f.readChat(ctx, t, created.Chat.ID)

	const updates = 8
	var wg sync.WaitGroup
	wg.Add(updates)
	errs := make([]error, updates)
	for i := 0; i < updates; i++ {
		i := i
		go func() {
			defer wg.Done()
			m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
			errs[i] = m.Update(ctx, func(_ *chatstate.Tx, _ database.Store) error { return nil })
		}()
	}
	wg.Wait()
	for i, err := range errs {
		require.NoError(t, err, "concurrent update %d failed", i)
	}
	after := f.readChat(ctx, t, created.Chat.ID)
	require.Equal(t, before.SnapshotVersion+int64(updates), after.SnapshotVersion,
		"snapshot_version advanced by exactly one per update")
}

// TestReadSnapshotNotBlockedByRowLock verifies that a ReadSnapshot
// completes while an Update holds the chat row's transition lock. The
// former FOR SHARE read would have queued behind it.
func TestReadSnapshotNotBlockedByRowLock(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitMedium)
	created := createTestChat(t, f)
	locker := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
	reader := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	lockEntered := make(chan struct{})
	releaseLock := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-releaseLock:
		default:
			close(releaseLock)
		}
	})

	// Goroutine A: hold the row lock and block.
	var lockErr error
	var lockWG sync.WaitGroup
	lockWG.Go(func() {
		lockErr = locker.Update(ctx, func(_ *chatstate.Tx, _ database.Store) error {
			close(lockEntered)
			if !waitForChan(ctx, releaseLock) {
				return ctx.Err()
			}
			return nil
		})
	})
	require.True(t, waitForChan(ctx, lockEntered), "Update callback never started")

	// The read must complete without the lock being released.
	var read database.Chat
	var readErr error
	var readWG sync.WaitGroup
	readWG.Go(func() {
		readErr = reader.ReadSnapshot(func(store database.Store) error {
			var err error
			read, err = store.GetChatByID(ctx, created.Chat.ID)
			return err
		})
	})
	require.True(t, waitForWaitGroup(ctx, &readWG), "ReadSnapshot blocked behind the row lock")
	require.NoError(t, readErr)
	require.Equal(t, created.Chat.ID, read.ID)

	close(releaseLock)
	require.True(t, waitForWaitGroup(ctx, &lockWG), "Update did not finish")
	require.NoError(t, lockErr)
}

// TestHeartbeatUpsertNotBlockedByRowLock verifies that the transition
// lock admits foreign key child writes but still serializes transitions.
// Inserting a heartbeat row takes FOR KEY SHARE on the chat row for its
// foreign key check, which FOR UPDATE would block.
func TestHeartbeatUpsertNotBlockedByRowLock(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitMedium)
	created := createTestChat(t, f)
	locker := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
	waiter := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	lockEntered := make(chan struct{})
	releaseLock := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-releaseLock:
		default:
			close(releaseLock)
		}
	})

	var lockErr error
	var lockWG sync.WaitGroup
	lockWG.Go(func() {
		lockErr = locker.Update(ctx, func(_ *chatstate.Tx, _ database.Store) error {
			close(lockEntered)
			if !waitForChan(ctx, releaseLock) {
				return ctx.Err()
			}
			return nil
		})
	})
	require.True(t, waitForChan(ctx, lockEntered), "Update callback never started")

	// A new runner ID makes the upsert an insert, which runs the foreign
	// key check.
	var heartbeatErr error
	var heartbeatWG sync.WaitGroup
	heartbeatWG.Go(func() {
		heartbeatErr = f.DB.UpsertChatHeartbeat(ctx, database.UpsertChatHeartbeatParams{
			ChatID:   created.Chat.ID,
			RunnerID: uuid.New(),
		})
	})
	require.True(t, waitForWaitGroup(ctx, &heartbeatWG), "heartbeat upsert blocked behind the row lock")
	require.NoError(t, heartbeatErr)

	var waiterErr error
	var waiterWG sync.WaitGroup
	waiterWG.Go(func() {
		waiterErr = waiter.Update(ctx, func(_ *chatstate.Tx, _ database.Store) error { return nil })
	})
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		var waiting int
		err := f.SQLDB.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM pg_stat_activity
WHERE datname = current_database()
	AND pid <> pg_backend_pid()
	AND wait_event_type = 'Lock'
	AND query LIKE '%-- name: LockChatForTransition%'
`).Scan(&waiting)
		return err == nil && waiting == 1
	}, testutil.IntervalFast, "wait for the second Update to block on the row lock")
	require.NoError(t, ctx.Err(), "waiting for the second Update to block")

	close(releaseLock)
	require.True(t, waitForWaitGroup(ctx, &lockWG), "first Update did not finish")
	require.NoError(t, lockErr)
	require.True(t, waitForWaitGroup(ctx, &waiterWG), "second Update did not finish")
	require.NoError(t, waiterErr)
}

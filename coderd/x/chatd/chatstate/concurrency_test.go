package chatstate_test

import (
	"context"
	"sync"
	"testing"

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
// completes while an Update holds the chat row's transition lock. The former FOR SHARE read would have queued behind it.
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

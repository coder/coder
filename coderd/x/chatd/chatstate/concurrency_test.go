package chatstate_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

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

// stillBlocked returns true if c has not received a value and has not
// been closed. The caller must already have established a happens-before
// ordering via another channel so this check is meaningful.
func stillBlocked(c <-chan struct{}) bool {
	select {
	case <-c:
		return false
	default:
		return true
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

type lockAttemptStore struct {
	database.Store

	attempted chan struct{}
	once      *sync.Once
}

func newLockAttemptStore(store database.Store, attempted chan struct{}) *lockAttemptStore {
	return &lockAttemptStore{
		Store:     store,
		attempted: attempted,
		once:      new(sync.Once),
	}
}

func (s *lockAttemptStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		return fn(&lockAttemptStore{
			Store:     tx,
			attempted: s.attempted,
			once:      s.once,
		})
	}, opts)
}

func (s *lockAttemptStore) LockChatAndBumpSnapshotVersion(ctx context.Context, id uuid.UUID) (database.Chat, error) {
	s.once.Do(func() { close(s.attempted) })
	return s.Store.LockChatAndBumpSnapshotVersion(ctx, id)
}

// TestLockLocksChatRow verifies that ChatMachine.Lock holds the chat
// row's FOR UPDATE lock until the callback returns, so a concurrent
// ChatMachine.Update cannot enter its callback until the Lock
// callback releases.
func TestLockLocksChatRow(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitMedium)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
	updateLockAttempted := make(chan struct{})
	updateMachine := chatstate.NewChatMachine(
		newLockAttemptStore(f.DB, updateLockAttempted),
		f.Pub,
		created.Chat.ID,
	)

	lockEntered := make(chan struct{})
	releaseLock := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-releaseLock:
		default:
			close(releaseLock)
		}
	})
	updateEntered := make(chan struct{})

	// Goroutine A: hold a Lock and block.
	var lockErr error
	var lockWG sync.WaitGroup
	lockWG.Go(func() {
		lockErr = m.Lock(ctx, func(_ database.Store) error {
			close(lockEntered)
			select {
			case <-releaseLock:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	})

	// Wait until A is inside its Lock callback (and therefore holds
	// the FOR UPDATE lock).
	require.True(t, waitForChan(ctx, lockEntered), "Lock callback never started")

	// Goroutine B: try to Update the same chat. It must block on
	// LockChatAndBumpSnapshotVersion until A releases.
	var updateErr error
	var updateWG sync.WaitGroup
	updateWG.Go(func() {
		updateErr = updateMachine.Update(ctx, func(_ *chatstate.Tx, _ database.Store) error {
			close(updateEntered)
			return nil
		})
	})

	require.True(t, waitForChan(ctx, updateLockAttempted),
		"Update never attempted to lock the chat row")
	// Sleep to give a chance for the update to enter the callback.
	// This isn't a deterministic solution - on a low resource, contended system
	// it's possible that the Update won't call the callback even if the lock
	// implementation is incorrect and doesn't block. But in most cases, this wait should be enough.
	time.Sleep(50 * time.Millisecond)
	require.True(t, stillBlocked(updateEntered),
		"Update entered while Lock was still held")

	// Release Lock and confirm Update completes successfully.
	close(releaseLock)
	require.True(t, waitForChan(ctx, updateEntered),
		"Update callback never started after Lock released")
	require.True(t, waitForWaitGroup(ctx, &updateWG), "Update did not finish")
	require.True(t, waitForWaitGroup(ctx, &lockWG), "Lock did not finish")
	require.NoError(t, lockErr)
	require.NoError(t, updateErr)
}

// TestLockRollsBackCallbackError verifies that a Lock callback
// returning an error rolls back the surrounding transaction.
func TestLockRollsBackCallbackError(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	before := f.readChat(ctx, t, created.Chat.ID)
	publishedBefore := len(f.Pub.channels)

	sentinel := xerrors.New("lock callback error")
	err := m.Lock(ctx, func(store database.Store) error {
		// Try a write that should be rolled back.
		_, werr := store.UpdateChatByID(ctx, database.UpdateChatByIDParams{
			ID:    created.Chat.ID,
			Title: "rollback-me",
		})
		require.NoError(t, werr)
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)

	after := f.readChat(ctx, t, created.Chat.ID)
	require.Equal(t, before.Title, after.Title, "Lock callback error rolls back writes")
	require.Equal(t, publishedBefore, len(f.Pub.channels), "Lock publishes nothing on error")
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

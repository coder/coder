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
		_, werr := store.UpdateChatTitleByID(ctx, database.UpdateChatTitleByIDParams{
			ID:          created.Chat.ID,
			Title:       "rollback-me",
			TitleSource: database.ChatTitleSourceUser,
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

// TestReadSnapshotDoesNotBlockWriters verifies the guarantee that
// replaced the FOR SHARE lock: an open ReadSnapshot neither blocks a
// concurrent Update nor observes its commit. The Update completes while
// the reader is still inside its callback, the reader keeps seeing the
// state from before the commit for the rest of the snapshot, and the
// commit is visible, and its chat:update published, for the next read.
func TestReadSnapshotDoesNotBlockWriters(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitMedium)
	created := createTestChat(t, f)
	reader := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
	writer := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	readStarted := make(chan struct{})
	writerDone := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-writerDone:
		default:
			close(writerDone)
		}
	})

	// Goroutine A: open a snapshot, read the chat, then hold the
	// snapshot open until the writer has committed and read again.
	var first, second database.Chat
	var readErr error
	var readWG sync.WaitGroup
	readWG.Go(func() {
		readErr = reader.ReadSnapshot(func(store database.Store) error {
			var err error
			first, err = store.GetChatByID(ctx, created.Chat.ID)
			if err != nil {
				return err
			}
			close(readStarted)
			if !waitForChan(ctx, writerDone) {
				return ctx.Err()
			}
			second, err = store.GetChatByID(ctx, created.Chat.ID)
			return err
		})
	})
	require.True(t, waitForChan(ctx, readStarted), "ReadSnapshot callback never started")

	// Goroutine B: a transition on the same chat. It must commit while
	// A still holds its snapshot; with the former FOR SHARE lock it would
	// have waited for A to finish.
	publishedBefore := len(f.Pub.channels)
	var updateErr error
	var updateWG sync.WaitGroup
	updateWG.Go(func() {
		updateErr = writer.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
			_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
			return err
		})
	})
	require.True(t, waitForWaitGroup(ctx, &updateWG), "Update did not finish while the snapshot was open")
	require.NoError(t, updateErr)
	require.Greater(t, len(f.Pub.channels), publishedBefore, "the committed transition published its hint")

	close(writerDone)
	require.True(t, waitForWaitGroup(ctx, &readWG), "ReadSnapshot did not finish")
	require.NoError(t, readErr)

	// The snapshot is one consistent state: the commit is invisible to it.
	require.Equal(t, created.Chat.SnapshotVersion, first.SnapshotVersion)
	require.Equal(t, first.SnapshotVersion, second.SnapshotVersion, "a commit during the snapshot must not be visible to it")
	require.Equal(t, database.ChatStatusRunning, second.Status)

	// The next read observes the commit, which is what the stream loop's
	// hint-driven resync relies on.
	after := f.readChat(ctx, t, created.Chat.ID)
	require.Equal(t, created.Chat.SnapshotVersion+1, after.SnapshotVersion)
	require.Equal(t, database.ChatStatusWaiting, after.Status)
}

// TestReadSnapshotNotBlockedByRowLock verifies the other direction: a
// ReadSnapshot completes while another transaction holds the chat row's
// FOR UPDATE lock. The former FOR SHARE read would have queued behind it.
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
		lockErr = locker.Lock(ctx, func(_ database.Store) error {
			close(lockEntered)
			if !waitForChan(ctx, releaseLock) {
				return ctx.Err()
			}
			return nil
		})
	})
	require.True(t, waitForChan(ctx, lockEntered), "Lock callback never started")

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
	require.True(t, waitForWaitGroup(ctx, &lockWG), "Lock did not finish")
	require.NoError(t, lockErr)
}

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

// roundTripCounts records the chat reads issued inside a transaction so
// tests can assert what runs while the transition lock is held.
type roundTripCounts struct {
	mu              sync.Mutex
	bump            int
	transitionState int
	getChatByID     int
	countQueued     int
	heartbeatStale  int
}

func (c *roundTripCounts) inc(field *int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	*field++
}

// countingStore counts chat reads only on the transactional handle
// handed to InTx callbacks, so setup reads on the root store do not
// pollute the numbers.
type countingStore struct {
	database.Store
	counts *roundTripCounts
	inTx   bool
}

func (s *countingStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		return fn(&countingStore{Store: tx, counts: s.counts, inTx: true})
	}, opts)
}

func (s *countingStore) LockChatAndBumpSnapshotVersion(ctx context.Context, id uuid.UUID) (database.LockChatAndBumpSnapshotVersionRow, error) {
	if s.inTx {
		s.counts.inc(&s.counts.bump)
	}
	return s.Store.LockChatAndBumpSnapshotVersion(ctx, id)
}

func (s *countingStore) GetChatTransitionState(ctx context.Context, arg database.GetChatTransitionStateParams) (database.GetChatTransitionStateRow, error) {
	if s.inTx {
		s.counts.inc(&s.counts.transitionState)
	}
	return s.Store.GetChatTransitionState(ctx, arg)
}

func (s *countingStore) GetChatByID(ctx context.Context, id uuid.UUID) (database.Chat, error) {
	if s.inTx {
		s.counts.inc(&s.counts.getChatByID)
	}
	return s.Store.GetChatByID(ctx, id)
}

func (s *countingStore) CountChatQueuedMessages(ctx context.Context, id uuid.UUID) (int64, error) {
	if s.inTx {
		s.counts.inc(&s.counts.countQueued)
	}
	return s.Store.CountChatQueuedMessages(ctx, id)
}

func (s *countingStore) IsChatHeartbeatStale(ctx context.Context, arg database.IsChatHeartbeatStaleParams) (bool, error) {
	if s.inTx {
		s.counts.inc(&s.counts.heartbeatStale)
	}
	return s.Store.IsChatHeartbeatStale(ctx, arg)
}

// TestUpdateSingleTransitionReadsNothingUnderLock pins the round-trip
// budget of a transition: the bump seeds validation and one lean read
// feeds the publish, so no other chat read runs while the row lock is
// held. Reads creeping back here directly lengthen lock hold time.
func TestUpdateSingleTransitionReadsNothingUnderLock(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	counts := &roundTripCounts{}
	m := chatstate.NewChatMachine(&countingStore{Store: f.DB, counts: counts}, f.Pub, created.Chat.ID)

	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))

	require.Equal(t, 1, counts.bump)
	require.Equal(t, 1, counts.transitionState)
	require.Zero(t, counts.getChatByID, "GetChatByID must not run under the transition lock")
	require.Zero(t, counts.countQueued, "CountChatQueuedMessages must not run under the transition lock")
	require.Zero(t, counts.heartbeatStale, "IsChatHeartbeatStale must not run under the transition lock")

	after, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, after.Status)
}

// TestUpdateBundleRereadsAfterSeedConsumed proves the bump seed is
// single-use: the first transition validates against it, and the second
// transition in the same callback performs a real read so it observes the
// first transition's write rather than stale seeded state.
func TestUpdateBundleRereadsAfterSeedConsumed(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	counts := &roundTripCounts{}
	m := chatstate.NewChatMachine(&countingStore{Store: f.DB, counts: counts}, f.Pub, created.Chat.ID)

	// R0 -> W (FinishTurn) -> XW (SetArchived). SetArchived is only
	// valid from W, so it must see FinishTurn's write.
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		if _, err := tx.FinishTurn(chatstate.FinishTurnInput{}); err != nil {
			return err
		}
		_, err := tx.SetArchived(chatstate.SetArchivedInput{Archived: true})
		return err
	}))

	require.Equal(t, 1, counts.bump)
	require.Equal(t, 1, counts.transitionState)
	require.Equal(t, 1, counts.getChatByID, "second transition re-reads once the seed is consumed")
	require.Equal(t, 1, counts.countQueued)

	after, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.True(t, after.Archived)
	require.Equal(t, database.ChatStatusWaiting, after.Status)
}

// TestCurrentDoesNotConsumeSeed verifies a callback can inspect state via
// Current and then run a transition without either step reading the chat
// row again. This is the worker acquisition pattern.
func TestCurrentDoesNotConsumeSeed(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	counts := &roundTripCounts{}
	m := chatstate.NewChatMachine(&countingStore{Store: f.DB, counts: counts}, f.Pub, created.Chat.ID)

	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		chat, state, err := tx.Current()
		if err != nil {
			return err
		}
		require.Equal(t, created.Chat.ID, chat.ID)
		require.Equal(t, chatstate.StateR0, state)
		_, err = tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))

	require.Zero(t, counts.getChatByID)
	require.Zero(t, counts.countQueued)
}

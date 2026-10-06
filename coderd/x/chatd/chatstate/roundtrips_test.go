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
	mu             sync.Mutex
	lock           int
	chatWrites     int
	getChatByID    int
	countQueued    int
	heartbeatStale int
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

func (s *countingStore) LockChatForTransition(ctx context.Context, id uuid.UUID) (database.LockChatForTransitionRow, error) {
	if s.inTx {
		s.counts.inc(&s.counts.lock)
	}
	return s.Store.LockChatForTransition(ctx, id)
}

func (s *countingStore) UpdateChatExecutionState(ctx context.Context, arg database.UpdateChatExecutionStateParams) (database.Chat, error) {
	if s.inTx {
		s.counts.inc(&s.counts.chatWrites)
	}
	return s.Store.UpdateChatExecutionState(ctx, arg)
}

func (s *countingStore) BumpChatSnapshotVersion(ctx context.Context, arg database.BumpChatSnapshotVersionParams) (database.Chat, error) {
	if s.inTx {
		s.counts.inc(&s.counts.chatWrites)
	}
	return s.Store.BumpChatSnapshotVersion(ctx, arg)
}

func (s *countingStore) IncrementChatGenerationAttempt(ctx context.Context, id uuid.UUID) (int64, error) {
	if s.inTx {
		s.counts.inc(&s.counts.chatWrites)
	}
	return s.Store.IncrementChatGenerationAttempt(ctx, id)
}

func (s *countingStore) UpdateChatRetryState(ctx context.Context, arg database.UpdateChatRetryStateParams) (database.Chat, error) {
	if s.inTx {
		s.counts.inc(&s.counts.chatWrites)
	}
	return s.Store.UpdateChatRetryState(ctx, arg)
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

// TestUpdateSingleTransitionValidatesFromLockedRow pins the round-trip
// budget of a transition: validation uses the row the lock returned, so
// the only chat reads while the row lock is held are the publication
// reads that run after the commit write. Reads creeping back in before
// the commit write directly lengthen lock hold time.
func TestUpdateSingleTransitionValidatesFromLockedRow(t *testing.T) {
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

	require.Equal(t, 1, counts.lock)
	require.Equal(t, 1, counts.chatWrites, "a transition updates the chats row exactly once")
	require.Equal(t, 1, counts.getChatByID, "only the publication read runs under the transition lock")
	require.Equal(t, 1, counts.countQueued, "only the publication read runs under the transition lock")
	require.Zero(t, counts.heartbeatStale, "a non-runnable result publishes no ownership hint")

	after, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, after.Status)
	require.Equal(t, created.Chat.SnapshotVersion+1, after.SnapshotVersion)
}

// TestUpdateWritesChatRowOnce pins the single-commit-write contract for
// callback shapes that have no execution-state write of their own: a
// history-only transition ends in exactly one chats UPDATE, Update's
// bump-only write. A metadata-only callback also gets exactly one commit
// write, but its own raw metadata UPDATE is a second chats write that
// Update does not fold in, so the counts here cover commit writes only.
func TestUpdateWritesChatRowOnce(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	counts := &roundTripCounts{}
	m := chatstate.NewChatMachine(&countingStore{Store: f.DB, counts: counts}, f.Pub, created.Chat.ID)

	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		msg := userTextMessage("step", f.User.ID, f.Model.ID)
		msg.Role = database.ChatMessageRoleAssistant
		_, err := tx.CommitStep(chatstate.CommitStepInput{Messages: []chatstate.Message{msg}})
		return err
	}))
	require.Equal(t, 1, counts.chatWrites, "CommitStep commits through Update's bump-only write")

	afterStep, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, created.Chat.SnapshotVersion+1, afterStep.SnapshotVersion)
	require.Equal(t, afterStep.SnapshotVersion, afterStep.HistoryVersion, "the commit write records the history change")
	msgs, err := f.DB.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: created.Chat.ID})
	require.NoError(t, err)
	require.Equal(t, afterStep.SnapshotVersion, msgs[len(msgs)-1].Revision, "messages carry the committed version")

	counts.chatWrites = 0
	require.NoError(t, m.Update(ctx, func(_ *chatstate.Tx, store database.Store) error {
		_, err := store.UpdateChatTitleByID(ctx, database.UpdateChatTitleByIDParams{ID: created.Chat.ID, Title: "renamed", TitleSource: database.ChatTitleSourceUser})
		return err
	}))
	require.Equal(t, 1, counts.chatWrites, "a metadata-only callback still gets exactly one commit write")

	afterTitle, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.Equal(t, afterStep.SnapshotVersion+1, afterTitle.SnapshotVersion)
	require.Equal(t, afterStep.HistoryVersion, afterTitle.HistoryVersion, "no history change recorded")
}

// TestUpdateBundleRereadsAfterLockedRowConsumed proves the locked row is
// single-use: the first transition validates against it, and the second
// transition in the same callback performs a real read so it observes the
// first transition's write rather than the stale locked row.
func TestUpdateBundleRereadsAfterLockedRowConsumed(t *testing.T) {
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

	require.Equal(t, 1, counts.lock)
	require.Equal(t, 2, counts.getChatByID, "second transition re-reads once the locked row is consumed, then publication reads")
	require.Equal(t, 2, counts.countQueued)

	after, err := f.DB.GetChatByID(ctx, created.Chat.ID)
	require.NoError(t, err)
	require.True(t, after.Archived)
	require.Equal(t, database.ChatStatusWaiting, after.Status)
}

// TestCurrentDoesNotConsumeLockedRow verifies a callback can inspect
// state via Current and then run a transition without either step
// reading the chat row again; only the publication read follows the
// commit write. This is the worker acquisition pattern.
func TestCurrentDoesNotConsumeLockedRow(t *testing.T) {
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

	require.Equal(t, 1, counts.getChatByID, "only the publication read")
	require.Equal(t, 1, counts.countQueued, "only the publication read")
}

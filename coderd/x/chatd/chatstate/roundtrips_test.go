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

func (s *countingStore) LockChatForTransition(ctx context.Context, id uuid.UUID) (database.Chat, error) {
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
// budget of a transition: validation uses the row the lock returned and
// the queue count taken right after it, so the only other chat reads
// while the row lock is held are the publication reads that run after
// the commit write. Reads creeping back in before
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
	require.Equal(t, 2, counts.countQueued, "the post-lock count and the publication read")
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

// TestUpdateRejectsStagingAfterCommitWrite runs real transition bundles
// in one Update. Staging history or queue changes after a commit write
// must fail and roll back, since recording them would need a second
// chats UPDATE; staging before a commit write must keep working.
func TestUpdateRejectsStagingAfterCommitWrite(t *testing.T) {
	t.Parallel()

	acquire := func(tx *chatstate.Tx) error {
		_, err := tx.Acquire(chatstate.AcquireInput{WorkerID: uuid.New(), RunnerID: uuid.New()})
		return err
	}
	commitStep := func(f *testFixture) func(*chatstate.Tx) error {
		return func(tx *chatstate.Tx) error {
			msg := userTextMessage("step", f.User.ID, f.Model.ID)
			msg.Role = database.ChatMessageRoleAssistant
			_, err := tx.CommitStep(chatstate.CommitStepInput{Messages: []chatstate.Message{msg}})
			return err
		}
	}
	deleteQueued := func(id int64) func(*chatstate.Tx) error {
		return func(tx *chatstate.Tx) error {
			_, err := tx.DeleteQueuedMessage(chatstate.DeleteQueuedMessageInput{QueuedMessageID: id})
			return err
		}
	}
	finishTurn := func(tx *chatstate.Tx) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}

	tests := []struct {
		name string
		// steps builds the bundle; queuedID is a message queued on the
		// chat before the bundle runs.
		steps      func(f *testFixture, queuedID int64) []func(*chatstate.Tx) error
		wantErr    bool
		wantWrites int
	}{
		{
			name: "HistoryAfterCommit",
			steps: func(f *testFixture, _ int64) []func(*chatstate.Tx) error {
				return []func(*chatstate.Tx) error{acquire, commitStep(f)}
			},
			wantErr: true,
		},
		{
			name: "QueueAfterCommit",
			steps: func(_ *testFixture, queuedID int64) []func(*chatstate.Tx) error {
				return []func(*chatstate.Tx) error{acquire, deleteQueued(queuedID)}
			},
			wantErr: true,
		},
		{
			name: "StageThenCommit",
			steps: func(f *testFixture, _ int64) []func(*chatstate.Tx) error {
				return []func(*chatstate.Tx) error{commitStep(f), acquire}
			},
			wantWrites: 1,
		},
		{
			name: "CommitStageCommit",
			steps: func(f *testFixture, _ int64) []func(*chatstate.Tx) error {
				return []func(*chatstate.Tx) error{acquire, commitStep(f), finishTurn}
			},
			wantWrites: 2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newTestFixture(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			created := createTestChat(t, f)
			setup := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
			queued := sendQueuedMessage(t, f, setup, "queued")
			require.NotNil(t, queued.QueuedMessage)
			before, err := f.DB.GetChatByID(ctx, created.Chat.ID)
			require.NoError(t, err)

			counts := &roundTripCounts{}
			m := chatstate.NewChatMachine(&countingStore{Store: f.DB, counts: counts}, f.Pub, created.Chat.ID)
			err = m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
				for _, step := range tc.steps(f, queued.QueuedMessage.ID) {
					if err := step(tx); err != nil {
						return err
					}
				}
				return nil
			})

			after, getErr := f.DB.GetChatByID(ctx, created.Chat.ID)
			require.NoError(t, getErr)
			if tc.wantErr {
				require.ErrorIs(t, err, chatstate.ErrStagedAfterCommitWrite)
				require.Equal(t, before, after, "the bundle must roll back")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantWrites, counts.chatWrites)
			require.Equal(t, after.SnapshotVersion, after.HistoryVersion, "the staged history change is recorded")
			msgs, err := f.DB.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: created.Chat.ID})
			require.NoError(t, err)
			for _, msg := range msgs {
				require.LessOrEqual(t, msg.Revision, after.SnapshotVersion, "no message carries an uncommitted version")
			}
		})
	}
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
	require.Equal(t, 3, counts.countQueued, "post-lock count, second transition re-read, then publication read")

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
	require.Equal(t, 2, counts.countQueued, "the post-lock count and the publication read")
}

// TestSetFamilyArchivedValidatesFromLockedRow pins the chat reads under
// a member's transition lock: classifying the member must not consume
// the locked row that SetArchived then validates against.
func TestSetFamilyArchivedValidatesFromLockedRow(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	require.NoError(t, chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID).Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))

	counts := &roundTripCounts{}
	family, err := chatstate.SetFamilyArchived(ctx, &countingStore{Store: f.DB, counts: counts}, f.Pub, chatstate.SetFamilyArchivedInput{
		RootID:   created.Chat.ID,
		Archived: true,
	})
	require.NoError(t, err)
	require.Len(t, family, 1)
	require.True(t, family[0].Archived)

	require.Equal(t, 1, counts.lock)
	require.Equal(t, 2, counts.getChatByID, "the archived row reload and the publication read")
	require.Equal(t, 2, counts.countQueued, "the post-lock count and the publication read")
}

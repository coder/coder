package chatstate_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/testutil"
)

// Scenario tests for queued-message edits. The matrix cases cover each cell;
// these cover multi-step flows.

func setEditing(t *testing.T, f *testFixture, m *chatstate.ChatMachine, id int64, editing bool) chatstate.EditQueuedMessageResult {
	t.Helper()
	var res chatstate.EditQueuedMessageResult
	require.NoError(t, m.Update(testutil.Context(t, testutil.WaitShort), func(tx *chatstate.Tx, _ database.Store) error {
		var err error
		res, err = tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{QueuedMessageID: id, Editing: &editing})
		return err
	}))
	return res
}

func finishTurn(t *testing.T, f *testFixture, m *chatstate.ChatMachine) chatstate.FinishTurnResult {
	t.Helper()
	var res chatstate.FinishTurnResult
	require.NoError(t, m.Update(testutil.Context(t, testutil.WaitShort), func(tx *chatstate.Tx, _ database.Store) error {
		var err error
		res, err = tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))
	return res
}

// TestEditing_WhileRunning: five rows, row 3 under edit. Rows 1 and 2
// promote at their turn boundaries; the third boundary pauses the chat;
// releasing row 3 resumes it.
func TestEditing_WhileRunning(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := createTestChat(t, f).Chat.ID
	m := chatstate.NewChatMachine(f.DB, f.Pub, chatID)

	var ids []int64
	for i := 1; i <= 5; i++ {
		ids = append(ids, sendQueuedMessage(t, f, m, "row").QueuedMessage.ID)
	}
	setEditing(t, f, m, ids[2], true)
	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, chatID), "an edit behind the head changes nothing")

	require.NotNil(t, finishTurn(t, f, m).PromotedMessage, "row 1 promoted")
	require.NotNil(t, finishTurn(t, f, m).PromotedMessage, "row 2 promoted")
	third := finishTurn(t, f, m)
	require.Nil(t, third.PromotedMessage, "row 3 is under edit")
	require.Equal(t, chatstate.StateP, f.classify(ctx, t, chatID))
	require.Equal(t, ids[2:], queuedIDsByPosition(ctx, t, f, chatID))

	// FinishTurn is not admitted from P.
	err := m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	})
	require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed)

	// Releasing the head resumes with row 3.
	setEditing(t, f, m, ids[2], false)
	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, chatID))
	require.Equal(t, ids[3:], queuedIDsByPosition(ctx, t, f, chatID))
}

// TestEditing_InterruptionPauses: a head under edit pauses the chat at the end
// of an interruption too.
func TestEditing_InterruptionPauses(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := createTestChat(t, f).Chat.ID
	m := chatstate.NewChatMachine(f.DB, f.Pub, chatID)

	queued := sendInterruptMessage(t, f, m, "interrupting").QueuedMessage
	setEditing(t, f, m, queued.ID, true)
	require.Equal(t, chatstate.StateI1, f.classify(ctx, t, chatID))
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishInterruption(chatstate.FinishInterruptionInput{})
		return err
	}))
	require.Equal(t, chatstate.StateP, f.classify(ctx, t, chatID))
}

// TestEditing_MovesBetweenRows: row 2 under edit, begin editing row 1.
// The marker moves in one transition; from P, moving it off the head
// is refused.
func TestEditing_MovesBetweenRows(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := createTestChat(t, f).Chat.ID
	m := chatstate.NewChatMachine(f.DB, f.Pub, chatID)

	first := sendQueuedMessage(t, f, m, "one").QueuedMessage.ID
	second := sendQueuedMessage(t, f, m, "two").QueuedMessage.ID
	setEditing(t, f, m, second, true)
	moved := setEditing(t, f, m, first, true)
	require.True(t, moved.QueuedMessage.EditingSince.Valid)
	require.False(t, requireQueuedMessageByID(ctx, t, f, chatID, second).EditingSince.Valid, "previous edit ended")
	again := setEditing(t, f, m, first, true)
	require.Equal(t, moved.QueuedMessage.EditingSince.Time, again.QueuedMessage.EditingSince.Time, "re-beginning is a no-op")

	finishTurn(t, f, m)
	require.Equal(t, chatstate.StateP, f.classify(ctx, t, chatID))
	editing := true
	err := m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{QueuedMessageID: second, Editing: &editing})
		return err
	})
	require.ErrorIs(t, err, chatstate.ErrPausedQueuedHeadUnderEdit)
	require.True(t, requireQueuedMessageByID(ctx, t, f, chatID, first).EditingSince.Valid, "the head keeps its edit")
}

// TestEditing_ContentEditKeepsOverridesUnlessGiven: a content-only edit
// preserves the row's model and effort; an edit with overrides replaces
// them.
func TestEditing_ContentEditKeepsOverridesUnlessGiven(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := createTestChat(t, f).Chat.ID
	m := chatstate.NewChatMachine(f.DB, f.Pub, chatID)
	queued := sendQueuedMessage(t, f, m, "original").QueuedMessage

	edit := func(input chatstate.EditQueuedMessageInput) chatstate.EditQueuedMessageResult {
		var res chatstate.EditQueuedMessageResult
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
			var err error
			res, err = tx.EditQueuedMessage(input)
			return err
		}))
		return res
	}
	res := edit(chatstate.EditQueuedMessageInput{QueuedMessageID: queued.ID, Content: userMessageContent(t, "edited")})
	assertQueuedMessageText(t, res.QueuedMessage, "edited")
	require.Equal(t, queued.ModelConfigID, res.QueuedMessage.ModelConfigID)
	require.Equal(t, queued.ReasoningEffort, res.QueuedMessage.ReasoningEffort)

	res = edit(chatstate.EditQueuedMessageInput{
		QueuedMessageID:         queued.ID,
		Content:                 userMessageContent(t, "edited again"),
		ReasoningEffortOverride: database.NullChatReasoningEffort{ChatReasoningEffort: database.ChatReasoningEffortHigh, Valid: true},
	})
	require.Equal(t, database.ChatReasoningEffortHigh, res.QueuedMessage.ReasoningEffort.ChatReasoningEffort)

	err := m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{QueuedMessageID: queued.ID})
		return err
	})
	require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed, "neither content nor editing is an error")
}

// TestEditing_StaleChatsIgnoresPaused: GetStaleChats reports waiting with
// rows, not P.
func TestEditing_StaleChatsIgnoresPaused(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	seeded := seedPaused(t, f, 1)
	threshold := time.Now().Add(time.Hour)
	//nolint:gocritic // GetStaleChats is a system sweep query.
	sysCtx := dbauthz.AsSystemRestricted(ctx)

	contains := func(chats []database.Chat) bool {
		for _, c := range chats {
			if c.ID == seeded.chatID {
				return true
			}
		}
		return false
	}
	stale, err := f.DB.GetStaleChats(sysCtx, threshold)
	require.NoError(t, err)
	require.False(t, contains(stale), "paused is not reported")

	// The same rows under waiting are reported: nothing will promote them.
	chat, err := f.DB.GetChatByID(ctx, seeded.chatID)
	require.NoError(t, err)
	_, err = f.DB.UpdateChatExecutionState(ctx, database.UpdateChatExecutionStateParams{
		ID:                       chat.ID,
		Status:                   database.ChatStatusWaiting,
		WorkerID:                 chat.WorkerID,
		RunnerID:                 chat.RunnerID,
		LastError:                chat.LastError,
		RequiresActionDeadlineAt: chat.RequiresActionDeadlineAt,
	})
	require.NoError(t, err)
	stale, err = f.DB.GetStaleChats(sysCtx, threshold)
	require.NoError(t, err)
	require.True(t, contains(stale), "waiting with rows is reported")
}

// TestEditing_QueueListingFollowsProcessingOrder: the listing follows
// position, which "send now" on a later row changes.
func TestEditing_QueueListingFollowsProcessingOrder(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := createTestChat(t, f).Chat.ID
	m := chatstate.NewChatMachine(f.DB, f.Pub, chatID)

	first := sendQueuedMessage(t, f, m, "first").QueuedMessage.ID
	second := sendQueuedMessage(t, f, m, "second").QueuedMessage.ID
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.PromoteQueuedMessage(chatstate.PromoteQueuedMessageInput{QueuedMessageID: second})
		return err
	}))
	listed, err := f.DB.GetChatQueuedMessages(ctx, chatID)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	require.Equal(t, []int64{second, first}, []int64{listed[0].ID, listed[1].ID})
}

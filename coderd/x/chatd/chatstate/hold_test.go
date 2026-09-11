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

// Scenario tests for queue holds. The matrix cases prove each cell
// once; these prove the end-to-end stories the cells add up to.

func setHeld(t *testing.T, f *testFixture, m *chatstate.ChatMachine, id int64, held bool) chatstate.EditQueuedMessageResult {
	t.Helper()
	var res chatstate.EditQueuedMessageResult
	require.NoError(t, m.Update(testutil.Context(t, testutil.WaitShort), func(tx *chatstate.Tx, _ database.Store) error {
		var err error
		res, err = tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{QueuedMessageID: id, Held: &held})
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

// TestHold_EditWhileRunning is story 1 and 2: five rows, row 3 held.
// Rows 1 and 2 drain at their turn boundaries; the third boundary
// pauses the chat instead of promoting row 3; releasing row 3 resumes.
func TestHold_EditWhileRunning(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := createTestChat(t, f).Chat.ID
	m := chatstate.NewChatMachine(f.DB, f.Pub, chatID)

	var ids []int64
	for i := 1; i <= 5; i++ {
		ids = append(ids, sendQueuedMessage(t, f, m, "row").QueuedMessage.ID)
	}
	setHeld(t, f, m, ids[2], true)
	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, chatID), "a hold behind the head changes nothing")

	require.NotNil(t, finishTurn(t, f, m).PromotedMessage, "row 1 promoted")
	require.NotNil(t, finishTurn(t, f, m).PromotedMessage, "row 2 promoted")
	third := finishTurn(t, f, m)
	require.Nil(t, third.PromotedMessage, "row 3 is held")
	require.Equal(t, chatstate.StateP, f.classify(ctx, t, chatID))
	require.Equal(t, ids[2:], queuedIDsByPosition(ctx, t, f, chatID))

	// P is idle to the worker: FinishTurn is not admitted, so nothing
	// spins.
	err := m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	})
	require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed)

	// Cancel: releasing the head resumes the chat with row 3.
	setHeld(t, f, m, ids[2], false)
	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, chatID))
	require.Equal(t, ids[3:], queuedIDsByPosition(ctx, t, f, chatID))
}

// TestHold_InterruptionPauses is story 1 during an interruption.
func TestHold_InterruptionPauses(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := createTestChat(t, f).Chat.ID
	m := chatstate.NewChatMachine(f.DB, f.Pub, chatID)

	queued := sendInterruptMessage(t, f, m, "interrupting").QueuedMessage
	setHeld(t, f, m, queued.ID, true)
	require.Equal(t, chatstate.StateI1, f.classify(ctx, t, chatID))
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishInterruption(chatstate.FinishInterruptionInput{})
		return err
	}))
	require.Equal(t, chatstate.StateP, f.classify(ctx, t, chatID))
}

// TestHold_MovesBetweenRows is story 7: row 2 held, hold row 1. The hold
// moves in one transition, so there is never a moment with no hold; on
// a paused chat moving it off the head is refused instead.
func TestHold_MovesBetweenRows(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := createTestChat(t, f).Chat.ID
	m := chatstate.NewChatMachine(f.DB, f.Pub, chatID)

	first := sendQueuedMessage(t, f, m, "one").QueuedMessage.ID
	second := sendQueuedMessage(t, f, m, "two").QueuedMessage.ID
	setHeld(t, f, m, second, true)
	moved := setHeld(t, f, m, first, true)
	require.True(t, moved.QueuedMessage.HeldAt.Valid)
	require.False(t, requireQueuedMessageByID(ctx, t, f, chatID, second).HeldAt.Valid, "previous hold released")
	again := setHeld(t, f, m, first, true)
	require.Equal(t, moved.QueuedMessage.HeldAt.Time, again.QueuedMessage.HeldAt.Time, "re-holding is a no-op")

	finishTurn(t, f, m)
	require.Equal(t, chatstate.StateP, f.classify(ctx, t, chatID))
	held := true
	err := m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{QueuedMessageID: second, Held: &held})
		return err
	})
	require.ErrorIs(t, err, chatstate.ErrPausedHeadMustResume)
	require.True(t, requireQueuedMessageByID(ctx, t, f, chatID, first).HeldAt.Valid, "the head keeps its hold")
}

// TestHold_ContentEditKeepsOverridesUnlessGiven: a content-only edit
// preserves the row's model and effort; an edit with overrides replaces
// them.
func TestHold_ContentEditKeepsOverridesUnlessGiven(t *testing.T) {
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
	require.ErrorIs(t, err, chatstate.ErrTransitionNotAllowed, "neither content nor held is an error")
}

// TestHold_StaleChatsIgnoresPaused: GetStaleChats reports waiting with
// a promotable head, not P.
func TestHold_StaleChatsIgnoresPaused(t *testing.T) {
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
	require.False(t, contains(stale), "paused is not stranded")

	_, err = f.DB.UpdateChatQueuedMessageHeld(ctx, database.UpdateChatQueuedMessageHeldParams{ChatID: seeded.chatID, ID: seeded.queuedMessageIDs[0], Held: false})
	require.NoError(t, err)
	stale, err = f.DB.GetStaleChats(sysCtx, threshold)
	require.NoError(t, err)
	require.True(t, contains(stale), "waiting with a promotable head is stranded")
}

// TestHold_QueueListingFollowsProcessingOrder: the client-visible queue
// is ordered like the machine processes it, which "send now" on a later
// row is the case that breaks created_at order.
func TestHold_QueueListingFollowsProcessingOrder(t *testing.T) {
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

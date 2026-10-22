package chatstate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// Scenario tests for queued-message edits: multi-step flows the matrix
// cells do not cover.

func setEditing(t *testing.T, m *chatstate.ChatMachine, id int64, editing bool) chatstate.EditQueuedMessageResult {
	t.Helper()
	var res chatstate.EditQueuedMessageResult
	require.NoError(t, m.Update(testutil.Context(t, testutil.WaitShort), func(tx *chatstate.Tx, _ database.Store) error {
		var err error
		res, err = tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{QueuedMessageID: id, Editing: &editing})
		return err
	}))
	return res
}

func finishTurn(t *testing.T, m *chatstate.ChatMachine) chatstate.FinishTurnResult {
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
// ending row 3's edit resumes with row 3.
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
	setEditing(t, m, ids[2], true)
	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, chatID), "an edit behind the head changes nothing")

	require.NotNil(t, finishTurn(t, m).PromotedMessage, "row 1 promoted")
	require.NotNil(t, finishTurn(t, m).PromotedMessage, "row 2 promoted")
	third := finishTurn(t, m)
	require.Nil(t, third.PromotedMessage, "row 3 is under edit")
	require.Equal(t, chatstate.StateP, f.classify(ctx, t, chatID))
	require.Equal(t, ids[2:], queuedIDsByPosition(ctx, t, f, chatID))

	// Ending the edit resumes with row 3.
	setEditing(t, m, ids[2], false)
	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, chatID))
	require.Equal(t, ids[3:], queuedIDsByPosition(ctx, t, f, chatID))
}

// TestEditing_UnownedInterruptRunsAheadOfEditedHead: an interrupt sent
// to an unowned running chat whose head is under edit is promoted at
// once, ahead of that row, which stays queued and blocked. The chat
// pauses at that row when the interrupt's turn ends. Owned chats and
// the other paused states are matrix cells.
func TestEditing_UnownedInterruptRunsAheadOfEditedHead(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	seeded := seedBlockedHead(t, f, chatstate.StateR1, 0)
	m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)

	var res chatstate.SendMessageResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		var err error
		res, err = tx.SendMessage(chatstate.SendMessageInput{
			Message:      userTextMessage("interrupt", f.User.ID, f.Model.ID),
			BusyBehavior: chatstate.BusyBehaviorInterrupt,
			MaxQueueSize: codersdk.DefaultChatMaxQueuedMessagesPerChat,
		})
		return err
	}))
	require.True(t, res.FinishedInterruption)
	require.Nil(t, res.QueuedMessage, "the interrupt was promoted, so it is no longer queued")

	require.Equal(t, chatstate.StateR1P, f.classify(ctx, t, seeded.chatID))
	history := historyMessageIDs(ctx, t, f, seeded.chatID)
	assertChatMessageText(t, requireChatMessageByID(ctx, t, f, history[len(history)-1]), "interrupt")
	require.Equal(t, seeded.queuedMessageIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID))
	assertQueueEditMarkers(ctx, t, f, seeded.chatID, seeded.editingQueuedID)

	require.Nil(t, finishTurn(t, m).PromotedMessage, "the row under edit is not promoted")
	require.Equal(t, chatstate.StateP, f.classify(ctx, t, seeded.chatID))
	require.Equal(t, seeded.queuedMessageIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID))
}

// TestEditing_SendCapacityInPausedStates fills the queue of each paused
// state. A queued send is refused; an interrupt that runs ahead of the
// queue does not add to it and is accepted.
func TestEditing_SendCapacityInPausedStates(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	for _, tc := range []struct {
		state chatstate.ExecutionState
		seed  func(t *testing.T) seededChat
		// interruptQueues is true where an interrupt joins the queue.
		interruptQueues bool
	}{
		{chatstate.StateE1P, func(t *testing.T) seededChat { return seedBlockedHead(t, f, chatstate.StateE1, 1) }, false},
		{chatstate.StateA1P, func(t *testing.T) seededChat { return seedBlockedHead(t, f, chatstate.StateA1, 1) }, false},
		{chatstate.StateP, func(t *testing.T) seededChat { return seedPaused(t, f, 1) }, false},
		{chatstate.StateR1P, func(t *testing.T) seededChat { return seedBlockedHead(t, f, chatstate.StateR1, 1) }, true},
		{chatstate.StateI1P, func(t *testing.T) seededChat { return seedBlockedHead(t, f, chatstate.StateI1, 1) }, true},
	} {
		seeded := tc.seed(t)
		chat := f.readChat(ctx, t, seeded.chatID)
		queue, err := chatstate.LoadQueueState(ctx, f.DB, seeded.chatID)
		require.NoError(t, err)
		require.Equal(t, tc.state, chatstate.ClassifyExecutionState(chat, queue, true))
		full := int64(len(seeded.queuedMessageIDs))

		var queueFull *chatstate.MessageQueueFullError
		err = chatstate.CheckSendMessageQueueCapacity(chat, queue, full, chatstate.BusyBehaviorQueue, int(full))
		require.ErrorAs(t, err, &queueFull, "%s: a queued send to a full queue", tc.state)
		err = chatstate.CheckSendMessageQueueCapacity(chat, queue, full, chatstate.BusyBehaviorInterrupt, int(full))
		if tc.interruptQueues {
			require.ErrorAs(t, err, &queueFull, "%s: an interrupt joins the full queue", tc.state)
		} else {
			require.NoError(t, err, "%s: an interrupt runs ahead of the full queue", tc.state)
		}
	}
}

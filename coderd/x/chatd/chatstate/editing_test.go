package chatstate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
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

	require.Len(t, finishTurn(t, m).PromotedMessages, 1, "row 1 promoted")
	require.Len(t, finishTurn(t, m).PromotedMessages, 1, "row 2 promoted")
	third := finishTurn(t, m)
	require.Empty(t, third.PromotedMessages, "row 3 is under edit")
	require.Equal(t, chatstate.StateP, f.classify(ctx, t, chatID))
	require.Equal(t, ids[2:], queuedIDsByPosition(ctx, t, f, chatID))

	// Ending the edit resumes with row 3.
	setEditing(t, m, ids[2], false)
	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, chatID))
	require.Equal(t, ids[3:], queuedIDsByPosition(ctx, t, f, chatID))
}

// TestEditing_SendCapacityInPausedStates fills the queue of each paused
// state. A queue send is refused. A steer send to E1P or P behind a queue
// head is delivered at once, so it fits; behind a steer head, or in a
// busy state, it waits and is refused.
func TestEditing_SendCapacityInPausedStates(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	for _, tc := range []struct {
		state     chatstate.ExecutionState
		seed      func(t *testing.T) seededChat
		steerFits bool
	}{
		{chatstate.StateE1P, func(t *testing.T) seededChat { return seedBlockedHead(t, f, chatstate.StateE1, 1) }, true},
		{chatstate.StateE1P, func(t *testing.T) seededChat { return seedSteerHeadUnderEdit(t, f, chatstate.StateE1P) }, false},
		{chatstate.StateA1P, func(t *testing.T) seededChat { return seedBlockedHead(t, f, chatstate.StateA1, 1) }, false},
		{chatstate.StateP, func(t *testing.T) seededChat { return seedPaused(t, f, 1) }, true},
		{chatstate.StateP, func(t *testing.T) seededChat {
			return pauseSeeded(t, f, seedSteerHeadUnderEdit(t, f, chatstate.StateR1P))
		}, false},
		{chatstate.StateR1P, func(t *testing.T) seededChat { return seedBlockedHead(t, f, chatstate.StateR1, 1) }, false},
		{chatstate.StateI1P, func(t *testing.T) seededChat { return seedBlockedHead(t, f, chatstate.StateI1, 1) }, false},
	} {
		seeded := tc.seed(t)
		chat := f.readChat(ctx, t, seeded.chatID)
		queue, err := chatstate.LoadQueueState(ctx, f.DB, seeded.chatID)
		require.NoError(t, err)
		require.Equal(t, tc.state, chatstate.ClassifyExecutionState(chat, queue, true))
		full := int64(len(seeded.queuedMessageIDs))

		var queueFull *chatstate.MessageQueueFullError
		err = chatstate.CheckSendMessageQueueCapacity(chat, queue, chatstate.BusyBehaviorQueue, full, int(full))
		require.ErrorAs(t, err, &queueFull, "%s: a queue send to a full queue", tc.state)
		err = chatstate.CheckSendMessageQueueCapacity(chat, queue, chatstate.BusyBehaviorSteer, full, int(full))
		var sent chatstate.SendMessageResult
		sendErr := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID).Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
			var err error
			sent, err = tx.SendMessage(chatstate.SendMessageInput{
				Message:      userTextMessage("steer at the cap", f.User.ID, f.Model.ID),
				BusyBehavior: chatstate.BusyBehaviorSteer,
				MaxQueueSize: int(full),
			})
			return err
		})
		if tc.steerFits {
			require.NoError(t, err, "%s: a steer send behind a queue head", tc.state)
			require.NoError(t, sendErr, "%s: a steer send behind a queue head", tc.state)
			require.Nil(t, sent.QueuedMessage, "%s: the steer send is delivered at once", tc.state)
			require.NotEmpty(t, sent.InsertedMessages)
		} else {
			require.ErrorAs(t, err, &queueFull, "%s: a steer send that waits", tc.state)
			require.ErrorAs(t, sendErr, &queueFull, "%s: a steer send that waits", tc.state)
		}
	}
}

// TestEditing_LeavingPaused covers DeleteQueuedMessage and ending an
// edit from P. Staying in P does not rewrite the chat row and returns no
// chat; leaving P returns the updated chat.
func TestEditing_LeavingPaused(t *testing.T) {
	t.Parallel()
	deleteAt := func(idx int) func(*chatstate.Tx, seededChat) (*database.Chat, error) {
		return func(tx *chatstate.Tx, seeded seededChat) (*database.Chat, error) {
			res, err := tx.DeleteQueuedMessage(chatstate.DeleteQueuedMessageInput{QueuedMessageID: seeded.queuedMessageIDs[idx]})
			return res.Chat, err
		}
	}
	endEditAt := func(idx int) func(*chatstate.Tx, seededChat) (*database.Chat, error) {
		return func(tx *chatstate.Tx, seeded seededChat) (*database.Chat, error) {
			res, err := tx.EditQueuedMessage(chatstate.EditQueuedMessageInput{QueuedMessageID: seeded.queuedMessageIDs[idx], Editing: new(false)})
			return res.Chat, err
		}
	}

	for _, tc := range []struct {
		name  string
		extra int
		apply func(*chatstate.Tx, seededChat) (*database.Chat, error)
		want  chatstate.ExecutionState
	}{
		{"DeleteBehindHead", 1, deleteAt(1), chatstate.StateP},
		{"EndEditBehindHead", 1, endEditAt(1), chatstate.StateP},
		{"DeleteLastRow", 0, deleteAt(0), chatstate.StateW},
		{"DeleteHead", 1, deleteAt(0), chatstate.StateR0},
		{"EndHeadEdit", 0, endEditAt(0), chatstate.StateR0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newTestFixture(t)
			ctx := testutil.Context(t, testutil.WaitShort)
			seeded := seedPaused(t, f, tc.extra)
			before := f.readChat(ctx, t, seeded.chatID)
			require.Equal(t, chatstate.StateP, f.classify(ctx, t, seeded.chatID))

			var chat *database.Chat
			require.NoError(t, chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID).Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
				var err error
				chat, err = tc.apply(tx, seeded)
				return err
			}))
			require.Equal(t, tc.want, f.classify(ctx, t, seeded.chatID))
			after := f.readChat(ctx, t, seeded.chatID)
			if tc.want == chatstate.StateP {
				require.Nil(t, chat, "staying in P reports no chat")
				require.True(t, before.UpdatedAt.Equal(after.UpdatedAt), "staying in P does not rewrite the chat row")
			} else {
				require.NotNil(t, chat, "leaving P reports the updated chat")
				require.Equal(t, after.Status, chat.Status)
				require.Equal(t, after.SnapshotVersion, chat.SnapshotVersion)
			}
		})
	}
}

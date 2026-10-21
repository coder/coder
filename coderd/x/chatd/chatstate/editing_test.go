package chatstate_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// Scenario tests for queued-message edits (multi-step flows the matrix
// cells do not cover) and the queue queries the edit endpoints read.

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

// TestEditing_ContentEditKeepsOverridesUnlessGiven: a content-only edit
// keeps the row's model and effort; an edit with overrides replaces them.
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

	res = edit(chatstate.EditQueuedMessageInput{QueuedMessageID: queued.ID, Content: userMessageContent(t, "edited once more")})
	assertQueuedMessageText(t, res.QueuedMessage, "edited once more")
	require.Equal(t, database.ChatReasoningEffortHigh, res.QueuedMessage.ReasoningEffort.ChatReasoningEffort, "a content-only edit keeps the override")
}

// The listing follows position, which PromoteQueuedMessage on a later
// row changes.
func TestGetChatQueuedMessages_OrdersByPosition(t *testing.T) {
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

// TestEditing_InterruptRunsAheadOfEditedHead sends an interrupt in each
// paused state. The interrupt message reaches history before the row
// under edit, which stays queued and blocked, and the chat pauses at that
// row when the interrupt's turn ends.
func TestEditing_InterruptRunsAheadOfEditedHead(t *testing.T) {
	t.Parallel()

	interrupt := func(t *testing.T, f *testFixture, m *chatstate.ChatMachine, own bool) chatstate.SendMessageResult {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitShort)
		var res chatstate.SendMessageResult
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			if own {
				if err := ownChat(ctx, tx, store, tx.ChatID()); err != nil {
					return err
				}
			}
			var err error
			res, err = tx.SendMessage(chatstate.SendMessageInput{
				Message:      userTextMessage("interrupt", f.User.ID, f.Model.ID),
				BusyBehavior: chatstate.BusyBehaviorInterrupt,
				MaxQueueSize: codersdk.DefaultChatMaxQueuedMessagesPerChat,
			})
			return err
		}))
		return res
	}
	// requireRanAheadThenPaused checks that the newest history message is
	// the interrupt, that the edited row is still the blocked head, and
	// that finishing the interrupt's turn pauses the chat at that row.
	requireRanAheadThenPaused := func(ctx context.Context, t *testing.T, f *testFixture, m *chatstate.ChatMachine, seeded seededChat) {
		t.Helper()
		require.Equal(t, chatstate.StateR1P, f.classify(ctx, t, seeded.chatID))
		history := historyMessageIDs(ctx, t, f, seeded.chatID)
		last := requireChatMessageByID(ctx, t, f, history[len(history)-1])
		assertChatMessageText(t, last, "interrupt")
		require.Equal(t, seeded.queuedMessageIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID))
		assertQueueEditMarkers(ctx, t, f, seeded.chatID, seeded.editingQueuedID)

		require.Nil(t, finishTurn(t, m).PromotedMessage, "the row under edit is not promoted")
		require.Equal(t, chatstate.StateP, f.classify(ctx, t, seeded.chatID))
		require.Equal(t, seeded.queuedMessageIDs, queuedIDsByPosition(ctx, t, f, seeded.chatID))
	}

	t.Run("OwnedRunning", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		seeded := seedBlockedHead(t, f, chatstate.StateR1, 0)
		m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)

		res := interrupt(t, f, m, true)
		require.NotNil(t, res.QueuedMessage)
		require.False(t, res.FinishedInterruption)
		require.Equal(t, chatstate.StateI1, f.classify(ctx, t, seeded.chatID), "the interrupt is the ready head")
		require.Equal(t, append([]int64{res.QueuedMessage.ID}, seeded.queuedMessageIDs...), queuedIDsByPosition(ctx, t, f, seeded.chatID))

		var finished chatstate.FinishInterruptionResult
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
			var err error
			finished, err = tx.FinishInterruption(chatstate.FinishInterruptionInput{})
			return err
		}))
		require.NotNil(t, finished.PromotedMessage)
		requireQueuedMessageLink(t, *finished.PromotedMessage, res.QueuedMessage.ID)
		requireRanAheadThenPaused(ctx, t, f, m, seeded)
	})

	t.Run("UnownedRunning", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		seeded := seedBlockedHead(t, f, chatstate.StateR1, 0)
		m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)

		res := interrupt(t, f, m, false)
		require.True(t, res.FinishedInterruption)
		require.Nil(t, res.QueuedMessage, "the interrupt was promoted, so it is no longer queued")
		requireRanAheadThenPaused(ctx, t, f, m, seeded)
	})

	t.Run("RequiresAction", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		seeded := seedBlockedHead(t, f, chatstate.StateA1, 0)
		require.Equal(t, chatstate.StateA1P, f.classify(ctx, t, seeded.chatID))
		m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)

		res := interrupt(t, f, m, false)
		require.Nil(t, res.QueuedMessage)
		require.Len(t, res.InsertedMessages, 2, "a tool-result cancellation, then the interrupt")
		assertToolResultForCall(t, requireChatMessageByID(ctx, t, f, res.InsertedMessages[0].ID), seeded.pendingToolCallID)
		requireRanAheadThenPaused(ctx, t, f, m, seeded)
	})

	t.Run("ErrorWithBlockedHead", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		seeded := seedBlockedHead(t, f, chatstate.StateE1, 0)
		require.Equal(t, chatstate.StateE1P, f.classify(ctx, t, seeded.chatID))
		m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)

		res := interrupt(t, f, m, false)
		require.Nil(t, res.QueuedMessage)
		require.False(t, f.readChat(ctx, t, seeded.chatID).LastError.Valid)
		requireRanAheadThenPaused(ctx, t, f, m, seeded)
	})

	t.Run("Paused", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		seeded := seedPaused(t, f, 1)
		require.Equal(t, chatstate.StateP, f.classify(ctx, t, seeded.chatID))
		m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)

		res := interrupt(t, f, m, false)
		require.Nil(t, res.QueuedMessage)
		require.Len(t, res.InsertedMessages, 1)
		requireRanAheadThenPaused(ctx, t, f, m, seeded)
	})

	// An interrupt during an interruption joins the tail, so in I1P it
	// waits behind the row under edit.
	t.Run("Interrupting", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		seeded := seedBlockedHead(t, f, chatstate.StateI1, 0)
		require.Equal(t, chatstate.StateI1P, f.classify(ctx, t, seeded.chatID))
		m := chatstate.NewChatMachine(f.DB, f.Pub, seeded.chatID)

		res := interrupt(t, f, m, true)
		require.NotNil(t, res.QueuedMessage)
		require.Equal(t, chatstate.StateI1P, f.classify(ctx, t, seeded.chatID))
		require.Equal(t, append(append([]int64{}, seeded.queuedMessageIDs...), res.QueuedMessage.ID), queuedIDsByPosition(ctx, t, f, seeded.chatID))
	})
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

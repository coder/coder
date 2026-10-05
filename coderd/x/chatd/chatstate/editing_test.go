package chatstate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
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

// TestEditing_AutomationRowRefused: a row queued by an automation refuses
// new content and beginning an edit, from a running chat and from P,
// without touching the row or the queue. Ending an edit is allowed and,
// from P, leaves paused.
func TestEditing_AutomationRowRefused(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := createTestChat(t, f).Chat.ID
	m := chatstate.NewChatMachine(f.DB, f.Pub, chatID)
	queued := queueAutomationMessage(t, f, m, "from automation", provenanceFor(f.newAutomation(t)))

	editing := true
	refused := []chatstate.EditQueuedMessageInput{
		{QueuedMessageID: queued.ID, Content: userMessageContent(t, "rewritten")},
		{QueuedMessageID: queued.ID, Editing: &editing},
	}
	requireRefused := func(wantEditing bool) {
		t.Helper()
		before := f.readChat(ctx, t, chatID)
		for _, input := range refused {
			err := m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
				_, err := tx.EditQueuedMessage(input)
				return err
			})
			require.ErrorIs(t, err, chatstate.ErrQueuedMessageFromAutomation)
		}
		row, err := f.DB.GetChatQueuedMessageByID(ctx, database.GetChatQueuedMessageByIDParams{ID: queued.ID, ChatID: chatID})
		require.NoError(t, err)
		assertQueuedMessageText(t, row, "from automation")
		require.Equal(t, wantEditing, row.EditingSince.Valid)
		after := f.readChat(ctx, t, chatID)
		require.Equal(t, before.QueueVersion, after.QueueVersion, "a refused edit does not bump queue_version")
		require.Equal(t, before.SnapshotVersion, after.SnapshotVersion, "a refused edit does not bump the snapshot")
	}

	requireRefused(false)
	res := setEditing(t, m, queued.ID, false)
	require.False(t, res.QueuedMessage.EditingSince.Valid)
	assertQueuedMessageText(t, res.QueuedMessage, "from automation")
	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, chatID))

	// A marker set directly in the store pauses the chat at the row, and
	// ending the edit is the way out.
	beginQueuedMessageEdit(ctx, t, f, chatID, queued.ID)
	require.Nil(t, finishTurn(t, m).PromotedMessage)
	require.Equal(t, chatstate.StateP, f.classify(ctx, t, chatID))
	requireRefused(true)
	setEditing(t, m, queued.ID, false)
	require.Equal(t, chatstate.StateR0, f.classify(ctx, t, chatID), "ending the edit promotes the automation row")
	require.Empty(t, queuedIDsByPosition(ctx, t, f, chatID))
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

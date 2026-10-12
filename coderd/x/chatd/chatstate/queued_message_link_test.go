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

// TestQueuedMessageLinkIdenticalContent verifies that two queued
// messages with byte-identical content each promote into a distinct
// history message linked to its own queue row, in FIFO order.
func TestQueuedMessageLinkIdenticalContent(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	first := sendQueuedMessage(t, f, m, "twin")
	require.NotNil(t, first.QueuedMessage)
	second := sendQueuedMessage(t, f, m, "twin")
	require.NotNil(t, second.QueuedMessage)
	require.Equal(t, first.QueuedMessage.Content, second.QueuedMessage.Content)
	require.Less(t, first.QueuedMessage.ID, second.QueuedMessage.ID)
	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, created.Chat.ID))

	promoted := make([]database.ChatMessage, 0, 2)
	for range 2 {
		var finish chatstate.FinishTurnResult
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
			var err error
			finish, err = tx.FinishTurn(chatstate.FinishTurnInput{})
			return err
		}))
		require.NotNil(t, finish.PromotedMessage)
		promoted = append(promoted, requireChatMessageByID(ctx, t, f, finish.PromotedMessage.ID))
	}
	require.Equal(t, chatstate.StateR0, f.classify(ctx, t, created.Chat.ID))

	require.NotEqual(t, promoted[0].ID, promoted[1].ID)
	assertChatMessageText(t, promoted[0], "twin")
	assertChatMessageText(t, promoted[1], "twin")
	requireQueuedMessageLink(t, promoted[0], first.QueuedMessage.ID)
	requireQueuedMessageLink(t, promoted[1], second.QueuedMessage.ID)
}

// TestQueuedMessageLinkEditReplacementUnlinked verifies that editing a
// promoted message produces a replacement that is not linked to the
// queue, because the replacement was not promoted from a queue row.
func TestQueuedMessageLinkEditReplacementUnlinked(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	queued := sendQueuedMessage(t, f, m, "promote then edit")
	require.NotNil(t, queued.QueuedMessage)
	var finish chatstate.FinishTurnResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		var err error
		finish, err = tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	}))
	require.NotNil(t, finish.PromotedMessage)
	requireQueuedMessageLink(t, *finish.PromotedMessage, queued.QueuedMessage.ID)
	require.Equal(t, chatstate.StateR0, f.classify(ctx, t, created.Chat.ID))

	var edit chatstate.EditMessageResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		var err error
		edit, err = tx.EditMessage(chatstate.EditMessageInput{
			MessageID: finish.PromotedMessage.ID,
			CreatedBy: f.User.ID,
			Content: mustMarshalParts(t, []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("edited"),
			}),
		})
		return err
	}))
	replacement := requireChatMessageByID(ctx, t, f, edit.ReplacementMessage.ID)
	require.NotEqual(t, finish.PromotedMessage.ID, replacement.ID)
	require.False(t, replacement.QueuedMessageID.Valid,
		"edit replacements are not promoted from the queue")
}

// TestQueuedMessagePromotionRequiresSingleDelete verifies that a
// promotion whose queue-row delete does not remove exactly one row
// fails and rolls back the whole transition.
func TestQueuedMessagePromotionRequiresSingleDelete(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

	queued := sendQueuedMessage(t, f, m, "delete count guard")
	require.NotNil(t, queued.QueuedMessage)
	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, created.Chat.ID))
	historyBefore := historyMessageIDs(ctx, t, f, created.Chat.ID)

	guarded := chatstate.NewChatMachine(&queueDeleteCountStore{Store: f.DB}, f.Pub, created.Chat.ID)
	err := guarded.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
		return err
	})
	require.ErrorContains(t, err, "deleted 0 rows")
	require.NotErrorIs(t, err, chatstate.ErrQueuedMessageNotFound)

	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, created.Chat.ID))
	requireQueuedMessageByID(ctx, t, f, created.Chat.ID, queued.QueuedMessage.ID)
	require.Equal(t, historyBefore, historyMessageIDs(ctx, t, f, created.Chat.ID),
		"failed promotion inserts no message")
}

// queueDeleteCountStore reports that queue-row deletes affected no rows,
// simulating a broken delete invariant inside the real transaction.
type queueDeleteCountStore struct {
	database.Store
}

func (s *queueDeleteCountStore) InTx(fn func(database.Store) error, opts *database.TxOptions) error {
	return s.Store.InTx(func(store database.Store) error {
		return fn(&queueDeleteCountStore{Store: store})
	}, opts)
}

func (*queueDeleteCountStore) DeleteChatQueuedMessageReturningCount(
	context.Context,
	database.DeleteChatQueuedMessageReturningCountParams,
) (int64, error) {
	return 0, nil
}

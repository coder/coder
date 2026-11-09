package chatstate_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// clientQueueIDs returns the queue in the order clients receive it: the
// stream snapshot (stream_loop.go) and the messages API
// (coderd/exp_chats.go) both read GetChatQueuedMessages.
func clientQueueIDs(t *testing.T, f *testFixture, chatID uuid.UUID) []int64 {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	rows, err := f.DB.GetChatQueuedMessages(ctx, chatID)
	require.NoError(t, err)
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

// TestClientQueueOrderMatchesPromotionOrder checks that clients receive the
// queue in the order promotions take it (position, id), including when
// position and created_at disagree.
func TestClientQueueOrderMatchesPromotionOrder(t *testing.T) {
	t.Parallel()

	// An explicit promote on a running chat moves the target to the head
	// by position only; created_at keeps the old order.
	t.Run("PromoteWhileRunning", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

		first := sendQueuedMessage(t, f, m, "first")
		second := sendQueuedMessage(t, f, m, "second")
		require.Equal(t, chatstate.StateR1, f.classify(ctx, t, created.Chat.ID))

		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
			_, err := tx.PromoteQueuedMessage(chatstate.PromoteQueuedMessageInput{
				QueuedMessageID: second.QueuedMessage.ID,
			})
			return err
		}))
		require.Equal(t, chatstate.StateI1, f.classify(ctx, t, created.Chat.ID))

		promotionOrder := queuedIDsByPosition(ctx, t, f, created.Chat.ID)
		require.Equal(t, []int64{second.QueuedMessage.ID, first.QueuedMessage.ID}, promotionOrder)
		require.Equal(t, promotionOrder, clientQueueIDs(t, f, created.Chat.ID),
			"clients see the promoted message behind the message that will run after it")
	})

	// Two senders: the first transaction starts (fixing now() and so
	// created_at) before the second, but takes the chat lock after it.
	// In production the window is the round trip between BEGIN and the
	// lock statement; the outer transaction widens it.
	t.Run("ConcurrentSenders", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

		var early, late chatstate.SendMessageResult
		require.NoError(t, f.DB.InTx(func(tx database.Store) error {
			// The first statement pins the transaction start time.
			if _, err := tx.GetDatabaseNow(ctx); err != nil {
				return err
			}
			// The other sender commits on its own connection meanwhile.
			late = sendQueuedMessage(t, f, m, "committed first")
			// Update reuses this transaction and only now locks the chat.
			return chatstate.NewChatMachine(tx, f.Pub, created.Chat.ID).Update(ctx, func(stx *chatstate.Tx, _ database.Store) error {
				var err error
				early, err = stx.SendMessage(chatstate.SendMessageInput{
					Message:      userTextMessage("began first", f.User.ID, f.Model.ID),
					BusyBehavior: chatstate.BusyBehaviorQueue,
					MaxQueueSize: codersdk.DefaultChatMaxQueuedMessagesPerChat,
				})
				return err
			})
		}, nil))

		require.NotNil(t, early.QueuedMessage)
		require.NotNil(t, late.QueuedMessage)
		require.True(t, early.QueuedMessage.CreatedAt.Before(late.QueuedMessage.CreatedAt),
			"the scenario needs created_at to disagree with position")
		promotionOrder := queuedIDsByPosition(ctx, t, f, created.Chat.ID)
		require.Equal(t, []int64{late.QueuedMessage.ID, early.QueuedMessage.ID}, promotionOrder)
		require.Equal(t, promotionOrder, clientQueueIDs(t, f, created.Chat.ID),
			"clients see the later-locked message first, but it is promoted second")
	})
}

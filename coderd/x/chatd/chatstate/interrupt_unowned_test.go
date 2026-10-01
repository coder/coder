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

// An unowned running chat has no runner to finish an interruption, so
// Interrupt finishes it inline instead of leaving the chat in
// interrupting until a worker (and a capacity slot) is free.
func TestInterruptUnownedRunningChat(t *testing.T) {
	t.Parallel()

	t.Run("StopLandsInWaiting", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		// An orphaned tool call from an earlier runner gets a synthetic
		// cancellation so the history stays valid.
		callID := "call_" + uuid.NewString()
		commitAssistantToolCall(t, f, m, nonDynamicAssistantToolCallMessage(t, f.Model.ID, callID))

		var result chatstate.InterruptResult
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
			var err error
			result, err = tx.Interrupt(chatstate.InterruptInput{Reason: "stop"})
			return err
		}))

		require.Equal(t, chatstate.StateW, f.classify(ctx, t, created.Chat.ID))
		chat := f.readChat(ctx, t, created.Chat.ID)
		require.False(t, chat.WorkerID.Valid)
		require.False(t, chat.RunnerID.Valid)
		require.Len(t, result.CancellationMessages, 1)
		require.Equal(t, database.ChatMessageRoleTool, result.CancellationMessages[0].Role)
	})

	t.Run("StopWithQueuePromotesUnowned", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		queued := sendQueuedMessage(t, f, m, "queued")
		require.NotNil(t, queued.QueuedMessage)
		before := historyMessageIDs(ctx, t, f, created.Chat.ID)

		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
			_, err := tx.Interrupt(chatstate.InterruptInput{Reason: "stop"})
			return err
		}))

		require.Equal(t, chatstate.StateR0, f.classify(ctx, t, created.Chat.ID))
		chat := f.readChat(ctx, t, created.Chat.ID)
		require.False(t, chat.WorkerID.Valid, "the promoted turn must still go through admission")
		after := historyMessageIDs(ctx, t, f, created.Chat.ID)
		require.Len(t, after, len(before)+1)
		requireQueuedMessageDeleted(ctx, t, f, created.Chat.ID, queued.QueuedMessage.ID)
	})

	t.Run("OwnedChatStillInterrupts", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
			if err := ownChat(ctx, tx, store, created.Chat.ID); err != nil {
				return err
			}
			_, err := tx.Interrupt(chatstate.InterruptInput{Reason: "stop"})
			return err
		}))
		require.Equal(t, chatstate.StateI0, f.classify(ctx, t, created.Chat.ID))
	})
}

func TestSendMessageInterruptUnownedRunningChat(t *testing.T) {
	t.Parallel()

	send := func(t *testing.T, f *testFixture, m *chatstate.ChatMachine, body string) chatstate.SendMessageResult {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitShort)
		var result chatstate.SendMessageResult
		require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
			var err error
			result, err = tx.SendMessage(chatstate.SendMessageInput{
				Message:      userTextMessage(body, f.User.ID, f.Model.ID),
				BusyBehavior: chatstate.BusyBehaviorInterrupt,
				MaxQueueSize: codersdk.DefaultChatMaxQueuedMessagesPerChat,
			})
			return err
		}))
		return result
	}

	t.Run("EmptyQueuePromotesTheMessage", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)

		result := send(t, f, m, "interrupting")

		require.Nil(t, result.QueuedMessage, "the message is promoted, not left queued")
		require.NotEmpty(t, result.InsertedMessages)
		promoted := result.InsertedMessages[len(result.InsertedMessages)-1]
		require.Equal(t, database.ChatMessageRoleUser, promoted.Role)
		assertChatMessageText(t, promoted, "interrupting")
		require.False(t, result.PromotedQueuedAt.IsZero())
		require.Equal(t, chatstate.StateR0, f.classify(ctx, t, created.Chat.ID))
		chat := f.readChat(ctx, t, created.Chat.ID)
		require.False(t, chat.WorkerID.Valid, "the promoted turn must still go through admission")
	})

	t.Run("QueuedHeadIsPromotedFirst", func(t *testing.T) {
		t.Parallel()
		f := newTestFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		created := createTestChat(t, f)
		m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID)
		first := sendQueuedMessage(t, f, m, "first")
		require.NotNil(t, first.QueuedMessage)

		result := send(t, f, m, "second")

		require.NotNil(t, result.QueuedMessage, "the new message stays queued behind the promoted head")
		require.NotEmpty(t, result.InsertedMessages)
		assertChatMessageText(t, result.InsertedMessages[len(result.InsertedMessages)-1], "first")
		requireQueuedMessageDeleted(ctx, t, f, created.Chat.ID, first.QueuedMessage.ID)
		require.Equal(t, chatstate.StateR1, f.classify(ctx, t, created.Chat.ID))
		chat := f.readChat(ctx, t, created.Chat.ID)
		require.False(t, chat.WorkerID.Valid)
	})
}

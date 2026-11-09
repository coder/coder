package chatstate_test

import (
	"cmp"
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// ownershipPublishCount returns the number of `chat:ownership` messages
// recorded so far on the test publisher. Tests use it to assert that
// transitions do or do not publish an ownership hint.
func (r *recordingPubsub) ownershipPublishCount() int {
	count := 0
	for _, c := range r.channels {
		if c == coderdpubsub.ChatStateOwnershipChannel {
			count++
		}
	}
	return count
}

// sendQueuedMessage seeds one queued user message via SendMessage with
// BusyBehaviorQueue. The chat must already be in a state that allows
// SendMessage (typically R0, R1, or I*).
func sendQueuedMessage(t *testing.T, f *testFixture, m *chatstate.ChatMachine, body string) chatstate.SendMessageResult {
	t.Helper()
	return sendMessageWithBehavior(t, f, m, body, chatstate.BusyBehaviorQueue)
}

// sendInterruptMessage seeds one queued user message via SendMessage
// with BusyBehaviorInterrupt on an owned chat.
func sendInterruptMessage(t *testing.T, f *testFixture, m *chatstate.ChatMachine, chatID uuid.UUID, body string) chatstate.SendMessageResult {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	var send chatstate.SendMessageResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		if err := ownChat(ctx, tx, store, chatID); err != nil {
			return err
		}
		var err error
		send, err = tx.SendMessage(chatstate.SendMessageInput{
			Message:      userTextMessage(body, f.User.ID, f.Model.ID),
			BusyBehavior: chatstate.BusyBehaviorInterrupt,
			MaxQueueSize: codersdk.DefaultChatMaxQueuedMessagesPerChat,
		})
		return err
	}))
	return send
}

func sendMessageWithBehavior(t *testing.T, f *testFixture, m *chatstate.ChatMachine, body string, behavior chatstate.BusyBehavior) chatstate.SendMessageResult {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	var send chatstate.SendMessageResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		var err error
		send, err = tx.SendMessage(chatstate.SendMessageInput{
			Message:      userTextMessage(body, f.User.ID, f.Model.ID),
			BusyBehavior: behavior,
			MaxQueueSize: codersdk.DefaultChatMaxQueuedMessagesPerChat,
		})
		return err
	}))
	return send
}

func queuedRowsByPosition(ctx context.Context, t *testing.T, f *testFixture, chatID uuid.UUID) []database.ChatQueuedMessage {
	t.Helper()
	rows, err := f.DB.GetChatQueuedMessages(ctx, chatID)
	require.NoError(t, err)
	slices.SortFunc(rows, func(a, b database.ChatQueuedMessage) int {
		return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.ID, b.ID))
	})
	return rows
}

func queuedIDsByPosition(ctx context.Context, t *testing.T, f *testFixture, chatID uuid.UUID) []int64 {
	t.Helper()
	rows := queuedRowsByPosition(ctx, t, f, chatID)
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

func historyMessageIDs(ctx context.Context, t *testing.T, f *testFixture, chatID uuid.UUID) []int64 {
	t.Helper()
	msgs, err := f.DB.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
		ChatID: chatID,
	})
	require.NoError(t, err)
	out := make([]int64, len(msgs))
	for i, m := range msgs {
		out[i] = m.ID
	}
	return out
}

// ownChat gives an unowned chat a worker and runner inside tx, as an
// acquired chat has. Interrupting an unowned running chat finishes the
// interruption inline, so tests of the interrupting states need an
// owner.
func ownChat(ctx context.Context, tx *chatstate.Tx, store database.Store, chatID uuid.UUID) error {
	chat, err := store.GetChatByID(ctx, chatID)
	if err != nil {
		return err
	}
	if chat.WorkerID.Valid {
		return nil
	}
	_, err = tx.Acquire(chatstate.AcquireInput{WorkerID: uuid.New(), RunnerID: uuid.New()})
	return err
}

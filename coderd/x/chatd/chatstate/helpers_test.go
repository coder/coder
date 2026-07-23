package chatstate_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
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
	ctx := testutil.Context(t, testutil.WaitShort)
	var send chatstate.SendMessageResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		var err error
		send, err = tx.SendMessage(chatstate.SendMessageInput{
			Message:      userTextMessage(body, f.User.ID, f.Model.ID),
			BusyBehavior: chatstate.BusyBehaviorQueue,
		})
		return err
	}))
	return send
}

// sendInterruptMessage seeds one queued user message via SendMessage
// with BusyBehaviorInterrupt. From R0/R1 this transitions the chat to
// `interrupting` and appends the new user message to the queue tail.
func sendInterruptMessage(t *testing.T, f *testFixture, m *chatstate.ChatMachine, body string) chatstate.SendMessageResult {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	var send chatstate.SendMessageResult
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		var err error
		send, err = tx.SendMessage(chatstate.SendMessageInput{
			Message:      userTextMessage(body, f.User.ID, f.Model.ID),
			BusyBehavior: chatstate.BusyBehaviorInterrupt,
		})
		return err
	}))
	return send
}

// queuedIDsByPosition returns the queued-message IDs for the chat in
// queue order.
func queuedIDsByPosition(ctx context.Context, t *testing.T, f *testFixture, chatID uuid.UUID) []int64 {
	t.Helper()
	rows, err := f.DB.GetChatQueuedMessagesByPosition(ctx, chatID)
	require.NoError(t, err)
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

// historyMessageIDs returns the chat history message IDs ordered by
// row id. Used to assert that PromoteQueuedMessage from R1/I1 does NOT
// insert any history rows.
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

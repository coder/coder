package chatstate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/testutil"
)

// TestLoadQueueState pins the classifier input: only the physical head
// row decides HasPromotableHead. A held head hides the whole queue from
// the state machine regardless of what sits behind it; a held row
// behind an unheld head changes nothing.
func TestLoadQueueState(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)

	created := createTestChat(t, f)
	chatID := created.Chat.ID
	m := chatstate.NewChatMachine(f.DB, f.Pub, chatID)

	queue, err := chatstate.LoadQueueState(ctx, f.DB, chatID)
	require.NoError(t, err)
	require.False(t, queue.HasPromotableHead, "empty queue")

	first := sendQueuedMessage(t, f, m, "first")
	second := sendQueuedMessage(t, f, m, "second")
	require.NotNil(t, first.QueuedMessage)
	require.NotNil(t, second.QueuedMessage)

	queue, err = chatstate.LoadQueueState(ctx, f.DB, chatID)
	require.NoError(t, err)
	require.True(t, queue.HasPromotableHead, "unheld head")

	setHeld := func(id int64, held bool) {
		_, err := f.DB.UpdateChatQueuedMessageHeld(ctx, database.UpdateChatQueuedMessageHeldParams{
			ChatID: chatID,
			ID:     id,
			Held:   held,
		})
		require.NoError(t, err)
	}

	setHeld(second.QueuedMessage.ID, true)
	queue, err = chatstate.LoadQueueState(ctx, f.DB, chatID)
	require.NoError(t, err)
	require.True(t, queue.HasPromotableHead, "held row behind an unheld head")

	setHeld(second.QueuedMessage.ID, false)
	setHeld(first.QueuedMessage.ID, true)
	queue, err = chatstate.LoadQueueState(ctx, f.DB, chatID)
	require.NoError(t, err)
	require.False(t, queue.HasPromotableHead, "held head with an unheld row behind it")
	require.Equal(t, chatstate.StateR0, f.classify(ctx, t, chatID), "running with a held head")

	setHeld(first.QueuedMessage.ID, false)
	queue, err = chatstate.LoadQueueState(ctx, f.DB, chatID)
	require.NoError(t, err)
	require.True(t, queue.HasPromotableHead, "released head")
	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, chatID))
}

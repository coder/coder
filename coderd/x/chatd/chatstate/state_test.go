package chatstate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/testutil"
)

// TestLoadQueueState pins the classifier input: rows exist, and whether
// the physical head is held. A held row behind the head is invisible.
func TestLoadQueueState(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	chatID := createTestChat(t, f).Chat.ID
	m := chatstate.NewChatMachine(f.DB, f.Pub, chatID)

	queue, err := chatstate.LoadQueueState(ctx, f.DB, chatID)
	require.NoError(t, err)
	require.Equal(t, chatstate.QueueState{}, queue, "empty queue")

	first := sendQueuedMessage(t, f, m, "first").QueuedMessage.ID
	second := sendQueuedMessage(t, f, m, "second").QueuedMessage.ID
	setHeld := func(id int64, held bool) {
		_, err := f.DB.UpdateChatQueuedMessageHeld(ctx, database.UpdateChatQueuedMessageHeldParams{ChatID: chatID, ID: id, Held: held})
		require.NoError(t, err)
	}

	setHeld(second, true)
	queue, err = chatstate.LoadQueueState(ctx, f.DB, chatID)
	require.NoError(t, err)
	require.Equal(t, chatstate.QueueState{HasRows: true}, queue, "held row behind an unheld head")

	setHeld(second, false)
	setHeld(first, true)
	queue, err = chatstate.LoadQueueState(ctx, f.DB, chatID)
	require.NoError(t, err)
	require.Equal(t, chatstate.QueueState{HasRows: true, HeadHeld: true}, queue, "held head")
	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, chatID), "a held head does not change a busy state")
}

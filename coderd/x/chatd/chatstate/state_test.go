package chatstate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/testutil"
)

// TestLoadQueueState pins the classifier input: rows exist, and whether
// the physical head is under edit. An edit behind the head is invisible.
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
	setEditing := func(id int64, editing bool) {
		_, err := f.DB.UpdateChatQueuedMessageEditing(ctx, database.UpdateChatQueuedMessageEditingParams{ChatID: chatID, ID: id, Editing: editing})
		require.NoError(t, err)
	}

	setEditing(second, true)
	queue, err = chatstate.LoadQueueState(ctx, f.DB, chatID)
	require.NoError(t, err)
	require.Equal(t, chatstate.QueueState{HasRows: true}, queue, "edit behind the head")

	setEditing(second, false)
	setEditing(first, true)
	queue, err = chatstate.LoadQueueState(ctx, f.DB, chatID)
	require.NoError(t, err)
	require.Equal(t, chatstate.QueueState{HasRows: true, Paused: true}, queue, "head under edit")
	require.Equal(t, chatstate.StateR1, f.classify(ctx, t, chatID), "a head under edit does not change a busy state")
}

package chatd

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

// TestEnsureChatTreeRootPublishesNothing verifies that creating a root and
// adopting chats emits no watch event, no ownership hint, and no state
// update for the root.
func TestEnsureChatTreeRootPublishesNothing(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	recorder := newRecordingPubsub(ps)
	server := newInternalTestServer(t, db, recorder, chatprovider.ProviderAPIKeys{})
	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)

	_ = dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "legacy",
	})

	root, created, err := server.EnsureChatTreeRoot(ctx, EnsureChatTreeRootOptions{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		ResolveModelConfigID: func(context.Context) (uuid.UUID, error) {
			return model.ID, nil
		},
	})
	require.NoError(t, err)
	require.True(t, created)

	require.Empty(t, recorder.watchEvents(t))
	require.Empty(t, recorder.ownershipMessages(t))
	require.Empty(t, recorder.stateUpdateMessages(t, root.ID))

	// The root's history is system messages only.
	messages, err := db.GetChatMessagesForPromptByChatID(ctx, root.ID)
	require.NoError(t, err)
	require.NotEmpty(t, messages)
	for _, m := range messages {
		require.Equal(t, database.ChatMessageRoleSystem, m.Role)
	}
	require.Equal(t, database.ChatStatusWaiting, root.Status)
}

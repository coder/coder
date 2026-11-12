package chatd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

// TestSubagentSpawnStartsWithAutomationsOff creates a child of a chat whose
// switch is on, the way spawn_agent does. The child starts with the switch
// off: the tool is for root chats, and no product path turns a child's switch
// on.
func TestSubagentSpawnStartsWithAutomationsOff(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	parent := createInternalParentChat(ctx, t, server, db, org.ID, user.ID, model.ID, "parent")
	require.NoError(t, db.UpdateChatManageAutomationsEnabledByID(ctx, database.UpdateChatManageAutomationsEnabledByIDParams{
		ID: parent.ID, ManageAutomationsEnabled: true,
	}))
	parent, err := db.GetChatByID(ctx, parent.ID)
	require.NoError(t, err)
	require.True(t, parent.ManageAutomationsEnabled)

	child, err := server.createChildSubagentChatWithOptions(ctx, parent, "do work", "", childSubagentChatOptions{})
	require.NoError(t, err)

	stored, err := db.GetChatByID(ctx, child.ID)
	require.NoError(t, err)
	require.False(t, stored.ManageAutomationsEnabled)
}

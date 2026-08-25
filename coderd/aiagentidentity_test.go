package coderd_test

import (
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aiagentidentity"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/entity"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestAIAgentVisibilityAndListing(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	client := coderdtest.New(t, &coderdtest.Options{Database: db})
	first := coderdtest.CreateFirstUser(t, client)

	agentUser, err := aiagentidentity.Create(ctx, db, aiagentidentity.CreateParams{
		OwnerID:        first.UserID,
		OrganizationID: first.OrganizationID,
		OriginType:     entity.CreationSiteTypeWorkspace,
		OriginID:       uuid.New(),
	})
	require.NoError(t, err)

	users, err := client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.False(t, slices.ContainsFunc(users.Users, func(user codersdk.User) bool {
		return user.ID == agentUser.ID
	}))

	agents, err := client.AIAgentsByUser(ctx, codersdk.Me)
	require.NoError(t, err)
	require.Len(t, agents, 1)
	require.Equal(t, agentUser.ID, agents[0].ID)
	// The name is computed, so the endpoint derives the same string the agent
	// would have carried in a users row.
	require.Equal(t, entity.DisplayName(entity.CreationSiteTypeWorkspace, agentUser.ID), agents[0].Username)
	require.Equal(t, codersdk.AIAgentOriginWorkspace, agents[0].OriginType)
	require.False(t, agents[0].Deleted)

	// Retirement in the ledger is what the endpoint reports as deleted.
	require.NoError(t, entity.RetireAIAgent(ctx, db, agentUser.ID, entity.EventAIAgentKill,
		entity.Ref{Type: entity.TypeUser, ID: first.UserID}, dbtime.Now()))
	agents, err = client.AIAgentsByUser(ctx, codersdk.Me)
	require.NoError(t, err)
	require.Len(t, agents, 1)
	require.True(t, agents[0].Deleted)
}

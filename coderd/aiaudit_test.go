package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestAIAuditAgents(t *testing.T) {
	t.Parallel()

	t.Run("SponsorScope", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitLong)

		agent := seedAIAgent(ctx, t, db, member.ID)

		// The caller is the default sponsor.
		agents, err := memberClient.AIAuditAgents(ctx, "")
		require.NoError(t, err)
		require.Len(t, agents, 1)
		require.Equal(t, agent.UserID, agents[0].UserID)
		require.Equal(t, member.ID, agents[0].OwnerUserID)
		require.Equal(t, string(database.AIAgentOriginWorkspace), agents[0].OriginType)
		require.NotEmpty(t, agents[0].Username)

		// "me" and the caller's own username resolve identically without
		// any audit permission.
		agents, err = memberClient.AIAuditAgents(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Len(t, agents, 1)
		agents, err = memberClient.AIAuditAgents(ctx, member.Username)
		require.NoError(t, err)
		require.Len(t, agents, 1)

		// A sponsor with no agents gets an empty list.
		agents, err = client.AIAuditAgents(ctx, "")
		require.NoError(t, err)
		require.Empty(t, agents)

		// Members cannot name another sponsor, existing or not; both are
		// 403 so the parameter cannot probe usernames.
		var sdkErr *codersdk.Error
		_, err = memberClient.AIAuditAgents(ctx, "nonexistent-user")
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})

	t.Run("AuditorCrossSponsor", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		auditorClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleAuditor())
		ctx := testutil.Context(t, testutil.WaitLong)

		agent := seedAIAgent(ctx, t, db, member.ID)

		// Auditors can name another sponsor by username or ID.
		agents, err := auditorClient.AIAuditAgents(ctx, member.Username)
		require.NoError(t, err)
		require.Len(t, agents, 1)
		require.Equal(t, agent.UserID, agents[0].UserID)
		agents, err = auditorClient.AIAuditAgents(ctx, member.ID.String())
		require.NoError(t, err)
		require.Len(t, agents, 1)

		// Unknown sponsors are a validation error for authorized callers.
		var sdkErr *codersdk.Error
		_, err = auditorClient.AIAuditAgents(ctx, "nonexistent-user")
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})
}

func seedAIAgent(ctx context.Context, t *testing.T, db database.Store, ownerID uuid.UUID) database.AIAgent {
	t.Helper()

	id := uuid.New()
	//nolint:gocritic // Unit test seeds identity rows directly.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	_, err := db.InsertAIAgentUser(sysCtx, database.InsertAIAgentUserParams{
		ID:        id,
		Username:  "ai-" + id.String()[:8],
		CreatedAt: dbtime.Now(),
	})
	require.NoError(t, err)
	agent, err := db.InsertAIAgent(sysCtx, database.InsertAIAgentParams{
		UserID:      id,
		OwnerUserID: ownerID,
		OriginType:  database.AIAgentOriginWorkspace,
		OriginID:    uuid.New(),
		CreatedAt:   dbtime.Now(),
	})
	require.NoError(t, err)
	return agent
}

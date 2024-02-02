package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspacePortShare(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	ownerClient, db := coderdtest.NewWithDatabase(t, nil)
	owner := coderdtest.CreateFirstUser(t, ownerClient)
	client, user := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

	tmpDir := t.TempDir()
	r := dbfake.WorkspaceBuild(t, db, database.Workspace{
		OrganizationID: owner.OrganizationID,
		OwnerID:        user.ID,
	}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
		agents[0].Directory = tmpDir
		return agents
	}).Do()
	agents, err := db.GetWorkspaceAgentsInLatestBuildByWorkspaceID(dbauthz.As(ctx, coderdtest.AuthzUserSubject(user, owner.OrganizationID)), r.Workspace.ID)
	require.NoError(t, err)

	// negative level
	err = client.UpdateWorkspaceAgentPortShare(ctx, r.Workspace.ID, codersdk.UpdateWorkspaceAgentPortShareRequest{
		AgentName:  agents[0].Name,
		Port:       8080,
		ShareLevel: codersdk.WorkspaceAgentPortShareLevel(-1),
	})
	require.Error(t, err)

	// level too high
	err = client.UpdateWorkspaceAgentPortShare(ctx, r.Workspace.ID, codersdk.UpdateWorkspaceAgentPortShareRequest{
		AgentName:  agents[0].Name,
		Port:       8080,
		ShareLevel: codersdk.WorkspaceAgentPortShareLevel(3),
	})
	require.Error(t, err)

	// OK, ignoring template max port share level because we are AGPL
	err = client.UpdateWorkspaceAgentPortShare(ctx, r.Workspace.ID, codersdk.UpdateWorkspaceAgentPortShareRequest{
		AgentName:  agents[0].Name,
		Port:       8080,
		ShareLevel: codersdk.WorkspaceAgentPortShareLevelPublic,
	})
	require.NoError(t, err)

	ps, err := db.GetWorkspaceAgentPortShare(ctx, database.GetWorkspaceAgentPortShareParams{
		WorkspaceID: r.Workspace.ID,
		AgentName:   agents[0].Name,
		Port:        8080,
	})
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceAgentPortShareLevelPublic, ps.ShareLevel)
}

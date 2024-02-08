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

func TestPostWorkspaceAgentPortShare(t *testing.T) {
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

	// owner level should fail
	err = client.CreateWorkspaceAgentPortShare(ctx, r.Workspace.ID, codersdk.UpdateWorkspaceAgentPortShareRequest{
		AgentName:  agents[0].Name,
		Port:       8080,
		ShareLevel: codersdk.WorkspaceAgentPortShareLevel("owner"),
	})
	require.Error(t, err)

	// invalid level should fail
	err = client.CreateWorkspaceAgentPortShare(ctx, r.Workspace.ID, codersdk.UpdateWorkspaceAgentPortShareRequest{
		AgentName:  agents[0].Name,
		Port:       8080,
		ShareLevel: codersdk.WorkspaceAgentPortShareLevel("invalid"),
	})
	require.Error(t, err)

	// OK, ignoring template max port share level because we are AGPL
	err = client.CreateWorkspaceAgentPortShare(ctx, r.Workspace.ID, codersdk.UpdateWorkspaceAgentPortShareRequest{
		AgentName:  agents[0].Name,
		Port:       8080,
		ShareLevel: codersdk.WorkspaceAgentPortShareLevelPublic,
	})
	require.NoError(t, err)

	ps, err := db.GetWorkspaceAgentPortShare(dbauthz.As(ctx, coderdtest.AuthzUserSubject(user, owner.OrganizationID)), database.GetWorkspaceAgentPortShareParams{
		WorkspaceID: r.Workspace.ID,
		AgentName:   agents[0].Name,
		Port:        8080,
	})
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceAgentPortShareLevelPublic, ps.ShareLevel)
}

func TestGetWorkspaceAgentPortShares(t *testing.T) {
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

	err = client.CreateWorkspaceAgentPortShare(ctx, r.Workspace.ID, codersdk.UpdateWorkspaceAgentPortShareRequest{
		AgentName:  agents[0].Name,
		Port:       8080,
		ShareLevel: codersdk.WorkspaceAgentPortShareLevelPublic,
	})
	require.NoError(t, err)

	ps, err := client.GetWorkspaceAgentPortShares(ctx, r.Workspace.ID)
	require.NoError(t, err)
	require.Len(t, ps.Shares, 1)
	require.EqualValues(t, agents[0].Name, ps.Shares[0].AgentName)
	require.EqualValues(t, 8080, ps.Shares[0].Port)
	require.EqualValues(t, codersdk.WorkspaceAgentPortShareLevelPublic, ps.Shares[0].ShareLevel)
}

func TestDeleteWorkspaceAgentPortShare(t *testing.T) {
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

	err = client.CreateWorkspaceAgentPortShare(ctx, r.Workspace.ID, codersdk.UpdateWorkspaceAgentPortShareRequest{
		AgentName:  agents[0].Name,
		Port:       8080,
		ShareLevel: codersdk.WorkspaceAgentPortShareLevelPublic,
	})
	require.NoError(t, err)

	err = client.DeleteWorkspaceAgentPortShare(ctx, r.Workspace.ID, codersdk.DeleteWorkspaceAgentPortShareRequest{
		AgentName: agents[0].Name,
		Port:      8080,
	})
	require.NoError(t, err)

	_, err = db.GetWorkspaceAgentPortShare(dbauthz.As(ctx, coderdtest.AuthzUserSubject(user, owner.OrganizationID)), database.GetWorkspaceAgentPortShareParams{
		WorkspaceID: r.Workspace.ID,
		AgentName:   agents[0].Name,
		Port:        8080,
	})
	require.Error(t, err)
}

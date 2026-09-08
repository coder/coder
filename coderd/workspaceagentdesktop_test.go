package coderd_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestWorkspaceAgentDesktop covers the authorization and readiness checks of
// the desktop endpoint. The successful path requires a real desktop runtime
// inside the agent and is exercised manually.
func TestWorkspaceAgentDesktop(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T) (*codersdk.Client, codersdk.CreateFirstUserResponse, codersdk.WorkspaceAgent) {
		t.Helper()
		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentWorkspaceDesktop)}
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{
			DeploymentValues: dv,
		})
		owner := coderdtest.CreateFirstUser(t, client)
		r := dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: owner.OrganizationID,
			OwnerID:        owner.UserID,
		}).WithAgent().Do()
		workspace, err := client.Workspace(testutil.Context(t, testutil.WaitShort), r.Workspace.ID)
		require.NoError(t, err)
		require.Len(t, workspace.LatestBuild.Resources, 1)
		require.Len(t, workspace.LatestBuild.Resources[0].Agents, 1)
		return client, owner, workspace.LatestBuild.Resources[0].Agents[0]
	}

	t.Run("AgentNotConnected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, _, agent := setup(t)

		res, err := client.Request(ctx, http.MethodGet,
			fmt.Sprintf("/api/v2/workspaceagents/%s/desktop", agent.ID), nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusBadRequest, res.StatusCode)
	})

	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, owner, agent := setup(t)
		// A member of the organization with no access to the workspace
		// must not be able to tell the agent exists.
		other, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)

		res, err := other.Request(ctx, http.MethodGet,
			fmt.Sprintf("/api/v2/workspaceagents/%s/desktop", agent.ID), nil)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})
}

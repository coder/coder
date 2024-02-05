package coderd_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestJFrogXrayScan(t *testing.T) {
	t.Parallel()

	t.Run("Post/Get", func(t *testing.T) {
		t.Parallel()
		ownerClient, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureMultipleExternalAuth: 1},
			},
		})

		tac, ta := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleTemplateAdmin())

		wsResp := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: owner.OrganizationID,
			OwnerID:        ta.ID,
		}).WithAgent().Do()

		ws := coderdtest.MustWorkspace(t, tac, wsResp.Workspace.ID)
		require.Len(t, ws.LatestBuild.Resources, 1)
		require.Len(t, ws.LatestBuild.Resources[0].Agents, 1)

		agentID := ws.LatestBuild.Resources[0].Agents[0].ID
		expectedPayload := codersdk.JFrogXrayScan{
			WorkspaceID: ws.ID,
			AgentID:     agentID,
			Critical:    19,
			High:        5,
			Medium:      3,
			ResultsURL:  "https://hello-world",
		}

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := tac.PostJFrogXrayScan(ctx, expectedPayload)
		require.NoError(t, err)

		resp1, err := tac.JFrogXRayScan(ctx, ws.ID, agentID)
		require.NoError(t, err)
		require.Equal(t, expectedPayload, resp1)

		// Can update again without error.
		expectedPayload = codersdk.JFrogXrayScan{
			WorkspaceID: ws.ID,
			AgentID:     agentID,
			Critical:    20,
			High:        22,
			Medium:      8,
			ResultsURL:  "https://goodbye-world",
		}
		err = tac.PostJFrogXrayScan(ctx, expectedPayload)
		require.NoError(t, err)

		resp2, err := tac.JFrogXRayScan(ctx, ws.ID, agentID)
		require.NoError(t, err)
		require.NotEqual(t, expectedPayload, resp1)
		require.Equal(t, expectedPayload, resp2)
	})

	t.Run("MemberPostUnauthorized", func(t *testing.T) {
		t.Parallel()

		ownerClient, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{codersdk.FeatureMultipleExternalAuth: 1},
			},
		})

		memberClient, member := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

		wsResp := dbfake.WorkspaceBuild(t, db, database.Workspace{
			OrganizationID: owner.OrganizationID,
			OwnerID:        member.ID,
		}).WithAgent().Do()

		ws := coderdtest.MustWorkspace(t, memberClient, wsResp.Workspace.ID)
		require.Len(t, ws.LatestBuild.Resources, 1)
		require.Len(t, ws.LatestBuild.Resources[0].Agents, 1)

		agentID := ws.LatestBuild.Resources[0].Agents[0].ID
		expectedPayload := codersdk.JFrogXrayScan{
			WorkspaceID: ws.ID,
			AgentID:     agentID,
			Critical:    19,
			High:        5,
			Medium:      3,
			ResultsURL:  "https://hello-world",
		}

		ctx := testutil.Context(t, testutil.WaitMedium)
		err := memberClient.PostJFrogXrayScan(ctx, expectedPayload)
		require.Error(t, err)
		cerr, ok := codersdk.AsError(err)
		require.True(t, ok)
		require.Equal(t, http.StatusNotFound, cerr.StatusCode())

		err = ownerClient.PostJFrogXrayScan(ctx, expectedPayload)
		require.NoError(t, err)

		// We should still be able to fetch.
		resp1, err := memberClient.JFrogXRayScan(ctx, ws.ID, agentID)
		require.NoError(t, err)
		require.Equal(t, expectedPayload, resp1)
	})
}

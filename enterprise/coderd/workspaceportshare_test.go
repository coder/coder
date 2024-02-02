package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspacePortShare(t *testing.T) {
	t.Parallel()

	ownerClient, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureControlSharedPorts: 1,
			},
		},
	})
	client, user := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleTemplateAdmin())
	workspace, agent := setupWorkspaceAgent(t, client, codersdk.CreateFirstUserResponse{
		UserID:         user.ID,
		OrganizationID: owner.OrganizationID,
	}, 0)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	err := client.UpdateWorkspaceAgentPortShare(ctx, workspace.ID, codersdk.UpdateWorkspaceAgentPortShareRequest{
		AgentName:  agent.Name,
		Port:       8080,
		ShareLevel: codersdk.WorkspaceAgentPortShareLevelPublic,
	})
	require.NoError(t, err)

	ps, err := db.GetWorkspaceAgentPortShare(ctx, database.GetWorkspaceAgentPortShareParams{
		WorkspaceID: workspace.ID,
		AgentName:   agent.Name,
		Port:        8080,
	})
	require.NoError(t, err)
	require.Equal(t, codersdk.WorkspaceAgentPortShareLevelPublic, ps.ShareLevel)
}

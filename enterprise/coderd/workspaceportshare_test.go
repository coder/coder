package coderd_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspacePortShare(t *testing.T) {
	t.Parallel()

	dep := coderdtest.DeploymentValues(t)
	dep.Experiments = append(dep.Experiments, string(codersdk.ExperimentSharedPorts))
	ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			IncludeProvisionerDaemon: true,
			DeploymentValues:         dep,
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

	// try to update port share with template max port share level owner
	_, err := client.UpsertWorkspaceAgentPortShare(ctx, workspace.ID, codersdk.UpsertWorkspaceAgentPortShareRequest{
		AgentName:  agent.Name,
		Port:       8080,
		ShareLevel: codersdk.WorkspaceAgentPortShareLevelPublic,
	})
	require.Error(t, err, "Port sharing level not allowed")

	// update the template max port share level to public
	var level codersdk.WorkspaceAgentPortShareLevel = codersdk.WorkspaceAgentPortShareLevelPublic
	client.UpdateTemplateMeta(ctx, workspace.TemplateID, codersdk.UpdateTemplateMeta{
		MaxPortShareLevel: &level,
	})

	// OK
	ps, err := client.UpsertWorkspaceAgentPortShare(ctx, workspace.ID, codersdk.UpsertWorkspaceAgentPortShareRequest{
		AgentName:  agent.Name,
		Port:       8080,
		ShareLevel: codersdk.WorkspaceAgentPortShareLevelPublic,
	})
	require.NoError(t, err)
	require.EqualValues(t, codersdk.WorkspaceAgentPortShareLevelPublic, ps.ShareLevel)
}

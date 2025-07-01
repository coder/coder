package coderd_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

// TestAgentGitSSHKeyCustomRoles tests that the agent can fetch its git ssh key when
// the user has a custom role in a second workspace.
func TestAgentGitSSHKeyCustomRoles(t *testing.T) {
	t.Parallel()

	owner, _ := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureCustomRoles:                1,
				codersdk.FeatureMultipleOrganizations:      1,
				codersdk.FeatureExternalProvisionerDaemons: 1,
			},
		},
	})

	// When custom roles exist in a second organization
	org := coderdenttest.CreateOrganization(t, owner, coderdenttest.CreateOrganizationOptions{
		IncludeProvisionerDaemon: true,
	})

	ctx := testutil.Context(t, testutil.WaitShort)
	//nolint:gocritic // required to make orgs
	newRole, err := owner.CreateOrganizationRole(ctx, codersdk.Role{
		Name:            "custom",
		OrganizationID:  org.ID.String(),
		DisplayName:     "",
		SitePermissions: nil,
		OrganizationPermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
			codersdk.ResourceTemplate: {codersdk.ActionRead, codersdk.ActionCreate, codersdk.ActionUpdate},
		}),
		UserPermissions: nil,
	})
	require.NoError(t, err)

	// Create the new user
	client, _ := coderdtest.CreateAnotherUser(t, owner, org.ID, rbac.RoleIdentifier{Name: newRole.Name, OrganizationID: org.ID})

	// Create the workspace + agent
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, org.ID, &echo.Responses{
		Parse:          echo.ParseComplete,
		ProvisionPlan:  echo.PlanComplete,
		ProvisionApply: echo.ProvisionApplyWithAgent(authToken),
	})
	project := coderdtest.CreateTemplate(t, client, org.ID, version.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, project.ID)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, client, workspace.LatestBuild.ID)

	agentClient := agentsdk.New(client.URL)
	agentClient.SetSessionToken(authToken)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	agentKey, err := agentClient.GitSSHKey(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, agentKey.PrivateKey)
}

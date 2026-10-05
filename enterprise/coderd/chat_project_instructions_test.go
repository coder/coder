package coderd_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestChatProjectInstructionsUpdateOnlyRole(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentChatProjects)}
	ownerClient, firstUser := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{DeploymentValues: dv},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{codersdk.FeatureCustomRoles: 1},
		},
	})
	owner := codersdk.NewExperimentalClient(ownerClient)
	ctx := testutil.Context(t, testutil.WaitLong)

	project, err := owner.CreateChatProject(ctx, firstUser.OrganizationID, codersdk.CreateChatProjectRequest{
		Name: "Update Only Project",
	})
	require.NoError(t, err)

	role, err := owner.CreateOrganizationRole(ctx, codersdk.Role{
		Name:           "chat-project-update-only",
		OrganizationID: firstUser.OrganizationID.String(),
		OrganizationPermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
			codersdk.ResourceChatProject: {codersdk.ActionUpdate},
		}),
	})
	require.NoError(t, err)
	editorClient, editor := coderdtest.CreateAnotherUser(t, ownerClient, firstUser.OrganizationID,
		rbac.RoleIdentifier{Name: role.Name, OrganizationID: firstUser.OrganizationID})
	updateOnly := codersdk.NewExperimentalClient(editorClient)

	// The handler checks update permission, so a role without read can
	// still save and receives the saved value.
	saved, err := updateOnly.UpdateChatProjectInstructions(ctx, firstUser.OrganizationID, project.ID, codersdk.UpdateChatProjectInstructionsRequest{
		Instructions: "Keep answers short.",
	})
	require.NoError(t, err)
	require.Equal(t, "Keep answers short.", saved.Instructions)
	require.NotNil(t, saved.UpdatedBy)
	require.Equal(t, editor.ID, saved.UpdatedBy.ID)
}

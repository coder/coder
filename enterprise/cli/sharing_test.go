package cli_test

import (
	"bytes"
	"testing"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSharingShareEnterprise(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}

	t.Run("ShareWithGroups_Simple", func(t *testing.T) {
		t.Parallel()

		var (
			client, db, orgOwner = coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					DeploymentValues: dv,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureTemplateRBAC: 1,
					},
				},
			})
			workspaceOwnerClient, workspaceOwner = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID, rbac.ScopedRoleOrgAuditor(orgOwner.OrganizationID))
			workspace                            = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:        workspaceOwner.ID,
				OrganizationID: orgOwner.OrganizationID,
			}).Do().Workspace
			_, orgMember = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID)
		)

		ctx := testutil.Context(t, testutil.WaitMedium)

		group, err := client.CreateGroup(ctx, orgOwner.OrganizationID, codersdk.CreateGroupRequest{
			Name:        "new-group",
			DisplayName: "new-group",
		})
		require.NoError(t, err)
		group, err = client.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{orgMember.ID.String()},
		})
		require.NoError(t, err)

		var inv, root = clitest.New(t, "sharing", "share", workspace.Name, "--org", orgOwner.OrganizationID.String(), "--group", group.Name)
		clitest.SetupConfig(t, workspaceOwnerClient, root)

		out := bytes.NewBuffer(nil)
		inv.Stdout = out
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		acl, err := workspaceOwnerClient.WorkspaceACL(inv.Context(), workspace.ID)
		require.NoError(t, err)
		assert.Len(t, acl.Groups, 1)
		assert.Equal(t, acl.Groups[0].Group.ID, group.ID)
		assert.Equal(t, acl.Groups[0].Role, codersdk.WorkspaceRoleUse)
	})
}

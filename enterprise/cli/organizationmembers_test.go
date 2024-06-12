package cli_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestEnterpriseListOrganizationMembers(t *testing.T) {
	t.Parallel()

	t.Run("CustomRole", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentCustomRoles)}
		ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)
		//nolint:gocritic // only owners can patch roles
		customRole, err := ownerClient.PatchOrganizationRole(ctx, owner.OrganizationID, codersdk.Role{
			Name:            "custom",
			OrganizationID:  owner.OrganizationID.String(),
			DisplayName:     "Custom Role",
			SitePermissions: nil,
			OrganizationPermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}),
			UserPermissions: nil,
		})
		require.NoError(t, err)

		client, user := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin(), rbac.RoleIdentifier{
			Name:           customRole.Name,
			OrganizationID: owner.OrganizationID,
		}, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))

		inv, root := clitest.New(t, "organization", "members", "-c", "user_id,username,organization_roles")
		clitest.SetupConfig(t, client, root)

		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, buf.String(), user.Username)
		require.Contains(t, buf.String(), owner.UserID.String())
		// Check the display name is the value in the cli list
		require.Contains(t, buf.String(), customRole.DisplayName)
	})
}

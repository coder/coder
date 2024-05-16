package coderd_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestCustomRole(t *testing.T) {
	t.Parallel()

	// Create, assign, and use a custom role
	//nolint:gocritic
	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentCustomRoles)}
		owner, first := coderdenttest.New(t, &coderdenttest.Options{
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

		//nolint:gocritic // owner is required for this
		role, err := owner.UpsertCustomSiteRole(ctx, codersdk.Role{
			Name:        "test-role",
			DisplayName: "Testing Purposes",
			// Basically creating a template admin manually
			SitePermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceTemplate:  {codersdk.ActionCreate, codersdk.ActionRead, codersdk.ActionUpdate, codersdk.ActionViewInsights},
				codersdk.ResourceFile:      {codersdk.ActionCreate, codersdk.ActionRead},
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}),
			OrganizationPermissions: nil,
			UserPermissions:         nil,
		})
		require.NoError(t, err, "upsert role")

		// Assign the custom template admin role
		tmplAdmin, user := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, role.Name)

		// Assert the role exists
		roleNamesF := func(role codersdk.SlimRole) string { return role.Name }
		require.Contains(t, db2sdk.List(user.Roles, roleNamesF), role.Name)

		// Try to create a template version
		coderdtest.CreateTemplateVersion(t, tmplAdmin, first.OrganizationID, nil)
	})
}

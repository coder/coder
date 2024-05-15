package coderd_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestCustomRole(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)
		original, err := owner.ListSiteRoles(ctx)
		require.NoError(t, err, "list available")

		role, err := owner.UpsertCustomSiteRole(ctx, codersdk.RolePermissions{
			Name:        "test-role",
			DisplayName: "Testing Purposes",
			SitePermissions: []codersdk.Permission{
				// Let's make a template admin ourselves
				{
					Negate:       false,
					ResourceType: codersdk.ResourceTemplate,
					Action:       "",
				},
			},
			OrganizationPermissions: nil,
			UserPermissions:         nil,
		})
		require.NoError(t, err, "upsert role")

		coderdtest.CreateAnotherUser()

	})
}

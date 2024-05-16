package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestShowRoles(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentCustomRoles)}
		owner, admin := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})

		// Requires an owner
		client, _ := coderdtest.CreateAnotherUser(t, owner, admin.OrganizationID, rbac.RoleOwner())

		const expectedRole = "test-role"
		ctx := testutil.Context(t, testutil.WaitMedium)
		_, err := client.PatchRole(ctx, codersdk.Role{
			Name:        expectedRole,
			DisplayName: "Test Role",
			SitePermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceWorkspace: {codersdk.ActionRead, codersdk.ActionUpdate},
			}),
		})
		require.NoError(t, err, "create role")

		inv, conf := newCLI(t, "roles", "show", "test-role")

		pty := ptytest.New(t)
		inv.Stdout = pty.Output()
		clitest.SetupConfig(t, client, conf)

		err = inv.Run()
		require.NoError(t, err)

		matches := []string{
			"test-role", "2 permissions",
		}

		for _, match := range matches {
			pty.ExpectMatch(match)
		}
	})
}

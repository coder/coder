package cli_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestEditOrganizationRoles(t *testing.T) {
	t.Parallel()

	// Unit test uses --stdin and json as the role input. The interactive cli would
	// be hard to drive from a unit test.
	t.Run("JSON", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentCustomRoles)}
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
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
		inv, root := clitest.New(t, "organization", "roles", "edit", "--stdin")
		inv.Stdin = bytes.NewBufferString(fmt.Sprintf(`{
    "name": "new-role",
    "organization_id": "%s",
    "display_name": "",
    "site_permissions": [],
    "organization_permissions": [
		{
		  "resource_type": "workspace",
		  "action": "read"
		}
    ],
    "user_permissions": [],
    "assignable": false,
    "built_in": false
  }`, owner.OrganizationID.String()))
		//nolint:gocritic // only owners can edit roles
		clitest.SetupConfig(t, client, root)

		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, buf.String(), "new-role")
	})

	t.Run("InvalidRole", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentCustomRoles)}
		client, owner := coderdenttest.New(t, &coderdenttest.Options{
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
		inv, root := clitest.New(t, "organization", "roles", "edit", "--stdin")
		inv.Stdin = bytes.NewBufferString(fmt.Sprintf(`{
    "name": "new-role",
    "organization_id": "%s",
    "display_name": "",
    "site_permissions": [
		{
		  "resource_type": "workspace",
		  "action": "read"
		}
	],
    "organization_permissions": [
		{
		  "resource_type": "workspace",
		  "action": "read"
		}
    ],
    "user_permissions": [],
    "assignable": false,
    "built_in": false
  }`, owner.OrganizationID.String()))
		//nolint:gocritic // only owners can edit roles
		clitest.SetupConfig(t, client, root)

		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.ErrorContains(t, err, "not allowed to assign site wide permissions for an organization role")
	})
}

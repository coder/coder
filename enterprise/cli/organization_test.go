package cli_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestCreateOrganizationRoles(t *testing.T) {
	t.Parallel()

	// Unit test uses --stdin and json as the role input. The interactive cli would
	// be hard to drive from a unit test.
	t.Run("JSON", func(t *testing.T) {
		t.Parallel()

		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, root := clitest.New(t, "organization", "roles", "create", "--stdin")
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

		client, owner := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, root := clitest.New(t, "organization", "roles", "create", "--stdin")
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

func TestShowOrganizations(t *testing.T) {
	t.Parallel()

	t.Run("OnlyID", func(t *testing.T) {
		t.Parallel()

		ownerClient, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations:      1,
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
		})

		// Owner is required to make orgs
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, first.OrganizationID, rbac.RoleOwner())

		ctx := testutil.Context(t, testutil.WaitMedium)
		orgs := []string{"foo", "bar"}
		for _, orgName := range orgs {
			_, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
				Name: orgName,
			})
			require.NoError(t, err)
		}

		inv, root := clitest.New(t, "organizations", "show", "--only-id", "--org="+first.OrganizationID.String())
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)
		errC := make(chan error)
		go func() {
			errC <- inv.Run()
		}()
		require.NoError(t, <-errC)
		pty.ExpectMatch(first.OrganizationID.String())
	})

	t.Run("UsingFlag", func(t *testing.T) {
		t.Parallel()
		ownerClient, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations:      1,
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
		})

		// Owner is required to make orgs
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, first.OrganizationID, rbac.RoleOwner())

		ctx := testutil.Context(t, testutil.WaitMedium)
		orgs := map[string]codersdk.Organization{
			"foo": {},
			"bar": {},
		}
		for orgName := range orgs {
			org, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
				Name: orgName,
			})
			require.NoError(t, err)
			orgs[orgName] = org
		}

		inv, root := clitest.New(t, "organizations", "show", "selected", "--only-id", "-O=bar")
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)
		errC := make(chan error)
		go func() {
			errC <- inv.Run()
		}()
		require.NoError(t, <-errC)
		pty.ExpectMatch(orgs["bar"].ID.String())
	})
}

func TestUpdateOrganizationRoles(t *testing.T) {
	t.Parallel()

	t.Run("JSON", func(t *testing.T) {
		ownerClient, db, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleOwner())

		// Create a role in the DB with no permissions
		const expectedRole = "test-role"
		dbgen.CustomRole(t, db, database.CustomRole{
			Name:            expectedRole,
			DisplayName:     "Expected",
			SitePermissions: nil,
			OrgPermissions:  nil,
			UserPermissions: nil,
			OrganizationID: uuid.NullUUID{
				UUID:  owner.OrganizationID,
				Valid: true,
			},
		})

		// Update the new role via JSON
		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, root := clitest.New(t, "organization", "roles", "update", "--stdin")
		inv.Stdin = bytes.NewBufferString(fmt.Sprintf(`{
    "name": "test-role",
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
		require.Contains(t, buf.String(), "test-role")
		require.Contains(t, buf.String(), "1 permissions")
	})

	t.Run("InvalidRole", func(t *testing.T) {
		t.Parallel()

		ownerClient, _, owner := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleOwner())

		// Update the new role via JSON
		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, root := clitest.New(t, "organization", "roles", "update", "--stdin")
		inv.Stdin = bytes.NewBufferString(fmt.Sprintf(`{
    "name": "test-role",
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
		require.Error(t, err)
		require.ErrorContains(t, err, "The role test-role does not exist exists.")
	})
}

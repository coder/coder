package cli_test

import (
	"bytes"
	"fmt"
	"strings"
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
	"github.com/coder/coder/v2/testutil/expecter"
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
		stdout := expecter.NewAttachedToInvocation(t, inv)
		errC := make(chan error)
		go func() {
			errC <- inv.Run()
		}()
		require.NoError(t, <-errC)
		stdout.ExpectMatch(ctx, first.OrganizationID.String())
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
		stdout := expecter.NewAttachedToInvocation(t, inv)
		errC := make(chan error)
		go func() {
			errC <- inv.Run()
		}()
		require.NoError(t, <-errC)
		stdout.ExpectMatch(ctx, orgs["bar"].ID.String())
	})
}

func TestUpdateOrganizationRoles(t *testing.T) {
	t.Parallel()

	t.Run("JSON", func(t *testing.T) {
		t.Parallel()

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
		require.ErrorContains(t, err, "The role test-role does not exist.")
	})
}

func TestEditOrganization(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T) (*codersdk.Client, codersdk.CreateFirstUserResponse) {
		t.Helper()
		return coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
	}

	t.Run("SetRoles", func(t *testing.T) {
		t.Parallel()

		client, first := setup(t)
		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, root := clitest.New(t, "organizations", "edit", "-y",
			"--default-org-member-roles", codersdk.RoleOrganizationTemplateAdmin+", "+codersdk.RoleOrganizationAuditor,
			"--default-org-member-roles", codersdk.RoleOrganizationWorkspaceAccess,
			// Duplicates collapse.
			"--default-org-member-roles", codersdk.RoleOrganizationAuditor)
		//nolint:gocritic // only owners can update orgs
		clitest.SetupConfig(t, client, root)
		buf := new(bytes.Buffer)
		inv.Stdout = buf

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, buf.String(), "Default member roles: "+
			codersdk.RoleOrganizationTemplateAdmin+", "+
			codersdk.RoleOrganizationAuditor+", "+
			codersdk.RoleOrganizationWorkspaceAccess)

		//nolint:gocritic // only owners can read all orgs
		org, err := client.Organization(ctx, first.OrganizationID)
		require.NoError(t, err)
		require.Equal(t, []string{
			codersdk.RoleOrganizationTemplateAdmin,
			codersdk.RoleOrganizationAuditor,
			codersdk.RoleOrganizationWorkspaceAccess,
		}, org.DefaultOrgMemberRoles)
	})

	t.Run("ClearRolesRevokesAccess", func(t *testing.T) {
		t.Parallel()

		client, first := setup(t)
		ctx := testutil.Context(t, testutil.WaitMedium)
		memberClient, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)

		workspaceCheck := codersdk.AuthorizationRequest{Checks: map[string]codersdk.AuthorizationCheck{
			"create": {
				Object: codersdk.AuthorizationObject{
					ResourceType:   codersdk.ResourceWorkspace,
					OrganizationID: first.OrganizationID.String(),
					OwnerID:        "me",
				},
				Action: codersdk.ActionCreate,
			},
		}}
		before, err := memberClient.AuthCheck(ctx, workspaceCheck)
		require.NoError(t, err)
		require.True(t, before["create"], "member should start with workspace access")

		inv, root := clitest.New(t, "organizations", "edit", "-y", "--clear-default-org-member-roles")
		//nolint:gocritic // only owners can update orgs
		clitest.SetupConfig(t, client, root)
		buf := new(bytes.Buffer)
		inv.Stdout = buf

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, buf.String(), "Default member roles: (none)")

		//nolint:gocritic // only owners can read all orgs
		org, err := client.Organization(ctx, first.OrganizationID)
		require.NoError(t, err)
		require.NotNil(t, org.DefaultOrgMemberRoles)
		require.Len(t, org.DefaultOrgMemberRoles, 0)

		after, err := memberClient.AuthCheck(ctx, workspaceCheck)
		require.NoError(t, err)
		require.False(t, after["create"], "member should lose workspace access")
	})

	t.Run("LeavesOtherFields", func(t *testing.T) {
		t.Parallel()

		client, first := setup(t)
		ctx := testutil.Context(t, testutil.WaitMedium)
		//nolint:gocritic // only owners can read all orgs
		before, err := client.Organization(ctx, first.OrganizationID)
		require.NoError(t, err)

		inv, root := clitest.New(t, "organizations", "edit", "-y",
			"--default-org-member-roles", codersdk.RoleOrganizationAuditor)
		//nolint:gocritic // only owners can update orgs
		clitest.SetupConfig(t, client, root)
		inv.Stdout = new(bytes.Buffer)

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		//nolint:gocritic // only owners can read all orgs
		after, err := client.Organization(ctx, first.OrganizationID)
		require.NoError(t, err)
		require.Equal(t, []string{codersdk.RoleOrganizationAuditor}, after.DefaultOrgMemberRoles)
		require.NotEqual(t, before.DefaultOrgMemberRoles, after.DefaultOrgMemberRoles)
		require.Equal(t, before.Name, after.Name)
		require.Equal(t, before.DisplayName, after.DisplayName)
		require.Equal(t, before.Description, after.Description)
		require.Equal(t, before.Icon, after.Icon)
	})

	t.Run("RemovalRequiresConfirmation", func(t *testing.T) {
		t.Parallel()

		client, first := setup(t)
		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, root := clitest.New(t, "organizations", "edit", "--clear-default-org-member-roles")
		//nolint:gocritic // only owners can update orgs
		clitest.SetupConfig(t, client, root)
		pty := ptytest.New(t).Attach(inv)

		done := make(chan error, 1)
		go func() {
			done <- inv.WithContext(ctx).Run()
		}()

		pty.ExpectMatch(ctx, codersdk.RoleOrganizationWorkspaceAccess)
		pty.WriteLine("no")
		require.Error(t, testutil.RequireReceive(ctx, t, done))

		//nolint:gocritic // only owners can read all orgs
		org, err := client.Organization(ctx, first.OrganizationID)
		require.NoError(t, err)
		require.Equal(t, rbac.DefaultOrgMemberRoles(), org.DefaultOrgMemberRoles)
	})

	t.Run("AddingRolesSkipsConfirmation", func(t *testing.T) {
		t.Parallel()

		client, first := setup(t)
		ctx := testutil.Context(t, testutil.WaitMedium)
		roles := append(rbac.DefaultOrgMemberRoles(), codersdk.RoleOrganizationAuditor)
		inv, root := clitest.New(t, "organizations", "edit",
			"--default-org-member-roles", strings.Join(roles, ","))
		//nolint:gocritic // only owners can update orgs
		clitest.SetupConfig(t, client, root)
		inv.Stdout = new(bytes.Buffer)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		//nolint:gocritic // only owners can read all orgs
		org, err := client.Organization(ctx, first.OrganizationID)
		require.NoError(t, err)
		require.Equal(t, roles, org.DefaultOrgMemberRoles)
	})

	t.Run("InvalidFlags", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name          string
			args          []string
			errorContains string
		}{
			{
				name:          "NoFlags",
				args:          []string{},
				errorContains: "no changes requested",
			},
			{
				name:          "MutuallyExclusive",
				args:          []string{"--default-org-member-roles", codersdk.RoleOrganizationAuditor, "--clear-default-org-member-roles"},
				errorContains: "mutually exclusive",
			},
			{
				// serpent resets the slice on an empty value, so this
				// discards the earlier role instead of appending.
				name:          "EmptyValueDiscardsEarlierRoles",
				args:          []string{"--default-org-member-roles", codersdk.RoleOrganizationAuditor, "--default-org-member-roles", ""},
				errorContains: "requires at least one role",
			},
			{
				name:          "EmptyValueAlone",
				args:          []string{"--default-org-member-roles", ""},
				errorContains: "requires at least one role",
			},
			{
				name:          "EmptyRoleName",
				args:          []string{"--default-org-member-roles", codersdk.RoleOrganizationAuditor + ","},
				errorContains: "empty role name",
			},
			{
				name:          "WhitespaceRoleName",
				args:          []string{"--default-org-member-roles", " "},
				errorContains: "empty role name",
			},
			{
				name:          "NonBuiltInRole",
				args:          []string{"--default-org-member-roles", "not-a-built-in-role"},
				errorContains: "Invalid default_org_member_roles entry",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				client, first := setup(t)
				ctx := testutil.Context(t, testutil.WaitMedium)
				inv, root := clitest.New(t, append([]string{"organizations", "edit", "-y"}, tc.args...)...)
				//nolint:gocritic // only owners can update orgs
				clitest.SetupConfig(t, client, root)
				inv.Stdout = new(bytes.Buffer)

				err := inv.WithContext(ctx).Run()
				require.ErrorContains(t, err, tc.errorContains)

				//nolint:gocritic // only owners can read all orgs
				org, err := client.Organization(ctx, first.OrganizationID)
				require.NoError(t, err)
				require.Equal(t, rbac.DefaultOrgMemberRoles(), org.DefaultOrgMemberRoles)
			})
		}
	})

	t.Run("MemberDenied", func(t *testing.T) {
		t.Parallel()

		client, first := setup(t)
		ctx := testutil.Context(t, testutil.WaitMedium)
		memberClient, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)

		inv, root := clitest.New(t, "organizations", "edit", "-y",
			"--default-org-member-roles", codersdk.RoleOrganizationAdmin)
		clitest.SetupConfig(t, memberClient, root)
		inv.Stdout = new(bytes.Buffer)

		err := inv.WithContext(ctx).Run()
		require.Error(t, err)

		//nolint:gocritic // only owners can read all orgs
		org, err := client.Organization(ctx, first.OrganizationID)
		require.NoError(t, err)
		require.Equal(t, rbac.DefaultOrgMemberRoles(), org.DefaultOrgMemberRoles)
	})
}

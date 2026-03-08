package cli_test

import (
	"bytes"
	"encoding/json"
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
	"github.com/coder/coder/v2/testutil"
)

func TestShowOrganizationRoles(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		ownerClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())

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

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, root := clitest.New(t, "organization", "roles", "show")
		clitest.SetupConfig(t, client, root)

		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, buf.String(), expectedRole)
	})
}


func TestCreateOrganizationRoleFromExportedJSON(t *testing.T) {
	t.Parallel()

	t.Run("ArrayInput", func(t *testing.T) {
		t.Parallel()

		ownerClient, _ := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())

		// Simulate the JSON array output from 'roles show -ojson'.
		// This includes extra fields like 'assignable' and 'built_in'
		// that are present in AssignableRoles but not in Role.
		exportedRoles := []codersdk.AssignableRoles{
			{
				Role: codersdk.Role{
					Name:           "new-imported-role",
					OrganizationID: owner.OrganizationID.String(),
					DisplayName:    "Imported Role",
					SitePermissions: []codersdk.Permission{},
					OrganizationPermissions: []codersdk.Permission{
						{
							ResourceType: codersdk.ResourceWorkspace,
							Action:       codersdk.ActionRead,
						},
					},
					UserPermissions: []codersdk.Permission{},
				},
				Assignable: true,
				BuiltIn:    false,
			},
		}

		jsonBytes, err := json.Marshal(exportedRoles)
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, root := clitest.New(t, "organization", "roles", "create", "--stdin")
		clitest.SetupConfig(t, client, root)

		inv.Stdin = bytes.NewReader(jsonBytes)
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, buf.String(), "new-imported-role")
	})

	t.Run("SingleObjectInput", func(t *testing.T) {
		t.Parallel()

		ownerClient, _ := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())

		// A single role object (not wrapped in an array) should
		// still work.
		role := codersdk.Role{
			Name:                    "single-object-role",
			OrganizationID:          owner.OrganizationID.String(),
			DisplayName:             "Single Object Role",
			SitePermissions:         []codersdk.Permission{},
			OrganizationPermissions: []codersdk.Permission{},
			UserPermissions:         []codersdk.Permission{},
		}

		jsonBytes, err := json.Marshal(role)
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, root := clitest.New(t, "organization", "roles", "create", "--stdin")
		clitest.SetupConfig(t, client, root)

		inv.Stdin = bytes.NewReader(jsonBytes)
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, buf.String(), "single-object-role")
	})

	t.Run("MultipleElementArrayRejected", func(t *testing.T) {
		t.Parallel()

		ownerClient, _ := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())

		// An array with multiple roles should be rejected.
		roles := []codersdk.Role{
			{
				Name:           "role-one",
				OrganizationID: owner.OrganizationID.String(),
			},
			{
				Name:           "role-two",
				OrganizationID: owner.OrganizationID.String(),
			},
		}

		jsonBytes, err := json.Marshal(roles)
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, root := clitest.New(t, "organization", "roles", "create", "--stdin")
		clitest.SetupConfig(t, client, root)

		inv.Stdin = bytes.NewReader(jsonBytes)
		buf := new(bytes.Buffer)
		inv.Stdout = buf
		err = inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), fmt.Sprintf("json input array has %d elements", len(roles)))
	})

	t.Run("ShowThenCreateRoundTrip", func(t *testing.T) {
		t.Parallel()

		ownerClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())

		// Create a role in the database to export.
		dbgen.CustomRole(t, db, database.CustomRole{
			Name:            "export-me",
			DisplayName:     "Export Me",
			SitePermissions: nil,
			OrgPermissions:  nil,
			UserPermissions: nil,
			OrganizationID: uuid.NullUUID{
				UUID:  owner.OrganizationID,
				Valid: true,
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)

		// Export using 'show -ojson'.
		showInv, showRoot := clitest.New(t, "organization", "roles", "show", "export-me", "-ojson")
		clitest.SetupConfig(t, client, showRoot)
		showBuf := new(bytes.Buffer)
		showInv.Stdout = showBuf
		err := showInv.WithContext(ctx).Run()
		require.NoError(t, err)

		exportedJSON := showBuf.Bytes()
		require.NotEmpty(t, exportedJSON)

		// Modify the name so we can import as a new role.
		var exported []json.RawMessage
		err = json.Unmarshal(exportedJSON, &exported)
		require.NoError(t, err)
		require.Len(t, exported, 1)

		var roleMap map[string]any
		err = json.Unmarshal(exported[0], &roleMap)
		require.NoError(t, err)
		roleMap["name"] = "imported-from-export"
		modifiedJSON, err := json.Marshal([]any{roleMap})
		require.NoError(t, err)

		// Import the modified JSON via 'create --stdin'.
		createInv, createRoot := clitest.New(t, "organization", "roles", "create", "--stdin")
		clitest.SetupConfig(t, client, createRoot)
		createInv.Stdin = bytes.NewReader(modifiedJSON)
		createBuf := new(bytes.Buffer)
		createInv.Stdout = createBuf
		err = createInv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, createBuf.String(), "imported-from-export")
	})
}

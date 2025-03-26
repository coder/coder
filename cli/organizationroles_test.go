package cli_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
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

func TestCreateOrganizationRole(t *testing.T) {
	t.Parallel()

	t.Run("CreateViaJSON", func(t *testing.T) {
		t.Parallel()

		ownerClient, _ := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())

		// Prepare a new role definition
		const roleName = "test-create-role"
		newRole := codersdk.Role{
			Name:           roleName,
			DisplayName:    "Test Create Role",
			OrganizationID: owner.OrganizationID.String(),
			OrganizationPermissions: []codersdk.Permission{
				{
					ResourceType: "workspace",
					Action:       "read",
					Negate:       false,
				},
				{
					ResourceType: "template",
					Action:       "read",
					Negate:       false,
				},
			},
		}

		// Serialize to JSON
		roleJSON, err := json.Marshal(newRole)
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitMedium)
		
		// Run create command with JSON input
		inv, root := clitest.New(t, "organization", "roles", "create", "--stdin", "--output=json")
		clitest.SetupConfig(t, client, root)

		buf := new(bytes.Buffer)
		inv.Stdout = buf
		inv.Stdin = bytes.NewReader(roleJSON)
		
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Parse the output
		var outputRole codersdk.Role
		err = json.Unmarshal(buf.Bytes(), &outputRole)
		require.NoError(t, err)

		// Verify the role was created correctly
		assert.Equal(t, roleName, outputRole.Name)
		assert.Equal(t, "Test Create Role", outputRole.DisplayName)
		assert.Len(t, outputRole.OrganizationPermissions, 2)
		
		// Verify permissions
		hasWorkspaceRead := false
		hasTemplateRead := false
		
		for _, perm := range outputRole.OrganizationPermissions {
			if perm.ResourceType == "workspace" && perm.Action == "read" {
				hasWorkspaceRead = true
			}
			if perm.ResourceType == "template" && perm.Action == "read" {
				hasTemplateRead = true
			}
		}
		
		assert.True(t, hasWorkspaceRead, "should have workspace read permission")
		assert.True(t, hasTemplateRead, "should have template read permission")
		
		// Verify the role exists in the system
		roles, err := client.ListOrganizationRoles(ctx, owner.OrganizationID)
		require.NoError(t, err)
		
		var createdRole *codersdk.AssignableRoles
		for _, r := range roles {
			if r.Name == roleName {
				createdRole = &r
				break
			}
		}
		
		require.NotNil(t, createdRole, "role should exist")
		assert.Equal(t, "Test Create Role", createdRole.DisplayName)
	})

	t.Run("DryRun", func(t *testing.T) {
		t.Parallel()

		ownerClient, _ := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())

		// Prepare a new role definition
		const roleName = "test-dryrun-create-role"
		newRole := codersdk.Role{
			Name:           roleName,
			DisplayName:    "Test DryRun Create Role",
			OrganizationID: owner.OrganizationID.String(),
			OrganizationPermissions: []codersdk.Permission{
				{
					ResourceType: "workspace",
					Action:       "read",
					Negate:       false,
				},
			},
		}

		// Serialize to JSON
		roleJSON, err := json.Marshal(newRole)
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitMedium)
		
		// Run create command with dry-run flag
		inv, root := clitest.New(t, "organization", "roles", "create", "--stdin", "--dry-run", "--output=json")
		clitest.SetupConfig(t, client, root)

		buf := new(bytes.Buffer)
		inv.Stdout = buf
		inv.Stdin = bytes.NewReader(roleJSON)
		
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Parse the output
		var outputRole codersdk.Role
		err = json.Unmarshal(buf.Bytes(), &outputRole)
		require.NoError(t, err)

		// Verify the role matches what we sent
		assert.Equal(t, roleName, outputRole.Name)
		assert.Equal(t, "Test DryRun Create Role", outputRole.DisplayName)

		// Verify the role was not actually created in the system
		roles, err := client.ListOrganizationRoles(ctx, owner.OrganizationID)
		require.NoError(t, err)
		
		var createdRole *codersdk.AssignableRoles
		for _, r := range roles {
			if r.Name == roleName {
				createdRole = &r
				break
			}
		}
		
		assert.Nil(t, createdRole, "role should not exist after dry run")
	})

	t.Run("CreateExistingRole", func(t *testing.T) {
		t.Parallel()

		ownerClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())

		// Create a custom role first
		const roleName = "test-existing-role"
		dbgen.CustomRole(t, db, database.CustomRole{
			Name:            roleName,
			DisplayName:     "Existing Role",
			SitePermissions: nil,
			OrgPermissions: []database.CustomRolePermission{
				{
					Action:       "read",
					ResourceType: "workspace",
				},
			},
			UserPermissions: nil,
			OrganizationID: uuid.NullUUID{
				UUID:  owner.OrganizationID,
				Valid: true,
			},
		})

		// Prepare a new role with the same name
		duplicateRole := codersdk.Role{
			Name:           roleName,
			DisplayName:    "Duplicate Role",
			OrganizationID: owner.OrganizationID.String(),
			OrganizationPermissions: []codersdk.Permission{
				{
					ResourceType: "workspace",
					Action:       "read",
					Negate:       false,
				},
			},
		}

		// Serialize to JSON
		roleJSON, err := json.Marshal(duplicateRole)
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitMedium)
		
		// Run create command with JSON input
		inv, root := clitest.New(t, "organization", "roles", "create", "--stdin")
		clitest.SetupConfig(t, client, root)

		errBuf := new(bytes.Buffer)
		inv.Stderr = errBuf
		inv.Stdin = bytes.NewReader(roleJSON)
		
		err = inv.WithContext(ctx).Run()
		
		// Command should fail because the role already exists
		require.Error(t, err)
		assert.Contains(t, errBuf.String(), "already exists")
		assert.Contains(t, errBuf.String(), "update command")
	})
}

func TestUpdateOrganizationRole(t *testing.T) {
	t.Parallel()

	t.Run("UpdateViaJSON", func(t *testing.T) {
		t.Parallel()

		ownerClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())

		// Create a custom role first
		const roleName = "test-update-role"
		dbgen.CustomRole(t, db, database.CustomRole{
			Name:            roleName,
			DisplayName:     "Before Update",
			SitePermissions: nil,
			OrgPermissions: []database.CustomRolePermission{
				{
					Action:       "read",
					ResourceType: "workspace",
				},
			},
			UserPermissions: nil,
			OrganizationID: uuid.NullUUID{
				UUID:  owner.OrganizationID,
				Valid: true,
			},
		})

		// Prepare a role with updated permissions
		updatedRole := codersdk.Role{
			Name:           roleName,
			DisplayName:    "After Update",
			OrganizationID: owner.OrganizationID.String(),
			OrganizationPermissions: []codersdk.Permission{
				{
					ResourceType: "workspace",
					Action:       "read",
					Negate:       false,
				},
				{
					ResourceType: "template",
					Action:       "read",
					Negate:       false,
				},
			},
		}

		// Serialize to JSON
		roleJSON, err := json.Marshal(updatedRole)
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitMedium)
		
		// Run update command with JSON input
		inv, root := clitest.New(t, "organization", "roles", "update", "--stdin", "--output=json")
		clitest.SetupConfig(t, client, root)

		buf := new(bytes.Buffer)
		inv.Stdout = buf
		inv.Stdin = bytes.NewReader(roleJSON)
		
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Parse the output
		var outputRole codersdk.Role
		err = json.Unmarshal(buf.Bytes(), &outputRole)
		require.NoError(t, err)

		// Verify the role was updated correctly
		assert.Equal(t, roleName, outputRole.Name)
		assert.Equal(t, "After Update", outputRole.DisplayName)
		assert.Len(t, outputRole.OrganizationPermissions, 2)
		
		// Verify permissions
		hasWorkspaceRead := false
		hasTemplateRead := false
		
		for _, perm := range outputRole.OrganizationPermissions {
			if perm.ResourceType == "workspace" && perm.Action == "read" {
				hasWorkspaceRead = true
			}
			if perm.ResourceType == "template" && perm.Action == "read" {
				hasTemplateRead = true
			}
		}
		
		assert.True(t, hasWorkspaceRead, "should have workspace read permission")
		assert.True(t, hasTemplateRead, "should have template read permission")
	})

	t.Run("DryRun", func(t *testing.T) {
		t.Parallel()

		ownerClient, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())

		// Create a custom role first
		const roleName = "test-dryrun-role"
		dbgen.CustomRole(t, db, database.CustomRole{
			Name:            roleName,
			DisplayName:     "Original",
			SitePermissions: nil,
			OrgPermissions: []database.CustomRolePermission{
				{
					Action:       "read",
					ResourceType: "workspace",
				},
			},
			UserPermissions: nil,
			OrganizationID: uuid.NullUUID{
				UUID:  owner.OrganizationID,
				Valid: true,
			},
		})

		// Prepare a role with updated permissions
		updatedRole := codersdk.Role{
			Name:           roleName,
			DisplayName:    "Dry Run",
			OrganizationID: owner.OrganizationID.String(),
			OrganizationPermissions: []codersdk.Permission{
				{
					ResourceType: "workspace",
					Action:       "read",
					Negate:       false,
				},
				{
					ResourceType: "template",
					Action:       "read",
					Negate:       false,
				},
			},
		}

		// Serialize to JSON
		roleJSON, err := json.Marshal(updatedRole)
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitMedium)
		
		// Run update command with dry-run flag
		inv, root := clitest.New(t, "organization", "roles", "update", "--stdin", "--dry-run", "--output=json")
		clitest.SetupConfig(t, client, root)

		buf := new(bytes.Buffer)
		inv.Stdout = buf
		inv.Stdin = bytes.NewReader(roleJSON)
		
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Parse the output
		var outputRole codersdk.Role
		err = json.Unmarshal(buf.Bytes(), &outputRole)
		require.NoError(t, err)

		// Verify the role was not actually updated in the system
		roles, err := client.ListOrganizationRoles(ctx, owner.OrganizationID)
		require.NoError(t, err)
		
		var actualRole *codersdk.AssignableRoles
		for _, r := range roles {
			if r.Name == roleName {
				actualRole = &r
				break
			}
		}
		
		require.NotNil(t, actualRole, "role should exist")
		assert.Equal(t, "Original", actualRole.DisplayName, "display name should remain unchanged after dry run")
		assert.Len(t, actualRole.OrganizationPermissions, 1, "permissions should remain unchanged after dry run")
	})
	
	t.Run("NonExistentRole", func(t *testing.T) {
		t.Parallel()

		ownerClient, _ := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())

		// Prepare a role with a name that doesn't exist
		nonExistentRole := codersdk.Role{
			Name:           "non-existent-role",
			DisplayName:    "Non-Existent Role",
			OrganizationID: owner.OrganizationID.String(),
			OrganizationPermissions: []codersdk.Permission{
				{
					ResourceType: "workspace",
					Action:       "read",
					Negate:       false,
				},
			},
		}

		// Serialize to JSON
		roleJSON, err := json.Marshal(nonExistentRole)
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitMedium)
		
		// Run update command with JSON input
		inv, root := clitest.New(t, "organization", "roles", "update", "--stdin")
		clitest.SetupConfig(t, client, root)

		errBuf := new(bytes.Buffer)
		inv.Stderr = errBuf
		inv.Stdin = bytes.NewReader(roleJSON)
		
		err = inv.WithContext(ctx).Run()
		
		// Command should fail because the role doesn't exist
		require.Error(t, err)
		assert.Contains(t, errBuf.String(), "does not exist")
		assert.Contains(t, errBuf.String(), "create command")
	})

	t.Run("CommandLineNonExistentRole", func(t *testing.T) {
		t.Parallel()

		ownerClient, _ := coderdtest.NewWithDatabase(t, &coderdtest.Options{})
		owner := coderdtest.CreateFirstUser(t, ownerClient)
		client, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleUserAdmin())

		ctx := testutil.Context(t, testutil.WaitMedium)
		
		// Run update command with a non-existent role name
		inv, root := clitest.New(t, "organization", "roles", "update", "non-existent-role")
		clitest.SetupConfig(t, client, root)

		errBuf := new(bytes.Buffer)
		inv.Stderr = errBuf
		
		err := inv.WithContext(ctx).Run()
		
		// Command should fail because the role doesn't exist
		require.Error(t, err)
		assert.Contains(t, errBuf.String(), "does not exist")
		assert.Contains(t, errBuf.String(), "create command")
	})
}
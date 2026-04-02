package cli_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/testutil"
)

var roles = []string{"auditor", "user-admin"}

func TestUserEditRoles(t *testing.T) {
	t.Parallel()

	t.Run("UpdateUserRoles", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleOwner())
		_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())

		inv, root := clitest.New(t, "users", "edit-roles", member.Username, fmt.Sprintf("--roles=%s", strings.Join(roles, ",")))
		clitest.SetupConfig(t, userAdmin, root)

		// Create context with timeout
		ctx := testutil.Context(t, testutil.WaitShort)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		memberRoles, err := client.UserRoles(ctx, member.Username)
		require.NoError(t, err)

		require.ElementsMatch(t, memberRoles.Roles, roles)
	})

	t.Run("UserNotFound", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleUserAdmin())

		// Setup command with non-existent user
		inv, root := clitest.New(t, "users", "edit-roles", "nonexistentuser")
		clitest.SetupConfig(t, userAdmin, root)

		// Create context with timeout
		ctx := testutil.Context(t, testutil.WaitShort)

		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "fetch user")
	})

	t.Run("AddRole", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleOwner())
		_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())

		ctx := testutil.Context(t, testutil.WaitShort)

		// Member starts with no extra roles. Add auditor.
		inv, root := clitest.New(t, "users", "edit-roles", member.Username, "--add", "auditor")
		clitest.SetupConfig(t, userAdmin, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		memberRoles, err := client.UserRoles(ctx, member.Username)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"auditor"}, memberRoles.Roles)

		// Adding the same role again should be a no-op with an info
		// message — not an error.
		var stdout, stderr bytes.Buffer
		inv, root = clitest.New(t, "users", "edit-roles", member.Username, "--add", "auditor")
		clitest.SetupConfig(t, userAdmin, root)
		inv.Stdout = &stdout
		inv.Stderr = &stderr

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		memberRoles, err = client.UserRoles(ctx, member.Username)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"auditor"}, memberRoles.Roles)

		combinedOutput := stdout.String() + stderr.String()
		require.Contains(t, combinedOutput, "already")
	})

	t.Run("RemoveRole", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleOwner())
		_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())

		ctx := testutil.Context(t, testutil.WaitShort)

		// Give the member both auditor and user-admin roles.
		inv, root := clitest.New(t, "users", "edit-roles", member.Username, "--roles=auditor,user-admin")
		clitest.SetupConfig(t, userAdmin, root)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Verify both roles were set.
		memberRoles, err := client.UserRoles(ctx, member.Username)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"auditor", "user-admin"}, memberRoles.Roles)

		// Remove auditor, leaving only user-admin.
		inv, root = clitest.New(t, "users", "edit-roles", member.Username, "--remove", "auditor")
		clitest.SetupConfig(t, userAdmin, root)
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		memberRoles, err = client.UserRoles(ctx, member.Username)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"user-admin"}, memberRoles.Roles)

		// Removing a role the user doesn't have should be a no-op
		// with an info message — not an error.
		var stdout, stderr bytes.Buffer
		inv, root = clitest.New(t, "users", "edit-roles", member.Username, "--remove", "auditor")
		clitest.SetupConfig(t, userAdmin, root)
		inv.Stdout = &stdout
		inv.Stderr = &stderr

		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		memberRoles, err = client.UserRoles(ctx, member.Username)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"user-admin"}, memberRoles.Roles)

		combinedOutput := stdout.String() + stderr.String()
		require.Contains(t, combinedOutput, "already")
	})

	t.Run("AddAndRemoveTogether", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleOwner())
		_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())

		ctx := testutil.Context(t, testutil.WaitShort)

		// Give the member the auditor role first.
		inv, root := clitest.New(t, "users", "edit-roles", member.Username, "--roles=auditor")
		clitest.SetupConfig(t, userAdmin, root)
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// Verify setup.
		memberRoles, err := client.UserRoles(ctx, member.Username)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"auditor"}, memberRoles.Roles)

		// Add user-admin and remove auditor in one command.
		inv, root = clitest.New(t, "users", "edit-roles", member.Username, "--add", "user-admin", "--remove", "auditor")
		clitest.SetupConfig(t, userAdmin, root)
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		memberRoles, err = client.UserRoles(ctx, member.Username)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"user-admin"}, memberRoles.Roles)
	})

	t.Run("ConflictWithRolesFlag", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleOwner())
		_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())

		ctx := testutil.Context(t, testutil.WaitShort)

		// Using --roles together with --add should be rejected.
		inv, root := clitest.New(t, "users", "edit-roles", member.Username, "--roles", "auditor", "--add", "user-admin")
		clitest.SetupConfig(t, userAdmin, root)

		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "--roles cannot be used with --add or --remove")
	})

	t.Run("SameRoleInBothFlags", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleOwner())
		_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())

		ctx := testutil.Context(t, testutil.WaitShort)

		// Specifying the same role in --add and --remove is
		// contradictory and should be rejected.
		inv, root := clitest.New(t, "users", "edit-roles", member.Username, "--add", "auditor", "--remove", "auditor")
		clitest.SetupConfig(t, userAdmin, root)

		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot appear in both --add and --remove")
	})

	t.Run("InvalidRoleNameInAdd", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleOwner())
		_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())

		ctx := testutil.Context(t, testutil.WaitShort)

		// Adding a role that doesn't exist should fail validation.
		inv, root := clitest.New(t, "users", "edit-roles", member.Username, "--add", "nonexistent")
		clitest.SetupConfig(t, userAdmin, root)

		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "not valid")
	})

	t.Run("InsufficientPermissions", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		// memberClient is the caller — a regular member with no
		// elevated permissions.
		memberClient, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())

		ctx := testutil.Context(t, testutil.WaitShort)

		// A regular member should be rejected by the pre-flight
		// permissions check.
		inv, root := clitest.New(t, "users", "edit-roles", member.Username, "--add", "auditor")
		clitest.SetupConfig(t, memberClient, root)

		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "do not have permission")
	})
}

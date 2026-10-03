package cli_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/coder/v2/testutil/expecter"
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

		inv, root := clitest.New(t, "users", "edit-roles", member.Username, fmt.Sprintf("--roles=%s", strings.Join(roles, ",")), "-y")
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

	t.Run("PromptsBeforeApplying", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleOwner())
		_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())

		inv, root := clitest.New(t, "users", "edit-roles", member.Username, fmt.Sprintf("--roles=%s", strings.Join(roles, ",")))
		clitest.SetupConfig(t, userAdmin, root)

		ctx := testutil.Context(t, testutil.WaitShort)
		logger := testutil.Logger(t)
		stdout := expecter.NewAttachedToInvocation(t, inv)
		stdin := testutil.NewWriterAttachedToInvocation(t, logger.Named("stdin"), inv)

		errC := make(chan error, 1)
		go func() {
			errC <- inv.WithContext(ctx).Run()
		}()

		stdout.ExpectMatch(ctx, "Roles to add")
		stdout.ExpectMatch(ctx, "Continue?")
		stdin.WriteLine("yes")

		require.NoError(t, <-errC)

		memberRoles, err := client.UserRoles(ctx, member.Username)
		require.NoError(t, err)
		require.ElementsMatch(t, memberRoles.Roles, roles)
	})

	t.Run("NoChangeSkipsPrompt", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleOwner())
		_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())

		ctx := testutil.Context(t, testutil.WaitShort)

		// Set the member's roles first so the second call is a no-op.
		inv, root := clitest.New(t, "users", "edit-roles", member.Username, "--roles=auditor", "-y")
		clitest.SetupConfig(t, userAdmin, root)
		require.NoError(t, inv.WithContext(ctx).Run())

		inv, root = clitest.New(t, "users", "edit-roles", member.Username, "--roles=auditor")
		clitest.SetupConfig(t, userAdmin, root)
		var buf strings.Builder
		inv.Stdout = &buf

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)
		require.Contains(t, buf.String(), "No role changes")
	})

	t.Run("YesWithoutRolesErrors", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleOwner())
		_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())

		// No stdin is attached, so the command would hang if it tried to prompt.
		inv, root := clitest.New(t, "users", "edit-roles", member.Username, "-y")
		clitest.SetupConfig(t, userAdmin, root)

		ctx := testutil.Context(t, testutil.WaitShort)
		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.ErrorContains(t, err, "--roles is required")
	})
}

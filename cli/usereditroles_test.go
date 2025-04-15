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
)

var roles = []string{"auditor", "user-admin"}

func TestUserEditRoles(t *testing.T) {
	t.Parallel()

	t.Run("UpdateUserRoles", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		_, member := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleMember())

		inv, root := clitest.New(t, "users", "edit-roles", member.Username, fmt.Sprintf("--roles=%s", strings.Join(roles, ",")))
		clitest.SetupConfig(t, client, root)

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
}

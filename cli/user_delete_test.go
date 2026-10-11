package cli_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/coder/v2/testutil/expecter"
)

func TestUserDelete(t *testing.T) {
	t.Parallel()
	t.Run("Username", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleUserAdmin())

		pw, err := cryptorand.String(16)
		require.NoError(t, err)

		_, err = client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			Email:           "colin5@coder.com",
			Username:        "coolin",
			Password:        pw,
			UserLoginType:   codersdk.LoginTypePassword,
			OrganizationIDs: []uuid.UUID{owner.OrganizationID},
		})
		require.NoError(t, err)

		inv, root := clitest.New(t, "users", "delete", "coolin")
		clitest.SetupConfig(t, userAdmin, root)
		stdout := expecter.NewAttachedToInvocation(t, inv)
		errC := make(chan error)
		go func() {
			errC <- inv.Run()
		}()
		require.NoError(t, <-errC)
		stdout.ExpectMatch(ctx, "coolin")
	})

	t.Run("UserID", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleUserAdmin())

		pw, err := cryptorand.String(16)
		require.NoError(t, err)

		user, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			Email:           "colin5@coder.com",
			Username:        "coolin",
			Password:        pw,
			UserLoginType:   codersdk.LoginTypePassword,
			OrganizationIDs: []uuid.UUID{owner.OrganizationID},
		})
		require.NoError(t, err)

		inv, root := clitest.New(t, "users", "delete", user.ID.String())
		clitest.SetupConfig(t, userAdmin, root)
		stdout := expecter.NewAttachedToInvocation(t, inv)
		errC := make(chan error)
		go func() {
			errC <- inv.Run()
		}()
		require.NoError(t, <-errC)
		stdout.ExpectMatch(ctx, "coolin")
	})

	t.Run("UserID", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitMedium)
		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleUserAdmin())

		pw, err := cryptorand.String(16)
		require.NoError(t, err)

		user, err := client.CreateUserWithOrgs(ctx, codersdk.CreateUserRequestWithOrgs{
			Email:           "colin5@coder.com",
			Username:        "coolin",
			Password:        pw,
			UserLoginType:   codersdk.LoginTypePassword,
			OrganizationIDs: []uuid.UUID{owner.OrganizationID},
		})
		require.NoError(t, err)

		inv, root := clitest.New(t, "users", "delete", user.ID.String())
		clitest.SetupConfig(t, userAdmin, root)
		stdout := expecter.NewAttachedToInvocation(t, inv)
		errC := make(chan error)
		go func() {
			errC <- inv.Run()
		}()
		require.NoError(t, <-errC)
		stdout.ExpectMatch(ctx, "coolin")
	})

	// TODO: add a NoPerms test case. Fetching users without perms returns a
	// "user "testuser@coder.com" must be a member of at least one organization"
	// error.

	t.Run("DeleteSelf", func(t *testing.T) {
		t.Parallel()
		t.Run("Owner", func(t *testing.T) {
			client := coderdtest.New(t, nil)
			_ = coderdtest.CreateFirstUser(t, client)
			inv, root := clitest.New(t, "users", "delete", "me")
			//nolint:gocritic // The point of the test is to validate that a user cannot delete
			// themselves, the owner user is probably the most important user to test this with.
			clitest.SetupConfig(t, client, root)
			require.ErrorContains(t, inv.Run(), "You cannot delete yourself!")
		})
		t.Run("UserAdmin", func(t *testing.T) {
			client := coderdtest.New(t, nil)
			owner := coderdtest.CreateFirstUser(t, client)
			userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleUserAdmin())
			inv, root := clitest.New(t, "users", "delete", "me")
			clitest.SetupConfig(t, userAdmin, root)
			require.ErrorContains(t, inv.Run(), "You cannot delete yourself!")
		})
	})
}

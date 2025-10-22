package cli_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

func TestUserStatus(t *testing.T) {
	t.Parallel()

	t.Run("StatusSelf", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)

		inv, root := clitest.New(t, "users", "suspend", "me")
		clitest.SetupConfig(t, client, root)
		// Yes to the prompt
		inv.Stdin = bytes.NewReader([]byte("yes\n"))
		err := inv.Run()
		// Expect an error, as you cannot suspend yourself
		require.Error(t, err)
		require.ErrorContains(t, err, "cannot suspend yourself")
	})

	t.Run("StatusOther", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleUserAdmin())
		other, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		otherUser, err := other.User(context.Background(), codersdk.Me)
		require.NoError(t, err, "fetch user")

		inv, root := clitest.New(t, "users", "suspend", otherUser.Username)
		clitest.SetupConfig(t, userAdmin, root)
		// Yes to the prompt
		inv.Stdin = bytes.NewReader([]byte("yes\n"))
		err = inv.Run()
		require.NoError(t, err, "suspend user")

		// Check the user status
		otherUser, err = client.User(context.Background(), otherUser.Username)
		require.NoError(t, err, "fetch suspended user")
		require.Equal(t, codersdk.UserStatusSuspended, otherUser.Status, "suspended user")

		// Set back to active. Try using a uuid as well
		inv, root = clitest.New(t, "users", "activate", otherUser.ID.String())
		clitest.SetupConfig(t, userAdmin, root)
		// Yes to the prompt
		inv.Stdin = bytes.NewReader([]byte("yes\n"))
		err = inv.Run()
		require.NoError(t, err, "suspend user")

		// Check the user status
		otherUser, err = client.User(context.Background(), otherUser.ID.String())
		require.NoError(t, err, "fetch active user")
		require.Equal(t, codersdk.UserStatusActive, otherUser.Status, "active user")
	})
}

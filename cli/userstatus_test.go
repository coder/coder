package cli_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
)

// nolint:tparallel,paralleltest
func TestUserStatus(t *testing.T) {
	t.Parallel()
	client := coderdtest.New(t, nil)
	admin := coderdtest.CreateFirstUser(t, client)
	other, _ := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
	otherUser, err := other.User(context.Background(), codersdk.Me)
	require.NoError(t, err, "fetch user")

	t.Run("StatusSelf", func(t *testing.T) {
		cmd, root := clitest.New(t, "users", "suspend", "me")
		clitest.SetupConfig(t, client, root)
		// Yes to the prompt
		cmd.SetIn(bytes.NewReader([]byte("yes\n")))
		err := cmd.Execute()
		// Expect an error, as you cannot suspend yourself
		require.Error(t, err)
		require.ErrorContains(t, err, "cannot suspend yourself")
	})

	t.Run("StatusOther", func(t *testing.T) {
		require.Equal(t, codersdk.UserStatusActive, otherUser.Status, "start as active")

		cmd, root := clitest.New(t, "users", "suspend", otherUser.Username)
		clitest.SetupConfig(t, client, root)
		// Yes to the prompt
		cmd.SetIn(bytes.NewReader([]byte("yes\n")))
		err := cmd.Execute()
		require.NoError(t, err, "suspend user")

		// Check the user status
		otherUser, err = client.User(context.Background(), otherUser.Username)
		require.NoError(t, err, "fetch suspended user")
		require.Equal(t, codersdk.UserStatusSuspended, otherUser.Status, "suspended user")

		// Set back to active. Try using a uuid as well
		cmd, root = clitest.New(t, "users", "activate", otherUser.ID.String())
		clitest.SetupConfig(t, client, root)
		// Yes to the prompt
		cmd.SetIn(bytes.NewReader([]byte("yes\n")))
		err = cmd.Execute()
		require.NoError(t, err, "suspend user")

		// Check the user status
		otherUser, err = client.User(context.Background(), otherUser.ID.String())
		require.NoError(t, err, "fetch active user")
		require.Equal(t, codersdk.UserStatusActive, otherUser.Status, "active user")
	})
}

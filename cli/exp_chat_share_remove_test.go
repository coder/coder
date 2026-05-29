package cli_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestExpChatShareRemove(t *testing.T) {
	t.Parallel()

	t.Run("RemoveSharedUser", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, client)
		_, sharedUser := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "share remove user",
		})
		experimentalClient := codersdk.NewExperimentalClient(client)
		ctx := testutil.Context(t, testutil.WaitMedium)
		err := experimentalClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
			UserRoles: map[string]codersdk.ChatRole{sharedUser.ID.String(): codersdk.ChatRoleRead},
		})
		require.NoError(t, err)

		inv, root := clitest.New(t, "exp", "chat", "share", "remove", chat.ID.String(), "--user", sharedUser.Username)
		clitest.SetupConfig(t, client, root)

		out := new(bytes.Buffer)
		inv.Stdout = out
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		acl, err := experimentalClient.GetChatACL(ctx, chat.ID)
		require.NoError(t, err)
		for _, user := range acl.Users {
			assert.NotEqual(t, sharedUser.ID, user.ID)
		}
		assert.NotContains(t, out.String(), sharedUser.Username)
	})

	t.Run("RequiresActor", func(t *testing.T) {
		t.Parallel()

		chatID := "00000000-0000-0000-0000-000000000001"
		inv, _ := clitest.New(t, "exp", "chat", "share", "remove", chatID)

		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "at least one user or group must be provided")
	})

	t.Run("RejectsRoleSyntax", func(t *testing.T) {
		t.Parallel()

		chatID := "00000000-0000-0000-0000-000000000001"
		inv, _ := clitest.New(t, "exp", "chat", "share", "remove", chatID, "--user", "alice:read")

		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "roles are only accepted by chat share add")
	})

	t.Run("RejectsInvalidChatID", func(t *testing.T) {
		t.Parallel()

		inv, _ := clitest.New(t, "exp", "chat", "share", "remove", "not-a-uuid", "--user", "alice")

		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid chat ID")
	})
}

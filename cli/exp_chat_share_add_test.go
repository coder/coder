package cli_test

import (
	"bytes"
	"fmt"
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

func TestExpChatShareAdd(t *testing.T) {
	t.Parallel()

	t.Run("ShareWithUserExplicitReadRole", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, client)
		_, toShareWithUser := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "share add user",
		})

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, root := clitest.New(t, "exp", "chat", "share", "add", chat.ID.String(), "--user", toShareWithUser.Username+":read")
		clitest.SetupConfig(t, client, root) //nolint:gocritic // Chat ACL operations require the chat owner in this fixture.

		out := new(bytes.Buffer)
		inv.Stdout = out
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		acl, err := codersdk.NewExperimentalClient(client).GetChatACL(ctx, chat.ID)
		require.NoError(t, err)
		assert.Contains(t, acl.Users, codersdk.ChatUser{
			MinimalUser: codersdk.MinimalUser{
				ID:        toShareWithUser.ID,
				Username:  toShareWithUser.Username,
				Name:      toShareWithUser.Name,
				AvatarURL: toShareWithUser.AvatarURL,
			},
			Role: codersdk.ChatRoleRead,
		})
		assert.Contains(t, out.String(), toShareWithUser.Username)
		assert.Contains(t, out.String(), string(codersdk.ChatRoleRead))
	})

	t.Run("RequiresRole", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, client)
		_, toShareWithUser := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "share add user missing role",
		})

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, root := clitest.New(t, "exp", "chat", "share", "add", chat.ID.String(), "--user", toShareWithUser.Username)
		clitest.SetupConfig(t, client, root) //nolint:gocritic // Chat ACL operations require the chat owner in this fixture.

		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "must match pattern 'name:role'")
	})

	t.Run("ShareWithMultipleUsers", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, client)
		_, toShareWithUser1 := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		_, toShareWithUser2 := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "share add multiple users",
		})

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, root := clitest.New(t,
			"exp", "chat", "share", "add", chat.ID.String(),
			fmt.Sprintf("--user=%s:read,%s:read", toShareWithUser1.Username, toShareWithUser2.Username),
		)
		clitest.SetupConfig(t, client, root) //nolint:gocritic // Chat ACL operations require the chat owner in this fixture.

		out := new(bytes.Buffer)
		inv.Stdout = out
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		acl, err := codersdk.NewExperimentalClient(client).GetChatACL(ctx, chat.ID)
		require.NoError(t, err)
		assert.Contains(t, acl.Users, codersdk.ChatUser{
			MinimalUser: codersdk.MinimalUser{
				ID:        toShareWithUser1.ID,
				Username:  toShareWithUser1.Username,
				Name:      toShareWithUser1.Name,
				AvatarURL: toShareWithUser1.AvatarURL,
			},
			Role: codersdk.ChatRoleRead,
		})
		assert.Contains(t, acl.Users, codersdk.ChatUser{
			MinimalUser: codersdk.MinimalUser{
				ID:        toShareWithUser2.ID,
				Username:  toShareWithUser2.Username,
				Name:      toShareWithUser2.Name,
				AvatarURL: toShareWithUser2.AvatarURL,
			},
			Role: codersdk.ChatRoleRead,
		})
	})

	t.Run("RejectsUnknownRole", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, client)
		_, toShareWithUser := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "share add invalid role",
		})

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, root := clitest.New(t, "exp", "chat", "share", "add", chat.ID.String(), "--user", toShareWithUser.Username+":write")
		clitest.SetupConfig(t, client, root) //nolint:gocritic // Chat ACL operations require the chat owner in this fixture.

		err := inv.WithContext(ctx).Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid role \"write\"")
	})

	t.Run("RequiresActor", func(t *testing.T) {
		t.Parallel()

		chatID := "00000000-0000-0000-0000-000000000001"
		inv, _ := clitest.New(t, "exp", "chat", "share", "add", chatID)

		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "at least one user or group must be provided")
	})

	t.Run("RejectsInvalidChatID", func(t *testing.T) {
		t.Parallel()

		inv, _ := clitest.New(t, "exp", "chat", "share", "add", "not-a-uuid", "--user", "alice:read")

		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid chat ID")
	})
}

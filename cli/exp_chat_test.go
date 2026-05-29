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
		clitest.SetupConfig(t, client, root)

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

	t.Run("ShareWithUserDefaultReadRole", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, client)
		_, toShareWithUser := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "share add user default role",
		})

		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, root := clitest.New(t, "exp", "chat", "share", "add", chat.ID.String(), "--user", toShareWithUser.Username)
		clitest.SetupConfig(t, client, root)

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
		clitest.SetupConfig(t, client, root)

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
		clitest.SetupConfig(t, client, root)

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

func TestExpChatShareStatus(t *testing.T) {
	t.Parallel()

	t.Run("ListsSharedUsers", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		firstUser := coderdtest.CreateFirstUser(t, client)
		_, sharedUser := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
		chat := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           firstUser.UserID,
			LastModelConfigID: modelConfig.ID,
			Title:             "share status user",
		})
		experimentalClient := codersdk.NewExperimentalClient(client)
		ctx := testutil.Context(t, testutil.WaitMedium)
		err := experimentalClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
			UserRoles: map[string]codersdk.ChatRole{sharedUser.ID.String(): codersdk.ChatRoleRead},
		})
		require.NoError(t, err)

		inv, root := clitest.New(t, "exp", "chat", "share", "status", chat.ID.String())
		clitest.SetupConfig(t, client, root)

		out := new(bytes.Buffer)
		inv.Stdout = out
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		assert.Contains(t, out.String(), sharedUser.Username)
		assert.Contains(t, out.String(), string(codersdk.ChatRoleRead))
	})

	t.Run("RejectsInvalidChatID", func(t *testing.T) {
		t.Parallel()

		inv, _ := clitest.New(t, "exp", "chat", "share", "status", "not-a-uuid")

		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid chat ID")
	})

	t.Run("RequiresChatID", func(t *testing.T) {
		t.Parallel()

		inv, _ := clitest.New(t, "exp", "chat", "share", "status")

		err := inv.Run()
		require.Error(t, err)
	})
}

func TestExpChatContextAdd(t *testing.T) {
	t.Parallel()

	t.Run("RequiresWorkspaceOrDir", func(t *testing.T) {
		t.Parallel()

		inv, _ := clitest.New(t, "exp", "chat", "context", "add")

		err := inv.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "this command must be run inside a Coder workspace")
	})

	t.Run("AllowsExplicitDir", func(t *testing.T) {
		t.Parallel()

		inv, _ := clitest.New(t, "exp", "chat", "context", "add", "--dir", t.TempDir())

		err := inv.Run()
		if err != nil {
			require.NotContains(t, err.Error(), "this command must be run inside a Coder workspace")
		}
	})

	t.Run("AllowsWorkspaceEnv", func(t *testing.T) {
		t.Parallel()

		inv, _ := clitest.New(t, "exp", "chat", "context", "add")
		inv.Environ.Set("CODER", "true")

		err := inv.Run()
		if err != nil {
			require.NotContains(t, err.Error(), "this command must be run inside a Coder workspace")
		}
	})
}

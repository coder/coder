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
		clitest.SetupConfig(t, client, root) //nolint:gocritic // Chat ACL status requires the chat owner to fetch the chat ACL.

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

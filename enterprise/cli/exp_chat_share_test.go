package cli_test

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestExpChatShareGroups(t *testing.T) {
	t.Parallel()

	t.Run("AddGroup", func(t *testing.T) {
		t.Parallel()

		client, db, firstUser := newChatShareGroupServer(t)
		group := createChatShareGroup(t, client, firstUser.OrganizationID, "chat-share-add-group")
		chat := createChatShareChat(t, db, firstUser.OrganizationID, firstUser.UserID, "share add group")
		ctx := testutil.Context(t, testutil.WaitMedium)
		inv, root := clitest.New(t, "exp", "chat", "share", "add", chat.ID.String(), "--group", group.Name)
		clitest.SetupConfig(t, client, root) //nolint:gocritic // Chat ACL operations require the chat owner in this fixture.

		out := new(bytes.Buffer)
		inv.Stdout = out
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		acl, err := codersdk.NewExperimentalClient(client).GetChatACL(ctx, chat.ID)
		require.NoError(t, err)
		assert.Contains(t, chatShareGroupRoles(acl.Groups), group.ID)
		assert.Equal(t, codersdk.ChatRoleRead, chatShareGroupRoles(acl.Groups)[group.ID])
		assert.Contains(t, out.String(), group.Name)
		assert.Contains(t, out.String(), string(codersdk.ChatRoleRead))
	})

	t.Run("RemoveGroup", func(t *testing.T) {
		t.Parallel()

		client, db, firstUser := newChatShareGroupServer(t)
		group := createChatShareGroup(t, client, firstUser.OrganizationID, "chat-share-remove-group")
		chat := createChatShareChat(t, db, firstUser.OrganizationID, firstUser.UserID, "share remove group")
		ctx := testutil.Context(t, testutil.WaitMedium)
		experimentalClient := codersdk.NewExperimentalClient(client)
		err := experimentalClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
			GroupRoles: map[string]codersdk.ChatRole{group.ID.String(): codersdk.ChatRoleRead},
		})
		require.NoError(t, err)

		inv, root := clitest.New(t, "exp", "chat", "share", "remove", chat.ID.String(), "--group", group.Name)
		clitest.SetupConfig(t, client, root) //nolint:gocritic // Chat ACL operations require the chat owner in this fixture.

		out := new(bytes.Buffer)
		inv.Stdout = out
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		acl, err := experimentalClient.GetChatACL(ctx, chat.ID)
		require.NoError(t, err)
		assert.NotContains(t, chatShareGroupRoles(acl.Groups), group.ID)
		assert.NotContains(t, out.String(), group.Name)
	})

	t.Run("StatusGroup", func(t *testing.T) {
		t.Parallel()

		client, db, firstUser := newChatShareGroupServer(t)
		group := createChatShareGroup(t, client, firstUser.OrganizationID, "chat-share-status-group")
		chat := createChatShareChat(t, db, firstUser.OrganizationID, firstUser.UserID, "share status group")
		ctx := testutil.Context(t, testutil.WaitMedium)
		experimentalClient := codersdk.NewExperimentalClient(client)
		err := experimentalClient.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
			GroupRoles: map[string]codersdk.ChatRole{group.ID.String(): codersdk.ChatRoleRead},
		})
		require.NoError(t, err)

		inv, root := clitest.New(t, "exp", "chat", "share", "status", chat.ID.String())
		clitest.SetupConfig(t, client, root) //nolint:gocritic // Chat ACL operations require the chat owner in this fixture.

		out := new(bytes.Buffer)
		inv.Stdout = out
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		assert.Contains(t, out.String(), group.Name)
		assert.Contains(t, out.String(), string(codersdk.ChatRoleRead))
	})
}

func newChatShareGroupServer(t *testing.T) (*codersdk.Client, database.Store, codersdk.CreateFirstUserResponse) {
	t.Helper()

	return coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		},
	})
}

func createChatShareGroup(t testing.TB, client *codersdk.Client, organizationID uuid.UUID, name string) codersdk.Group {
	t.Helper()

	group, err := client.CreateGroup(t.Context(), organizationID, codersdk.CreateGroupRequest{
		Name:        name,
		DisplayName: name,
	})
	require.NoError(t, err)
	return group
}

func createChatShareChat(t testing.TB, db database.Store, organizationID uuid.UUID, ownerID uuid.UUID, title string) database.Chat {
	t.Helper()

	modelConfig := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	return dbgen.Chat(t, db, database.Chat{
		OrganizationID:    organizationID,
		OwnerID:           ownerID,
		LastModelConfigID: modelConfig.ID,
		Title:             title,
	})
}

func chatShareGroupRoles(groups []codersdk.ChatGroup) map[uuid.UUID]codersdk.ChatRole {
	roles := make(map[uuid.UUID]codersdk.ChatRole, len(groups))
	for _, group := range groups {
		roles[group.ID] = group.Role
	}
	return roles
}

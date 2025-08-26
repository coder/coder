package cli_test

import (
	"testing"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSharingShare(t *testing.T) {
	t.Parallel()

	t.Run("ShareWithUsers+Simple", func(t *testing.T) {
		t.Parallel()

		var (
			client, db                           = coderdtest.NewWithDatabase(t, nil)
			orgOwner                             = coderdtest.CreateFirstUser(t, client)
			workspaceOwnerClient, workspaceOwner = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID)
			workspace                            = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:        workspaceOwner.ID,
				OrganizationID: orgOwner.OrganizationID,
			}).Do().Workspace
			_, toShareWithUser = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID)
		)

		ctx := testutil.Context(t, testutil.WaitMedium)
		var inv, root = clitest.New(t, "sharing", "share", workspace.Name, "--org", orgOwner.OrganizationID.String(), "--user", toShareWithUser.Username)
		clitest.SetupConfig(t, workspaceOwnerClient, root)

		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		// TODO: Test output of the command

		acl, err := workspaceOwnerClient.WorkspaceACL(inv.Context(), workspace.ID)
		assert.NoError(t, err)
		assert.Equal(t,
			codersdk.WorkspaceACL{
				Users: []codersdk.WorkspaceUser{{
					MinimalUser: codersdk.MinimalUser{
						ID:        toShareWithUser.ID,
						Username:  toShareWithUser.Username,
						AvatarURL: toShareWithUser.AvatarURL,
					},
					Role: codersdk.WorkspaceRole("use"),
				}},
			},
			acl)

		assert.True(t, false)
	})

	// t.Run("ShareWithUsers+Multiple", func(t *testing.T) {
	// 	t.Parallel()

	// 	assert.True(t, false)
	// })

	// t.Run("ShareWithUsers+UseRole", func(t *testing.T) {
	// 	t.Parallel()

	// 	assert.True(t, false)
	// })

	// t.Run("ShareWithUsers+AdminRole", func(t *testing.T) {
	// 	t.Parallel()

	// 	assert.True(t, false)
	// })

	// t.Run("ShareWithGroups+Simple", func(t *testing.T) {
	// 	t.Parallel()

	// 	assert.True(t, false)
	// })

	// t.Run("ShareWithGroups+Mutliple", func(t *testing.T) {
	// 	t.Parallel()

	// 	assert.True(t, false)
	// })

	// t.Run("ShareWithGroups+UseRole", func(t *testing.T) {
	// 	t.Parallel()

	// 	assert.True(t, false)
	// })

	// t.Run("ShareWithGroups+AdminRole", func(t *testing.T) {
	// 	t.Parallel()

	// 	assert.True(t, false)
	// })
}

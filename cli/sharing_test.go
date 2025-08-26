package cli_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSharingShare(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}

	t.Run("ShareWithUsers_Simple", func(t *testing.T) {
		t.Parallel()

		var (
			client, db = coderdtest.NewWithDatabase(t, &coderdtest.Options{
				DeploymentValues: dv,
			})
			orgOwner                             = coderdtest.CreateFirstUser(t, client)
			workspaceOwnerClient, workspaceOwner = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID, rbac.ScopedRoleOrgAuditor(orgOwner.OrganizationID))
			workspace                            = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:        workspaceOwner.ID,
				OrganizationID: orgOwner.OrganizationID,
			}).Do().Workspace
			_, toShareWithUser = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID)
		)

		ctx := testutil.Context(t, testutil.WaitMedium)
		var inv, root = clitest.New(t, "sharing", "share", workspace.Name, "--org", orgOwner.OrganizationID.String(), "--user", toShareWithUser.Username)
		clitest.SetupConfig(t, workspaceOwnerClient, root)

		out := bytes.NewBuffer(nil)
		inv.Stdout = out
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		acl, err := workspaceOwnerClient.WorkspaceACL(inv.Context(), workspace.ID)
		assert.NoError(t, err)
		assert.Contains(t, acl.Users, codersdk.WorkspaceUser{
			MinimalUser: codersdk.MinimalUser{
				ID:        toShareWithUser.ID,
				Username:  toShareWithUser.Username,
				AvatarURL: toShareWithUser.AvatarURL,
			},
			Role: codersdk.WorkspaceRole("use"),
		})

		assert.Contains(t, out.String(), toShareWithUser.Username)
		assert.Contains(t, out.String(), codersdk.WorkspaceRoleUse)
	})

	t.Run("ShareWithUsers_Multiple", func(t *testing.T) {
		t.Parallel()

		var (
			client, db = coderdtest.NewWithDatabase(t, &coderdtest.Options{
				DeploymentValues: dv,
			})
			orgOwner = coderdtest.CreateFirstUser(t, client)

			workspaceOwnerClient, workspaceOwner = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID, rbac.ScopedRoleOrgAuditor(orgOwner.OrganizationID))
			workspace                            = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:        workspaceOwner.ID,
				OrganizationID: orgOwner.OrganizationID,
			}).Do().Workspace

			_, toShareWithUser1 = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID)
			_, toShareWithUser2 = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID)
		)

		ctx := testutil.Context(t, testutil.WaitMedium)
		var inv, root = clitest.New(t,
			"sharing",
			"share", workspace.Name, "--org", orgOwner.OrganizationID.String(),
			fmt.Sprintf("--user=%s,%s", toShareWithUser1.Username, toShareWithUser2.Username),
		)
		clitest.SetupConfig(t, workspaceOwnerClient, root)

		out := bytes.NewBuffer(nil)
		inv.Stdout = out
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		acl, err := workspaceOwnerClient.WorkspaceACL(inv.Context(), workspace.ID)
		assert.NoError(t, err)
		assert.Contains(t, acl.Users, codersdk.WorkspaceUser{
			MinimalUser: codersdk.MinimalUser{
				ID:        toShareWithUser1.ID,
				Username:  toShareWithUser1.Username,
				AvatarURL: toShareWithUser1.AvatarURL,
			},
			Role: codersdk.WorkspaceRole("use"),
		})
		assert.Contains(t, acl.Users, codersdk.WorkspaceUser{
			MinimalUser: codersdk.MinimalUser{
				ID:        toShareWithUser2.ID,
				Username:  toShareWithUser2.Username,
				AvatarURL: toShareWithUser2.AvatarURL,
			},
			Role: codersdk.WorkspaceRole("use"),
		})

		// Test that the users appeart in the output
		outputLines := strings.Split(out.String(), "\n")
		userNames := []string{toShareWithUser1.Username, toShareWithUser2.Username}
		for _, username := range userNames {
			found := false
			for _, line := range outputLines {
				if strings.Contains(line, username) {
					found = true
					break
				}
			}

			assert.True(t, found, fmt.Sprintf("Expected to find username %s in output", username))
		}
	})

	// t.Run("ShareWithUsers_Roles", func(t *testing.T) {
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

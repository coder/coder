package cli_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
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
		inv, root := clitest.New(t, "sharing", "add", workspace.Name, "--org", orgOwner.OrganizationID.String(), "--user", toShareWithUser.Username)
		clitest.SetupConfig(t, workspaceOwnerClient, root)

		out := bytes.NewBuffer(nil)
		inv.Stdout = out
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		acl, err := workspaceOwnerClient.WorkspaceACL(inv.Context(), workspace.ID)
		require.NoError(t, err)
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
		inv, root := clitest.New(t,
			"sharing",
			"add", workspace.Name, "--org", orgOwner.OrganizationID.String(),
			fmt.Sprintf("--user=%s,%s", toShareWithUser1.Username, toShareWithUser2.Username),
		)
		clitest.SetupConfig(t, workspaceOwnerClient, root)

		out := bytes.NewBuffer(nil)
		inv.Stdout = out
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		acl, err := workspaceOwnerClient.WorkspaceACL(inv.Context(), workspace.ID)
		require.NoError(t, err)
		assert.Contains(t, acl.Users, codersdk.WorkspaceUser{
			MinimalUser: codersdk.MinimalUser{
				ID:        toShareWithUser1.ID,
				Username:  toShareWithUser1.Username,
				AvatarURL: toShareWithUser1.AvatarURL,
			},
			Role: codersdk.WorkspaceRoleUse,
		})
		assert.Contains(t, acl.Users, codersdk.WorkspaceUser{
			MinimalUser: codersdk.MinimalUser{
				ID:        toShareWithUser2.ID,
				Username:  toShareWithUser2.Username,
				AvatarURL: toShareWithUser2.AvatarURL,
			},
			Role: codersdk.WorkspaceRoleUse,
		})

		assert.Contains(t, out.String(), toShareWithUser1.Username)
		assert.Contains(t, out.String(), toShareWithUser2.Username)
	})

	t.Run("ShareWithUsers_Roles", func(t *testing.T) {
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
		inv, root := clitest.New(t, "sharing", "add", workspace.Name,
			"--org", orgOwner.OrganizationID.String(),
			"--user", fmt.Sprintf("%s:admin", toShareWithUser.Username),
		)
		clitest.SetupConfig(t, workspaceOwnerClient, root)

		out := bytes.NewBuffer(nil)
		inv.Stdout = out
		err := inv.WithContext(ctx).Run()
		require.NoError(t, err)

		acl, err := workspaceOwnerClient.WorkspaceACL(inv.Context(), workspace.ID)
		require.NoError(t, err)
		assert.Contains(t, acl.Users, codersdk.WorkspaceUser{
			MinimalUser: codersdk.MinimalUser{
				ID:        toShareWithUser.ID,
				Username:  toShareWithUser.Username,
				AvatarURL: toShareWithUser.AvatarURL,
			},
			Role: codersdk.WorkspaceRoleAdmin,
		})

		found := false
		for _, line := range strings.Split(out.String(), "\n") {
			if strings.Contains(line, toShareWithUser.Username) && strings.Contains(line, string(codersdk.WorkspaceRoleAdmin)) {
				found = true
				break
			}
		}
		assert.True(t, found, fmt.Sprintf("expected to find the username %s and role %s in the command: %s", toShareWithUser.Username, codersdk.WorkspaceRoleAdmin, out.String()))
	})
}

func TestSharingStatus(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}

	t.Run("ListSharedUsers", func(t *testing.T) {
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
			ctx                = testutil.Context(t, testutil.WaitMedium)
		)

		err := client.UpdateWorkspaceACL(ctx, workspace.ID, codersdk.UpdateWorkspaceACL{
			UserRoles: map[string]codersdk.WorkspaceRole{
				toShareWithUser.ID.String(): codersdk.WorkspaceRoleUse,
			},
		})
		require.NoError(t, err)

		inv, root := clitest.New(t, "sharing", "status", workspace.Name, "--org", orgOwner.OrganizationID.String())
		clitest.SetupConfig(t, workspaceOwnerClient, root)

		out := bytes.NewBuffer(nil)
		inv.Stdout = out
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		found := false
		for _, line := range strings.Split(out.String(), "\n") {
			if strings.Contains(line, toShareWithUser.Username) && strings.Contains(line, string(codersdk.WorkspaceRoleUse)) {
				found = true
			}

			if found {
				break
			}
		}
		assert.True(t, found, "expected to find username %s with role %s in the output: %s", toShareWithUser.Username, codersdk.WorkspaceRoleUse, out.String())
	})
}

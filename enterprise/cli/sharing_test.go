package cli_test

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestSharingShareEnterprise(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}

	t.Run("ShareWithGroups_Simple", func(t *testing.T) {
		t.Parallel()

		var (
			client, db, orgOwner = coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					DeploymentValues: dv,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureTemplateRBAC: 1,
					},
				},
			})
			workspaceOwnerClient, workspaceOwner = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID, rbac.ScopedRoleOrgAuditor(orgOwner.OrganizationID))
			workspace                            = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:        workspaceOwner.ID,
				OrganizationID: orgOwner.OrganizationID,
			}).Do().Workspace
			_, orgMember = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID)
		)

		ctx := testutil.Context(t, testutil.WaitMedium)

		group, err := createGroupWithMembers(ctx, client, orgOwner.OrganizationID, "new-group", []uuid.UUID{orgMember.ID})
		require.NoError(t, err)

		inv, root := clitest.New(t, "sharing", "share", workspace.Name, "--org", orgOwner.OrganizationID.String(), "--group", group.Name)
		clitest.SetupConfig(t, workspaceOwnerClient, root)

		out := bytes.NewBuffer(nil)
		inv.Stdout = out
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		acl, err := workspaceOwnerClient.WorkspaceACL(inv.Context(), workspace.ID)
		require.NoError(t, err)
		assert.Len(t, acl.Groups, 1)
		assert.Equal(t, acl.Groups[0].Group.ID, group.ID)
		assert.Equal(t, acl.Groups[0].Role, codersdk.WorkspaceRoleUse)

		found := false
		for _, line := range strings.Split(out.String(), "\n") {
			found = strings.Contains(line, group.Name) && strings.Contains(line, string(codersdk.WorkspaceRoleUse))
			if found {
				break
			}
		}
		assert.True(t, found, "Expected to find group name %s and role %s in output: %s", group.Name, codersdk.WorkspaceRoleUse, out.String())
	})

	t.Run("ShareWithGroups_Multiple", func(t *testing.T) {
		t.Parallel()

		var (
			client, db, orgOwner = coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					DeploymentValues: dv,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureTemplateRBAC: 1,
					},
				},
			})

			workspaceOwnerClient, workspaceOwner = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID, rbac.ScopedRoleOrgAuditor(orgOwner.OrganizationID))
			workspace                            = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:        workspaceOwner.ID,
				OrganizationID: orgOwner.OrganizationID,
			}).Do().Workspace

			_, wibbleMember = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID)
			_, wobbleMember = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID)
		)

		ctx := testutil.Context(t, testutil.WaitMedium)

		wibbleGroup, err := createGroupWithMembers(ctx, client, orgOwner.OrganizationID, "wibble", []uuid.UUID{wibbleMember.ID})
		require.NoError(t, err)

		wobbleGroup, err := createGroupWithMembers(ctx, client, orgOwner.OrganizationID, "wobble", []uuid.UUID{wobbleMember.ID})
		require.NoError(t, err)

		inv, root := clitest.New(t, "sharing", "share", workspace.Name, "--org", orgOwner.OrganizationID.String(),
			fmt.Sprintf("--group=%s,%s", wibbleGroup.Name, wobbleGroup.Name))
		clitest.SetupConfig(t, workspaceOwnerClient, root)

		out := bytes.NewBuffer(nil)
		inv.Stdout = out
		err = inv.WithContext(ctx).Run()
		require.NoError(t, err)

		acl, err := workspaceOwnerClient.WorkspaceACL(inv.Context(), workspace.ID)
		require.NoError(t, err)
		assert.Len(t, acl.Groups, 2)

		type workspaceGroup []codersdk.WorkspaceGroup
		assert.NotEqual(t, -1, slices.IndexFunc(workspaceGroup(acl.Groups), func(g codersdk.WorkspaceGroup) bool {
			return g.Group.ID == wibbleGroup.ID
		}))
		assert.NotEqual(t, -1, slices.IndexFunc(workspaceGroup(acl.Groups), func(g codersdk.WorkspaceGroup) bool {
			return g.Group.ID == wobbleGroup.ID
		}))

		t.Run("ShareWithGroups_Role", func(t *testing.T) {
			t.Parallel()

			var (
				client, db, orgOwner = coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
					Options: &coderdtest.Options{
						DeploymentValues: dv,
					},
					LicenseOptions: &coderdenttest.LicenseOptions{
						Features: license.Features{
							codersdk.FeatureTemplateRBAC: 1,
						},
					},
				})
				workspaceOwnerClient, workspaceOwner = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID, rbac.ScopedRoleOrgAuditor(orgOwner.OrganizationID))
				workspace                            = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
					OwnerID:        workspaceOwner.ID,
					OrganizationID: orgOwner.OrganizationID,
				}).Do().Workspace
				_, orgMember = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID)
			)

			ctx := testutil.Context(t, testutil.WaitMedium)

			group, err := createGroupWithMembers(ctx, client, orgOwner.OrganizationID, "new-group", []uuid.UUID{orgMember.ID})
			require.NoError(t, err)

			inv, root := clitest.New(t, "sharing", "share", workspace.Name, "--org", orgOwner.OrganizationID.String(), "--group", fmt.Sprintf("%s:admin", group.Name))
			clitest.SetupConfig(t, workspaceOwnerClient, root)

			out := bytes.NewBuffer(nil)
			inv.Stdout = out
			err = inv.WithContext(ctx).Run()
			require.NoError(t, err)

			acl, err := workspaceOwnerClient.WorkspaceACL(inv.Context(), workspace.ID)
			require.NoError(t, err)
			assert.Len(t, acl.Groups, 1)
			assert.Equal(t, acl.Groups[0].Group.ID, group.ID)
			assert.Equal(t, acl.Groups[0].Role, codersdk.WorkspaceRoleAdmin)

			found := false
			for _, line := range strings.Split(out.String(), "\n") {
				found = strings.Contains(line, group.Name) && strings.Contains(line, string(codersdk.WorkspaceRoleAdmin))
				if found {
					break
				}
			}
			assert.True(t, found, "Expected to find group name %s and role %s in output: %s", group.Name, codersdk.WorkspaceRoleAdmin, out.String())
		})
	})
}

func createGroupWithMembers(ctx context.Context, client *codersdk.Client, orgID uuid.UUID, name string, memberIDs []uuid.UUID) (codersdk.Group, error) {
	group, err := client.CreateGroup(ctx, orgID, codersdk.CreateGroupRequest{
		Name:        name,
		DisplayName: name,
	})
	if err != nil {
		return codersdk.Group{}, err
	}

	ids := make([]string, len(memberIDs))
	for i, id := range memberIDs {
		ids[i] = id.String()
	}

	return client.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
		AddUsers: ids,
	})
}

func TestSharingStatus(t *testing.T) {
	t.Parallel()

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentWorkspaceSharing)}

	t.Run("ListSharedUsers", func(t *testing.T) {
		t.Parallel()

		var (
			client, db, orgOwner = coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
				Options: &coderdtest.Options{
					DeploymentValues: dv,
				},
				LicenseOptions: &coderdenttest.LicenseOptions{
					Features: license.Features{
						codersdk.FeatureTemplateRBAC: 1,
					},
				},
			})
			workspaceOwnerClient, workspaceOwner = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID, rbac.ScopedRoleOrgAuditor(orgOwner.OrganizationID))
			workspace                            = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OwnerID:        workspaceOwner.ID,
				OrganizationID: orgOwner.OrganizationID,
			}).Do().Workspace
			_, orgMember = coderdtest.CreateAnotherUser(t, client, orgOwner.OrganizationID)
			ctx          = testutil.Context(t, testutil.WaitMedium)
		)

		group, err := createGroupWithMembers(ctx, client, orgOwner.OrganizationID, "new-group", []uuid.UUID{orgMember.ID})
		require.NoError(t, err)

		err = client.UpdateWorkspaceACL(ctx, workspace.ID, codersdk.UpdateWorkspaceACL{
			GroupRoles: map[string]codersdk.WorkspaceRole{
				group.ID.String(): codersdk.WorkspaceRoleUse,
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
			if strings.Contains(line, orgMember.Username) && strings.Contains(line, string(codersdk.WorkspaceRoleUse)) && strings.Contains(line, group.Name) {
				found = true
			}

			if found {
				break
			}
		}
		assert.True(t, found, "expected to find username %s with role %s in the output: %s", orgMember.Username, codersdk.WorkspaceRoleUse, out.String())
	})
}

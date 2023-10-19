package cli_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

// TestStart also tests restart since the tests are virtually identical.
func TestStart(t *testing.T) {
	t.Parallel()

	t.Run("RequireActiveVersion", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAccessControl:              1,
					codersdk.FeatureTemplateRBAC:               1,
					codersdk.FeatureAdvancedTemplateScheduling: 1,
				},
			},
		})

		// Create an initial version.
		oldVersion := coderdtest.CreateTemplateVersion(t, ownerClient, owner.OrganizationID, nil)
		// Create a template that mandates the promoted version.
		// This should be enforced for everyone except template admins.
		template := coderdtest.CreateTemplate(t, ownerClient, owner.OrganizationID, oldVersion.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, ownerClient, oldVersion.ID)
		require.Equal(t, oldVersion.ID, template.ActiveVersionID)
		template, err := ownerClient.UpdateTemplateMeta(ctx, template.ID, codersdk.UpdateTemplateMeta{
			RequireActiveVersion: true,
		})
		require.NoError(t, err)
		require.True(t, template.RequireActiveVersion)

		// Create a new version that we will promote.
		activeVersion := coderdtest.CreateTemplateVersion(t, ownerClient, owner.OrganizationID, nil, func(ctvr *codersdk.CreateTemplateVersionRequest) {
			ctvr.TemplateID = template.ID
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, ownerClient, activeVersion.ID)
		err = ownerClient.UpdateActiveTemplateVersion(ctx, template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: activeVersion.ID,
		})
		require.NoError(t, err)
		err = ownerClient.UpdateActiveTemplateVersion(ctx, template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: activeVersion.ID,
		})
		require.NoError(t, err)

		templateAdminClient, templateAdmin := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleTemplateAdmin())
		templateACLAdminClient, templateACLAdmin := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
		templateGroupACLAdminClient, templateGroupACLAdmin := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
		memberClient, member := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

		// Create a group so we can also test group template admin ownership.
		group, err := ownerClient.CreateGroup(ctx, owner.OrganizationID, codersdk.CreateGroupRequest{
			Name: "test",
		})
		require.NoError(t, err)

		// Add the user who gains template admin via group membership.
		group, err = ownerClient.PatchGroup(ctx, group.ID, codersdk.PatchGroupRequest{
			AddUsers: []string{templateGroupACLAdmin.ID.String()},
		})
		require.NoError(t, err)

		// Update the template for both users and groups.
		err = ownerClient.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				templateACLAdmin.ID.String(): codersdk.TemplateRoleAdmin,
			},
			GroupPerms: map[string]codersdk.TemplateRole{
				group.ID.String(): codersdk.TemplateRoleAdmin,
			},
		})
		require.NoError(t, err)

		type testcase struct {
			Name            string
			Client          *codersdk.Client
			WorkspaceOwner  uuid.UUID
			ExpectedVersion uuid.UUID
		}

		cases := []testcase{
			{
				Name:            "OwnerUnchanged",
				Client:          ownerClient,
				WorkspaceOwner:  owner.UserID,
				ExpectedVersion: oldVersion.ID,
			},
			{
				Name:            "TemplateAdminUnchanged",
				Client:          templateAdminClient,
				WorkspaceOwner:  templateAdmin.ID,
				ExpectedVersion: oldVersion.ID,
			},
			{
				Name:            "TemplateACLAdminUnchanged",
				Client:          templateACLAdminClient,
				WorkspaceOwner:  templateACLAdmin.ID,
				ExpectedVersion: oldVersion.ID,
			},
			{
				Name:            "TemplateGroupACLAdminUnchanged",
				Client:          templateGroupACLAdminClient,
				WorkspaceOwner:  templateGroupACLAdmin.ID,
				ExpectedVersion: oldVersion.ID,
			},
			{
				Name:            "MemberUpdates",
				Client:          memberClient,
				WorkspaceOwner:  member.ID,
				ExpectedVersion: activeVersion.ID,
			},
		}

		for _, cmd := range []string{"start", "restart"} {
			cmd := cmd
			t.Run(cmd, func(t *testing.T) {
				t.Parallel()
				for _, c := range cases {
					c := c
					t.Run(c.Name, func(t *testing.T) {
						t.Parallel()

						// Instantiate a new context for each subtest since
						// they can potentially be lengthy.
						ctx := testutil.Context(t, testutil.WaitMedium)
						// Create the workspace using the admin since we want
						// to force the old version.
						ws, err := ownerClient.CreateWorkspace(ctx, owner.OrganizationID, c.WorkspaceOwner.String(), codersdk.CreateWorkspaceRequest{
							TemplateVersionID: oldVersion.ID,
							Name:              coderdtest.RandomUsername(t),
							AutomaticUpdates:  codersdk.AutomaticUpdatesNever,
						})
						require.NoError(t, err)
						coderdtest.AwaitWorkspaceBuildJobCompleted(t, c.Client, ws.LatestBuild.ID)

						if cmd == "start" {
							// Stop the workspace so that we can start it.
							coderdtest.MustTransitionWorkspace(t, c.Client, ws.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop, func(req *codersdk.CreateWorkspaceBuildRequest) {
								req.TemplateVersionID = oldVersion.ID
							})
						}
						// Start the workspace. Every test permutation should
						// pass.
						inv, conf := newCLI(t, cmd, ws.Name, "-y")
						clitest.SetupConfig(t, c.Client, conf)
						err = inv.Run()
						require.NoError(t, err)

						ws = coderdtest.MustWorkspace(t, c.Client, ws.ID)
						require.Equal(t, c.ExpectedVersion, ws.LatestBuild.TemplateVersionID)
					})
				}
			})
		}
	})
}

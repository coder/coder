package cli_test

import (
	"bytes"
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
		templateAdminClient, templateAdmin := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleTemplateAdmin())

		// Create an initial version.
		oldVersion := coderdtest.CreateTemplateVersion(t, templateAdminClient, owner.OrganizationID, nil)
		// Create a template that mandates the promoted version.
		// This should be enforced for everyone except template admins.
		template := coderdtest.CreateTemplate(t, templateAdminClient, owner.OrganizationID, oldVersion.ID)
		coderdtest.AwaitTemplateVersionJobCompleted(t, templateAdminClient, oldVersion.ID)
		require.Equal(t, oldVersion.ID, template.ActiveVersionID)
		template = coderdtest.UpdateTemplateMeta(t, templateAdminClient, template.ID, codersdk.UpdateTemplateMeta{
			RequireActiveVersion: true,
		})
		require.True(t, template.RequireActiveVersion)

		// Create a new version that we will promote.
		activeVersion := coderdtest.CreateTemplateVersion(t, templateAdminClient, owner.OrganizationID, nil, func(ctvr *codersdk.CreateTemplateVersionRequest) {
			ctvr.TemplateID = template.ID
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, templateAdminClient, activeVersion.ID)
		err := templateAdminClient.UpdateActiveTemplateVersion(ctx, template.ID, codersdk.UpdateActiveTemplateVersion{
			ID: activeVersion.ID,
		})
		require.NoError(t, err)

		templateACLAdminClient, templateACLAdmin := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
		templateGroupACLAdminClient, templateGroupACLAdmin := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
		memberClient, member := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

		// Create a group so we can also test group template admin ownership.
		// Add the user who gains template admin via group membership.
		group := coderdtest.CreateGroup(t, ownerClient, owner.OrganizationID, "test", templateGroupACLAdmin)

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

						initialTemplateVersion := ws.LatestBuild.TemplateVersionID

						if cmd == "start" {
							// Stop the workspace so that we can start it.
							coderdtest.MustTransitionWorkspace(t, c.Client, ws.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)
						}
						// Start the workspace. Every test permutation should
						// pass.
						var buf bytes.Buffer
						inv, conf := newCLI(t, cmd, ws.Name, "-y")
						inv.Stdout = &buf
						clitest.SetupConfig(t, c.Client, conf)
						err = inv.Run()
						require.NoError(t, err)

						ws = coderdtest.MustWorkspace(t, c.Client, ws.ID)
						require.Equal(t, c.ExpectedVersion, ws.LatestBuild.TemplateVersionID)
						if initialTemplateVersion != ws.LatestBuild.TemplateVersionID {
							require.Contains(t, buf.String(), "Failed to restart with the template version from your last build. Policy may require you to restart with the current active template version.")
						}
					})
				}
			})
		}
	})

	t.Run("StartActivatesDormant", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		ownerClient, owner := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureAdvancedTemplateScheduling: 1,
				},
			},
		})

		version := coderdtest.CreateTemplateVersion(t, ownerClient, owner.OrganizationID, nil)
		_ = coderdtest.AwaitTemplateVersionJobCompleted(t, ownerClient, version.ID)
		template := coderdtest.CreateTemplate(t, ownerClient, owner.OrganizationID, version.ID)

		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
		workspace := coderdtest.CreateWorkspace(t, memberClient, owner.OrganizationID, template.ID)
		_ = coderdtest.AwaitWorkspaceBuildJobCompleted(t, memberClient, workspace.LatestBuild.ID)
		_ = coderdtest.MustTransitionWorkspace(t, memberClient, workspace.ID, database.WorkspaceTransitionStart, database.WorkspaceTransitionStop)
		err := memberClient.UpdateWorkspaceDormancy(ctx, workspace.ID, codersdk.UpdateWorkspaceDormancy{
			Dormant: true,
		})
		require.NoError(t, err)

		inv, root := newCLI(t, "start", workspace.Name)
		clitest.SetupConfig(t, memberClient, root)

		var buf bytes.Buffer
		inv.Stdout = &buf

		err = inv.Run()
		require.NoError(t, err)
		require.Contains(t, buf.String(), "Activating dormant workspace...")

		workspace = coderdtest.MustWorkspace(t, memberClient, workspace.ID)
		require.Equal(t, codersdk.WorkspaceTransitionStart, workspace.LatestBuild.Transition)
	})
}

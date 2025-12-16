package coderd_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func TestWorkspaceBuild(t *testing.T) {
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

	// For this test we create two templates:
	// tplA will be used to test creation of new workspaces.
	// tplB will be used to test builds on existing workspaces.
	// This is done to enable parallelization of the sub-tests without them interfering with each other.
	// Both templates mandate the promoted version.
	// This should be enforced for everyone except template admins.
	tplAv1 := coderdtest.CreateTemplateVersion(t, ownerClient, owner.OrganizationID, nil)
	tplA := coderdtest.CreateTemplate(t, ownerClient, owner.OrganizationID, tplAv1.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, ownerClient, tplAv1.ID)
	require.Equal(t, tplAv1.ID, tplA.ActiveVersionID)
	tplA = coderdtest.UpdateTemplateMeta(t, ownerClient, tplA.ID, codersdk.UpdateTemplateMeta{
		RequireActiveVersion: true,
	})
	require.True(t, tplA.RequireActiveVersion)
	tplAv2 := coderdtest.CreateTemplateVersion(t, ownerClient, owner.OrganizationID, nil, func(ctvr *codersdk.CreateTemplateVersionRequest) {
		ctvr.TemplateID = tplA.ID
	})
	coderdtest.AwaitTemplateVersionJobCompleted(t, ownerClient, tplAv2.ID)
	coderdtest.UpdateActiveTemplateVersion(t, ownerClient, tplA.ID, tplAv2.ID)

	tplBv1 := coderdtest.CreateTemplateVersion(t, ownerClient, owner.OrganizationID, nil)
	tplB := coderdtest.CreateTemplate(t, ownerClient, owner.OrganizationID, tplBv1.ID)
	coderdtest.AwaitTemplateVersionJobCompleted(t, ownerClient, tplBv1.ID)
	require.Equal(t, tplBv1.ID, tplB.ActiveVersionID)
	tplB = coderdtest.UpdateTemplateMeta(t, ownerClient, tplB.ID, codersdk.UpdateTemplateMeta{
		RequireActiveVersion: true,
	})
	require.True(t, tplB.RequireActiveVersion)

	templateAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleTemplateAdmin())
	templateACLAdminClient, templateACLAdmin := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
	templateGroupACLAdminClient, templateGroupACLAdmin := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
	memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

	// Create a group so we can also test group template admin ownership.
	// Add the user who gains template admin via group membership.
	group := coderdtest.CreateGroup(t, ownerClient, owner.OrganizationID, "test", templateGroupACLAdmin)

	// Update the template for both users and groups.
	//nolint:gocritic // test setup
	for _, tpl := range []codersdk.Template{tplA, tplB} {
		err := ownerClient.UpdateTemplateACL(ctx, tpl.ID, codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				templateACLAdmin.ID.String(): codersdk.TemplateRoleAdmin,
			},
			GroupPerms: map[string]codersdk.TemplateRole{
				group.ID.String(): codersdk.TemplateRoleAdmin,
			},
		})
		require.NoError(t, err, "updating template ACL for template %q", tpl.ID)
	}

	type testcase struct {
		Name               string
		Client             *codersdk.Client
		ExpectedStatusCode int
	}

	cases := []testcase{
		{
			Name:               "OwnerOK",
			Client:             ownerClient,
			ExpectedStatusCode: http.StatusOK,
		},
		{
			Name:               "TemplateAdminOK",
			Client:             templateAdminClient,
			ExpectedStatusCode: http.StatusOK,
		},
		{
			Name:               "TemplateACLAdminOK",
			Client:             templateACLAdminClient,
			ExpectedStatusCode: http.StatusOK,
		},
		{
			Name:               "TemplateGroupACLAdminOK",
			Client:             templateGroupACLAdminClient,
			ExpectedStatusCode: http.StatusOK,
		},
		{
			Name:               "MemberFailsToCreate",
			Client:             memberClient,
			ExpectedStatusCode: http.StatusForbidden,
		},
	}

	t.Run("NewWorkspace", func(t *testing.T) {
		t.Parallel()

		for _, c := range cases {
			t.Run(c.Name, func(t *testing.T) {
				t.Parallel()
				ws, err := c.Client.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
					TemplateVersionID: tplAv1.ID,
					Name:              testutil.GetRandomNameHyphenated(t),
					AutomaticUpdates:  codersdk.AutomaticUpdatesNever,
				})
				if c.ExpectedStatusCode == http.StatusOK {
					require.NoError(t, err)
					require.Equal(t, tplAv1.ID, ws.LatestBuild.TemplateVersionID, "workspace did not use expected version for case %q", c.Name)
				} else {
					require.Error(t, err)
					cerr, ok := codersdk.AsError(err)
					require.True(t, ok)
					require.Equal(t, c.ExpectedStatusCode, cerr.StatusCode())
				}
			})
		}
	})

	t.Run("ExistingWorkspace", func(t *testing.T) {
		t.Parallel()

		// Setup: create workspaces for each of the test cases.
		var extantWorkspaces []codersdk.Workspace
		for _, c := range cases {
			extantWs, err := c.Client.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateVersionID: tplB.ActiveVersionID,
				Name:              testutil.GetRandomNameHyphenated(t),
				AutomaticUpdates:  codersdk.AutomaticUpdatesNever,
			})
			require.NoError(t, err, "setup workspace for case %q", c.Name)
			extantWorkspaces = append(extantWorkspaces, extantWs)
		}

		// Setup: Create a new version of template B and promote it to be the active version.
		tplBv2 := coderdtest.CreateTemplateVersion(t, ownerClient, owner.OrganizationID, nil, func(ctvr *codersdk.CreateTemplateVersionRequest) {
			ctvr.TemplateID = tplB.ID
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, ownerClient, tplBv2.ID)
		coderdtest.UpdateActiveTemplateVersion(t, ownerClient, tplB.ID, tplBv2.ID)

		for idx, c := range cases {
			t.Run(c.Name, func(t *testing.T) {
				t.Parallel()
				// Stopping the workspace must always succeed.
				wb, err := c.Client.CreateWorkspaceBuild(ctx, extantWorkspaces[idx].ID, codersdk.CreateWorkspaceBuildRequest{
					Transition: codersdk.WorkspaceTransitionStop,
				})
				require.NoError(t, err, "stopping workspace for case %q", c.Name)
				coderdtest.AwaitWorkspaceBuildJobCompleted(t, c.Client, wb.ID)

				// Attempt to start the workspace with the given version.
				wb, err = c.Client.CreateWorkspaceBuild(ctx, extantWorkspaces[idx].ID, codersdk.CreateWorkspaceBuildRequest{
					Transition:        codersdk.WorkspaceTransitionStart,
					TemplateVersionID: tplBv1.ID,
				})
				if c.ExpectedStatusCode == http.StatusOK {
					require.NoError(t, err, "starting workspace for case %q", c.Name)
					coderdtest.AwaitWorkspaceBuildJobCompleted(t, c.Client, wb.ID)
					require.Equal(t, tplBv1.ID, wb.TemplateVersionID, "workspace did not use expected version for case %q", c.Name)
				} else {
					require.Error(t, err, "starting workspace for case %q", c.Name)
					cerr, ok := codersdk.AsError(err)
					require.True(t, ok)
					require.Equal(t, c.ExpectedStatusCode, cerr.StatusCode())
				}
			})
		}
	})
}

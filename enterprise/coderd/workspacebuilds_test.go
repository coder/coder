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
	t.Run("TemplateRequiresActiveVersion", func(t *testing.T) {
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
		template = coderdtest.UpdateTemplateMeta(t, ownerClient, template.ID, codersdk.UpdateTemplateMeta{
			RequireActiveVersion: true,
		})
		require.True(t, template.RequireActiveVersion)

		// Create a new version that we will promote.
		activeVersion := coderdtest.CreateTemplateVersion(t, ownerClient, owner.OrganizationID, nil, func(ctvr *codersdk.CreateTemplateVersionRequest) {
			ctvr.TemplateID = template.ID
		})
		coderdtest.AwaitTemplateVersionJobCompleted(t, ownerClient, activeVersion.ID)
		coderdtest.UpdateActiveTemplateVersion(t, ownerClient, template.ID, activeVersion.ID)

		templateAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID, rbac.RoleTemplateAdmin())
		templateACLAdminClient, templateACLAdmin := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
		templateGroupACLAdminClient, templateGroupACLAdmin := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)
		memberClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, owner.OrganizationID)

		// Create a group so we can also test group template admin ownership.
		// Add the user who gains template admin via group membership.
		group := coderdtest.CreateGroup(t, ownerClient, owner.OrganizationID, "test", templateGroupACLAdmin)

		// Update the template for both users and groups.
		//nolint:gocritic // test setup
		err := ownerClient.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
			UserPerms: map[string]codersdk.TemplateRole{
				templateACLAdmin.ID.String(): codersdk.TemplateRoleAdmin,
			},
			GroupPerms: map[string]codersdk.TemplateRole{
				group.ID.String(): codersdk.TemplateRoleAdmin,
			},
		})
		require.NoError(t, err)

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
				Name:               "MemberFails",
				Client:             memberClient,
				ExpectedStatusCode: http.StatusForbidden,
			},
		}

		for _, c := range cases {
			t.Run(c.Name, func(t *testing.T) {
				_, err = c.Client.CreateWorkspace(ctx, owner.OrganizationID, codersdk.Me, codersdk.CreateWorkspaceRequest{
					TemplateVersionID: oldVersion.ID,
					Name:              "abc123",
					AutomaticUpdates:  codersdk.AutomaticUpdatesNever,
				})
				if c.ExpectedStatusCode == http.StatusOK {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
					cerr, ok := codersdk.AsError(err)
					require.True(t, ok)
					require.Equal(t, c.ExpectedStatusCode, cerr.StatusCode())
				}
			})
		}
	})
}

package coderd_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/enterprise/coderd/license"
	"github.com/coder/coder/testutil"
)

func TestCheckACLPermissions(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	t.Cleanup(cancel)

	adminClient := coderdenttest.New(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		},
	})
	// Create adminClient, member, and org adminClient
	adminUser := coderdtest.CreateFirstUser(t, adminClient)
	_ = coderdenttest.AddLicense(t, adminClient, coderdenttest.LicenseOptions{
		Features: license.Features{
			codersdk.FeatureTemplateRBAC: 1,
		},
	})

	memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)
	memberUser, err := memberClient.User(ctx, codersdk.Me)
	require.NoError(t, err)
	orgAdminClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID, rbac.RoleOrgAdmin(adminUser.OrganizationID))
	orgAdminUser, err := orgAdminClient.User(ctx, codersdk.Me)
	require.NoError(t, err)

	version := coderdtest.CreateTemplateVersion(t, adminClient, adminUser.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJob(t, adminClient, version.ID)
	template := coderdtest.CreateTemplate(t, adminClient, adminUser.OrganizationID, version.ID)

	err = adminClient.UpdateTemplateACL(ctx, template.ID, codersdk.UpdateTemplateACL{
		UserPerms: map[string]codersdk.TemplateRole{
			memberUser.ID.String(): codersdk.TemplateRoleAdmin,
		},
	})
	require.NoError(t, err)

	const (
		updateSpecificTemplate = "read-specific-template"
	)
	params := map[string]codersdk.AuthorizationCheck{
		updateSpecificTemplate: {
			Object: codersdk.AuthorizationObject{
				ResourceType: rbac.ResourceTemplate.Type,
				ResourceID:   template.ID.String(),
			},
			Action: "write",
		},
	}

	testCases := []struct {
		Name   string
		Client *codersdk.Client
		UserID uuid.UUID
		Check  codersdk.AuthorizationResponse
	}{
		{
			Name:   "Admin",
			Client: adminClient,
			UserID: adminUser.UserID,
			Check: map[string]bool{
				updateSpecificTemplate: true,
			},
		},
		{
			Name:   "OrgAdmin",
			Client: orgAdminClient,
			UserID: orgAdminUser.ID,
			Check: map[string]bool{
				updateSpecificTemplate: true,
			},
		},
		{
			Name:   "Member",
			Client: memberClient,
			UserID: memberUser.ID,
			Check: map[string]bool{
				updateSpecificTemplate: true,
			},
		},
	}

	for _, c := range testCases {
		c := c

		t.Run("CheckAuthorization/"+c.Name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			t.Cleanup(cancel)

			resp, err := c.Client.AuthCheck(ctx, codersdk.AuthorizationRequest{Checks: params})
			require.NoError(t, err, "check perms")
			require.Equal(t, c.Check, resp)
		})
	}
}

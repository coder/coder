package coderd_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestCheckPermissions(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	t.Cleanup(cancel)

	adminClient := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	// Create adminClient, member, and org adminClient
	adminUser := coderdtest.CreateFirstUser(t, adminClient)
	memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)
	memberUser, err := memberClient.User(ctx, codersdk.Me)
	require.NoError(t, err)
	orgAdminClient, _ := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID, rbac.RoleOrgAdmin(adminUser.OrganizationID))
	orgAdminUser, err := orgAdminClient.User(ctx, codersdk.Me)
	require.NoError(t, err)

	version := coderdtest.CreateTemplateVersion(t, adminClient, adminUser.OrganizationID, nil)
	coderdtest.AwaitTemplateVersionJobCompleted(t, adminClient, version.ID)
	template := coderdtest.CreateTemplate(t, adminClient, adminUser.OrganizationID, version.ID)

	// With admin, member, and org admin
	const (
		readAllUsers           = "read-all-users"
		readOrgWorkspaces      = "read-org-workspaces"
		readMyself             = "read-myself"
		readOwnWorkspaces      = "read-own-workspaces"
		updateSpecificTemplate = "update-specific-template"
	)
	params := map[string]codersdk.AuthorizationCheck{
		readAllUsers: {
			Object: codersdk.AuthorizationObject{
				ResourceType: codersdk.ResourceUser,
			},
			Action: "read",
		},
		readMyself: {
			Object: codersdk.AuthorizationObject{
				ResourceType: codersdk.ResourceUser,
				OwnerID:      "me",
			},
			Action: "read",
		},
		readOwnWorkspaces: {
			Object: codersdk.AuthorizationObject{
				ResourceType: codersdk.ResourceWorkspace,
				OwnerID:      "me",
			},
			Action: "read",
		},
		readOrgWorkspaces: {
			Object: codersdk.AuthorizationObject{
				ResourceType:   codersdk.ResourceWorkspace,
				OrganizationID: adminUser.OrganizationID.String(),
			},
			Action: "read",
		},
		updateSpecificTemplate: {
			Object: codersdk.AuthorizationObject{
				ResourceType: codersdk.ResourceTemplate,
				ResourceID:   template.ID.String(),
			},
			Action: "update",
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
				readAllUsers:           true,
				readMyself:             true,
				readOwnWorkspaces:      true,
				readOrgWorkspaces:      true,
				updateSpecificTemplate: true,
			},
		},
		{
			Name:   "OrgAdmin",
			Client: orgAdminClient,
			UserID: orgAdminUser.ID,
			Check: map[string]bool{
				readAllUsers:           false,
				readMyself:             true,
				readOwnWorkspaces:      true,
				readOrgWorkspaces:      true,
				updateSpecificTemplate: true,
			},
		},
		{
			Name:   "Member",
			Client: memberClient,
			UserID: memberUser.ID,
			Check: map[string]bool{
				readAllUsers:           false,
				readMyself:             true,
				readOwnWorkspaces:      true,
				readOrgWorkspaces:      false,
				updateSpecificTemplate: false,
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

package coderd_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

func TestCheckPermissions(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	t.Cleanup(cancel)

	adminClient := coderdtest.New(t, nil)
	// Create adminClient, member, and org adminClient
	adminUser := coderdtest.CreateFirstUser(t, adminClient)
	memberClient := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID)
	memberUser, err := memberClient.User(ctx, codersdk.Me)
	require.NoError(t, err)
	orgAdminClient := coderdtest.CreateAnotherUser(t, adminClient, adminUser.OrganizationID, rbac.RoleOrgAdmin(adminUser.OrganizationID))
	orgAdminUser, err := orgAdminClient.User(ctx, codersdk.Me)
	require.NoError(t, err)

	// With admin, member, and org admin
	const (
		readAllUsers      = "read-all-users"
		readOrgWorkspaces = "read-org-workspaces"
		readMyself        = "read-myself"
		readOwnWorkspaces = "read-own-workspaces"
	)
	params := map[string]codersdk.AuthorizationCheck{
		readAllUsers: {
			Object: codersdk.AuthorizationObject{
				ResourceType: "users",
			},
			Action: "read",
		},
		readMyself: {
			Object: codersdk.AuthorizationObject{
				ResourceType: "users",
				OwnerID:      "me",
			},
			Action: "read",
		},
		readOwnWorkspaces: {
			Object: codersdk.AuthorizationObject{
				ResourceType: "workspaces",
				OwnerID:      "me",
			},
			Action: "read",
		},
		readOrgWorkspaces: {
			Object: codersdk.AuthorizationObject{
				ResourceType:   "workspaces",
				OrganizationID: adminUser.OrganizationID.String(),
			},
			Action: "read",
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
				readAllUsers:      true,
				readMyself:        true,
				readOwnWorkspaces: true,
				readOrgWorkspaces: true,
			},
		},
		{
			Name:   "OrgAdmin",
			Client: orgAdminClient,
			UserID: orgAdminUser.ID,
			Check: map[string]bool{
				readAllUsers:      false,
				readMyself:        true,
				readOwnWorkspaces: true,
				readOrgWorkspaces: true,
			},
		},
		{
			Name:   "Member",
			Client: memberClient,
			UserID: memberUser.ID,
			Check: map[string]bool{
				readAllUsers:      false,
				readMyself:        true,
				readOwnWorkspaces: true,
				readOrgWorkspaces: false,
			},
		},
	}

	for _, c := range testCases {
		c := c

		t.Run("CheckAuthorization/"+c.Name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			t.Cleanup(cancel)

			resp, err := c.Client.CheckAuthorization(ctx, codersdk.AuthorizationRequest{Checks: params})
			require.NoError(t, err, "check perms")
			require.Equal(t, c.Check, resp)
		})
	}
}

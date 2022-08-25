package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/testutil"
)

func TestAuthorization(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	// Create admin, member, and org admin
	admin := coderdtest.CreateFirstUser(t, client)
	member := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
	orgAdmin := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID, rbac.RoleOrgAdmin(admin.OrganizationID))

	// With admin, member, and org admin
	const (
		allUsers          = "read-all-users"
		readOrgWorkspaces = "read-org-workspaces"
		myself            = "read-myself"
		myWorkspace       = "read-my-workspace"
	)
	params := map[string]codersdk.UserAuthorization{
		allUsers: {
			Object: codersdk.UserAuthorizationObject{
				ResourceType: "users",
			},
			Action: "read",
		},
		myself: {
			Object: codersdk.UserAuthorizationObject{
				ResourceType: "users",
				OwnerID:      "me",
			},
			Action: "read",
		},
		myWorkspace: {
			Object: codersdk.UserAuthorizationObject{
				ResourceType: "workspaces",
				OwnerID:      "me",
			},
			Action: "read",
		},
		readOrgWorkspaces: {
			Object: codersdk.UserAuthorizationObject{
				ResourceType:   "workspaces",
				OrganizationID: admin.OrganizationID.String(),
			},
			Action: "read",
		},
	}

	testCases := []struct {
		Name   string
		Client *codersdk.Client
		Check  codersdk.UserAuthorizationResponse
	}{
		{
			Name:   "Admin",
			Client: client,
			Check: map[string]bool{
				allUsers: true, myself: true, myWorkspace: true, readOrgWorkspaces: true,
			},
		},
		{
			Name:   "Member",
			Client: member,
			Check: map[string]bool{
				allUsers: false, myself: true, myWorkspace: true, readOrgWorkspaces: false,
			},
		},
		{
			Name:   "OrgAdmin",
			Client: orgAdmin,
			Check: map[string]bool{
				allUsers: false, myself: true, myWorkspace: true, readOrgWorkspaces: true,
			},
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			resp, err := c.Client.CheckPermissions(ctx, codersdk.UserAuthorizationRequest{Checks: params})
			require.NoError(t, err, "check perms")
			require.Equal(t, resp, c.Check)
		})
	}
}

func TestListRoles(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	// Create admin, member, and org admin
	admin := coderdtest.CreateFirstUser(t, client)
	member := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
	orgAdmin := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID, rbac.RoleOrgAdmin(admin.OrganizationID))

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	t.Cleanup(cancel)

	otherOrg, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
		Name: "other",
	})
	require.NoError(t, err, "create org")

	const forbidden = "Forbidden"
	testCases := []struct {
		Name            string
		Client          *codersdk.Client
		APICall         func(context.Context) ([]codersdk.AssignableRoles, error)
		ExpectedRoles   []codersdk.AssignableRoles
		AuthorizedError string
	}{
		{
			// Members cannot assign any roles
			Name: "MemberListSite",
			APICall: func(ctx context.Context) ([]codersdk.AssignableRoles, error) {
				x, err := member.ListSiteRoles(ctx)
				return x, err
			},
			ExpectedRoles: convertRoles(map[string]bool{
				"owner":          false,
				"auditor":        false,
				"template-admin": false,
				"user-admin":     false,
			}),
		},
		{
			Name: "OrgMemberListOrg",
			APICall: func(ctx context.Context) ([]codersdk.AssignableRoles, error) {
				return member.ListOrganizationRoles(ctx, admin.OrganizationID)
			},
			ExpectedRoles: convertRoles(map[string]bool{
				rbac.RoleOrgAdmin(admin.OrganizationID): false,
			}),
		},
		{
			Name: "NonOrgMemberListOrg",
			APICall: func(ctx context.Context) ([]codersdk.AssignableRoles, error) {
				return member.ListOrganizationRoles(ctx, otherOrg.ID)
			},
			AuthorizedError: forbidden,
		},
		// Org admin
		{
			Name: "OrgAdminListSite",
			APICall: func(ctx context.Context) ([]codersdk.AssignableRoles, error) {
				return orgAdmin.ListSiteRoles(ctx)
			},
			ExpectedRoles: convertRoles(map[string]bool{
				"owner":          false,
				"auditor":        false,
				"template-admin": false,
				"user-admin":     false,
			}),
		},
		{
			Name: "OrgAdminListOrg",
			APICall: func(ctx context.Context) ([]codersdk.AssignableRoles, error) {
				return orgAdmin.ListOrganizationRoles(ctx, admin.OrganizationID)
			},
			ExpectedRoles: convertRoles(map[string]bool{
				rbac.RoleOrgAdmin(admin.OrganizationID): true,
			}),
		},
		{
			Name: "OrgAdminListOtherOrg",
			APICall: func(ctx context.Context) ([]codersdk.AssignableRoles, error) {
				return orgAdmin.ListOrganizationRoles(ctx, otherOrg.ID)
			},
			AuthorizedError: forbidden,
		},
		// Admin
		{
			Name: "AdminListSite",
			APICall: func(ctx context.Context) ([]codersdk.AssignableRoles, error) {
				return client.ListSiteRoles(ctx)
			},
			ExpectedRoles: convertRoles(map[string]bool{
				"owner":          true,
				"auditor":        true,
				"template-admin": true,
				"user-admin":     true,
			}),
		},
		{
			Name: "AdminListOrg",
			APICall: func(ctx context.Context) ([]codersdk.AssignableRoles, error) {
				return client.ListOrganizationRoles(ctx, admin.OrganizationID)
			},
			ExpectedRoles: convertRoles(map[string]bool{
				rbac.RoleOrgAdmin(admin.OrganizationID): true,
			}),
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
			defer cancel()

			roles, err := c.APICall(ctx)
			if c.AuthorizedError != "" {
				var apiErr *codersdk.Error
				require.ErrorAs(t, err, &apiErr)
				require.Equal(t, http.StatusForbidden, apiErr.StatusCode())
				require.Contains(t, apiErr.Message, c.AuthorizedError)
			} else {
				require.NoError(t, err)
				require.ElementsMatch(t, c.ExpectedRoles, roles)
			}
		})
	}
}

func convertRole(roleName string) codersdk.Role {
	role, _ := rbac.RoleByName(roleName)
	return codersdk.Role{
		DisplayName: role.DisplayName,
		Name:        role.Name,
	}
}

func convertRoles(assignableRoles map[string]bool) []codersdk.AssignableRoles {
	converted := make([]codersdk.AssignableRoles, 0, len(assignableRoles))
	for roleName, assignable := range assignableRoles {
		role := convertRole(roleName)
		converted = append(converted, codersdk.AssignableRoles{
			Role:       role,
			Assignable: assignable,
		})
	}
	return converted
}

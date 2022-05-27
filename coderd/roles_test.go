package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
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
			resp, err := c.Client.CheckPermissions(context.Background(), codersdk.UserAuthorizationRequest{Checks: params})
			require.NoError(t, err, "check perms")
			require.Equal(t, resp, c.Check)
		})
	}
}

func TestListRoles(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client := coderdtest.New(t, nil)
	// Create admin, member, and org admin
	admin := coderdtest.CreateFirstUser(t, client)
	member := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
	orgAdmin := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID, rbac.RoleOrgAdmin(admin.OrganizationID))

	otherOrg, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
		Name: "other",
	})
	require.NoError(t, err, "create org")

	const forbidden = "forbidden"

	testCases := []struct {
		Name            string
		Client          *codersdk.Client
		APICall         func() ([]codersdk.Role, error)
		ExpectedRoles   []codersdk.Role
		AuthorizedError string
	}{
		{
			Name: "MemberListSite",
			APICall: func() ([]codersdk.Role, error) {
				x, err := member.ListSiteRoles(ctx)
				return x, err
			},
			ExpectedRoles: convertRoles(rbac.SiteRoles()),
		},
		{
			Name: "OrgMemberListOrg",
			APICall: func() ([]codersdk.Role, error) {
				return member.ListOrganizationRoles(ctx, admin.OrganizationID)
			},
			ExpectedRoles: convertRoles(rbac.OrganizationRoles(admin.OrganizationID)),
		},
		{
			Name: "NonOrgMemberListOrg",
			APICall: func() ([]codersdk.Role, error) {
				return member.ListOrganizationRoles(ctx, otherOrg.ID)
			},
			AuthorizedError: forbidden,
		},
		// Org admin
		{
			Name: "OrgAdminListSite",
			APICall: func() ([]codersdk.Role, error) {
				return orgAdmin.ListSiteRoles(ctx)
			},
			ExpectedRoles: convertRoles(rbac.SiteRoles()),
		},
		{
			Name: "OrgAdminListOrg",
			APICall: func() ([]codersdk.Role, error) {
				return orgAdmin.ListOrganizationRoles(ctx, admin.OrganizationID)
			},
			ExpectedRoles: convertRoles(rbac.OrganizationRoles(admin.OrganizationID)),
		},
		{
			Name: "OrgAdminListOtherOrg",
			APICall: func() ([]codersdk.Role, error) {
				return orgAdmin.ListOrganizationRoles(ctx, otherOrg.ID)
			},
			AuthorizedError: forbidden,
		},
		// Admin
		{
			Name: "AdminListSite",
			APICall: func() ([]codersdk.Role, error) {
				return client.ListSiteRoles(ctx)
			},
			ExpectedRoles: convertRoles(rbac.SiteRoles()),
		},
		{
			Name: "AdminListOrg",
			APICall: func() ([]codersdk.Role, error) {
				return client.ListOrganizationRoles(ctx, admin.OrganizationID)
			},
			ExpectedRoles: convertRoles(rbac.OrganizationRoles(admin.OrganizationID)),
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			roles, err := c.APICall()
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

func convertRole(role rbac.Role) codersdk.Role {
	return codersdk.Role{
		DisplayName: role.DisplayName,
		Name:        role.Name,
	}
}

func convertRoles(roles []rbac.Role) []codersdk.Role {
	converted := make([]codersdk.Role, 0, len(roles))
	for _, role := range roles {
		converted = append(converted, convertRole(role))
	}
	return converted
}

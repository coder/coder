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

func TestListRoles(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client := coderdtest.New(t, nil)
	// Create admin, member, and org admin
	admin := coderdtest.CreateFirstUser(t, client)
	member := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)

	orgAdmin := coderdtest.CreateAnotherUser(t, client, admin.OrganizationID)
	orgAdminUser, err := orgAdmin.User(ctx, codersdk.Me)
	require.NoError(t, err)

	// TODO: @emyrk switch this to the admin when getting non-personal users is
	//	supported. `client.UpdateOrganizationMemberRoles(...)`
	_, err = orgAdmin.UpdateOrganizationMemberRoles(ctx, admin.OrganizationID, orgAdminUser.ID,
		codersdk.UpdateRoles{
			Roles: []string{rbac.RoleOrgMember(admin.OrganizationID), rbac.RoleOrgAdmin(admin.OrganizationID)},
		},
	)
	require.NoError(t, err, "update org member roles")

	otherOrg, err := client.CreateOrganization(ctx, admin.UserID, codersdk.CreateOrganizationRequest{
		Name: "other",
	})
	require.NoError(t, err, "create org")

	const unauth = "unauthorized"
	const notMember = "not a member of the organization"

	testCases := []struct {
		Name            string
		Client          *codersdk.Client
		APICall         func() ([]string, error)
		ExpectedRoles   []string
		AuthorizedError string
	}{
		{
			Name: "MemberListSite",
			APICall: func() ([]string, error) {
				x, err := member.ListSiteRoles(ctx)
				return x, err
			},
			AuthorizedError: unauth,
		},
		{
			Name: "OrgMemberListOrg",
			APICall: func() ([]string, error) {
				return member.ListOrganizationRoles(ctx, admin.OrganizationID)
			},
			AuthorizedError: unauth,
		},
		{
			Name: "NonOrgMemberListOrg",
			APICall: func() ([]string, error) {
				return member.ListOrganizationRoles(ctx, otherOrg.ID)
			},
			AuthorizedError: notMember,
		},
		// Org admin
		{
			Name: "OrgAdminListSite",
			APICall: func() ([]string, error) {
				return orgAdmin.ListSiteRoles(ctx)
			},
			AuthorizedError: unauth,
		},
		{
			Name: "OrgAdminListOrg",
			APICall: func() ([]string, error) {
				return orgAdmin.ListOrganizationRoles(ctx, admin.OrganizationID)
			},
			ExpectedRoles: rbac.OrganizationRoles(admin.OrganizationID),
		},
		{
			Name: "OrgAdminListOtherOrg",
			APICall: func() ([]string, error) {
				return orgAdmin.ListOrganizationRoles(ctx, otherOrg.ID)
			},
			AuthorizedError: notMember,
		},
		// Admin
		{
			Name: "AdminListSite",
			APICall: func() ([]string, error) {
				return client.ListSiteRoles(ctx)
			},
			ExpectedRoles: rbac.SiteRoles(),
		},
		{
			Name: "AdminListOrg",
			APICall: func() ([]string, error) {
				return client.ListOrganizationRoles(ctx, admin.OrganizationID)
			},
			ExpectedRoles: rbac.OrganizationRoles(admin.OrganizationID),
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
				require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
				require.Contains(t, apiErr.Message, c.AuthorizedError)
			} else {
				require.NoError(t, err)
				require.ElementsMatch(t, c.ExpectedRoles, roles)
			}
		})
	}
}

package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestListRoles(t *testing.T) {
	t.Parallel()

	requireUnauthorized := func(t *testing.T, err error) {
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
		require.Contains(t, apiErr.Message, "unauthorized")
	}

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
	require.NoError(t, err)

	testCases := []struct {
		Name          string
		Client        *codersdk.Client
		APICall       func() ([]string, error)
		ExpectedRoles []string
		Authorized    bool
	}{
		{
			Name: "MemberListSite",
			APICall: func() ([]string, error) {
				x, err := member.ListSiteRoles(ctx)
				return x, err
			},
			Authorized: false,
		},
		{
			Name: "OrgMemberListOrg",
			APICall: func() ([]string, error) {
				return member.ListOrganizationRoles(ctx, admin.OrganizationID)
			},
			Authorized: false,
		},
		{
			Name: "NonOrgMemberListOrg",
			APICall: func() ([]string, error) {
				return member.ListOrganizationRoles(ctx, uuid.New())
			},
			Authorized: false,
		},
		// Org admin
		{
			Name: "OrgAdminListSite",
			APICall: func() ([]string, error) {
				return orgAdmin.ListSiteRoles(ctx)
			},
			Authorized: false,
		},
		{
			Name: "OrgAdminListOrg",
			APICall: func() ([]string, error) {
				return orgAdmin.ListOrganizationRoles(ctx, admin.OrganizationID)
			},
			Authorized:    true,
			ExpectedRoles: rbac.ListOrganizationRoles(admin.OrganizationID),
		},
		{
			Name: "OrgAdminListOtherOrg",
			APICall: func() ([]string, error) {
				return orgAdmin.ListOrganizationRoles(ctx, uuid.New())
			},
			Authorized: false,
		},
		// Admin
		{
			Name: "AdminListSite",
			APICall: func() ([]string, error) {
				return client.ListSiteRoles(ctx)
			},
			Authorized:    true,
			ExpectedRoles: rbac.ListSiteRoles(),
		},
		{
			Name: "AdminListOrg",
			APICall: func() ([]string, error) {
				return client.ListOrganizationRoles(ctx, admin.OrganizationID)
			},
			Authorized:    true,
			ExpectedRoles: rbac.ListOrganizationRoles(admin.OrganizationID),
		},
	}

	for _, c := range testCases {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			roles, err := c.APICall()
			if !c.Authorized {
				requireUnauthorized(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, c.ExpectedRoles, roles)
			}
		})
	}
}

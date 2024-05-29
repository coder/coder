package coderd_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestListRoles(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	// Create owner, member, and org admin
	owner := coderdtest.CreateFirstUser(t, client)
	member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	orgAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleOrgAdmin(owner.OrganizationID))

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	t.Cleanup(cancel)

	otherOrg, err := client.CreateOrganization(ctx, codersdk.CreateOrganizationRequest{
		Name: "other",
	})
	require.NoError(t, err, "create org")

	const notFound = "Resource not found"
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
				return member.ListOrganizationRoles(ctx, owner.OrganizationID)
			},
			ExpectedRoles: convertRoles(map[string]bool{
				rbac.RoleOrgAdmin(owner.OrganizationID): false,
			}),
		},
		{
			Name: "NonOrgMemberListOrg",
			APICall: func(ctx context.Context) ([]codersdk.AssignableRoles, error) {
				return member.ListOrganizationRoles(ctx, otherOrg.ID)
			},
			AuthorizedError: notFound,
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
				return orgAdmin.ListOrganizationRoles(ctx, owner.OrganizationID)
			},
			ExpectedRoles: convertRoles(map[string]bool{
				rbac.RoleOrgAdmin(owner.OrganizationID): true,
			}),
		},
		{
			Name: "OrgAdminListOtherOrg",
			APICall: func(ctx context.Context) ([]codersdk.AssignableRoles, error) {
				return orgAdmin.ListOrganizationRoles(ctx, otherOrg.ID)
			},
			AuthorizedError: notFound,
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
				return client.ListOrganizationRoles(ctx, owner.OrganizationID)
			},
			ExpectedRoles: convertRoles(map[string]bool{
				rbac.RoleOrgAdmin(owner.OrganizationID): true,
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
				require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
				require.Contains(t, apiErr.Message, c.AuthorizedError)
			} else {
				require.NoError(t, err)
				ignorePerms := func(f codersdk.AssignableRoles) codersdk.AssignableRoles {
					return codersdk.AssignableRoles{
						Role: codersdk.Role{
							Name:        f.Name,
							DisplayName: f.DisplayName,
						},
						Assignable: f.Assignable,
						BuiltIn:    true,
					}
				}
				expected := db2sdk.List(c.ExpectedRoles, ignorePerms)
				found := db2sdk.List(roles, ignorePerms)
				require.ElementsMatch(t, expected, found)
			}
		})
	}
}

func TestListCustomRoles(t *testing.T) {
	t.Parallel()

	t.Run("Organizations", func(t *testing.T) {
		t.Parallel()

		client, db := coderdtest.NewWithDatabase(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)

		const roleName = "random_role"
		dbgen.CustomRole(t, db, must(rolestore.ConvertRoleToDB(rbac.Role{
			Name:        rbac.RoleName(roleName, owner.OrganizationID.String()),
			DisplayName: "Random Role",
			Site:        nil,
			Org: map[string][]rbac.Permission{
				owner.OrganizationID.String(): {
					{
						Negate:       false,
						ResourceType: rbac.ResourceWorkspace.Type,
						Action:       policy.ActionRead,
					},
				},
			},
			User: nil,
		})))

		ctx := testutil.Context(t, testutil.WaitShort)
		roles, err := client.ListOrganizationRoles(ctx, owner.OrganizationID)
		require.NoError(t, err)

		found := slices.ContainsFunc(roles, func(element codersdk.AssignableRoles) bool {
			return element.Name == roleName && element.OrganizationID == owner.OrganizationID.String()
		})
		require.Truef(t, found, "custom organization role listed")
	})
}

func convertRole(roleName string) codersdk.Role {
	role, _ := rbac.RoleByName(roleName)
	return db2sdk.Role(role)
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

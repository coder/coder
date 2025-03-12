package coderd_test

import (
	"bytes"
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/prebuilds"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

func TestCustomOrganizationRole(t *testing.T) {
	t.Parallel()
	templateAdminCustom := func(orgID uuid.UUID) codersdk.Role {
		return codersdk.Role{
			Name:           "test-role",
			DisplayName:    "Testing Purposes",
			OrganizationID: orgID.String(),
			// Basically creating a template admin manually
			SitePermissions: nil,
			OrganizationPermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceTemplate:  {codersdk.ActionCreate, codersdk.ActionRead, codersdk.ActionUpdate, codersdk.ActionViewInsights},
				codersdk.ResourceFile:      {codersdk.ActionCreate, codersdk.ActionRead},
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}),
			UserPermissions: nil,
		}
	}

	// Create, assign, and use a custom role
	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)

		//nolint:gocritic // owner is required for this
		role, err := owner.CreateOrganizationRole(ctx, templateAdminCustom(first.OrganizationID))
		require.NoError(t, err, "upsert role")

		// Assign the custom template admin role
		tmplAdmin, _ := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.RoleIdentifier{Name: role.Name, OrganizationID: first.OrganizationID})

		// Assert the role exists
		// TODO: At present user roles are not returned by the user endpoints.
		// 	Changing this might mess up the UI in how it renders the roles on the
		//	users page. When the users endpoint is updated, this should be uncommented.
		// roleNamesF := func(role codersdk.SlimRole) string { return role.Name }
		// require.Contains(t, db2sdk.List(user.Roles, roleNamesF), role.Name)

		// Try to create a template version
		coderdtest.CreateTemplateVersion(t, tmplAdmin, first.OrganizationID, nil)

		// Verify the role exists in the list
		allRoles, err := tmplAdmin.ListOrganizationRoles(ctx, first.OrganizationID)
		require.NoError(t, err)

		var foundRole codersdk.AssignableRoles
		require.True(t, slices.ContainsFunc(allRoles, func(selected codersdk.AssignableRoles) bool {
			if selected.Name == role.Name {
				foundRole = selected
				return true
			}
			return false
		}), "role missing from org role list")

		require.Len(t, foundRole.SitePermissions, 0)
		require.Len(t, foundRole.OrganizationPermissions, 7)
		require.Len(t, foundRole.UserPermissions, 0)
	})

	// Revoked licenses cannot modify/create custom roles, but they can
	// use the existing roles.
	t.Run("RevokedLicense", func(t *testing.T) {
		t.Parallel()
		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)

		//nolint:gocritic // owner is required for this
		role, err := owner.CreateOrganizationRole(ctx, templateAdminCustom(first.OrganizationID))
		require.NoError(t, err, "upsert role")

		// Remove the license to block premium functionality
		licenses, err := owner.Licenses(ctx)
		require.NoError(t, err, "get licenses")
		for _, license := range licenses {
			// Should be only 1...
			err := owner.DeleteLicense(ctx, license.ID)
			require.NoError(t, err, "delete license")
		}

		// Verify functionality is lost
		_, err = owner.UpdateOrganizationRole(ctx, templateAdminCustom(first.OrganizationID))
		require.ErrorContains(t, err, "Custom Roles is a Premium feature")

		// Assign the custom template admin role
		tmplAdmin, _ := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.RoleIdentifier{Name: role.Name, OrganizationID: first.OrganizationID})

		// Try to create a template version, eg using the custom role
		coderdtest.CreateTemplateVersion(t, tmplAdmin, first.OrganizationID, nil)
	})

	// Role patches are complete, as in the request overrides the existing role.
	t.Run("RoleOverrides", func(t *testing.T) {
		t.Parallel()
		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)
		//nolint:gocritic // owner is required for this
		role, err := owner.CreateOrganizationRole(ctx, templateAdminCustom(first.OrganizationID))
		require.NoError(t, err, "upsert role")

		// Assign the custom template admin role
		tmplAdmin, _ := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.RoleIdentifier{Name: role.Name, OrganizationID: first.OrganizationID})

		// Try to create a template version, eg using the custom role
		coderdtest.CreateTemplateVersion(t, tmplAdmin, first.OrganizationID, nil)

		//nolint:gocritic // owner is required for this
		newRole := templateAdminCustom(first.OrganizationID)
		// These are all left nil, which sets the custom role to have 0
		// permissions. Omitting this does not "inherit" what already
		// exists.
		newRole.SitePermissions = nil
		newRole.OrganizationPermissions = nil
		newRole.UserPermissions = nil
		_, err = owner.UpdateOrganizationRole(ctx, newRole)
		require.NoError(t, err, "upsert role with override")

		// The role should no longer have template perms
		data, err := echo.TarWithOptions(ctx, tmplAdmin.Logger(), nil)
		require.NoError(t, err)
		file, err := tmplAdmin.Upload(ctx, codersdk.ContentTypeTar, bytes.NewReader(data))
		require.NoError(t, err)
		_, err = tmplAdmin.CreateTemplateVersion(ctx, first.OrganizationID, codersdk.CreateTemplateVersionRequest{
			FileID:        file.ID,
			StorageMethod: codersdk.ProvisionerStorageMethodFile,
			Provisioner:   codersdk.ProvisionerTypeEcho,
		})
		require.ErrorContains(t, err, "forbidden")
	})

	t.Run("InvalidName", func(t *testing.T) {
		t.Parallel()
		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)

		//nolint:gocritic // owner is required for this
		_, err := owner.CreateOrganizationRole(ctx, codersdk.Role{
			Name:                    "Bad_Name", // No underscores allowed
			DisplayName:             "Testing Purposes",
			OrganizationID:          first.OrganizationID.String(),
			SitePermissions:         nil,
			OrganizationPermissions: nil,
			UserPermissions:         nil,
		})
		require.ErrorContains(t, err, "Validation")
	})

	t.Run("ReservedName", func(t *testing.T) {
		t.Parallel()
		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)

		//nolint:gocritic // owner is required for this
		_, err := owner.CreateOrganizationRole(ctx, codersdk.Role{
			Name:                    "owner", // Reserved
			DisplayName:             "Testing Purposes",
			OrganizationID:          first.OrganizationID.String(),
			SitePermissions:         nil,
			OrganizationPermissions: nil,
			UserPermissions:         nil,
		})
		require.ErrorContains(t, err, "Reserved")
	})

	// Attempt to add site & user permissions, which is not allowed
	t.Run("ExcessPermissions", func(t *testing.T) {
		t.Parallel()
		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)

		siteRole := templateAdminCustom(first.OrganizationID)
		siteRole.SitePermissions = []codersdk.Permission{
			{
				ResourceType: codersdk.ResourceWorkspace,
				Action:       codersdk.ActionRead,
			},
		}

		//nolint:gocritic // owner is required for this
		_, err := owner.CreateOrganizationRole(ctx, siteRole)
		require.ErrorContains(t, err, "site wide permissions")

		userRole := templateAdminCustom(first.OrganizationID)
		userRole.UserPermissions = []codersdk.Permission{
			{
				ResourceType: codersdk.ResourceWorkspace,
				Action:       codersdk.ActionRead,
			},
		}

		//nolint:gocritic // owner is required for this
		_, err = owner.UpdateOrganizationRole(ctx, userRole)
		require.ErrorContains(t, err, "not allowed to assign user permissions")
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)

		newRole := templateAdminCustom(first.OrganizationID)
		newRole.OrganizationID = "0000" // This is not a valid uuid

		//nolint:gocritic // owner is required for this
		_, err := owner.CreateOrganizationRole(ctx, newRole)
		require.ErrorContains(t, err, "Resource not found")
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()
		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})

		orgAdmin, orgAdminUser := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.ScopedRoleOrgAdmin(first.OrganizationID))
		ctx := testutil.Context(t, testutil.WaitMedium)

		createdRole, err := orgAdmin.CreateOrganizationRole(ctx, templateAdminCustom(first.OrganizationID))
		require.NoError(t, err, "upsert role")

		//nolint:gocritic // org_admin cannot assign to themselves
		_, err = owner.UpdateOrganizationMemberRoles(ctx, first.OrganizationID, orgAdminUser.ID.String(), codersdk.UpdateRoles{
			// Give the user this custom role, to ensure when it is deleted, the user
			// is ok to be used.
			Roles: []string{createdRole.Name, rbac.ScopedRoleOrgAdmin(first.OrganizationID).Name},
		})
		require.NoError(t, err, "assign custom role to user")

		existingRoles, err := orgAdmin.ListOrganizationRoles(ctx, first.OrganizationID)
		require.NoError(t, err)

		exists := slices.ContainsFunc(existingRoles, func(role codersdk.AssignableRoles) bool {
			return role.Name == createdRole.Name
		})
		require.True(t, exists, "custom role should exist")

		// Delete the role
		err = orgAdmin.DeleteOrganizationRole(ctx, first.OrganizationID, createdRole.Name)
		require.NoError(t, err)

		existingRoles, err = orgAdmin.ListOrganizationRoles(ctx, first.OrganizationID)
		require.NoError(t, err)

		exists = slices.ContainsFunc(existingRoles, func(role codersdk.AssignableRoles) bool {
			return role.Name == createdRole.Name
		})
		require.False(t, exists, "custom role should be deleted")

		// Verify you can still assign roles.
		// There used to be a bug that if a member had a delete role, they
		// could not be assigned roles anymore.
		//nolint:gocritic // org_admin cannot assign to themselves
		_, err = owner.UpdateOrganizationMemberRoles(ctx, first.OrganizationID, orgAdminUser.ID.String(), codersdk.UpdateRoles{
			Roles: []string{rbac.ScopedRoleOrgAdmin(first.OrganizationID).Name},
		})
		require.NoError(t, err)
	})

	// Verify deleting a custom role cascades to all members
	t.Run("DeleteRoleCascadeMembers", func(t *testing.T) {
		t.Parallel()

		// TODO: we should not be returning the prebuilds user in OrganizationMembers, and this is not returned in dbmem.
		if !dbtestutil.WillUsePostgres() {
			t.Skip("This test requires postgres")
		}

		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})

		orgAdmin, orgAdminUser := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.ScopedRoleOrgAdmin(first.OrganizationID))
		ctx := testutil.Context(t, testutil.WaitMedium)

		createdRole, err := orgAdmin.CreateOrganizationRole(ctx, templateAdminCustom(first.OrganizationID))
		require.NoError(t, err, "upsert role")

		customRoleIdentifier := rbac.RoleIdentifier{
			Name:           createdRole.Name,
			OrganizationID: first.OrganizationID,
		}

		// Create a few members with the role
		coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, customRoleIdentifier)
		coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.ScopedRoleOrgAdmin(first.OrganizationID), customRoleIdentifier)
		coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.ScopedRoleOrgTemplateAdmin(first.OrganizationID), rbac.ScopedRoleOrgAuditor(first.OrganizationID), customRoleIdentifier)

		// Verify members have the custom role
		originalMembers, err := orgAdmin.OrganizationMembers(ctx, first.OrganizationID)
		require.NoError(t, err)
		require.Len(t, originalMembers, 6) // 3 members + org admin + owner + prebuilds user
		for _, member := range originalMembers {
			if member.UserID == orgAdminUser.ID || member.UserID == first.UserID || member.UserID == prebuilds.OwnerID {
				continue
			}

			require.True(t, slices.ContainsFunc(member.Roles, func(role codersdk.SlimRole) bool {
				return role.Name == customRoleIdentifier.Name
			}), "member should have custom role")
		}

		err = orgAdmin.DeleteOrganizationRole(ctx, first.OrganizationID, createdRole.Name)
		require.NoError(t, err)

		// Verify the role was removed from all members
		members, err := orgAdmin.OrganizationMembers(ctx, first.OrganizationID)
		require.NoError(t, err)
		require.Len(t, members, 6) // 3 members + org admin + owner + prebuilds user
		for _, member := range members {
			require.False(t, slices.ContainsFunc(member.Roles, func(role codersdk.SlimRole) bool {
				return role.Name == customRoleIdentifier.Name
			}), "role should be removed from all users")

			// Verify the rest of the member's roles are unchanged
			original := originalMembers[slices.IndexFunc(originalMembers, func(haystack codersdk.OrganizationMemberWithUserData) bool {
				return haystack.UserID == member.UserID
			})]
			originalWithoutCustom := slices.DeleteFunc(original.Roles, func(role codersdk.SlimRole) bool {
				return role.Name == customRoleIdentifier.Name
			})
			require.ElementsMatch(t, originalWithoutCustom, member.Roles, "original roles are unchanged")
		}
	})
}

func TestListRoles(t *testing.T) {
	t.Parallel()

	client, owner := coderdenttest.New(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureExternalProvisionerDaemons: 1,
				codersdk.FeatureMultipleOrganizations:      1,
			},
		},
	})

	// Create owner, member, and org admin
	member, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
	orgAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.ScopedRoleOrgAdmin(owner.OrganizationID))

	otherOrg := coderdenttest.CreateOrganization(t, client, coderdenttest.CreateOrganizationOptions{})

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
			ExpectedRoles: convertRoles(map[rbac.RoleIdentifier]bool{
				{Name: codersdk.RoleOwner}:         false,
				{Name: codersdk.RoleAuditor}:       false,
				{Name: codersdk.RoleTemplateAdmin}: false,
				{Name: codersdk.RoleUserAdmin}:     false,
			}),
		},
		{
			Name: "OrgMemberListOrg",
			APICall: func(ctx context.Context) ([]codersdk.AssignableRoles, error) {
				return member.ListOrganizationRoles(ctx, owner.OrganizationID)
			},
			ExpectedRoles: convertRoles(map[rbac.RoleIdentifier]bool{
				{Name: codersdk.RoleOrganizationAdmin, OrganizationID: owner.OrganizationID}:                false,
				{Name: codersdk.RoleOrganizationAuditor, OrganizationID: owner.OrganizationID}:              false,
				{Name: codersdk.RoleOrganizationTemplateAdmin, OrganizationID: owner.OrganizationID}:        false,
				{Name: codersdk.RoleOrganizationUserAdmin, OrganizationID: owner.OrganizationID}:            false,
				{Name: codersdk.RoleOrganizationWorkspaceCreationBan, OrganizationID: owner.OrganizationID}: false,
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
			ExpectedRoles: convertRoles(map[rbac.RoleIdentifier]bool{
				{Name: codersdk.RoleOwner}:         false,
				{Name: codersdk.RoleAuditor}:       false,
				{Name: codersdk.RoleTemplateAdmin}: false,
				{Name: codersdk.RoleUserAdmin}:     false,
			}),
		},
		{
			Name: "OrgAdminListOrg",
			APICall: func(ctx context.Context) ([]codersdk.AssignableRoles, error) {
				return orgAdmin.ListOrganizationRoles(ctx, owner.OrganizationID)
			},
			ExpectedRoles: convertRoles(map[rbac.RoleIdentifier]bool{
				{Name: codersdk.RoleOrganizationAdmin, OrganizationID: owner.OrganizationID}:                true,
				{Name: codersdk.RoleOrganizationAuditor, OrganizationID: owner.OrganizationID}:              true,
				{Name: codersdk.RoleOrganizationTemplateAdmin, OrganizationID: owner.OrganizationID}:        true,
				{Name: codersdk.RoleOrganizationUserAdmin, OrganizationID: owner.OrganizationID}:            true,
				{Name: codersdk.RoleOrganizationWorkspaceCreationBan, OrganizationID: owner.OrganizationID}: true,
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
			ExpectedRoles: convertRoles(map[rbac.RoleIdentifier]bool{
				{Name: codersdk.RoleOwner}:         true,
				{Name: codersdk.RoleAuditor}:       true,
				{Name: codersdk.RoleTemplateAdmin}: true,
				{Name: codersdk.RoleUserAdmin}:     true,
			}),
		},
		{
			Name: "AdminListOrg",
			APICall: func(ctx context.Context) ([]codersdk.AssignableRoles, error) {
				return client.ListOrganizationRoles(ctx, owner.OrganizationID)
			},
			ExpectedRoles: convertRoles(map[rbac.RoleIdentifier]bool{
				{Name: codersdk.RoleOrganizationAdmin, OrganizationID: owner.OrganizationID}:                true,
				{Name: codersdk.RoleOrganizationAuditor, OrganizationID: owner.OrganizationID}:              true,
				{Name: codersdk.RoleOrganizationTemplateAdmin, OrganizationID: owner.OrganizationID}:        true,
				{Name: codersdk.RoleOrganizationUserAdmin, OrganizationID: owner.OrganizationID}:            true,
				{Name: codersdk.RoleOrganizationWorkspaceCreationBan, OrganizationID: owner.OrganizationID}: true,
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

func convertRole(roleName rbac.RoleIdentifier) codersdk.Role {
	role, _ := rbac.RoleByName(roleName)
	return db2sdk.RBACRole(role)
}

func convertRoles(assignableRoles map[rbac.RoleIdentifier]bool) []codersdk.AssignableRoles {
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

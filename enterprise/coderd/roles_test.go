package coderd_test

import (
	"bytes"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
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
		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentCustomRoles)}
		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)

		//nolint:gocritic // owner is required for this
		role, err := owner.PatchOrganizationRole(ctx, first.OrganizationID, templateAdminCustom(first.OrganizationID))
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
		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentCustomRoles)}
		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)

		//nolint:gocritic // owner is required for this
		role, err := owner.PatchOrganizationRole(ctx, first.OrganizationID, templateAdminCustom(first.OrganizationID))
		require.NoError(t, err, "upsert role")

		// Remove the license to block enterprise functionality
		licenses, err := owner.Licenses(ctx)
		require.NoError(t, err, "get licenses")
		for _, license := range licenses {
			// Should be only 1...
			err := owner.DeleteLicense(ctx, license.ID)
			require.NoError(t, err, "delete license")
		}

		// Verify functionality is lost
		_, err = owner.PatchOrganizationRole(ctx, first.OrganizationID, templateAdminCustom(first.OrganizationID))
		require.ErrorContains(t, err, "roles are not enabled")

		// Assign the custom template admin role
		tmplAdmin, _ := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, rbac.RoleIdentifier{Name: role.Name, OrganizationID: first.OrganizationID})

		// Try to create a template version, eg using the custom role
		coderdtest.CreateTemplateVersion(t, tmplAdmin, first.OrganizationID, nil)
	})

	// Role patches are complete, as in the request overrides the existing role.
	t.Run("RoleOverrides", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentCustomRoles)}
		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)
		//nolint:gocritic // owner is required for this
		role, err := owner.PatchOrganizationRole(ctx, first.OrganizationID, templateAdminCustom(first.OrganizationID))
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
		_, err = owner.PatchOrganizationRole(ctx, first.OrganizationID, newRole)
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
		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentCustomRoles)}
		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)

		//nolint:gocritic // owner is required for this
		_, err := owner.PatchOrganizationRole(ctx, first.OrganizationID, codersdk.Role{
			Name:                    "Bad_Name", // No underscores allowed
			DisplayName:             "Testing Purposes",
			SitePermissions:         nil,
			OrganizationPermissions: nil,
			UserPermissions:         nil,
		})
		require.ErrorContains(t, err, "Validation")
	})

	t.Run("MismatchedOrganizations", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentCustomRoles)}
		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitMedium)

		//nolint:gocritic // owner is required for this
		_, err := owner.PatchOrganizationRole(ctx, first.OrganizationID, templateAdminCustom(uuid.New()))
		require.ErrorContains(t, err, "does not match")
	})

	// Attempt to add site & user permissions, which is not allowed
	t.Run("ExcessPermissions", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentCustomRoles)}
		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
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
		_, err := owner.PatchOrganizationRole(ctx, first.OrganizationID, siteRole)
		require.ErrorContains(t, err, "site wide permissions")

		userRole := templateAdminCustom(first.OrganizationID)
		userRole.UserPermissions = []codersdk.Permission{
			{
				ResourceType: codersdk.ResourceWorkspace,
				Action:       codersdk.ActionRead,
			},
		}

		//nolint:gocritic // owner is required for this
		_, err = owner.PatchOrganizationRole(ctx, first.OrganizationID, userRole)
		require.ErrorContains(t, err, "not allowed to assign user permissions")
	})

	t.Run("InvalidUUID", func(t *testing.T) {
		t.Parallel()
		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentCustomRoles)}
		owner, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
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
		_, err := owner.PatchOrganizationRole(ctx, first.OrganizationID, newRole)
		require.ErrorContains(t, err, "Invalid request")
	})
}

package coderd_test

import (
	"bytes"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/testutil"
)

func TestCustomRole(t *testing.T) {
	t.Parallel()
	templateAdminCustom := codersdk.Role{
		Name:        "test-role",
		DisplayName: "Testing Purposes",
		// Basically creating a template admin manually
		SitePermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
			codersdk.ResourceTemplate:  {codersdk.ActionCreate, codersdk.ActionRead, codersdk.ActionUpdate, codersdk.ActionViewInsights},
			codersdk.ResourceFile:      {codersdk.ActionCreate, codersdk.ActionRead},
			codersdk.ResourceWorkspace: {codersdk.ActionRead},
		}),
		OrganizationPermissions: nil,
		UserPermissions:         nil,
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
		role, err := owner.PatchRole(ctx, templateAdminCustom)
		require.NoError(t, err, "upsert role")

		// Assign the custom template admin role
		tmplAdmin, user := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, role.Name)

		// Assert the role exists
		roleNamesF := func(role codersdk.SlimRole) string { return role.Name }
		require.Contains(t, db2sdk.List(user.Roles, roleNamesF), role.Name)

		// Try to create a template version
		coderdtest.CreateTemplateVersion(t, tmplAdmin, first.OrganizationID, nil)

		// Verify the role exists in the list
		allRoles, err := tmplAdmin.ListSiteRoles(ctx)
		require.NoError(t, err)

		require.True(t, slices.ContainsFunc(allRoles, func(selected codersdk.AssignableRoles) bool {
			return selected.Name == role.Name
		}), "role missing from site role list")
	})

	// Revoked licenses cannot modify/create custom roles, but they can
	// use the existing roles.
	t.Run("Revoked License", func(t *testing.T) {
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
		role, err := owner.PatchRole(ctx, templateAdminCustom)
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
		_, err = owner.PatchRole(ctx, templateAdminCustom)
		require.ErrorContains(t, err, "Custom roles is an Enterprise feature", "upsert role")

		// Assign the custom template admin role
		tmplAdmin, _ := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, role.Name)

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
		role, err := owner.PatchRole(ctx, templateAdminCustom)
		require.NoError(t, err, "upsert role")

		// Assign the custom template admin role
		tmplAdmin, _ := coderdtest.CreateAnotherUser(t, owner, first.OrganizationID, role.Name)

		// Try to create a template version, eg using the custom role
		coderdtest.CreateTemplateVersion(t, tmplAdmin, first.OrganizationID, nil)

		//nolint:gocritic // owner is required for this
		role, err = owner.PatchRole(ctx, codersdk.Role{
			Name:        templateAdminCustom.Name,
			DisplayName: templateAdminCustom.DisplayName,
			// These are all left nil, which sets the custom role to have 0
			// permissions. Omitting this does not "inherit" what already
			// exists.
			SitePermissions:         nil,
			OrganizationPermissions: nil,
			UserPermissions:         nil,
		})
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
		owner, _ := coderdenttest.New(t, &coderdenttest.Options{
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
		_, err := owner.PatchRole(ctx, codersdk.Role{
			Name:        "Bad_Name", // No underscores allowed
			DisplayName: "Testing Purposes",
			// Basically creating a template admin manually
			SitePermissions: codersdk.CreatePermissions(map[codersdk.RBACResource][]codersdk.RBACAction{
				codersdk.ResourceTemplate:  {codersdk.ActionCreate, codersdk.ActionRead, codersdk.ActionUpdate, codersdk.ActionViewInsights},
				codersdk.ResourceFile:      {codersdk.ActionCreate, codersdk.ActionRead},
				codersdk.ResourceWorkspace: {codersdk.ActionRead},
			}),
			OrganizationPermissions: nil,
			UserPermissions:         nil,
		})
		require.ErrorContains(t, err, "Validation")
	})
}

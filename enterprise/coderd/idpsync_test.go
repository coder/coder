package coderd_test

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/idpsync"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/runtimeconfig"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestGetGroupSyncConfig(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		owner, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles:           1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		orgAdmin, _ := coderdtest.CreateAnotherUser(t, owner, user.OrganizationID, rbac.ScopedRoleOrgAdmin(user.OrganizationID))

		ctx := testutil.Context(t, testutil.WaitShort)
		dbresv := runtimeconfig.OrganizationResolver(user.OrganizationID, runtimeconfig.NewStoreResolver(db))
		entry := runtimeconfig.MustNew[*idpsync.GroupSyncSettings]("group-sync-settings")
		//nolint:gocritic // Requires system context to set runtime config
		err := entry.SetRuntimeValue(dbauthz.AsSystemRestricted(ctx), dbresv, &idpsync.GroupSyncSettings{Field: "august"})
		require.NoError(t, err)

		settings, err := orgAdmin.GroupIDPSyncSettings(ctx, user.OrganizationID.String())
		require.NoError(t, err)
		require.Equal(t, "august", settings.Field)
	})

	t.Run("Legacy", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.OIDC.GroupField = "legacy-group"
		dv.OIDC.GroupRegexFilter = serpent.Regexp(*regexp.MustCompile("legacy-filter"))
		dv.OIDC.GroupMapping = serpent.Struct[map[string]string]{
			Value: map[string]string{
				"foo": "bar",
			},
		}

		owner, user := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				DeploymentValues: dv,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles:           1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		orgAdmin, _ := coderdtest.CreateAnotherUser(t, owner, user.OrganizationID, rbac.ScopedRoleOrgAdmin(user.OrganizationID))

		ctx := testutil.Context(t, testutil.WaitShort)

		settings, err := orgAdmin.GroupIDPSyncSettings(ctx, user.OrganizationID.String())
		require.NoError(t, err)
		require.Equal(t, dv.OIDC.GroupField.Value(), settings.Field)
		require.Equal(t, dv.OIDC.GroupRegexFilter.String(), settings.RegexFilter.String())
		require.Equal(t, dv.OIDC.GroupMapping.Value, settings.LegacyNameMapping)
	})
}

func TestPostGroupSyncConfig(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		owner, user := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles:           1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		orgAdmin, _ := coderdtest.CreateAnotherUser(t, owner, user.OrganizationID, rbac.ScopedRoleOrgAdmin(user.OrganizationID))

		// Test as org admin
		ctx := testutil.Context(t, testutil.WaitShort)
		settings, err := orgAdmin.PatchGroupIDPSyncSettings(ctx, user.OrganizationID.String(), codersdk.GroupSyncSettings{
			Field: "august",
		})
		require.NoError(t, err)
		require.Equal(t, "august", settings.Field)

		fetchedSettings, err := orgAdmin.GroupIDPSyncSettings(ctx, user.OrganizationID.String())
		require.NoError(t, err)
		require.Equal(t, "august", fetchedSettings.Field)
	})

	t.Run("NotAuthorized", func(t *testing.T) {
		t.Parallel()

		owner, user := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles:           1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		member, _ := coderdtest.CreateAnotherUser(t, owner, user.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := member.PatchGroupIDPSyncSettings(ctx, user.OrganizationID.String(), codersdk.GroupSyncSettings{
			Field: "august",
		})
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())

		_, err = member.GroupIDPSyncSettings(ctx, user.OrganizationID.String())
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())
	})
}

func TestGetRoleSyncConfig(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		owner, _, _, user := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles:           1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})
		orgAdmin, _ := coderdtest.CreateAnotherUser(t, owner, user.OrganizationID, rbac.ScopedRoleOrgAdmin(user.OrganizationID))

		ctx := testutil.Context(t, testutil.WaitShort)
		settings, err := orgAdmin.PatchRoleIDPSyncSettings(ctx, user.OrganizationID.String(), codersdk.RoleSyncSettings{
			Field: "august",
			Mapping: map[string][]string{
				"foo": {"bar"},
			},
		})
		require.NoError(t, err)
		require.Equal(t, "august", settings.Field)
		require.Equal(t, map[string][]string{"foo": {"bar"}}, settings.Mapping)

		settings, err = orgAdmin.RoleIDPSyncSettings(ctx, user.OrganizationID.String())
		require.NoError(t, err)
		require.Equal(t, "august", settings.Field)
		require.Equal(t, map[string][]string{"foo": {"bar"}}, settings.Mapping)
	})
}

func TestPostRoleSyncConfig(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		owner, user := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles:           1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		orgAdmin, _ := coderdtest.CreateAnotherUser(t, owner, user.OrganizationID, rbac.ScopedRoleOrgAdmin(user.OrganizationID))

		// Test as org admin
		ctx := testutil.Context(t, testutil.WaitShort)
		settings, err := orgAdmin.PatchRoleIDPSyncSettings(ctx, user.OrganizationID.String(), codersdk.RoleSyncSettings{
			Field: "august",
		})
		require.NoError(t, err)
		require.Equal(t, "august", settings.Field)

		fetchedSettings, err := orgAdmin.RoleIDPSyncSettings(ctx, user.OrganizationID.String())
		require.NoError(t, err)
		require.Equal(t, "august", fetchedSettings.Field)
	})

	t.Run("NotAuthorized", func(t *testing.T) {
		t.Parallel()

		owner, user := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles:           1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		member, _ := coderdtest.CreateAnotherUser(t, owner, user.OrganizationID)

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := member.PatchRoleIDPSyncSettings(ctx, user.OrganizationID.String(), codersdk.RoleSyncSettings{
			Field: "august",
		})
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())

		_, err = member.RoleIDPSyncSettings(ctx, user.OrganizationID.String())
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())
	})
}

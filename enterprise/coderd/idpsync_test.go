package coderd_test

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/google/uuid"
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

func TestGetGroupSyncSettings(t *testing.T) {
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

func TestPatchGroupSyncSettings(t *testing.T) {
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

func TestPatchGroupSyncConfig(t *testing.T) {
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

		orgID := user.OrganizationID
		orgAdmin, _ := coderdtest.CreateAnotherUser(t, owner, orgID, rbac.ScopedRoleOrgAdmin(user.OrganizationID))

		mapping := map[string][]uuid.UUID{"wibble": {uuid.New()}}

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := orgAdmin.PatchGroupIDPSyncSettings(ctx, orgID.String(), codersdk.GroupSyncSettings{
			Field:             "wibble",
			RegexFilter:       regexp.MustCompile("wib{2,}le"),
			AutoCreateMissing: false,
			Mapping:           mapping,
		})

		require.NoError(t, err)

		fetchedSettings, err := orgAdmin.GroupIDPSyncSettings(ctx, orgID.String())
		require.NoError(t, err)
		require.Equal(t, "wibble", fetchedSettings.Field)
		require.Equal(t, "wib{2,}le", fetchedSettings.RegexFilter.String())
		require.Equal(t, false, fetchedSettings.AutoCreateMissing)
		require.Equal(t, mapping, fetchedSettings.Mapping)

		ctx = testutil.Context(t, testutil.WaitShort)
		settings, err := orgAdmin.PatchGroupIDPSyncConfig(ctx, orgID.String(), codersdk.PatchGroupIDPSyncConfigRequest{
			Field:             "wobble",
			RegexFilter:       regexp.MustCompile("wob{2,}le"),
			AutoCreateMissing: true,
		})

		require.NoError(t, err)
		require.Equal(t, "wobble", settings.Field)
		require.Equal(t, "wob{2,}le", settings.RegexFilter.String())
		require.Equal(t, true, settings.AutoCreateMissing)
		require.Equal(t, mapping, settings.Mapping)

		fetchedSettings, err = orgAdmin.GroupIDPSyncSettings(ctx, orgID.String())
		require.NoError(t, err)
		require.Equal(t, "wobble", fetchedSettings.Field)
		require.Equal(t, "wob{2,}le", fetchedSettings.RegexFilter.String())
		require.Equal(t, true, fetchedSettings.AutoCreateMissing)
		require.Equal(t, mapping, fetchedSettings.Mapping)
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
		_, err := member.PatchGroupIDPSyncConfig(ctx, user.OrganizationID.String(), codersdk.PatchGroupIDPSyncConfigRequest{})
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())
	})
}

func TestPatchGroupSyncMapping(t *testing.T) {
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

		orgID := user.OrganizationID
		orgAdmin, _ := coderdtest.CreateAnotherUser(t, owner, orgID, rbac.ScopedRoleOrgAdmin(user.OrganizationID))
		// These IDs are easier to visually diff if the test fails than truly random
		// ones.
		orgs := []uuid.UUID{
			uuid.MustParse("00000000-b8bd-46bb-bb6c-6c2b2c0dd2ea"),
			uuid.MustParse("01000000-fbe8-464c-9429-fe01a03f3644"),
			uuid.MustParse("02000000-0926-407b-9998-39af62e3d0c5"),
		}

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := orgAdmin.PatchGroupIDPSyncSettings(ctx, orgID.String(), codersdk.GroupSyncSettings{
			Field:             "wibble",
			RegexFilter:       regexp.MustCompile("wib{2,}le"),
			AutoCreateMissing: true,
			Mapping:           map[string][]uuid.UUID{"wobble": {orgs[0]}},
		})
		require.NoError(t, err)

		ctx = testutil.Context(t, testutil.WaitShort)
		settings, err := orgAdmin.PatchGroupIDPSyncMapping(ctx, orgID.String(), codersdk.PatchGroupIDPSyncMappingRequest{
			Add: []codersdk.IDPSyncMapping[uuid.UUID]{
				{Given: "wibble", Gets: orgs[0]},
				{Given: "wobble", Gets: orgs[1]},
				{Given: "wobble", Gets: orgs[2]},
			},
			// Remove takes priority over Add, so "3" should not actually be added to wooble.
			Remove: []codersdk.IDPSyncMapping[uuid.UUID]{
				{Given: "wobble", Gets: orgs[1]},
			},
		})

		expected := map[string][]uuid.UUID{
			"wibble": {orgs[0]},
			"wobble": {orgs[0], orgs[2]},
		}

		require.NoError(t, err)
		require.Equal(t, expected, settings.Mapping)

		fetchedSettings, err := orgAdmin.GroupIDPSyncSettings(ctx, orgID.String())
		require.NoError(t, err)
		require.Equal(t, "wibble", fetchedSettings.Field)
		require.Equal(t, "wib{2,}le", fetchedSettings.RegexFilter.String())
		require.Equal(t, true, fetchedSettings.AutoCreateMissing)
		require.Equal(t, expected, fetchedSettings.Mapping)
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
		_, err := member.PatchGroupIDPSyncMapping(ctx, user.OrganizationID.String(), codersdk.PatchGroupIDPSyncMappingRequest{})
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())
	})
}

func TestGetRoleSyncSettings(t *testing.T) {
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

func TestPatchRoleSyncSettings(t *testing.T) {
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

func TestPatchRoleSyncConfig(t *testing.T) {
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

		orgID := user.OrganizationID
		orgAdmin, _ := coderdtest.CreateAnotherUser(t, owner, orgID, rbac.ScopedRoleOrgAdmin(user.OrganizationID))

		mapping := map[string][]string{"wibble": {"group-01"}}

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := orgAdmin.PatchRoleIDPSyncSettings(ctx, orgID.String(), codersdk.RoleSyncSettings{
			Field:   "wibble",
			Mapping: mapping,
		})

		require.NoError(t, err)

		fetchedSettings, err := orgAdmin.RoleIDPSyncSettings(ctx, orgID.String())
		require.NoError(t, err)
		require.Equal(t, "wibble", fetchedSettings.Field)
		require.Equal(t, mapping, fetchedSettings.Mapping)

		ctx = testutil.Context(t, testutil.WaitShort)
		settings, err := orgAdmin.PatchRoleIDPSyncConfig(ctx, orgID.String(), codersdk.PatchRoleIDPSyncConfigRequest{
			Field: "wobble",
		})

		require.NoError(t, err)
		require.Equal(t, "wobble", settings.Field)
		require.Equal(t, mapping, settings.Mapping)

		fetchedSettings, err = orgAdmin.RoleIDPSyncSettings(ctx, orgID.String())
		require.NoError(t, err)
		require.Equal(t, "wobble", fetchedSettings.Field)
		require.Equal(t, mapping, fetchedSettings.Mapping)
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
		_, err := member.PatchGroupIDPSyncConfig(ctx, user.OrganizationID.String(), codersdk.PatchGroupIDPSyncConfigRequest{})
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())
	})
}

func TestPatchRoleSyncMapping(t *testing.T) {
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

		orgID := user.OrganizationID
		orgAdmin, _ := coderdtest.CreateAnotherUser(t, owner, orgID, rbac.ScopedRoleOrgAdmin(user.OrganizationID))

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := orgAdmin.PatchRoleIDPSyncSettings(ctx, orgID.String(), codersdk.RoleSyncSettings{
			Field:   "wibble",
			Mapping: map[string][]string{"wobble": {"group-00"}},
		})
		require.NoError(t, err)

		ctx = testutil.Context(t, testutil.WaitShort)
		settings, err := orgAdmin.PatchRoleIDPSyncMapping(ctx, orgID.String(), codersdk.PatchRoleIDPSyncMappingRequest{
			Add: []codersdk.IDPSyncMapping[string]{
				{Given: "wibble", Gets: "group-00"},
				{Given: "wobble", Gets: "group-01"},
				{Given: "wobble", Gets: "group-02"},
			},
			// Remove takes priority over Add, so "3" should not actually be added to wooble.
			Remove: []codersdk.IDPSyncMapping[string]{
				{Given: "wobble", Gets: "group-01"},
			},
		})

		expected := map[string][]string{
			"wibble": {"group-00"},
			"wobble": {"group-00", "group-02"},
		}

		require.NoError(t, err)
		require.Equal(t, expected, settings.Mapping)

		fetchedSettings, err := orgAdmin.RoleIDPSyncSettings(ctx, orgID.String())
		require.NoError(t, err)
		require.Equal(t, "wibble", fetchedSettings.Field)
		require.Equal(t, expected, fetchedSettings.Mapping)
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
		_, err := member.PatchGroupIDPSyncMapping(ctx, user.OrganizationID.String(), codersdk.PatchGroupIDPSyncMappingRequest{})
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())
	})
}

func TestGetOrganizationSyncSettings(t *testing.T) {
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

		expected := map[string][]uuid.UUID{"foo": {user.OrganizationID}}

		ctx := testutil.Context(t, testutil.WaitShort)
		settings, err := owner.PatchOrganizationIDPSyncSettings(ctx, codersdk.OrganizationSyncSettings{
			Field:   "august",
			Mapping: expected,
		})

		require.NoError(t, err)
		require.Equal(t, "august", settings.Field)
		require.Equal(t, expected, settings.Mapping)

		settings, err = owner.OrganizationIDPSyncSettings(ctx)
		require.NoError(t, err)
		require.Equal(t, "august", settings.Field)
		require.Equal(t, expected, settings.Mapping)
	})
}

func TestPatchOrganizationSyncSettings(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		owner, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles:           1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		//nolint:gocritic // Only owners can change Organization IdP sync settings
		settings, err := owner.PatchOrganizationIDPSyncSettings(ctx, codersdk.OrganizationSyncSettings{
			Field: "august",
		})
		require.NoError(t, err)
		require.Equal(t, "august", settings.Field)

		fetchedSettings, err := owner.OrganizationIDPSyncSettings(ctx)
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

func TestPatchOrganizationSyncConfig(t *testing.T) {
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

		mapping := map[string][]uuid.UUID{"wibble": {user.OrganizationID}}

		ctx := testutil.Context(t, testutil.WaitShort)
		//nolint:gocritic // Only owners can change Organization IdP sync settings
		_, err := owner.PatchOrganizationIDPSyncSettings(ctx, codersdk.OrganizationSyncSettings{
			Field:         "wibble",
			AssignDefault: true,
			Mapping:       mapping,
		})

		require.NoError(t, err)

		fetchedSettings, err := owner.OrganizationIDPSyncSettings(ctx)
		require.NoError(t, err)
		require.Equal(t, "wibble", fetchedSettings.Field)
		require.Equal(t, true, fetchedSettings.AssignDefault)
		require.Equal(t, mapping, fetchedSettings.Mapping)

		ctx = testutil.Context(t, testutil.WaitShort)
		settings, err := owner.PatchOrganizationIDPSyncConfig(ctx, codersdk.PatchOrganizationIDPSyncConfigRequest{
			Field: "wobble",
		})

		require.NoError(t, err)
		require.Equal(t, "wobble", settings.Field)
		require.Equal(t, false, settings.AssignDefault)
		require.Equal(t, mapping, settings.Mapping)

		fetchedSettings, err = owner.OrganizationIDPSyncSettings(ctx)
		require.NoError(t, err)
		require.Equal(t, "wobble", fetchedSettings.Field)
		require.Equal(t, false, fetchedSettings.AssignDefault)
		require.Equal(t, mapping, fetchedSettings.Mapping)
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
		_, err := member.PatchOrganizationIDPSyncConfig(ctx, codersdk.PatchOrganizationIDPSyncConfigRequest{})
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())
	})
}

func TestPatchOrganizationSyncMapping(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		owner, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureCustomRoles:           1,
					codersdk.FeatureMultipleOrganizations: 1,
				},
			},
		})

		// These IDs are easier to visually diff if the test fails than truly random
		// ones.
		orgs := []uuid.UUID{
			uuid.MustParse("00000000-b8bd-46bb-bb6c-6c2b2c0dd2ea"),
			uuid.MustParse("01000000-fbe8-464c-9429-fe01a03f3644"),
			uuid.MustParse("02000000-0926-407b-9998-39af62e3d0c5"),
		}

		ctx := testutil.Context(t, testutil.WaitShort)
		//nolint:gocritic // Only owners can change Organization IdP sync settings
		settings, err := owner.PatchOrganizationIDPSyncMapping(ctx, codersdk.PatchOrganizationIDPSyncMappingRequest{
			Add: []codersdk.IDPSyncMapping[uuid.UUID]{
				{Given: "wibble", Gets: orgs[0]},
				{Given: "wobble", Gets: orgs[0]},
				{Given: "wobble", Gets: orgs[1]},
				{Given: "wobble", Gets: orgs[2]},
			},
			Remove: []codersdk.IDPSyncMapping[uuid.UUID]{
				{Given: "wobble", Gets: orgs[1]},
			},
		})

		expected := map[string][]uuid.UUID{
			"wibble": {orgs[0]},
			"wobble": {orgs[0], orgs[2]},
		}

		require.NoError(t, err)
		require.Equal(t, expected, settings.Mapping)

		fetchedSettings, err := owner.OrganizationIDPSyncSettings(ctx)
		require.NoError(t, err)
		require.Equal(t, expected, fetchedSettings.Mapping)
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
		_, err := member.PatchOrganizationIDPSyncMapping(ctx, codersdk.PatchOrganizationIDPSyncMappingRequest{})
		var apiError *codersdk.Error
		require.ErrorAs(t, err, &apiError)
		require.Equal(t, http.StatusForbidden, apiError.StatusCode())
	})
}

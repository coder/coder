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
			uuid.MustParse("03000000-92f6-4bfd-bba6-0f54667b131c"),
			uuid.MustParse("04000000-b9d0-46fe-910f-6e2ea0c62caa"),
			uuid.MustParse("05000000-67c0-4c19-a52d-0dc3f65abee0"),
			uuid.MustParse("06000000-a8a8-4a2c-bdd0-b59aa6882b55"),
			uuid.MustParse("07000000-5390-4cc7-a9c8-e4330a683ae7"),
		}

		ctx := testutil.Context(t, testutil.WaitShort)
		//nolint:gocritic // Only owners can change Organization IdP sync settings
		settings, err := owner.PatchOrganizationIDPSyncMapping(ctx, codersdk.PatchOrganizationIDPSyncMappingRequest{
			Add: []codersdk.IDPSyncMapping[uuid.UUID]{
				{Given: "wibble", Gets: orgs[0]},
				{Given: "wibble", Gets: orgs[1]},
				{Given: "wobble", Gets: orgs[0]},
				{Given: "wobble", Gets: orgs[1]},
				{Given: "wobble", Gets: orgs[2]},
				{Given: "wobble", Gets: orgs[3]},
				{Given: "wooble", Gets: orgs[0]},
			},
			// Remove takes priority over Add, so "3" should not actually be added to wooble.
			Remove: []codersdk.IDPSyncMapping[uuid.UUID]{
				{Given: "wobble", Gets: orgs[3]},
			},
		})

		expected := map[string][]uuid.UUID{
			"wibble": {orgs[0], orgs[1]},
			"wobble": {orgs[0], orgs[1], orgs[2]},
			"wooble": {orgs[0]},
		}

		require.NoError(t, err)
		require.Equal(t, expected, settings.Mapping)

		fetchedSettings, err := owner.OrganizationIDPSyncSettings(ctx)
		require.NoError(t, err)
		require.Equal(t, expected, fetchedSettings.Mapping)

		ctx = testutil.Context(t, testutil.WaitShort)
		settings, err = owner.PatchOrganizationIDPSyncMapping(ctx, codersdk.PatchOrganizationIDPSyncMappingRequest{
			Add: []codersdk.IDPSyncMapping[uuid.UUID]{
				{Given: "wibble", Gets: orgs[2]},
				{Given: "wobble", Gets: orgs[3]},
				{Given: "wooble", Gets: orgs[0]},
			},
			// Remove takes priority over Add, so `f` should not actually be added.
			Remove: []codersdk.IDPSyncMapping[uuid.UUID]{
				{Given: "wibble", Gets: orgs[0]},
				{Given: "wobble", Gets: orgs[1]},
			},
		})

		expected = map[string][]uuid.UUID{
			"wibble": {orgs[1], orgs[2]},
			"wobble": {orgs[0], orgs[2], orgs[3]},
			"wooble": {orgs[0]},
		}

		require.NoError(t, err)
		require.Equal(t, expected, settings.Mapping)

		fetchedSettings, err = owner.OrganizationIDPSyncSettings(ctx)
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

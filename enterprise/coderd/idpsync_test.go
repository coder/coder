package coderd_test

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
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

	// The subtests below check that GET rewrites a stored row it can repair,
	// replaces the row, and records the rewrite as Coder's own action.

	t.Run("RewritesStoredRow", func(t *testing.T) {
		t.Parallel()

		s := newGroupSyncTestServer(t)
		missing := uuid.New()
		storeGroupSyncSettings(t, s, idpsync.GroupSyncSettings{
			Field:   "groups",
			Mapping: map[string][]uuid.UUID{"eng": {s.inside, s.outside, missing}},
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		got, err := s.orgAdmin.GroupIDPSyncSettingsWithWarnings(ctx, s.orgID.String())
		require.NoError(t, err)
		require.Equal(t, map[string][]uuid.UUID{"eng": {s.inside}}, got.Mapping)
		requireWarnings(t, got, map[string]codersdk.GroupSyncFindingCode{
			"/mapping/eng/1": codersdk.GroupSyncFindingOutsideGroup,
			"/mapping/eng/2": codersdk.GroupSyncFindingMissingGroup,
		})
		require.Equal(t, map[string][]uuid.UUID{"eng": {s.inside}}, storedGroupSyncSettings(t, s).Mapping,
			"the stored row must be replaced with the rewrite")

		again, err := s.orgAdmin.GroupIDPSyncSettingsWithWarnings(ctx, s.orgID.String())
		require.NoError(t, err)
		requireWarnings(t, again, nil) // A rewritten row is valid, so a second fetch has nothing to report.
	})

	t.Run("RewriteAuditedAsCoder", func(t *testing.T) {
		t.Parallel()

		s := newGroupSyncTestServer(t)
		storeGroupSyncSettings(t, s, idpsync.GroupSyncSettings{
			Field:   "groups",
			Mapping: map[string][]uuid.UUID{"eng": {s.outside}},
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := s.orgAdmin.GroupIDPSyncSettingsWithWarnings(ctx, s.orgID.String())
		require.NoError(t, err)
		entries := groupSyncRewriteAudits(t, s)
		require.Len(t, entries, 1, "the rewrite is recorded once, as Coder's action")
		require.Equal(t, s.orgAdminID, entries[0].UserID, "the entry names the user whose fetch caused the rewrite")

		_, err = s.orgAdmin.GroupIDPSyncSettingsWithWarnings(ctx, s.orgID.String())
		require.NoError(t, err)
		require.Len(t, groupSyncRewriteAudits(t, s), 1, "a fetch that rewrites nothing records nothing")
	})

	t.Run("ValidRowReturnedAsStored", func(t *testing.T) {
		t.Parallel()

		s := newGroupSyncTestServer(t)
		storeGroupSyncSettings(t, s, idpsync.GroupSyncSettings{
			Field:       "groups",
			Mapping:     map[string][]uuid.UUID{"eng": {s.inside}},
			RegexFilter: regexp.MustCompile(""),
		})

		ctx := testutil.Context(t, testutil.WaitShort)
		got, err := s.orgAdmin.GroupIDPSyncSettingsWithWarnings(ctx, s.orgID.String())
		require.NoError(t, err)
		require.NotNil(t, got.RegexFilter, "an empty filter is valid, so it is not rewritten to null")
		require.Empty(t, got.RegexFilter.String())
		requireWarnings(t, got, nil)
		require.Empty(t, groupSyncRewriteAudits(t, s))
	})

	t.Run("NotRewrittenRowKept", func(t *testing.T) {
		t.Parallel()

		s := newGroupSyncTestServer(t)
		stored := idpsync.GroupSyncSettings{
			Field:             "groups",
			Mapping:           map[string][]uuid.UUID{"eng": {s.inside, s.outside}},
			LegacyNameMapping: map[string]string{"idp-eng": "eng"},
		}
		storeGroupSyncSettings(t, s, stored)

		ctx := testutil.Context(t, testutil.WaitShort)
		got, err := s.orgAdmin.GroupIDPSyncSettingsWithWarnings(ctx, s.orgID.String())
		require.NoError(t, err)
		require.Equal(t, stored.Mapping, got.Mapping, "a legacy mapping cannot be rewritten, so nothing is")
		requireWarnings(t, got, map[string]codersdk.GroupSyncFindingCode{
			"/legacy_group_name_mapping": codersdk.GroupSyncFindingLegacyMapping,
			"/mapping/eng/1":             codersdk.GroupSyncFindingOutsideGroup,
		})
		for _, w := range got.Warnings {
			require.JSONEq(t, string(w.Received), string(w.Accepted), "nothing was rewritten, so each warning accepts what it received")
		}
		require.Equal(t, stored.Mapping, storedGroupSyncSettings(t, s).Mapping)
		require.Empty(t, groupSyncRewriteAudits(t, s))
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

	// The subtests below check that the full-replace endpoint rejects UUIDs
	// outside the organization or naming no group, and rewrites or keeps the
	// rest as relaxed mode says.

	t.Run("RejectsOutsideGroup", func(t *testing.T) {
		t.Parallel()

		s := newGroupSyncTestServer(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := s.orgAdmin.PatchGroupIDPSyncSettingsWithWarnings(ctx, s.orgID.String(), codersdk.GroupSyncSettings{
			Field:   "groups",
			Mapping: map[string][]uuid.UUID{"eng": {s.outside}},
		})
		requireGroupSyncRejected(t, err, "/mapping/eng/0", codersdk.GroupSyncFindingOutsideGroup)

		fetched, err := s.orgAdmin.GroupIDPSyncSettingsWithWarnings(ctx, s.orgID.String())
		require.NoError(t, err)
		require.Empty(t, fetched.Field, "a rejected request saves nothing")
	})

	t.Run("RejectsMissingGroup", func(t *testing.T) {
		t.Parallel()

		s := newGroupSyncTestServer(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := s.orgAdmin.PatchGroupIDPSyncSettingsWithWarnings(ctx, s.orgID.String(), codersdk.GroupSyncSettings{
			Field:   "groups",
			Mapping: map[string][]uuid.UUID{"eng": {s.inside, uuid.New()}},
		})
		requireGroupSyncRejected(t, err, "/mapping/eng/1", codersdk.GroupSyncFindingMissingGroup)
	})

	t.Run("RejectsLegacyMappingAsBefore", func(t *testing.T) {
		t.Parallel()

		s := newGroupSyncTestServer(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := s.orgAdmin.PatchGroupIDPSyncSettingsWithWarnings(ctx, s.orgID.String(), codersdk.GroupSyncSettings{
			Field:             "groups",
			LegacyNameMapping: map[string]string{"idp-eng": "eng"},
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		require.Contains(t, apiErr.Validations, codersdk.ValidationError{Field: "legacy_group_name_mapping", Detail: "field is not allowed"},
			"the existing rejection keeps its existing message")
	})

	t.Run("RewritesWithWarnings", func(t *testing.T) {
		t.Parallel()

		s := newGroupSyncTestServer(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		got, err := s.orgAdmin.PatchGroupIDPSyncSettingsWithWarnings(ctx, s.orgID.String(), codersdk.GroupSyncSettings{
			Field: "groups",
			// The nil UUID (R1) and the organization's Everyone group (R5)
			// are removed; the group inside the organization stays.
			Mapping: map[string][]uuid.UUID{"eng": {uuid.Nil, s.orgID, s.inside}},
		})
		require.NoError(t, err)
		require.Equal(t, map[string][]uuid.UUID{"eng": {s.inside}}, got.Mapping)
		requireWarnings(t, got, map[string]codersdk.GroupSyncFindingCode{
			"/mapping/eng/0": codersdk.GroupSyncFindingNilUUID,
			"/mapping/eng/1": codersdk.GroupSyncFindingEveryoneGroup,
		})

		fetched, err := s.orgAdmin.GroupIDPSyncSettingsWithWarnings(ctx, s.orgID.String())
		require.NoError(t, err)
		require.Equal(t, map[string][]uuid.UUID{"eng": {s.inside}}, fetched.Mapping, "the rewrite is what was saved")
	})

	t.Run("KeepsEmptyKey", func(t *testing.T) {
		t.Parallel()

		s := newGroupSyncTestServer(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		sent := map[string][]uuid.UUID{"": {s.inside}}
		got, err := s.orgAdmin.PatchGroupIDPSyncSettingsWithWarnings(ctx, s.orgID.String(), codersdk.GroupSyncSettings{
			Field:   "groups",
			Mapping: sent,
		})
		require.NoError(t, err)
		require.Equal(t, sent, got.Mapping, "an empty key cannot be rewritten, so the settings are saved as received")
		requireWarnings(t, got, map[string]codersdk.GroupSyncFindingCode{
			"/mapping/": codersdk.GroupSyncFindingEmptyMappingKey,
		})
		require.JSONEq(t, string(got.Warnings[0].Received), string(got.Warnings[0].Accepted))
	})

	t.Run("ValidSavedAsReceived", func(t *testing.T) {
		t.Parallel()

		s := newGroupSyncTestServer(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		got, err := s.orgAdmin.PatchGroupIDPSyncSettingsWithWarnings(ctx, s.orgID.String(), codersdk.GroupSyncSettings{
			Field:       "groups",
			Mapping:     map[string][]uuid.UUID{"eng": {s.inside}},
			RegexFilter: regexp.MustCompile(""),
		})
		require.NoError(t, err)
		require.NotNil(t, got.RegexFilter, "an empty filter is valid, so it is saved as received")
		require.Empty(t, got.RegexFilter.String())
		requireWarnings(t, got, nil)
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
					codersdk.FeatureTemplateRBAC:          1,
				},
			},
		})

		orgID := user.OrganizationID
		orgAdmin, _ := coderdtest.CreateAnotherUser(t, owner, orgID, rbac.ScopedRoleOrgAdmin(user.OrganizationID))

		// Settings must name a group that exists in the organization.
		group := coderdtest.CreateGroup(t, owner, orgID, "wibble-group")
		mapping := map[string][]uuid.UUID{"wibble": {group.ID}}

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
			Field: "wobble",
			// The filter must still match the key "wibble", so that the
			// mapping is preserved.
			RegexFilter:       regexp.MustCompile("w[io]b{2,}le"),
			AutoCreateMissing: true,
		})

		require.NoError(t, err)
		require.Equal(t, "wobble", settings.Field)
		require.Equal(t, "w[io]b{2,}le", settings.RegexFilter.String())
		require.Equal(t, true, settings.AutoCreateMissing)
		require.Equal(t, mapping, settings.Mapping)

		fetchedSettings, err = orgAdmin.GroupIDPSyncSettings(ctx, orgID.String())
		require.NoError(t, err)
		require.Equal(t, "wobble", fetchedSettings.Field)
		require.Equal(t, "w[io]b{2,}le", fetchedSettings.RegexFilter.String())
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

	t.Run("FilterExcludingKeyRemovesMember", func(t *testing.T) {
		t.Parallel()

		s := newGroupSyncTestServer(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := s.orgAdmin.PatchGroupIDPSyncSettingsWithWarnings(ctx, s.orgID.String(), codersdk.GroupSyncSettings{
			Field:   "groups",
			Mapping: map[string][]uuid.UUID{"eng": {s.inside}, "ops": {s.inside}},
		})
		require.NoError(t, err)

		// A filter the key "ops" does not match makes that member dead; R4
		// removes it on the merge route.
		got, err := s.orgAdmin.PatchGroupIDPSyncConfigWithWarnings(ctx, s.orgID.String(), codersdk.PatchGroupIDPSyncConfigRequest{
			Field:       "groups",
			RegexFilter: regexp.MustCompile("^eng"),
		})
		require.NoError(t, err)
		require.Equal(t, map[string][]uuid.UUID{"eng": {s.inside}}, got.Mapping)
		requireWarnings(t, got, map[string]codersdk.GroupSyncFindingCode{
			"/mapping/ops": codersdk.GroupSyncFindingFilterExcludedKey,
		})
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
					codersdk.FeatureTemplateRBAC:          1,
				},
			},
		})

		orgID := user.OrganizationID
		orgAdmin, _ := coderdtest.CreateAnotherUser(t, owner, orgID, rbac.ScopedRoleOrgAdmin(user.OrganizationID))
		// Settings must name groups that exist in the organization.
		orgs := []uuid.UUID{
			coderdtest.CreateGroup(t, owner, orgID, "group-0").ID,
			coderdtest.CreateGroup(t, owner, orgID, "group-1").ID,
			coderdtest.CreateGroup(t, owner, orgID, "group-2").ID,
		}

		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := orgAdmin.PatchGroupIDPSyncSettings(ctx, orgID.String(), codersdk.GroupSyncSettings{
			Field: "wibble",
			// The filter must match both keys, "wibble" and "wobble".
			RegexFilter:       regexp.MustCompile("w[io]b{2,}le"),
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
		require.Equal(t, "w[io]b{2,}le", fetchedSettings.RegexFilter.String())
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

	// The subtests below check that the mapping-diff endpoint rejects UUIDs
	// added from outside the organization or naming no group, and that a
	// stale entry already stored does not block a change.

	t.Run("RejectsOutsideGroupInAdd", func(t *testing.T) {
		t.Parallel()

		s := newGroupSyncTestServer(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := s.orgAdmin.PatchGroupIDPSyncMappingWithWarnings(ctx, s.orgID.String(), codersdk.PatchGroupIDPSyncMappingRequest{
			Add: []codersdk.IDPSyncMapping[uuid.UUID]{{Given: "eng", Gets: s.outside}},
		})
		requireGroupSyncRejected(t, err, "/mapping/eng/0", codersdk.GroupSyncFindingOutsideGroup)
	})

	t.Run("RejectsMissingGroupInAdd", func(t *testing.T) {
		t.Parallel()

		s := newGroupSyncTestServer(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		_, err := s.orgAdmin.PatchGroupIDPSyncMappingWithWarnings(ctx, s.orgID.String(), codersdk.PatchGroupIDPSyncMappingRequest{
			Add: []codersdk.IDPSyncMapping[uuid.UUID]{{Given: "eng", Gets: uuid.New()}},
		})
		requireGroupSyncRejected(t, err, "/mapping/eng/0", codersdk.GroupSyncFindingMissingGroup)
	})

	t.Run("MissingGetsIsDropped", func(t *testing.T) {
		t.Parallel()

		s := newGroupSyncTestServer(t)
		// The stored field is set, so that the merged settings have no
		// finding that is kept; a kept finding would keep the nil UUID too,
		// since a rewrite is all or nothing.
		storeGroupSyncSettings(t, s, idpsync.GroupSyncSettings{Field: "groups"})
		ctx := testutil.Context(t, testutil.WaitShort)
		// A pair with no Gets decodes as the nil UUID, which R1 removes.
		got, err := s.orgAdmin.PatchGroupIDPSyncMappingWithWarnings(ctx, s.orgID.String(), codersdk.PatchGroupIDPSyncMappingRequest{
			Add: []codersdk.IDPSyncMapping[uuid.UUID]{{Given: "eng"}},
		})
		require.NoError(t, err)
		require.Equal(t, map[string][]uuid.UUID{"eng": {}}, got.Mapping)
		requireWarnings(t, got, map[string]codersdk.GroupSyncFindingCode{
			"/mapping/eng/0": codersdk.GroupSyncFindingNilUUID,
		})
	})

	t.Run("StaleStoredEntryDoesNotBlock", func(t *testing.T) {
		t.Parallel()

		s := newGroupSyncTestServer(t)
		storeGroupSyncSettings(t, s, idpsync.GroupSyncSettings{
			Field:   "groups",
			Mapping: map[string][]uuid.UUID{"old": {uuid.New()}},
		})

		// The stored row is read through the fetch route before merging, so
		// its stale entry is removed rather than rejecting the request.
		ctx := testutil.Context(t, testutil.WaitShort)
		got, err := s.orgAdmin.PatchGroupIDPSyncMappingWithWarnings(ctx, s.orgID.String(), codersdk.PatchGroupIDPSyncMappingRequest{
			Add: []codersdk.IDPSyncMapping[uuid.UUID]{{Given: "eng", Gets: s.inside}},
		})
		require.NoError(t, err)
		require.Equal(t, map[string][]uuid.UUID{"eng": {s.inside}, "old": {}}, got.Mapping)
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

// groupSyncTestServer is an enterprise server set up for the group sync
// settings tests that check validation, rewriting, and auditing.
type groupSyncTestServer struct {
	owner      *codersdk.Client
	db         database.Store
	auditor    *audit.MockAuditor
	orgID      uuid.UUID
	orgAdmin   *codersdk.Client
	orgAdminID uuid.UUID
	// inside is a group in the organization orgID.
	inside uuid.UUID
	// outside is a group in another organization.
	outside uuid.UUID
}

func newGroupSyncTestServer(t *testing.T) groupSyncTestServer {
	t.Helper()
	auditor := audit.NewMock()
	owner, db, user := coderdenttest.NewWithDatabase(t, &coderdenttest.Options{
		AuditLogging: true,
		Options:      &coderdtest.Options{Auditor: auditor},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureCustomRoles:           1,
				codersdk.FeatureMultipleOrganizations: 1,
				codersdk.FeatureTemplateRBAC:          1,
				codersdk.FeatureAuditLog:              1,
			},
		},
	})
	orgAdmin, orgAdminUser := coderdtest.CreateAnotherUser(t, owner, user.OrganizationID, rbac.ScopedRoleOrgAdmin(user.OrganizationID))
	other := coderdenttest.CreateOrganization(t, owner, coderdenttest.CreateOrganizationOptions{})
	return groupSyncTestServer{
		owner:      owner,
		db:         db,
		auditor:    auditor,
		orgID:      user.OrganizationID,
		orgAdmin:   orgAdmin,
		orgAdminID: orgAdminUser.ID,
		inside:     coderdtest.CreateGroup(t, owner, user.OrganizationID, "inside").ID,
		outside:    coderdtest.CreateGroup(t, owner, other.ID, "outside").ID,
	}
}

// groupSyncSettingsEntry is the runtime config entry that holds an
// organization's group sync settings.
var groupSyncSettingsEntry = runtimeconfig.MustNew[*idpsync.GroupSyncSettings]("group-sync-settings")

// storeGroupSyncSettings writes settings straight into the stored row,
// bypassing the API, so that a test can set up a row the API would refuse.
func storeGroupSyncSettings(t *testing.T, s groupSyncTestServer, settings idpsync.GroupSyncSettings) {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	resolver := runtimeconfig.OrganizationResolver(s.orgID, runtimeconfig.NewStoreResolver(s.db))
	require.NoError(t, groupSyncSettingsEntry.SetRuntimeValue(dbauthz.AsSystemRestricted(ctx), resolver, &settings))
}

// storedGroupSyncSettings reads the stored row straight from the database.
func storedGroupSyncSettings(t *testing.T, s groupSyncTestServer) idpsync.GroupSyncSettings {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitShort)
	resolver := runtimeconfig.OrganizationResolver(s.orgID, runtimeconfig.NewStoreResolver(s.db))
	settings, err := groupSyncSettingsEntry.Resolve(dbauthz.AsSystemRestricted(ctx), resolver)
	require.NoError(t, err)
	return *settings
}

// requireGroupSyncRejected checks that err is a 400 that names, among its
// validations, the finding at path with the given code.
func requireGroupSyncRejected(t *testing.T, err error, path string, code codersdk.GroupSyncFindingCode) {
	t.Helper()
	var apiErr *codersdk.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	require.Contains(t, apiErr.Validations, codersdk.ValidationError{Field: path, Detail: "rejected: " + string(code)})
}

// requireWarnings checks that resp's warnings name exactly the given
// findings, each by path and code, and that received_settings goes with them:
// absent when there are no warnings, and otherwise the settings in which each
// warning's path finds the value the warning received.
func requireWarnings(t *testing.T, resp codersdk.GroupSyncSettingsResponse, want map[string]codersdk.GroupSyncFindingCode) {
	t.Helper()
	got := make(map[string]codersdk.GroupSyncFindingCode, len(resp.Warnings))
	for _, w := range resp.Warnings {
		got[w.Path] = w.Code
	}
	if want == nil {
		want = map[string]codersdk.GroupSyncFindingCode{}
	}
	require.Equal(t, want, got)

	if len(resp.Warnings) == 0 {
		require.Nil(t, resp.ReceivedSettings, "received_settings is left out when there are no warnings")
		return
	}
	require.NotNil(t, resp.ReceivedSettings, "received_settings comes with the warnings")
	encoded, err := json.Marshal(resp.ReceivedSettings)
	require.NoError(t, err)
	var doc any
	require.NoError(t, json.Unmarshal(encoded, &doc))
	for _, w := range resp.Warnings {
		element := doc
		if w.Path != "" {
			for _, token := range strings.Split(strings.TrimPrefix(w.Path, "/"), "/") {
				token = codersdk.UnescapeJSONPointerToken(token)
				switch node := element.(type) {
				case map[string]any:
					element = node[token]
				case []any:
					i, err := strconv.Atoi(token)
					require.NoError(t, err, "warning path %q indexes an array with %q", w.Path, token)
					require.Less(t, i, len(node), "warning path %q indexes past the end of its array", w.Path)
					element = node[i]
				default:
					require.Failf(t, "bad warning path", "warning path %q goes through a scalar", w.Path)
				}
			}
		}
		found, err := json.Marshal(element)
		require.NoError(t, err)
		require.JSONEq(t, string(w.Received), string(found), "warning at %q must receive the value its path finds in received_settings", w.Path)
	}
}

// groupSyncRewriteAudits returns the audit entries Coder recorded for
// rewrites of stored group sync settings.
func groupSyncRewriteAudits(t *testing.T, s groupSyncTestServer) []database.AuditLog {
	t.Helper()
	var out []database.AuditLog
	for _, entry := range s.auditor.AuditLogs() {
		var fields map[string]string
		if json.Unmarshal(entry.AdditionalFields, &fields) != nil {
			continue
		}
		if fields["automatic_actor"] == "coder" && fields["automatic_subsystem"] == string(audit.BackgroundSubsystemGroupSyncRewrite) {
			out = append(out, entry)
		}
	}
	return out
}

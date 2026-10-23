package coderd_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestOrganizationSkillSettings(t *testing.T) {
	t.Parallel()

	auditor := audit.NewMockWithDiffFn(func(old, newVal any) audit.Map {
		oldOrg, oldOK := old.(database.Organization)
		newOrg, newOK := newVal.(database.Organization)
		if !oldOK || !newOK {
			return audit.Map{}
		}
		return audit.Map{"members_can_disable_skills": {Old: oldOrg.MembersCanDisableSkills, New: newOrg.MembersCanDisableSkills}}
	})
	ownerClient := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
	firstUser := coderdtest.CreateFirstUser(t, ownerClient)
	orgID := firstUser.OrganizationID
	memberRawClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, orgID)
	member := codersdk.NewExperimentalClient(memberRawClient)
	adminRawClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, orgID, rbac.ScopedRoleOrgAdmin(orgID))
	admin := codersdk.NewExperimentalClient(adminRawClient)
	ctx := testutil.Context(t, testutil.WaitMedium)

	settings, err := member.OrganizationSkillSettings(ctx, orgID)
	require.NoError(t, err)
	require.False(t, settings.MembersCanDisableSkills)

	_, err = member.UpdateOrganizationSkillSettings(ctx, orgID, codersdk.UpdateOrganizationSkillSettingsRequest{MembersCanDisableSkills: true})
	sdkErr := requireSDKErrorStatus(t, err, http.StatusForbidden)
	assert.Equal(t, "You don't have permission to update organization skill settings.", sdkErr.Message)

	auditor.ResetLogs()
	settings, err = admin.UpdateOrganizationSkillSettings(ctx, orgID, codersdk.UpdateOrganizationSkillSettingsRequest{MembersCanDisableSkills: true})
	require.NoError(t, err)
	require.True(t, settings.MembersCanDisableSkills)
	settings, err = member.OrganizationSkillSettings(ctx, orgID)
	require.NoError(t, err)
	require.True(t, settings.MembersCanDisableSkills)

	logs := auditor.AuditLogs()
	require.Len(t, logs, 1)
	assert.Equal(t, database.AuditActionWrite, logs[0].Action)
	assert.Equal(t, database.ResourceTypeOrganization, logs[0].ResourceType)
	assert.Equal(t, orgID, logs[0].ResourceID)
	assert.EqualValues(t, http.StatusOK, logs[0].StatusCode)
	var diff map[string]codersdk.AuditDiffField
	require.NoError(t, json.Unmarshal(logs[0].Diff, &diff))
	assert.Equal(t, false, diff["members_can_disable_skills"].Old)
	assert.Equal(t, true, diff["members_can_disable_skills"].New)
}

func TestOrganizationSkillPreferences(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ownerRawClient := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
	firstUser := coderdtest.CreateFirstUser(t, ownerRawClient)
	orgID := firstUser.OrganizationID
	owner := codersdk.NewExperimentalClient(ownerRawClient)
	memberRawClient, memberUser := coderdtest.CreateAnotherUser(t, ownerRawClient, orgID)
	member := codersdk.NewExperimentalClient(memberRawClient)
	_, otherUser := coderdtest.CreateAnotherUser(t, ownerRawClient, orgID)

	everyone := database.ChatACL{orgID.String(): {Permissions: []policy.Action{policy.ActionRead}}}
	orgSkill := func(orgID uuid.UUID, acl database.ChatACL) database.Skill {
		return dbgen.OrganizationSkill(t, db, database.Skill{
			OrganizationID: uuid.NullUUID{UUID: orgID, Valid: true},
			GroupACL:       acl,
			UserACL:        database.ChatACL{},
		})
	}
	first := orgSkill(orgID, everyone)
	second := orgSkill(orgID, everyone)
	private := orgSkill(orgID, database.ChatACL{})
	otherOrg := dbgen.Organization(t, db, database.Organization{})
	foreign := orgSkill(otherOrg.ID, database.ChatACL{otherOrg.ID.String(): {Permissions: []policy.Action{policy.ActionRead}}})
	ctx := testutil.Context(t, testutil.WaitLong)

	prefs, err := member.OrganizationSkillPreferences(ctx, orgID, codersdk.Me)
	require.NoError(t, err)
	require.Equal(t, codersdk.OrganizationSkillPreferences{DisabledSkillIDs: []uuid.UUID{}}, prefs)

	// Members cannot see or change another member's choices.
	_, err = member.OrganizationSkillPreferences(ctx, orgID, otherUser.ID.String())
	requireSDKErrorStatus(t, err, http.StatusNotFound)
	_, err = member.UpdateOrganizationSkillPreferences(ctx, orgID, otherUser.ID.String(), codersdk.UpdateOrganizationSkillPreferencesRequest{
		AllSkillsDisabled: ptr.Ref(true),
	})
	requireSDKErrorStatus(t, err, http.StatusNotFound)

	_, err = member.UpdateOrganizationSkillPreferences(ctx, orgID, codersdk.Me, codersdk.UpdateOrganizationSkillPreferencesRequest{
		AllSkillsDisabled: ptr.Ref(true),
	})
	requireSDKErrorStatus(t, err, http.StatusForbidden)

	_, err = owner.UpdateOrganizationSkillSettings(ctx, orgID, codersdk.UpdateOrganizationSkillSettingsRequest{MembersCanDisableSkills: true})
	require.NoError(t, err)

	_, err = member.UpdateOrganizationSkillPreferences(ctx, orgID, codersdk.Me, codersdk.UpdateOrganizationSkillPreferencesRequest{})
	requireSDKErrorStatus(t, err, http.StatusBadRequest)
	for name, key := range map[string]string{
		"NotUUID":    "not-a-uuid",
		"Unknown":    uuid.NewString(),
		"Unreadable": private.ID.String(),
		"OtherOrg":   foreign.ID.String(),
	} {
		_, err := member.UpdateOrganizationSkillPreferences(ctx, orgID, codersdk.Me, codersdk.UpdateOrganizationSkillPreferencesRequest{
			Skills: map[string]bool{key: false, first.ID.String(): false},
		})
		sdkErr := requireSDKErrorStatus(t, err, http.StatusBadRequest, name)
		require.Len(t, sdkErr.Validations, 1, name)
		require.Equal(t, "skills."+key, sdkErr.Validations[0].Field, name)
	}
	// Two spellings of one skill ID make the request ambiguous.
	braced := "{" + first.ID.String() + "}"
	_, err = member.UpdateOrganizationSkillPreferences(ctx, orgID, codersdk.Me, codersdk.UpdateOrganizationSkillPreferencesRequest{
		Skills: map[string]bool{first.ID.String(): false, braced: true},
	})
	sdkErr := requireSDKErrorStatus(t, err, http.StatusBadRequest)
	require.Len(t, sdkErr.Validations, 2)
	require.ElementsMatch(t, []string{"skills." + first.ID.String(), "skills." + braced}, []string{sdkErr.Validations[0].Field, sdkErr.Validations[1].Field})
	// A rejected request stores nothing.
	prefs, err = member.OrganizationSkillPreferences(ctx, orgID, codersdk.Me)
	require.NoError(t, err)
	require.Empty(t, prefs.DisabledSkillIDs)

	update := func(req codersdk.UpdateOrganizationSkillPreferencesRequest) codersdk.OrganizationSkillPreferences {
		t.Helper()
		prefs, err := member.UpdateOrganizationSkillPreferences(ctx, orgID, codersdk.Me, req)
		require.NoError(t, err)
		require.True(t, prefs.MembersCanDisableSkills)
		return prefs
	}
	prefs = update(codersdk.UpdateOrganizationSkillPreferencesRequest{
		Skills: map[string]bool{first.ID.String(): false, second.ID.String(): false},
	})
	require.ElementsMatch(t, []uuid.UUID{first.ID, second.ID}, prefs.DisabledSkillIDs)
	require.False(t, prefs.AllSkillsDisabled)

	// Only the listed skills change.
	prefs = update(codersdk.UpdateOrganizationSkillPreferencesRequest{
		Skills: map[string]bool{first.ID.String(): true},
	})
	require.Equal(t, []uuid.UUID{second.ID}, prefs.DisabledSkillIDs)

	// The master switch keeps the per-skill choices, and a later per-skill
	// change keeps the master switch.
	prefs = update(codersdk.UpdateOrganizationSkillPreferencesRequest{AllSkillsDisabled: ptr.Ref(true)})
	require.True(t, prefs.AllSkillsDisabled)
	require.Equal(t, []uuid.UUID{second.ID}, prefs.DisabledSkillIDs)
	prefs = update(codersdk.UpdateOrganizationSkillPreferencesRequest{
		Skills: map[string]bool{second.ID.String(): true},
	})
	require.True(t, prefs.AllSkillsDisabled)
	require.Empty(t, prefs.DisabledSkillIDs)

	// Site owners can read another user's choices, like other personal
	// settings.
	prefs, err = owner.OrganizationSkillPreferences(ctx, orgID, memberUser.ID.String())
	require.NoError(t, err)
	require.True(t, prefs.AllSkillsDisabled)
}

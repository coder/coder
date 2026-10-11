package coderd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestOrganizationSkillAccess(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ownerClient := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
	firstUser := coderdtest.CreateFirstUser(t, ownerClient)
	orgID := firstUser.OrganizationID
	admin := codersdk.NewExperimentalClient(ownerClient)
	memberRawClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, orgID)
	member := codersdk.NewExperimentalClient(memberRawClient)
	sharedRawClient, sharedUser := coderdtest.CreateAnotherUser(t, ownerClient, orgID)
	shared := codersdk.NewExperimentalClient(sharedRawClient)
	auditorRawClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, orgID, rbac.ScopedRoleOrgAuditor(orgID))
	auditor := codersdk.NewExperimentalClient(auditorRawClient)

	everyoneACL := func() database.ChatACL {
		return database.ChatACL{orgID.String(): {Permissions: []policy.Action{policy.ActionRead}}}
	}
	everyone := dbgen.OrganizationSkill(t, db, database.Skill{
		OrganizationID: uuid.NullUUID{UUID: orgID, Valid: true},
		Name:           "everyone-skill",
		GroupACL:       everyoneACL(),
		UserACL:        database.ChatACL{},
	})
	userShared := dbgen.OrganizationSkill(t, db, database.Skill{
		OrganizationID: uuid.NullUUID{UUID: orgID, Valid: true},
		Name:           "user-shared-skill",
		GroupACL:       database.ChatACL{},
		UserACL: database.ChatACL{
			sharedUser.ID.String(): {Permissions: []policy.Action{policy.ActionRead}},
		},
	})

	t.Run("MemberReadsThroughEveryone", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		requireOrganizationSkillListed(ctx, t, member, orgID, everyone.Name)
		_, err := member.OrganizationSkillByName(ctx, orgID, everyone.Name)
		require.NoError(t, err)

		_, err = member.CreateOrganizationSkill(ctx, orgID, codersdk.CreateSkillRequest{
			Content: userSkillMarkdown("member-created", "Denied", "Body."),
		})
		sdkErr := requireSDKErrorStatus(t, err, http.StatusForbidden)
		assert.Equal(t, "You don't have permission to create organization skills.", sdkErr.Message)
		_, err = member.UpdateOrganizationSkill(ctx, orgID, everyone.Name, codersdk.UpdateSkillRequest{Enabled: ptr.Ref(false)})
		requireSDKErrorStatus(t, err, http.StatusForbidden)
		err = member.DeleteOrganizationSkill(ctx, orgID, everyone.Name)
		requireSDKErrorStatus(t, err, http.StatusForbidden)
	})

	t.Run("CreateSeedsEveryoneGrant", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		_, err := admin.CreateOrganizationSkill(ctx, orgID, codersdk.CreateSkillRequest{
			Content: userSkillMarkdown("api-created-skill", "Created", "Body."),
		})
		require.NoError(t, err)
		requireOrganizationSkillListed(ctx, t, member, orgID, "api-created-skill")
		_, err = member.OrganizationSkillByName(ctx, orgID, "api-created-skill")
		require.NoError(t, err)
	})

	t.Run("UserACLGrantsOnlyThatUser", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		requireOrganizationSkillListed(ctx, t, shared, orgID, userShared.Name)
		_, err := shared.OrganizationSkillByName(ctx, orgID, userShared.Name)
		require.NoError(t, err)

		requireOrganizationSkillNotListed(ctx, t, member, orgID, userShared.Name)
		_, err = member.OrganizationSkillByName(ctx, orgID, userShared.Name)
		requireSDKErrorStatus(t, err, http.StatusNotFound)
		_, err = member.UpdateOrganizationSkill(ctx, orgID, userShared.Name, codersdk.UpdateSkillRequest{Enabled: ptr.Ref(false)})
		requireSDKErrorStatus(t, err, http.StatusNotFound)
		err = member.DeleteOrganizationSkill(ctx, orgID, userShared.Name)
		requireSDKErrorStatus(t, err, http.StatusNotFound)
	})

	t.Run("OrgAuditorReads", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		requireOrganizationSkillListed(ctx, t, auditor, orgID, userShared.Name)
		_, err := auditor.OrganizationSkillByName(ctx, orgID, userShared.Name)
		require.NoError(t, err)
		_, err = auditor.UpdateOrganizationSkill(ctx, orgID, userShared.Name, codersdk.UpdateSkillRequest{Enabled: ptr.Ref(false)})
		sdkErr := requireSDKErrorStatus(t, err, http.StatusForbidden)
		assert.Equal(t, "You don't have permission to update this organization skill.", sdkErr.Message)
		err = auditor.DeleteOrganizationSkill(ctx, orgID, userShared.Name)
		sdkErr = requireSDKErrorStatus(t, err, http.StatusForbidden)
		assert.Equal(t, "You don't have permission to delete this organization skill.", sdkErr.Message)
	})

	t.Run("OtherOrganizationMember", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		otherOrg := dbgen.Organization(t, db, database.Organization{})
		foreignUser := dbgen.User(t, db, database.User{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: otherOrg.ID, UserID: foreignUser.ID})
		_, token := dbgen.APIKey(t, db, database.APIKey{UserID: foreignUser.ID})
		foreignRawClient := codersdk.New(ownerClient.URL)
		foreignRawClient.SetSessionToken(token)
		foreign := codersdk.NewExperimentalClient(foreignRawClient)

		_, err := foreign.OrganizationSkills(ctx, orgID)
		requireSDKErrorStatus(t, err, http.StatusNotFound)
		_, err = foreign.OrganizationSkillByName(ctx, orgID, everyone.Name)
		requireSDKErrorStatus(t, err, http.StatusNotFound)
	})

	t.Run("ScopedAPIKeys", func(t *testing.T) {
		t.Parallel()

		scopedClient := func(scope database.APIKeyScope) *codersdk.ExperimentalClient {
			_, token := dbgen.APIKey(t, db, database.APIKey{
				UserID: firstUser.UserID,
				Scopes: database.APIKeyScopes{"organization:read", scope},
			})
			client := codersdk.New(ownerClient.URL)
			client.SetSessionToken(token)
			return codersdk.NewExperimentalClient(client)
		}

		ctx := testutil.Context(t, testutil.WaitMedium)
		reader := scopedClient("organization_skill:read")
		requireOrganizationSkillListed(ctx, t, reader, orgID, everyone.Name)
		_, err := reader.OrganizationSkillByName(ctx, orgID, everyone.Name)
		require.NoError(t, err)
		_, err = reader.CreateOrganizationSkill(ctx, orgID, codersdk.CreateSkillRequest{
			Content: userSkillMarkdown("scoped-created", "Denied", "Body."),
		})
		requireSDKErrorStatus(t, err, http.StatusForbidden)
		_, err = reader.UpdateOrganizationSkill(ctx, orgID, everyone.Name, codersdk.UpdateSkillRequest{Enabled: ptr.Ref(false)})
		requireSDKErrorStatus(t, err, http.StatusForbidden)

		for _, scope := range []database.APIKeyScope{"organization_skill:update", "organization_skill:delete"} {
			_, err := scopedClient(scope).OrganizationSkillByName(ctx, orgID, everyone.Name)
			requireSDKErrorStatus(t, err, http.StatusNotFound, scope)
		}

		// A PATCH response carries the skill's content, so an update-only key
		// must not reach the handler.
		updater := scopedClient("organization_skill:update")
		res, err := updater.Request(ctx, http.MethodPatch,
			fmt.Sprintf("/api/experimental/organizations/%s/skills/%s", orgID, everyone.Name),
			codersdk.UpdateSkillRequest{Enabled: ptr.Ref(false)})
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, http.StatusNotFound, res.StatusCode)
		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		assert.NotContains(t, string(body), everyone.Content)
		got, err := admin.OrganizationSkillByName(ctx, orgID, everyone.Name)
		require.NoError(t, err)
		assert.True(t, got.Enabled, "denied PATCH must not apply")
	})
}

func TestOrganizationSkillLimit(t *testing.T) {
	t.Parallel()

	ownerClient, db := coderdtest.NewWithDatabase(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient)
	client := codersdk.NewExperimentalClient(ownerClient)
	ctx := testutil.Context(t, testutil.WaitLong)

	for i := range skills.MaxPersonalSkillsPerUser {
		dbgen.OrganizationSkill(t, db, database.Skill{
			OrganizationID: uuid.NullUUID{UUID: firstUser.OrganizationID, Valid: true},
			Name:           fmt.Sprintf("org-limit-skill-%03d", i),
		})
	}

	_, err := client.CreateOrganizationSkill(ctx, firstUser.OrganizationID, codersdk.CreateSkillRequest{
		Content: userSkillMarkdown("org-limit-overflow", "Limit", "Body."),
	})
	sdkErr := requireSDKErrorStatus(t, err, http.StatusConflict)
	assert.Equal(t, "Organization skill limit reached.", sdkErr.Message)
	assert.Equal(t,
		fmt.Sprintf("Each organization can have at most %d skills.", skills.MaxPersonalSkillsPerUser),
		sdkErr.Detail,
	)
}

// TestOrganizationSkillAuditsDeniedWrites covers denied writes, which the
// middleware admits for readers and the handler audits before rejecting.
func TestOrganizationSkillAuditsDeniedWrites(t *testing.T) {
	t.Parallel()

	auditor := audit.NewMock()
	ownerClient := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
	firstUser := coderdtest.CreateFirstUser(t, ownerClient)
	orgID := firstUser.OrganizationID
	client := codersdk.NewExperimentalClient(ownerClient)
	memberRawClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, orgID)
	member := codersdk.NewExperimentalClient(memberRawClient)
	ctx := testutil.Context(t, testutil.WaitMedium)

	skill, err := client.CreateOrganizationSkill(ctx, orgID, codersdk.CreateSkillRequest{
		Content: userSkillMarkdown("audited-skill", "Audit", "Body."),
	})
	require.NoError(t, err)
	auditor.ResetLogs()
	_, err = member.UpdateOrganizationSkill(ctx, orgID, skill.Name, codersdk.UpdateSkillRequest{Enabled: ptr.Ref(false)})
	requireSDKErrorStatus(t, err, http.StatusForbidden)
	err = member.DeleteOrganizationSkill(ctx, orgID, skill.Name)
	requireSDKErrorStatus(t, err, http.StatusForbidden)

	logs := auditor.AuditLogs()
	require.Len(t, logs, 2)
	for i, action := range []database.AuditAction{database.AuditActionWrite, database.AuditActionDelete} {
		assert.Equal(t, action, logs[i].Action)
		assert.EqualValues(t, http.StatusForbidden, logs[i].StatusCode)
		assert.Equal(t, database.ResourceTypeOrganizationSkill, logs[i].ResourceType)
		assert.Equal(t, skill.ID, logs[i].ResourceID)
		assert.Equal(t, skill.Name, logs[i].ResourceTarget)
		assert.Equal(t, orgID, logs[i].OrganizationID)
	}
}

func TestOrganizationSkillUpdateAuditsLockedRow(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	auditor := audit.NewMockWithDiffFn(func(old, newVal any) audit.Map {
		oldSkill, oldOK := old.(database.AuditableOrganizationSkill)
		newSkill, newOK := newVal.(database.AuditableOrganizationSkill)
		if !oldOK || !newOK {
			return audit.Map{}
		}
		return audit.Map{"description": {Old: oldSkill.Description, New: newSkill.Description}}
	})
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	ownerClient := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps, Auditor: auditor})
	firstUser := coderdtest.CreateFirstUser(t, ownerClient)
	client := codersdk.NewExperimentalClient(ownerClient)
	skill, err := client.CreateOrganizationSkill(ctx, firstUser.OrganizationID, codersdk.CreateSkillRequest{
		Content: userSkillMarkdown("locked-audit-skill", "Before", "Body."),
	})
	require.NoError(t, err)

	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	_, err = tx.ExecContext(ctx, `UPDATE skills SET description = 'Concurrent' WHERE id = $1`, skill.ID)
	require.NoError(t, err)

	patchErr := make(chan error, 1)
	go func() {
		_, err := client.UpdateOrganizationSkill(ctx, firstUser.OrganizationID, skill.Name, codersdk.UpdateSkillRequest{Enabled: ptr.Ref(false)})
		patchErr <- err
	}()
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		var waits int
		err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pg_stat_activity
WHERE datname = current_database() AND pid <> pg_backend_pid() AND wait_event_type = 'Lock'`).Scan(&waits)
		return err == nil && waits > 0
	}, testutil.IntervalFast, "PATCH waits for the skill row lock")
	require.NoError(t, tx.Commit())
	require.NoError(t, testutil.RequireReceive(ctx, t, patchErr))

	var log database.AuditLog
	for _, entry := range auditor.AuditLogs() {
		if entry.ResourceID == skill.ID && entry.Action == database.AuditActionWrite {
			log = entry
		}
	}
	var diff map[string]codersdk.AuditDiffField
	require.NoError(t, json.Unmarshal(log.Diff, &diff))
	require.Equal(t, "Concurrent", diff["description"].Old)
}

func requireOrganizationSkillListed(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, orgID uuid.UUID, name string) codersdk.SkillMetadata {
	t.Helper()
	list, err := client.OrganizationSkills(ctx, orgID)
	require.NoError(t, err)
	for _, skill := range list {
		if skill.Name == name {
			return skill
		}
	}
	require.Failf(t, "organization skill not listed", "%q missing from %v", name, list)
	return codersdk.SkillMetadata{}
}

func requireOrganizationSkillNotListed(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, orgID uuid.UUID, name string) {
	t.Helper()
	list, err := client.OrganizationSkills(ctx, orgID)
	require.NoError(t, err)
	for _, skill := range list {
		require.NotEqual(t, name, skill.Name, "organization skill unexpectedly listed")
	}
}

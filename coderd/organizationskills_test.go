package coderd_test

import (
	"context"
	"fmt"
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

func TestOrganizationSkillsCRUD(t *testing.T) {
	t.Parallel()

	ownerClient := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient)
	orgID := firstUser.OrganizationID
	orgAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, orgID, rbac.ScopedRoleOrgAdmin(orgID))

	for name, client := range map[string]*codersdk.ExperimentalClient{
		"OrgAdmin":  codersdk.NewExperimentalClient(orgAdminClient),
		"SiteOwner": codersdk.NewExperimentalClient(ownerClient),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			skillName := "crud-" + uuid.NewString()[:8]
			content := userSkillMarkdown(skillName, "Initial description", "Body.")
			created, err := client.CreateOrganizationSkill(ctx, orgID, codersdk.CreateSkillRequest{Content: content})
			require.NoError(t, err)
			assert.Equal(t, skillName, created.Name)
			assert.Equal(t, "Initial description", created.Description)
			assert.Equal(t, content, created.Content)
			assert.True(t, created.Enabled)

			listed := requireOrganizationSkillListed(ctx, t, client, orgID, skillName)
			assert.Equal(t, created.ID, listed.ID)
			assert.True(t, listed.Enabled)

			got, err := client.OrganizationSkillByName(ctx, orgID, skillName)
			require.NoError(t, err)
			assert.Equal(t, content, got.Content)

			updatedContent := userSkillMarkdown(skillName, "Updated description", "Updated body.")
			updated, err := client.UpdateOrganizationSkill(ctx, orgID, skillName, codersdk.UpdateSkillRequest{Content: ptr.Ref(updatedContent)})
			require.NoError(t, err)
			assert.Equal(t, "Updated description", updated.Description)
			assert.Equal(t, updatedContent, updated.Content)

			disabled, err := client.UpdateOrganizationSkill(ctx, orgID, skillName, codersdk.UpdateSkillRequest{Enabled: ptr.Ref(false)})
			require.NoError(t, err)
			assert.False(t, disabled.Enabled)
			assert.Equal(t, updatedContent, disabled.Content)
			listed = requireOrganizationSkillListed(ctx, t, client, orgID, skillName)
			assert.False(t, listed.Enabled)

			require.NoError(t, client.DeleteOrganizationSkill(ctx, orgID, skillName))
			_, err = client.OrganizationSkillByName(ctx, orgID, skillName)
			requireSDKErrorStatus(t, err, http.StatusNotFound)
		})
	}
}

func TestOrganizationSkillValidationAndConflicts(t *testing.T) {
	t.Parallel()

	ownerClient := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, ownerClient)
	orgID := firstUser.OrganizationID
	client := codersdk.NewExperimentalClient(ownerClient)
	ctx := testutil.Context(t, testutil.WaitMedium)

	content := userSkillMarkdown("validated-skill", "Valid", "Body.")
	_, err := client.CreateOrganizationSkill(ctx, orgID, codersdk.CreateSkillRequest{Content: content})
	require.NoError(t, err)

	_, err = client.CreateOrganizationSkill(ctx, orgID, codersdk.CreateSkillRequest{Content: content})
	sdkErr := requireSDKErrorStatus(t, err, http.StatusConflict)
	assert.Equal(t, "A skill with that name already exists.", sdkErr.Message)
	assert.Equal(t, "Choose a different name, or edit the existing skill.", sdkErr.Detail)

	_, err = client.UpdateOrganizationSkill(ctx, orgID, "validated-skill", codersdk.UpdateSkillRequest{
		Content: ptr.Ref(userSkillMarkdown("renamed-skill", "Valid", "Body.")),
	})
	sdkErr = requireSDKErrorStatus(t, err, http.StatusBadRequest)
	assert.Equal(t, "Skill name in path does not match frontmatter name.", sdkErr.Message)

	_, err = client.UpdateOrganizationSkill(ctx, orgID, "validated-skill", codersdk.UpdateSkillRequest{})
	sdkErr = requireSDKErrorStatus(t, err, http.StatusBadRequest)
	assert.Equal(t, "No skill fields to update.", sdkErr.Message)
}

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

	t.Run("ReadScopedAPIKey", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitMedium)
		_, token := dbgen.APIKey(t, db, database.APIKey{
			UserID: firstUser.UserID,
			Scopes: database.APIKeyScopes{"organization:read", "organization_skill:read"},
		})
		scopedRawClient := codersdk.New(ownerClient.URL)
		scopedRawClient.SetSessionToken(token)
		scoped := codersdk.NewExperimentalClient(scopedRawClient)

		requireOrganizationSkillListed(ctx, t, scoped, orgID, everyone.Name)
		_, err := scoped.OrganizationSkillByName(ctx, orgID, everyone.Name)
		require.NoError(t, err)
		_, err = scoped.CreateOrganizationSkill(ctx, orgID, codersdk.CreateSkillRequest{
			Content: userSkillMarkdown("scoped-created", "Denied", "Body."),
		})
		requireSDKErrorStatus(t, err, http.StatusForbidden)
		_, err = scoped.UpdateOrganizationSkill(ctx, orgID, everyone.Name, codersdk.UpdateSkillRequest{Enabled: ptr.Ref(false)})
		requireSDKErrorStatus(t, err, http.StatusForbidden)
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

func TestOrganizationSkillAudit(t *testing.T) {
	t.Parallel()

	auditor := audit.NewMock()
	ownerClient := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
	firstUser := coderdtest.CreateFirstUser(t, ownerClient)
	orgID := firstUser.OrganizationID
	client := codersdk.NewExperimentalClient(ownerClient)
	memberRawClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, orgID)
	member := codersdk.NewExperimentalClient(memberRawClient)
	ctx := testutil.Context(t, testutil.WaitMedium)
	auditor.ResetLogs()

	skill, err := client.CreateOrganizationSkill(ctx, orgID, codersdk.CreateSkillRequest{
		Content: userSkillMarkdown("audited-skill", "Audit", "Body."),
	})
	require.NoError(t, err)
	_, err = member.UpdateOrganizationSkill(ctx, orgID, skill.Name, codersdk.UpdateSkillRequest{Enabled: ptr.Ref(false)})
	requireSDKErrorStatus(t, err, http.StatusForbidden)
	err = member.DeleteOrganizationSkill(ctx, orgID, skill.Name)
	requireSDKErrorStatus(t, err, http.StatusForbidden)
	_, err = client.UpdateOrganizationSkill(ctx, orgID, skill.Name, codersdk.UpdateSkillRequest{Enabled: ptr.Ref(false)})
	require.NoError(t, err)
	require.NoError(t, client.DeleteOrganizationSkill(ctx, orgID, skill.Name))

	logs := auditor.AuditLogs()
	require.Len(t, logs, 5)
	for i, want := range []struct {
		action database.AuditAction
		status int32
	}{
		{database.AuditActionCreate, http.StatusCreated},
		{database.AuditActionWrite, http.StatusForbidden},
		{database.AuditActionDelete, http.StatusForbidden},
		{database.AuditActionWrite, http.StatusOK},
		{database.AuditActionDelete, http.StatusNoContent},
	} {
		assert.Equal(t, want.action, logs[i].Action)
		assert.Equal(t, want.status, logs[i].StatusCode)
		assert.Equal(t, database.ResourceTypeOrganizationSkill, logs[i].ResourceType)
		assert.Equal(t, skill.ID, logs[i].ResourceID)
		assert.Equal(t, skill.Name, logs[i].ResourceTarget)
		assert.Equal(t, orgID, logs[i].OrganizationID)
	}
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

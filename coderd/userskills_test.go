package coderd_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestUserSkillsCRUD(t *testing.T) {
	t.Parallel()

	adminClient := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, adminClient)
	ownerClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)
	owner := codersdk.NewExperimentalClient(ownerClient)
	ctx := testutil.Context(t, testutil.WaitMedium)

	content := userSkillMarkdown("crud-skill", "Initial description", "Use this skill for CRUD tests.")
	created, err := owner.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateUserSkillRequest{Content: content})
	require.NoError(t, err)
	assert.NotZero(t, created.ID)
	assert.Equal(t, "crud-skill", created.Name)
	assert.Equal(t, "Initial description", created.Description)
	assert.Equal(t, content, created.Content)
	assert.NotZero(t, created.CreatedAt)
	assert.NotZero(t, created.UpdatedAt)

	list, err := owner.UserSkills(ctx, codersdk.Me)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, created.ID, list[0].ID)
	assert.Equal(t, "crud-skill", list[0].Name)
	assert.Equal(t, "Initial description", list[0].Description)

	res, err := owner.Request(ctx, http.MethodGet, "/api/experimental/users/me/skills", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	var rawList []map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(res.Body).Decode(&rawList))
	require.Len(t, rawList, 1)
	assert.NotContains(t, rawList[0], "content")

	got, err := owner.UserSkillByName(ctx, codersdk.Me, "crud-skill")
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, content, got.Content)

	updatedContent := userSkillMarkdown("crud-skill", "Updated description", "Updated body.")
	updated, err := owner.UpdateUserSkill(ctx, codersdk.Me, "crud-skill", codersdk.UpdateUserSkillRequest{Content: updatedContent})
	require.NoError(t, err)
	assert.Equal(t, created.ID, updated.ID)
	assert.Equal(t, "Updated description", updated.Description)
	assert.Equal(t, updatedContent, updated.Content)

	require.NoError(t, owner.DeleteUserSkill(ctx, codersdk.Me, "crud-skill"))
	_, err = owner.UserSkillByName(ctx, codersdk.Me, "crud-skill")
	requireSDKErrorStatus(t, err, http.StatusNotFound)
}

func TestUserSkillValidationAndConflicts(t *testing.T) {
	t.Parallel()

	adminClient := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, adminClient)
	ownerClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)
	otherClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)
	owner := codersdk.NewExperimentalClient(ownerClient)
	other := codersdk.NewExperimentalClient(otherClient)
	ctx := testutil.Context(t, testutil.WaitMedium)

	tests := []struct {
		name            string
		content         string
		expectedMessage string
	}{
		{
			name:            "MissingFrontmatterDelimiters",
			content:         "name: missing-frontmatter\n\nBody.",
			expectedMessage: "Invalid skill content.",
		},
		{
			name: "MissingName",
			content: "---\n" +
				"description: Missing name\n" +
				"---\n\nBody.",
			expectedMessage: "Invalid skill name.",
		},
		{
			name:            "NonKebabCaseName",
			content:         userSkillMarkdown("NotKebab", "Invalid", "Body."),
			expectedMessage: "Invalid skill name.",
		},
		{
			name:            "EmptyBody",
			content:         userSkillMarkdown("empty-body", "Invalid", "   \n"),
			expectedMessage: "Skill body is required.",
		},
		{
			name:            "TooLarge",
			content:         strings.Repeat("a", skills.MaxPersonalSkillSizeBytes+1),
			expectedMessage: "Skill content is too large.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			subCtx := testutil.Context(t, testutil.WaitMedium)
			_, err := owner.CreateUserSkill(subCtx, codersdk.Me, codersdk.CreateUserSkillRequest{Content: tt.content})
			sdkErr := requireSDKErrorStatus(t, err, http.StatusBadRequest)
			assert.Equal(t, tt.expectedMessage, sdkErr.Message)
		})
	}

	t.Run("PatchEmptyBody", func(t *testing.T) {
		patchValidationContent := userSkillMarkdown("patch-validation", "Valid", "Body.")
		_, err := owner.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateUserSkillRequest{Content: patchValidationContent})
		require.NoError(t, err)
		_, err = owner.UpdateUserSkill(ctx, codersdk.Me, "patch-validation", codersdk.UpdateUserSkillRequest{
			Content: userSkillMarkdown("patch-validation", "Invalid", "   \n"),
		})
		sdkErr := requireSDKErrorStatus(t, err, http.StatusBadRequest)
		assert.Equal(t, "Skill body is required.", sdkErr.Message)
	})

	sharedContent := userSkillMarkdown("shared-skill", "Shared", "Shared body.")
	_, err := owner.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateUserSkillRequest{Content: sharedContent})
	require.NoError(t, err)
	_, err = owner.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateUserSkillRequest{Content: sharedContent})
	requireSDKErrorStatus(t, err, http.StatusConflict)

	_, err = other.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateUserSkillRequest{Content: sharedContent})
	require.NoError(t, err)
}

func TestUserSkillLimit(t *testing.T) {
	t.Parallel()

	adminClient := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, adminClient)
	ownerClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)
	owner := codersdk.NewExperimentalClient(ownerClient)
	ctx := testutil.Context(t, testutil.WaitLong)

	for i := range skills.MaxPersonalSkillsPerUser {
		name := fmt.Sprintf("limit-skill-%03d", i)
		_, err := owner.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateUserSkillRequest{
			Content: userSkillMarkdown(name, "Limit", "Body."),
		})
		require.NoError(t, err)
	}

	_, err := owner.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateUserSkillRequest{
		Content: userSkillMarkdown("limit-skill-overflow", "Limit", "Body."),
	})
	sdkErr := requireSDKErrorStatus(t, err, http.StatusForbidden)
	assert.Equal(t, "Personal skill limit reached.", sdkErr.Message)
	assert.Equal(t,
		fmt.Sprintf("Each user can have at most %d personal skills.", skills.MaxPersonalSkillsPerUser),
		sdkErr.Detail,
	)
}

func TestUserSkillLimitConcurrentCreates(t *testing.T) {
	t.Parallel()

	adminClient := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, adminClient)
	ownerClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)
	owner := codersdk.NewExperimentalClient(ownerClient)
	ctx := testutil.Context(t, testutil.WaitLong)

	for i := 0; i < skills.MaxPersonalSkillsPerUser-1; i++ {
		name := fmt.Sprintf("concurrent-limit-skill-%03d", i)
		_, err := owner.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateUserSkillRequest{
			Content: userSkillMarkdown(name, "Limit", "Body."),
		})
		require.NoError(t, err)
	}

	const attempts = 8
	start := make(chan struct{})
	results := make(chan error, attempts)
	for i := range attempts {
		go func() {
			<-start
			name := fmt.Sprintf("concurrent-limit-overflow-%03d", i)
			_, err := owner.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateUserSkillRequest{
				Content: userSkillMarkdown(name, "Limit", "Body."),
			})
			results <- err
		}()
	}
	close(start)

	successes := 0
	for range attempts {
		err := <-results
		if err == nil {
			successes++
			continue
		}
		requireSDKErrorStatus(t, err, http.StatusForbidden)
	}
	assert.Equal(t, 1, successes)

	list, err := owner.UserSkills(ctx, codersdk.Me)
	require.NoError(t, err)
	assert.Len(t, list, skills.MaxPersonalSkillsPerUser)
}

func TestUserSkillRequestAllowsEscapedMaxSizeContent(t *testing.T) {
	t.Parallel()

	adminClient := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, adminClient)
	ownerClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)
	owner := codersdk.NewExperimentalClient(ownerClient)
	ctx := testutil.Context(t, testutil.WaitMedium)

	prefix := "---\nname: escaped-limit-skill\ndescription: Escaped\n---\n\n"
	suffix := "\n"
	bodyLen := skills.MaxPersonalSkillSizeBytes - len(prefix) - len(suffix)
	require.Positive(t, bodyLen)
	content := prefix + strings.Repeat(`"`, bodyLen) + suffix
	require.Len(t, []byte(content), skills.MaxPersonalSkillSizeBytes)

	raw, err := json.Marshal(codersdk.CreateUserSkillRequest{Content: content})
	require.NoError(t, err)
	require.Greater(t, len(raw), skills.MaxPersonalSkillSizeBytes+1024)

	created, err := owner.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateUserSkillRequest{
		Content: content,
	})
	require.NoError(t, err)
	assert.Equal(t, "escaped-limit-skill", created.Name)
}

func TestUserSkillMissingAndUpdateMismatch(t *testing.T) {
	t.Parallel()

	adminClient := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, adminClient)
	ownerClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)
	owner := codersdk.NewExperimentalClient(ownerClient)
	ctx := testutil.Context(t, testutil.WaitMedium)

	_, err := owner.UserSkillByName(ctx, codersdk.Me, "missing-skill")
	requireSDKErrorStatus(t, err, http.StatusNotFound)

	_, err = owner.UpdateUserSkill(ctx, codersdk.Me, "missing-skill", codersdk.UpdateUserSkillRequest{
		Content: userSkillMarkdown("missing-skill", "Missing", "Body."),
	})
	requireSDKErrorStatus(t, err, http.StatusNotFound)

	err = owner.DeleteUserSkill(ctx, codersdk.Me, "missing-skill")
	requireSDKErrorStatus(t, err, http.StatusNotFound)

	_, err = owner.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateUserSkillRequest{
		Content: userSkillMarkdown("old-name", "Old", "Body."),
	})
	require.NoError(t, err)
	_, err = owner.UpdateUserSkill(ctx, codersdk.Me, "old-name", codersdk.UpdateUserSkillRequest{
		Content: userSkillMarkdown("new-name", "New", "Body."),
	})
	sdkErr := requireSDKErrorStatus(t, err, http.StatusBadRequest)
	assert.Contains(t, sdkErr.Message, "skill name in path does not match frontmatter name")
	assert.Equal(t, `path has "old-name", frontmatter has "new-name"`, sdkErr.Detail)
}

func TestUserSkillAuthorization(t *testing.T) {
	t.Parallel()

	adminClient := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, adminClient)
	ownerClient, ownerUser := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)
	otherClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)
	admin := codersdk.NewExperimentalClient(adminClient)
	owner := codersdk.NewExperimentalClient(ownerClient)
	other := codersdk.NewExperimentalClient(otherClient)
	ctx := testutil.Context(t, testutil.WaitMedium)
	targetUser := ownerUser.Username

	_, err := owner.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateUserSkillRequest{
		Content: userSkillMarkdown("auth-skill", "Auth", "Body."),
	})
	require.NoError(t, err)

	_, err = other.UserSkills(ctx, targetUser)
	requireSDKErrorStatus(t, err, http.StatusNotFound)
	_, err = other.UserSkillByName(ctx, targetUser, "auth-skill")
	requireSDKErrorStatus(t, err, http.StatusNotFound)
	_, err = other.CreateUserSkill(ctx, targetUser, codersdk.CreateUserSkillRequest{
		Content: userSkillMarkdown("denied-create", "Denied", "Body."),
	})
	requireSDKErrorStatus(t, err, http.StatusNotFound)
	_, err = other.UpdateUserSkill(ctx, targetUser, "auth-skill", codersdk.UpdateUserSkillRequest{
		Content: userSkillMarkdown("auth-skill", "Denied", "Body."),
	})
	requireSDKErrorStatus(t, err, http.StatusNotFound)
	err = other.DeleteUserSkill(ctx, targetUser, "auth-skill")
	requireSDKErrorStatus(t, err, http.StatusNotFound)

	list, err := admin.UserSkills(ctx, targetUser)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "auth-skill", list[0].Name)
	got, err := admin.UserSkillByName(ctx, targetUser, "auth-skill")
	require.NoError(t, err)
	assert.Equal(t, "auth-skill", got.Name)
	updated, err := admin.UpdateUserSkill(ctx, targetUser, "auth-skill", codersdk.UpdateUserSkillRequest{
		Content: userSkillMarkdown("auth-skill", "Admin update", "Updated by admin."),
	})
	require.NoError(t, err)
	assert.Equal(t, "Admin update", updated.Description)
	require.NoError(t, admin.DeleteUserSkill(ctx, targetUser, "auth-skill"))
}

func TestUserSkillSoftDeleteCleanup(t *testing.T) {
	t.Parallel()

	adminClient, _, api := coderdtest.NewWithAPI(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, adminClient)
	ownerClient, ownerUser := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)
	owner := codersdk.NewExperimentalClient(ownerClient)
	ctx := testutil.Context(t, testutil.WaitMedium)

	_, err := owner.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateUserSkillRequest{
		Content: userSkillMarkdown("soft-delete-skill", "Soft delete", "Body."),
	})
	require.NoError(t, err)

	require.NoError(t, adminClient.DeleteUser(ctx, ownerUser.ID))
	_, err = api.Database.GetUserSkillByUserIDAndName(
		dbauthz.AsSystemRestricted(ctx),
		database.GetUserSkillByUserIDAndNameParams{
			UserID: ownerUser.ID,
			Name:   "soft-delete-skill",
		},
	)
	require.ErrorIs(t, err, sql.ErrNoRows)

	_, err = api.Database.InsertUserSkill(
		dbauthz.AsSystemRestricted(ctx),
		database.InsertUserSkillParams{
			UserID:      ownerUser.ID,
			Name:        "after-soft-delete",
			Description: "Soft delete",
			Content:     userSkillMarkdown("after-soft-delete", "Soft delete", "Body."),
		},
	)
	require.ErrorContains(t, err, "Cannot create user_skill for deleted user")
}

//nolint:paralleltest,tparallel // Subtests share one auditor and run sequentially.
func TestUserSkillAudit(t *testing.T) {
	t.Parallel()

	auditor := audit.NewMock()
	adminClient := coderdtest.New(t, &coderdtest.Options{Auditor: auditor})
	firstUser := coderdtest.CreateFirstUser(t, adminClient)
	memberClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)
	member := codersdk.NewExperimentalClient(memberClient)
	ctx := testutil.Context(t, testutil.WaitMedium)
	auditor.ResetLogs()

	genName := func(t *testing.T) string {
		return strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	}

	t.Run("CreateEmitsLog", func(t *testing.T) {
		auditor.ResetLogs()
		name := genName(t)

		skill, err := member.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateUserSkillRequest{
			Content: userSkillMarkdown(name, "Audit", "Body."),
		})
		require.NoError(t, err)

		logs := auditor.AuditLogs()
		require.Len(t, logs, 1)
		assert.Equal(t, database.AuditActionCreate, logs[0].Action)
		assert.Equal(t, skill.ID, logs[0].ResourceID)
		assert.Equal(t, skill.Name, logs[0].ResourceTarget)
		assert.EqualValues(t, http.StatusCreated, logs[0].StatusCode)
	})

	t.Run("UpdateEmitsLog", func(t *testing.T) {
		auditor.ResetLogs()
		name := genName(t)

		skill, err := member.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateUserSkillRequest{
			Content: userSkillMarkdown(name, "Initial", "Body."),
		})
		require.NoError(t, err)
		_, err = member.UpdateUserSkill(ctx, codersdk.Me, name, codersdk.UpdateUserSkillRequest{
			Content: userSkillMarkdown(name, "Updated", "Updated body."),
		})
		require.NoError(t, err)

		logs := auditor.AuditLogs()
		require.Len(t, logs, 2)
		assert.Equal(t, database.AuditActionCreate, logs[0].Action)
		assert.Equal(t, database.AuditActionWrite, logs[1].Action)
		assert.Equal(t, skill.ID, logs[1].ResourceID)
		assert.Equal(t, skill.Name, logs[1].ResourceTarget)
		assert.EqualValues(t, http.StatusOK, logs[1].StatusCode)
	})

	t.Run("DeleteEmitsLog", func(t *testing.T) {
		auditor.ResetLogs()
		name := genName(t)

		skill, err := member.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateUserSkillRequest{
			Content: userSkillMarkdown(name, "Delete", "Body."),
		})
		require.NoError(t, err)
		require.NoError(t, member.DeleteUserSkill(ctx, codersdk.Me, name))

		logs := auditor.AuditLogs()
		require.Len(t, logs, 2)
		assert.Equal(t, database.AuditActionCreate, logs[0].Action)
		assert.Equal(t, database.AuditActionDelete, logs[1].Action)
		assert.Equal(t, skill.ID, logs[1].ResourceID)
		assert.Equal(t, skill.Name, logs[1].ResourceTarget)
		assert.EqualValues(t, http.StatusNoContent, logs[1].StatusCode)
	})
}

func userSkillMarkdown(name string, description string, body string) string {
	return fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s\n", name, description, body)
}

func requireSDKErrorStatus(t *testing.T, err error, status int, msgAndArgs ...any) *codersdk.Error {
	t.Helper()
	require.Error(t, err, msgAndArgs...)
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr, msgAndArgs...)
	assert.Equal(t, status, sdkErr.StatusCode(), msgAndArgs...)
	return sdkErr
}

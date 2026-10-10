package coderd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/skills"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// skillOwnerAPI drives the personal or organization skill routes through the
// same calls, so behavior both owners share is asserted for each of them.
type skillOwnerAPI struct {
	path         string
	resourceType database.ResourceType
	auditOrgID   uuid.UUID
	client       *codersdk.ExperimentalClient
	create       func(context.Context, codersdk.CreateSkillRequest) (codersdk.Skill, error)
	list         func(context.Context) ([]codersdk.SkillMetadata, error)
	get          func(context.Context, string) (codersdk.Skill, error)
	update       func(context.Context, string, codersdk.UpdateSkillRequest) (codersdk.Skill, error)
	delete       func(context.Context, string) error
}

func personalSkillAPI(client *codersdk.ExperimentalClient) skillOwnerAPI {
	return skillOwnerAPI{
		path:         "/api/experimental/users/me/skills",
		resourceType: database.ResourceTypeUserSkill,
		client:       client,
		create: func(ctx context.Context, req codersdk.CreateSkillRequest) (codersdk.Skill, error) {
			return client.CreateUserSkill(ctx, codersdk.Me, req)
		},
		list: func(ctx context.Context) ([]codersdk.SkillMetadata, error) {
			return client.UserSkills(ctx, codersdk.Me)
		},
		get: func(ctx context.Context, name string) (codersdk.Skill, error) {
			return client.UserSkillByName(ctx, codersdk.Me, name)
		},
		update: func(ctx context.Context, name string, req codersdk.UpdateSkillRequest) (codersdk.Skill, error) {
			return client.UpdateUserSkill(ctx, codersdk.Me, name, req)
		},
		delete: func(ctx context.Context, name string) error { return client.DeleteUserSkill(ctx, codersdk.Me, name) },
	}
}

func organizationSkillAPI(client *codersdk.ExperimentalClient, orgID uuid.UUID) skillOwnerAPI {
	return skillOwnerAPI{
		path:         fmt.Sprintf("/api/experimental/organizations/%s/skills", orgID),
		resourceType: database.ResourceTypeOrganizationSkill,
		auditOrgID:   orgID,
		client:       client,
		create: func(ctx context.Context, req codersdk.CreateSkillRequest) (codersdk.Skill, error) {
			return client.CreateOrganizationSkill(ctx, orgID, req)
		},
		list: func(ctx context.Context) ([]codersdk.SkillMetadata, error) {
			return client.OrganizationSkills(ctx, orgID)
		},
		get: func(ctx context.Context, name string) (codersdk.Skill, error) {
			return client.OrganizationSkillByName(ctx, orgID, name)
		},
		update: func(ctx context.Context, name string, req codersdk.UpdateSkillRequest) (codersdk.Skill, error) {
			return client.UpdateOrganizationSkill(ctx, orgID, name, req)
		},
		delete: func(ctx context.Context, name string) error { return client.DeleteOrganizationSkill(ctx, orgID, name) },
	}
}

// skillOwnerCases each start a deployment and return a caller that manages
// one owner's skills, plus a peer that manages another owner of the same kind.
var skillOwnerCases = []struct {
	name  string
	setup func(t *testing.T, opts *coderdtest.Options) (owner, peer skillOwnerAPI)
}{
	{
		name: "Personal",
		setup: func(t *testing.T, opts *coderdtest.Options) (skillOwnerAPI, skillOwnerAPI) {
			adminClient := coderdtest.New(t, opts)
			firstUser := coderdtest.CreateFirstUser(t, adminClient)
			ownerClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)
			peerClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)
			return personalSkillAPI(codersdk.NewExperimentalClient(ownerClient)), personalSkillAPI(codersdk.NewExperimentalClient(peerClient))
		},
	},
	{
		name: "OrgAdmin",
		setup: func(t *testing.T, opts *coderdtest.Options) (skillOwnerAPI, skillOwnerAPI) {
			ownerClient, db := coderdtest.NewWithDatabase(t, opts)
			firstUser := coderdtest.CreateFirstUser(t, ownerClient)
			orgAdminClient, _ := coderdtest.CreateAnotherUser(t, ownerClient, firstUser.OrganizationID, rbac.ScopedRoleOrgAdmin(firstUser.OrganizationID))
			peerOrg := dbgen.Organization(t, db, database.Organization{})
			return organizationSkillAPI(codersdk.NewExperimentalClient(orgAdminClient), firstUser.OrganizationID),
				organizationSkillAPI(codersdk.NewExperimentalClient(ownerClient), peerOrg.ID)
		},
	},
	{
		name: "SiteOwner",
		setup: func(t *testing.T, opts *coderdtest.Options) (skillOwnerAPI, skillOwnerAPI) {
			ownerClient, db := coderdtest.NewWithDatabase(t, opts)
			firstUser := coderdtest.CreateFirstUser(t, ownerClient)
			client := codersdk.NewExperimentalClient(ownerClient)
			peerOrg := dbgen.Organization(t, db, database.Organization{})
			return organizationSkillAPI(client, firstUser.OrganizationID), organizationSkillAPI(client, peerOrg.ID)
		},
	},
}

func TestSkillsCRUD(t *testing.T) {
	t.Parallel()

	for _, tc := range skillOwnerCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			owner, _ := tc.setup(t, nil)
			ctx := testutil.Context(t, testutil.WaitMedium)

			emptyList, err := owner.list(ctx)
			require.NoError(t, err)
			assert.NotNil(t, emptyList)
			assert.Empty(t, emptyList)
			rawEmptyList := requireRawSkillList(ctx, t, owner)
			assert.NotNil(t, rawEmptyList)
			assert.Empty(t, rawEmptyList)

			content := userSkillMarkdown("crud-skill", "Initial description", "Use this skill for CRUD tests.")
			created, err := owner.create(ctx, codersdk.CreateSkillRequest{Content: content})
			require.NoError(t, err)
			assert.NotZero(t, created.ID)
			assert.Equal(t, "crud-skill", created.Name)
			assert.Equal(t, "Initial description", created.Description)
			assert.Equal(t, content, created.Content)
			assert.True(t, created.Enabled)
			assert.NotZero(t, created.CreatedAt)
			assert.NotZero(t, created.UpdatedAt)

			list, err := owner.list(ctx)
			require.NoError(t, err)
			require.Len(t, list, 1)
			assert.Equal(t, created.ID, list[0].ID)
			assert.Equal(t, "crud-skill", list[0].Name)
			assert.Equal(t, "Initial description", list[0].Description)
			assert.True(t, list[0].Enabled)
			rawList := requireRawSkillList(ctx, t, owner)
			require.Len(t, rawList, 1)
			assert.NotContains(t, rawList[0], "content")

			got, err := owner.get(ctx, "crud-skill")
			require.NoError(t, err)
			assert.Equal(t, created.ID, got.ID)
			assert.Equal(t, content, got.Content)

			updatedContent := userSkillMarkdown("crud-skill", "Updated description", "Updated body.")
			updated, err := owner.update(ctx, "crud-skill", codersdk.UpdateSkillRequest{Content: ptr.Ref(updatedContent)})
			require.NoError(t, err)
			assert.Equal(t, created.ID, updated.ID)
			assert.Equal(t, "Updated description", updated.Description)
			assert.Equal(t, updatedContent, updated.Content)
			assert.True(t, updated.Enabled)

			disabled, err := owner.update(ctx, "crud-skill", codersdk.UpdateSkillRequest{Enabled: ptr.Ref(false)})
			require.NoError(t, err)
			assert.False(t, disabled.Enabled)
			assert.Equal(t, updatedContent, disabled.Content)
			assert.Equal(t, "Updated description", disabled.Description)
			list, err = owner.list(ctx)
			require.NoError(t, err)
			require.Len(t, list, 1)
			assert.False(t, list[0].Enabled)
			got, err = owner.get(ctx, "crud-skill")
			require.NoError(t, err)
			assert.False(t, got.Enabled)

			require.NoError(t, owner.delete(ctx, "crud-skill"))
			_, err = owner.get(ctx, "crud-skill")
			requireSDKErrorStatus(t, err, http.StatusNotFound)
		})
	}
}

func requireRawSkillList(ctx context.Context, t *testing.T, owner skillOwnerAPI) []map[string]json.RawMessage {
	t.Helper()
	res, err := owner.client.Request(ctx, http.MethodGet, owner.path, nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	var raw []map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(res.Body).Decode(&raw))
	return raw
}

func TestSkillValidationAndConflicts(t *testing.T) {
	t.Parallel()

	for _, tc := range skillOwnerCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			owner, peer := tc.setup(t, nil)
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
					name:            "NameTooLong",
					content:         userSkillMarkdown(strings.Repeat("a", skills.MaxPersonalSkillNameBytes+1), "Invalid", "Body."),
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

					ctx := testutil.Context(t, testutil.WaitMedium)
					_, err := owner.create(ctx, codersdk.CreateSkillRequest{Content: tt.content})
					sdkErr := requireSDKErrorStatus(t, err, http.StatusBadRequest)
					assert.Equal(t, tt.expectedMessage, sdkErr.Message)
				})
			}

			t.Run("PatchEmptyBody", func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitMedium)
				_, err := owner.create(ctx, codersdk.CreateSkillRequest{Content: userSkillMarkdown("patch-validation", "Valid", "Body.")})
				require.NoError(t, err)
				_, err = owner.update(ctx, "patch-validation", codersdk.UpdateSkillRequest{
					Content: ptr.Ref(userSkillMarkdown("patch-validation", "Invalid", "   \n")),
				})
				sdkErr := requireSDKErrorStatus(t, err, http.StatusBadRequest)
				assert.Equal(t, "Skill body is required.", sdkErr.Message)
			})

			t.Run("PatchNoFields", func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitMedium)
				_, err := owner.create(ctx, codersdk.CreateSkillRequest{Content: userSkillMarkdown("patch-no-fields", "Valid", "Body.")})
				require.NoError(t, err)
				_, err = owner.update(ctx, "patch-no-fields", codersdk.UpdateSkillRequest{})
				sdkErr := requireSDKErrorStatus(t, err, http.StatusBadRequest)
				assert.Equal(t, "No skill fields to update.", sdkErr.Message)
			})

			t.Run("PatchNameMismatch", func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitMedium)
				_, err := owner.create(ctx, codersdk.CreateSkillRequest{Content: userSkillMarkdown("old-name", "Old", "Body.")})
				require.NoError(t, err)
				_, err = owner.update(ctx, "old-name", codersdk.UpdateSkillRequest{
					Content: ptr.Ref(userSkillMarkdown("new-name", "New", "Body.")),
				})
				sdkErr := requireSDKErrorStatus(t, err, http.StatusBadRequest)
				assert.Equal(t, "Skill name in path does not match frontmatter name.", sdkErr.Message)
				assert.Equal(t, `path has "old-name", frontmatter has "new-name"`, sdkErr.Detail)
			})

			t.Run("MissingSkill", func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitMedium)
				_, err := owner.get(ctx, "missing-skill")
				requireSDKErrorStatus(t, err, http.StatusNotFound)
				_, err = owner.update(ctx, "missing-skill", codersdk.UpdateSkillRequest{
					Content: ptr.Ref(userSkillMarkdown("missing-skill", "Missing", "Body.")),
				})
				requireSDKErrorStatus(t, err, http.StatusNotFound)
				err = owner.delete(ctx, "missing-skill")
				requireSDKErrorStatus(t, err, http.StatusNotFound)
			})

			t.Run("SameNameConflictsOnlyWithinOwner", func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitMedium)
				sharedContent := userSkillMarkdown("shared-skill", "Shared", "Shared body.")
				_, err := owner.create(ctx, codersdk.CreateSkillRequest{Content: sharedContent})
				require.NoError(t, err)
				_, err = owner.create(ctx, codersdk.CreateSkillRequest{Content: sharedContent})
				sdkErr := requireSDKErrorStatus(t, err, http.StatusConflict)
				assert.Equal(t, "A skill with that name already exists.", sdkErr.Message)
				_, err = peer.create(ctx, codersdk.CreateSkillRequest{Content: sharedContent})
				require.NoError(t, err)
			})
		})
	}
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
		_, err := owner.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateSkillRequest{
			Content: userSkillMarkdown(name, "Limit", "Body."),
		})
		require.NoError(t, err)
	}

	_, err := owner.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateSkillRequest{
		Content: userSkillMarkdown("limit-skill-overflow", "Limit", "Body."),
	})
	sdkErr := requireSDKErrorStatus(t, err, http.StatusConflict)
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

	for i := range skills.MaxPersonalSkillsPerUser - 1 {
		name := fmt.Sprintf("concurrent-limit-skill-%03d", i)
		_, err := owner.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateSkillRequest{
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
			_, err := owner.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateSkillRequest{
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
		requireSDKErrorStatus(t, err, http.StatusConflict)
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

	raw, err := json.Marshal(codersdk.CreateSkillRequest{Content: content})
	require.NoError(t, err)
	require.Greater(t, len(raw), skills.MaxPersonalSkillSizeBytes+1024)

	created, err := owner.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateSkillRequest{
		Content: content,
	})
	require.NoError(t, err)
	assert.Equal(t, "escaped-limit-skill", created.Name)
}

func TestUserSkillAuthorization(t *testing.T) {
	t.Parallel()

	adminClient := coderdtest.New(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, adminClient)
	ownerClient, ownerUser := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)
	otherClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)
	userAdminClient, _ := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID, rbac.RoleUserAdmin())
	admin := codersdk.NewExperimentalClient(adminClient)
	owner := codersdk.NewExperimentalClient(ownerClient)
	other := codersdk.NewExperimentalClient(otherClient)
	userAdmin := codersdk.NewExperimentalClient(userAdminClient)
	ctx := testutil.Context(t, testutil.WaitMedium)
	targetUser := ownerUser.Username

	_, err := owner.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateSkillRequest{
		Content: userSkillMarkdown("auth-skill", "Auth", "Body."),
	})
	require.NoError(t, err)

	_, err = other.UserSkills(ctx, targetUser)
	requireSDKErrorStatus(t, err, http.StatusNotFound)
	_, err = other.UserSkillByName(ctx, targetUser, "auth-skill")
	requireSDKErrorStatus(t, err, http.StatusNotFound)
	_, err = other.CreateUserSkill(ctx, targetUser, codersdk.CreateSkillRequest{
		Content: userSkillMarkdown("denied-create", "Denied", "Body."),
	})
	requireSDKErrorStatus(t, err, http.StatusNotFound)
	_, err = other.UpdateUserSkill(ctx, targetUser, "auth-skill", codersdk.UpdateSkillRequest{
		Content: ptr.Ref(userSkillMarkdown("auth-skill", "Denied", "Body.")),
	})
	requireSDKErrorStatus(t, err, http.StatusNotFound)
	err = other.DeleteUserSkill(ctx, targetUser, "auth-skill")
	requireSDKErrorStatus(t, err, http.StatusNotFound)

	_, err = userAdmin.UserSkills(ctx, targetUser)
	requireSDKErrorStatus(t, err, http.StatusNotFound)
	_, err = userAdmin.UserSkillByName(ctx, targetUser, "auth-skill")
	requireSDKErrorStatus(t, err, http.StatusNotFound)
	_, err = userAdmin.CreateUserSkill(ctx, targetUser, codersdk.CreateSkillRequest{
		Content: userSkillMarkdown("denied-admin-create", "Denied", "Body."),
	})
	requireSDKErrorStatus(t, err, http.StatusForbidden)
	_, err = userAdmin.UpdateUserSkill(ctx, targetUser, "auth-skill", codersdk.UpdateSkillRequest{
		Content: ptr.Ref(userSkillMarkdown("auth-skill", "Denied", "Body.")),
	})
	requireSDKErrorStatus(t, err, http.StatusForbidden)
	err = userAdmin.DeleteUserSkill(ctx, targetUser, "auth-skill")
	requireSDKErrorStatus(t, err, http.StatusNotFound)

	_, err = admin.CreateUserSkill(ctx, targetUser, codersdk.CreateSkillRequest{
		Content: userSkillMarkdown("admin-created", "Admin create", "Created by admin."),
	})
	requireSDKErrorStatus(t, err, http.StatusForbidden)

	list, err := admin.UserSkills(ctx, targetUser)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "auth-skill", list[0].Name)
	got, err := admin.UserSkillByName(ctx, targetUser, "auth-skill")
	require.NoError(t, err)
	assert.Equal(t, "auth-skill", got.Name)
	_, err = admin.UpdateUserSkill(ctx, targetUser, "auth-skill", codersdk.UpdateSkillRequest{
		Content: ptr.Ref(userSkillMarkdown("auth-skill", "Admin update", "Updated by admin.")),
	})
	requireSDKErrorStatus(t, err, http.StatusForbidden)
	require.NoError(t, admin.DeleteUserSkill(ctx, targetUser, "auth-skill"))
}

func TestUserSkillSoftDeleteCleanup(t *testing.T) {
	t.Parallel()

	adminClient, _, api := coderdtest.NewWithAPI(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, adminClient)
	ownerClient, ownerUser := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)
	owner := codersdk.NewExperimentalClient(ownerClient)
	ctx := testutil.Context(t, testutil.WaitMedium)

	_, err := owner.CreateUserSkill(ctx, codersdk.Me, codersdk.CreateSkillRequest{
		Content: userSkillMarkdown("soft-delete-skill", "Soft delete", "Body."),
	})
	require.NoError(t, err)

	require.NoError(t, adminClient.DeleteUser(ctx, ownerUser.ID))
	readAuthzCtx := dbauthz.AsSystemRestricted(ctx)
	_, err = api.Database.GetUserSkillByUserIDAndName(
		readAuthzCtx,
		database.GetUserSkillByUserIDAndNameParams{
			UserID: ownerUser.ID,
			Name:   "soft-delete-skill",
		},
	)
	require.ErrorIs(t, err, sql.ErrNoRows)

	createAuthzCtx := dbauthz.As(ctx, rbac.Subject{
		Type:  rbac.SubjectTypeUser,
		ID:    ownerUser.ID.String(),
		Roles: rbac.RoleIdentifiers{rbac.RoleMember()},
		Scope: rbac.ScopeAll,
	}.WithCachedASTValue())
	_, err = api.Database.InsertUserSkill(
		createAuthzCtx,
		database.InsertUserSkillParams{
			ID:          uuid.New(),
			UserID:      ownerUser.ID,
			Name:        "after-soft-delete",
			Description: "Soft delete",
			Content:     userSkillMarkdown("after-soft-delete", "Soft delete", "Body."),
		},
	)
	require.True(t, database.IsCheckViolation(err, database.CheckConstraint("user_skill_user_deleted")))
	require.ErrorContains(t, err, "Cannot create user_skill for deleted user")
}

func TestUserSkillDatabaseConstraints(t *testing.T) {
	t.Parallel()

	adminClient, _, api := coderdtest.NewWithAPI(t, nil)
	firstUser := coderdtest.CreateFirstUser(t, adminClient)
	_, ownerUser := coderdtest.CreateAnotherUser(t, adminClient, firstUser.OrganizationID)
	tests := []struct {
		name       string
		params     database.InsertUserSkillParams
		constraint database.CheckConstraint
	}{
		{
			name: "NameFormat",
			params: database.InsertUserSkillParams{
				ID:          uuid.New(),
				UserID:      ownerUser.ID,
				Name:        "not kebab",
				Description: "Invalid",
				Content:     userSkillMarkdown("not kebab", "Invalid", "Body."),
			},
			constraint: database.CheckSkillsNameFormat,
		},
		{
			name: "NameSize",
			params: database.InsertUserSkillParams{
				ID:          uuid.New(),
				UserID:      ownerUser.ID,
				Name:        strings.Repeat("a", skills.MaxPersonalSkillNameBytes+1),
				Description: "Invalid",
				Content:     userSkillMarkdown("too-long-name", "Invalid", "Body."),
			},
			constraint: database.CheckSkillsNameSize,
		},
		{
			name: "ContentSize",
			params: database.InsertUserSkillParams{
				ID:          uuid.New(),
				UserID:      ownerUser.ID,
				Name:        "content-too-large",
				Description: "Invalid",
				Content:     strings.Repeat("a", skills.MaxPersonalSkillSizeBytes+1),
			},
			constraint: database.CheckSkillsContentSize,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			authzCtx := dbauthz.As(ctx, coderdtest.AuthzUserSubject(ownerUser))

			_, err := api.Database.InsertUserSkill(authzCtx, tt.params)
			require.True(t, database.IsCheckViolation(err, tt.constraint), "expected %s, got %v", tt.constraint, err)
		})
	}
}

func TestUserSkillSchemaConstants(t *testing.T) {
	t.Parallel()

	_, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)

	ctx := testutil.Context(t, testutil.WaitMedium)
	var triggerDef string
	require.NoError(t, sqlDB.QueryRowContext(
		ctx,
		`SELECT pg_get_functiondef('enforce_skills_per_owner_limit'::regproc)`,
	).Scan(&triggerDef))
	assert.Contains(t, triggerDef, fmt.Sprintf("skill_limit constant int := %d;", skills.MaxPersonalSkillsPerUser))

	constraints := map[database.CheckConstraint]string{
		database.CheckSkillsNameSize:    fmt.Sprintf("octet_length(name) <= %d", skills.MaxPersonalSkillNameBytes),
		database.CheckSkillsNameFormat:  "name ~ '^[a-z0-9]+(-[a-z0-9]+)*$'::text",
		database.CheckSkillsContentSize: fmt.Sprintf("octet_length(content) <= %d", skills.MaxPersonalSkillSizeBytes),
	}
	for constraint, expected := range constraints {
		t.Run(string(constraint), func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitMedium)
			var constraintDef string
			require.NoError(t, sqlDB.QueryRowContext(
				ctx,
				`SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = $1`,
				constraint,
			).Scan(&constraintDef))
			assert.Contains(t, constraintDef, expected)
		})
	}
}

func TestSkillAudit(t *testing.T) {
	t.Parallel()

	for _, tc := range skillOwnerCases {
		//nolint:paralleltest,tparallel // Subtests share one auditor and run sequentially.
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			auditor := audit.NewMock()
			owner, _ := tc.setup(t, &coderdtest.Options{Auditor: auditor})
			ctx := testutil.Context(t, testutil.WaitMedium)
			auditor.ResetLogs()

			genName := func(t *testing.T) string {
				return strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
			}
			requireLog := func(t *testing.T, log database.AuditLog, action database.AuditAction, status int, skill codersdk.Skill) {
				t.Helper()
				assert.Equal(t, action, log.Action)
				assert.EqualValues(t, status, log.StatusCode)
				assert.Equal(t, owner.resourceType, log.ResourceType)
				assert.Equal(t, owner.auditOrgID, log.OrganizationID)
				assert.Equal(t, skill.ID, log.ResourceID)
				assert.Equal(t, skill.Name, log.ResourceTarget)
			}

			t.Run("CreateEmitsLog", func(t *testing.T) {
				auditor.ResetLogs()
				skill, err := owner.create(ctx, codersdk.CreateSkillRequest{Content: userSkillMarkdown(genName(t), "Audit", "Body.")})
				require.NoError(t, err)

				logs := auditor.AuditLogs()
				require.Len(t, logs, 1)
				requireLog(t, logs[0], database.AuditActionCreate, http.StatusCreated, skill)
			})

			t.Run("UpdateEmitsLog", func(t *testing.T) {
				name := genName(t)
				skill, err := owner.create(ctx, codersdk.CreateSkillRequest{Content: userSkillMarkdown(name, "Initial", "Body.")})
				require.NoError(t, err)
				auditor.ResetLogs()
				_, err = owner.update(ctx, name, codersdk.UpdateSkillRequest{
					Content: ptr.Ref(userSkillMarkdown(name, "Updated", "Updated body.")),
				})
				require.NoError(t, err)

				logs := auditor.AuditLogs()
				require.Len(t, logs, 1)
				requireLog(t, logs[0], database.AuditActionWrite, http.StatusOK, skill)
			})

			t.Run("DeleteEmitsLog", func(t *testing.T) {
				name := genName(t)
				skill, err := owner.create(ctx, codersdk.CreateSkillRequest{Content: userSkillMarkdown(name, "Delete", "Body.")})
				require.NoError(t, err)
				auditor.ResetLogs()
				require.NoError(t, owner.delete(ctx, name))

				logs := auditor.AuditLogs()
				require.Len(t, logs, 1)
				requireLog(t, logs[0], database.AuditActionDelete, http.StatusNoContent, skill)
			})

			t.Run("ReadsDoNotEmitLogs", func(t *testing.T) {
				name := genName(t)
				_, err := owner.create(ctx, codersdk.CreateSkillRequest{Content: userSkillMarkdown(name, "Read", "Body.")})
				require.NoError(t, err)
				auditor.ResetLogs()

				_, err = owner.list(ctx)
				require.NoError(t, err)
				_, err = owner.get(ctx, name)
				require.NoError(t, err)
				assert.Empty(t, auditor.AuditLogs())
			})

			t.Run("ValidationFailureDoesNotEmitLog", func(t *testing.T) {
				auditor.ResetLogs()
				_, err := owner.create(ctx, codersdk.CreateSkillRequest{Content: userSkillMarkdown("bad-name", "Invalid", "   \n")})
				requireSDKErrorStatus(t, err, http.StatusBadRequest)
				assert.Empty(t, auditor.AuditLogs())
			})

			t.Run("MissingSkillFailuresDoNotEmitLogs", func(t *testing.T) {
				auditor.ResetLogs()
				_, err := owner.update(ctx, "missing-audit-skill", codersdk.UpdateSkillRequest{
					Content: ptr.Ref(userSkillMarkdown("missing-audit-skill", "Missing", "Body.")),
				})
				requireSDKErrorStatus(t, err, http.StatusNotFound)
				err = owner.delete(ctx, "missing-audit-skill")
				requireSDKErrorStatus(t, err, http.StatusNotFound)
				assert.Empty(t, auditor.AuditLogs())
			})
		})
	}
}

func userSkillMarkdown(name string, description string, body string) string {
	return fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s\n", name, description, body)
}

func requireSDKErrorStatus(t *testing.T, err error, status int, msgAndArgs ...any) *codersdk.Error {
	t.Helper()
	require.Error(t, err, msgAndArgs...)
	sdkErr := coderdtest.SDKError(t, err)
	require.Equal(t, status, sdkErr.StatusCode(), msgAndArgs...)
	return sdkErr
}

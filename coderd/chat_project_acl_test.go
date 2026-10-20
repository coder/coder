package coderd_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestChatProjectSharing(t *testing.T) {
	t.Parallel()

	t.Run("User", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		mAudit := audit.NewMock()
		client, _ := newChatProjectClient(t, func(opts *coderdtest.Options) {
			opts.Auditor = mAudit
		})
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)
		project := createChatProject(t, client, firstUser.OrganizationID, "Shared Project")
		memory, err := client.CreateChatProjectMemory(ctx, project.OrganizationID, project.ID, codersdk.CreateChatProjectMemoryRequest{
			Name:        "shared-notes",
			Description: "Notes for sharees",
			Body:        "Sharees read and write project memories.",
		})
		require.NoError(t, err)
		_, err = client.UpdateChatProjectInstructions(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectInstructionsRequest{Instructions: "Owner instructions."})
		require.NoError(t, err)

		sharee, shareeUser := newChatProjectMember(t, client, firstUser.OrganizationID)
		_, err = sharee.GetChatProject(ctx, project.OrganizationID, project.ID)
		requireSDKError(t, err, http.StatusNotFound)
		_, err = sharee.ChatProjectACL(ctx, project.OrganizationID, project.ID)
		requireSDKError(t, err, http.StatusNotFound)
		err = sharee.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{firstUser.UserID.String(): codersdk.ChatProjectRoleUse},
		})
		requireSDKError(t, err, http.StatusNotFound)

		mAudit.ResetLogs()
		err = client.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{shareeUser.ID.String(): codersdk.ChatProjectRoleUse},
		})
		require.NoError(t, err)
		require.True(t, mAudit.Contains(t, database.AuditLog{
			Action:       database.AuditActionWrite,
			ResourceType: database.ResourceTypeChatProject,
			ResourceID:   project.ID,
			UserID:       firstUser.UserID,
		}))

		acl, err := client.ChatProjectACL(ctx, project.OrganizationID, project.ID)
		require.NoError(t, err)
		require.Empty(t, acl.Groups)
		require.Len(t, acl.Users, 1)
		require.Equal(t, shareeUser.ID, acl.Users[0].ID)
		require.Equal(t, codersdk.ChatProjectRoleUse, acl.Users[0].Role)

		requireChatProjectListed(t, sharee, project.ID)
		shared, err := sharee.GetChatProject(ctx, project.OrganizationID, project.ID)
		require.NoError(t, err)
		require.Equal(t, codersdk.ChatProjectPermissions{}, shared.Permissions)
		owned, err := client.GetChatProject(ctx, project.OrganizationID, project.ID)
		require.NoError(t, err)
		require.Equal(t, codersdk.ChatProjectPermissions{Update: true, Delete: true, Share: true}, owned.Permissions)
		memories, err := sharee.ListChatProjectMemories(ctx, project.OrganizationID, project.ID)
		require.NoError(t, err)
		require.Len(t, memories, 1)
		require.Equal(t, memory.ID, memories[0].ID)
		_, err = sharee.CreateChatProjectMemory(ctx, project.OrganizationID, project.ID, codersdk.CreateChatProjectMemoryRequest{
			Name:        "sharee-notes",
			Description: "Written by a sharee",
			Body:        "Sharee memory.",
		})
		require.NoError(t, err)
		require.NoError(t, sharee.DeleteChatProjectMemory(ctx, project.OrganizationID, project.ID, memory.ID))
		instructions, err := sharee.ChatProjectInstructions(ctx, project.OrganizationID, project.ID)
		require.NoError(t, err)
		require.Equal(t, "Owner instructions.", instructions.Instructions)
		_, err = sharee.UpdateChatProjectInstructions(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectInstructionsRequest{Instructions: "Sharee instructions."})
		requireSDKError(t, err, http.StatusForbidden)
		err = sharee.DeleteChatProjectInstructions(ctx, project.OrganizationID, project.ID)
		requireSDKError(t, err, http.StatusForbidden)
		shareeChat := createChatInProject(t, sharee, project.OrganizationID, &project.ID)
		require.Equal(t, &project.ID, shareeChat.ProjectID)
		_, err = sharee.ChatProjectACL(ctx, project.OrganizationID, project.ID)
		require.NoError(t, err)

		ownerChat := createChatInProject(t, client, project.OrganizationID, &project.ID)
		_, err = sharee.GetChat(ctx, ownerChat.ID)
		requireSDKError(t, err, http.StatusNotFound)

		name := "Renamed by sharee"
		_, err = sharee.UpdateChatProject(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectRequest{Name: &name})
		requireSDKError(t, err, http.StatusForbidden)
		err = sharee.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{firstUser.UserID.String(): codersdk.ChatProjectRoleUse},
		})
		requireSDKError(t, err, http.StatusForbidden)
		err = sharee.DeleteChatProject(ctx, project.OrganizationID, project.ID)
		requireSDKError(t, err, http.StatusForbidden)

		err = client.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{shareeUser.ID.String(): codersdk.ChatProjectRoleDeleted},
		})
		require.NoError(t, err)
		_, err = sharee.GetChatProject(ctx, project.OrganizationID, project.ID)
		requireSDKError(t, err, http.StatusNotFound)
		projects, err := sharee.ListChatProjects(ctx)
		require.NoError(t, err)
		require.Empty(t, projects)
		_, err = sharee.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: project.OrganizationID,
			ProjectID:      &project.ID,
			Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "revoked"}},
		})
		requireChatProjectNotFound(t, err)
	})

	t.Run("Admin", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _ := newChatProjectClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		project := createChatProject(t, client, firstUser.OrganizationID, "Admin Project")
		admin, adminUser := newChatProjectMember(t, client, firstUser.OrganizationID)
		_, otherUser := newChatProjectMember(t, client, firstUser.OrganizationID)

		err := client.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{adminUser.ID.String(): codersdk.ChatProjectRoleAdmin},
		})
		require.NoError(t, err)

		name := "Renamed by admin"
		updated, err := admin.UpdateChatProject(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectRequest{Name: &name})
		require.NoError(t, err)
		require.Equal(t, name, updated.Name)
		require.Equal(t, codersdk.ChatProjectPermissions{Update: true, Share: true}, updated.Permissions)
		acl, err := client.ChatProjectACL(ctx, project.OrganizationID, project.ID)
		require.NoError(t, err)
		require.Len(t, acl.Users, 1)
		require.Equal(t, codersdk.ChatProjectRoleAdmin, acl.Users[0].Role)

		err = admin.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{otherUser.ID.String(): codersdk.ChatProjectRoleUse},
		})
		require.NoError(t, err)
		err = admin.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{adminUser.ID.String(): codersdk.ChatProjectRoleUse},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Cannot change your own project sharing role.", sdkErr.Message)

		memory, err := admin.CreateChatProjectMemory(ctx, project.OrganizationID, project.ID, codersdk.CreateChatProjectMemoryRequest{
			Name:        "admin-notes",
			Description: "Written by an admin sharee",
			Body:        "Admin memory.",
		})
		require.NoError(t, err)
		require.NoError(t, admin.DeleteChatProjectMemory(ctx, project.OrganizationID, project.ID, memory.ID))
		_, err = admin.UpdateChatProjectInstructions(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectInstructionsRequest{Instructions: "Admin instructions."})
		require.NoError(t, err)
		instructions, err := client.ChatProjectInstructions(ctx, project.OrganizationID, project.ID)
		require.NoError(t, err)
		require.Equal(t, "Admin instructions.", instructions.Instructions)
		require.NoError(t, admin.DeleteChatProjectInstructions(ctx, project.OrganizationID, project.ID))
		instructions, err = client.ChatProjectInstructions(ctx, project.OrganizationID, project.ID)
		require.NoError(t, err)
		require.Empty(t, instructions.Instructions)

		err = admin.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{firstUser.UserID.String(): codersdk.ChatProjectRoleUse},
		})
		requireSDKError(t, err, http.StatusBadRequest)

		err = admin.DeleteChatProject(ctx, project.OrganizationID, project.ID)
		requireSDKError(t, err, http.StatusForbidden)
	})

	t.Run("GroupAndEveryone", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatProjectClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		groupProject := createChatProject(t, client, firstUser.OrganizationID, "Group Project")
		orgProject := createChatProject(t, client, firstUser.OrganizationID, "Organization Project")

		groupMember, groupMemberUser := newChatProjectMember(t, client, firstUser.OrganizationID)
		nonMember, _ := newChatProjectMember(t, client, firstUser.OrganizationID)
		group := dbgen.Group(t, db, database.Group{OrganizationID: firstUser.OrganizationID})
		dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: group.ID, UserID: groupMemberUser.ID})

		err := client.UpdateChatProjectACL(ctx, groupProject.OrganizationID, groupProject.ID, codersdk.UpdateChatProjectACL{
			GroupRoles: map[string]codersdk.ChatProjectRole{group.ID.String(): codersdk.ChatProjectRoleUse},
		})
		require.NoError(t, err)
		// The Everyone group's ID is the organization ID.
		err = client.UpdateChatProjectACL(ctx, orgProject.OrganizationID, orgProject.ID, codersdk.UpdateChatProjectACL{
			GroupRoles: map[string]codersdk.ChatProjectRole{firstUser.OrganizationID.String(): codersdk.ChatProjectRoleUse},
		})
		require.NoError(t, err)

		acl, err := client.ChatProjectACL(ctx, orgProject.OrganizationID, orgProject.ID)
		require.NoError(t, err)
		require.Len(t, acl.Groups, 1)
		require.Equal(t, firstUser.OrganizationID, acl.Groups[0].ID)
		require.Equal(t, database.EveryoneGroup, acl.Groups[0].Name)
		require.Equal(t, codersdk.ChatProjectRoleUse, acl.Groups[0].Role)

		projects, err := groupMember.ListChatProjects(ctx)
		require.NoError(t, err)
		require.ElementsMatch(t, []uuid.UUID{groupProject.ID, orgProject.ID}, chatProjectIDs(projects))
		projects, err = nonMember.ListChatProjects(ctx)
		require.NoError(t, err)
		require.ElementsMatch(t, []uuid.UUID{orgProject.ID}, chatProjectIDs(projects))
		_, err = nonMember.GetChatProject(ctx, groupProject.OrganizationID, groupProject.ID)
		requireSDKError(t, err, http.StatusNotFound)

		// Members of another organization do not reach the Everyone grant.
		otherOrganization := dbgen.Organization(t, db, database.Organization{IsDefault: false})
		outsiderRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, otherOrganization.ID)
		outsider := codersdk.NewExperimentalClient(outsiderRaw)
		projects, err = outsider.ListChatProjects(ctx)
		require.NoError(t, err)
		require.Empty(t, projects)

		err = client.UpdateChatProjectACL(ctx, groupProject.OrganizationID, groupProject.ID, codersdk.UpdateChatProjectACL{
			GroupRoles: map[string]codersdk.ChatProjectRole{group.ID.String(): codersdk.ChatProjectRoleDeleted},
		})
		require.NoError(t, err)
		err = client.UpdateChatProjectACL(ctx, orgProject.OrganizationID, orgProject.ID, codersdk.UpdateChatProjectACL{
			GroupRoles: map[string]codersdk.ChatProjectRole{firstUser.OrganizationID.String(): codersdk.ChatProjectRoleDeleted},
		})
		require.NoError(t, err)
		projects, err = groupMember.ListChatProjects(ctx)
		require.NoError(t, err)
		require.Empty(t, projects)
		_, err = nonMember.GetChatProject(ctx, orgProject.OrganizationID, orgProject.ID)
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("Validation", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatProjectClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		project := createChatProject(t, client, firstUser.OrganizationID, "Validated Project")
		_, member := newChatProjectMember(t, client, firstUser.OrganizationID)

		err := client.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{firstUser.UserID.String(): codersdk.ChatProjectRoleUse},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "The project owner cannot be added to the project's sharing list.", sdkErr.Message)

		err = client.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{
				member.ID.String():                  codersdk.ChatProjectRoleAdmin,
				strings.ToUpper(member.ID.String()): codersdk.ChatProjectRoleDeleted,
			},
		})
		sdkErr = requireSDKError(t, err, http.StatusBadRequest)
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "user_roles", sdkErr.Validations[0].Field)

		err = client.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{member.ID.String(): "owner"},
		})
		requireSDKError(t, err, http.StatusBadRequest)

		err = client.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{uuid.NewString(): codersdk.ChatProjectRoleUse},
		})
		requireSDKError(t, err, http.StatusBadRequest)

		otherOrganization := dbgen.Organization(t, db, database.Organization{IsDefault: false})
		foreignGroup := dbgen.Group(t, db, database.Group{OrganizationID: otherOrganization.ID})
		err = client.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			GroupRoles: map[string]codersdk.ChatProjectRole{foreignGroup.ID.String(): codersdk.ChatProjectRoleUse},
		})
		sdkErr = requireSDKError(t, err, http.StatusBadRequest)
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "group_roles", sdkErr.Validations[0].Field)

		_, foreignUser := coderdtest.CreateAnotherUser(t, client.Client, otherOrganization.ID)
		err = client.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{foreignUser.ID.String(): codersdk.ChatProjectRoleUse},
		})
		sdkErr = requireSDKError(t, err, http.StatusBadRequest)
		require.Len(t, sdkErr.Validations, 1)
		require.Equal(t, "user_roles", sdkErr.Validations[0].Field)

		acl, err := client.ChatProjectACL(ctx, project.OrganizationID, project.ID)
		require.NoError(t, err)
		require.Empty(t, acl.Users)
		require.Empty(t, acl.Groups)

		// Entries that skipped validation: out-of-org users grant nothing and
		// are hidden; foreign groups still grant their in-org members and show.
		ownerUser, err := client.User(ctx, codersdk.Me)
		require.NoError(t, err)
		ownerCtx := dbauthz.As(ctx, coderdtest.AuthzUserSubjectWithDB(ctx, t, db, ownerUser))
		require.NoError(t, db.UpdateChatProjectACLByID(ownerCtx, database.UpdateChatProjectACLByIDParams{
			ID:       project.ID,
			UserACL:  database.ChatACL{foreignUser.ID.String(): {Permissions: []policy.Action{policy.ActionRead}}},
			GroupACL: database.ChatACL{foreignGroup.ID.String(): {Permissions: []policy.Action{policy.ActionRead}}},
		}))
		acl, err = client.ChatProjectACL(ctx, project.OrganizationID, project.ID)
		require.NoError(t, err)
		require.Empty(t, acl.Users)
		require.Len(t, acl.Groups, 1)
		require.Equal(t, foreignGroup.ID, acl.Groups[0].ID)
		require.NoError(t, db.UpdateChatProjectACLByID(ownerCtx, database.UpdateChatProjectACLByIDParams{
			ID:       project.ID,
			UserACL:  database.ChatACL{},
			GroupACL: database.ChatACL{},
		}))

		err = client.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{strings.ToUpper(member.ID.String()): codersdk.ChatProjectRoleUse},
		})
		require.NoError(t, err)
		stored, err := db.GetChatProjectByID(dbauthz.AsSystemRestricted(ctx), project.ID)
		require.NoError(t, err)
		require.Contains(t, stored.UserACL, member.ID.String())
		err = client.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{"{" + member.ID.String() + "}": codersdk.ChatProjectRoleDeleted},
		})
		require.NoError(t, err)
		acl, err = client.ChatProjectACL(ctx, project.OrganizationID, project.ID)
		require.NoError(t, err)
		require.Empty(t, acl.Users)
	})

	t.Run("AdministratorCannotBindUnsharedProject", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _ := newChatProjectClient(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)
		member, _ := newChatProjectMember(t, client, firstUser.OrganizationID)
		memberProject := createChatProject(t, member, firstUser.OrganizationID, "Member Project")

		adminView, err := client.GetChatProject(ctx, memberProject.OrganizationID, memberProject.ID)
		require.NoError(t, err)
		require.Equal(t, codersdk.ChatProjectPermissions{Update: true, Delete: true, Share: true}, adminView.Permissions)
		_, err = client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: memberProject.OrganizationID,
			ProjectID:      &memberProject.ID,
			Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "not shared"}},
		})
		requireChatProjectNotFound(t, err)
	})
}

// deleteProjectBeforeInsertChatStore deletes the chat's project right before
// InsertChat, as a concurrent project delete would.
type deleteProjectBeforeInsertChatStore struct {
	database.Store
}

func (s deleteProjectBeforeInsertChatStore) InTx(function func(database.Store) error, txOpts *database.TxOptions) error {
	return s.Store.InTx(func(tx database.Store) error {
		return function(deleteProjectBeforeInsertChatStore{Store: tx})
	}, txOpts)
}

func (s deleteProjectBeforeInsertChatStore) InsertChat(ctx context.Context, arg database.InsertChatParams) (database.Chat, error) {
	if arg.ProjectID.Valid {
		if err := s.DeleteChatProjectByID(ctx, arg.ProjectID.UUID); err != nil {
			return database.Chat{}, err
		}
	}
	return s.Store.InsertChat(ctx, arg)
}

func TestPostChatsProjectDeletedBeforeInsert(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	rawDB, ps := dbtestutil.NewDB(t)
	client, _ := newChatProjectClient(t, func(opts *coderdtest.Options) {
		opts.Database = deleteProjectBeforeInsertChatStore{Store: rawDB}
		opts.Pubsub = ps
	})
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModel(t, client)
	project := createChatProject(t, client, firstUser.OrganizationID, "Racing Project")

	_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: project.OrganizationID,
		ProjectID:      &project.ID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
	})
	sdkErr := requireSDKError(t, err, http.StatusNotFound)
	require.Equal(t, "Chat project not found.", sdkErr.Message)
}

func TestChatProjectACLMemberRemovalBeforeGrant(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	client, _ := newChatProjectClient(t, func(opts *coderdtest.Options) {
		opts.Database = db
		opts.Pubsub = ps
	})
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID
	project := createChatProject(t, client, orgID, "Removal Race Project")
	sharee, shareeUser := newChatProjectMember(t, client, orgID)

	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	_, err = tx.ExecContext(ctx, `DELETE FROM organization_members WHERE organization_id = $1 AND user_id = $2`, orgID, shareeUser.ID)
	require.NoError(t, err)

	patchErr := make(chan error, 1)
	go func() {
		patchErr <- client.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{shareeUser.ID.String(): codersdk.ChatProjectRoleUse},
		})
	}()
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		if len(patchErr) > 0 {
			return true
		}
		var waits int
		err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pg_stat_activity
WHERE datname = current_database() AND pid <> pg_backend_pid() AND wait_event_type = 'Lock'`).Scan(&waits)
		return err == nil && waits > 0
	}, testutil.IntervalFast, "PATCH returns or waits for the membership lock")
	require.NoError(t, tx.Commit())

	err = testutil.RequireReceive(ctx, t, patchErr)
	requireSDKError(t, err, http.StatusBadRequest)

	var granted bool
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT user_acl ? $2 FROM chat_projects WHERE id = $1`, project.ID, shareeUser.ID.String()).Scan(&granted))
	require.False(t, granted)
	dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: orgID, UserID: shareeUser.ID})
	_, err = sharee.GetChatProject(ctx, orgID, project.ID)
	requireSDKError(t, err, http.StatusNotFound)
}

func newChatProjectMember(t testing.TB, client *codersdk.ExperimentalClient, organizationID uuid.UUID) (*codersdk.ExperimentalClient, codersdk.User) {
	t.Helper()
	raw, user := coderdtest.CreateAnotherUser(t, client.Client, organizationID)
	return codersdk.NewExperimentalClient(raw), user
}

func requireChatProjectListed(t testing.TB, client *codersdk.ExperimentalClient, projectID uuid.UUID) {
	t.Helper()
	projects, err := client.ListChatProjects(testutil.Context(t, testutil.WaitLong))
	require.NoError(t, err)
	require.Contains(t, chatProjectIDs(projects), projectID)
}

func chatProjectIDs(projects []codersdk.ChatProject) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(projects))
	for _, project := range projects {
		ids = append(ids, project.ID)
	}
	return ids
}

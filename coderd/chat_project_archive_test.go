package coderd_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestChatProjectArchive archives a project with chats of its owner and of
// a sharee, checks that an archived project is read-only, and unarchives
// it. Archiving needs the delete permission, so project ACL grants do not
// allow it.
func TestChatProjectArchive(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	siteOwner, api := newChatProjectClientWithAPI(t)
	firstUser := coderdtest.CreateFirstUser(t, siteOwner.Client)
	_ = createChatModel(t, siteOwner)
	orgID := firstUser.OrganizationID

	newMember := func(roles ...rbac.RoleIdentifier) (*codersdk.ExperimentalClient, codersdk.User) {
		raw, user := coderdtest.CreateAnotherUser(t, siteOwner.Client, orgID, roles...)
		return codersdk.NewExperimentalClient(raw), user
	}
	owner, _ := newMember()
	orgAdmin, _ := newMember(rbac.ScopedRoleOrgAdmin(orgID))
	sharedAdmin, sharedAdminUser := newMember()
	sharedUse, sharedUseUser := newMember()

	project := createChatProject(t, owner, orgID, "Archive me")
	require.NoError(t, owner.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACL{
		UserRoles: map[string]codersdk.ChatProjectRole{
			sharedAdminUser.ID.String(): codersdk.ChatProjectRoleAdmin,
			sharedUseUser.ID.String():   codersdk.ChatProjectRoleUse,
		},
	}))
	memory, err := owner.CreateChatProjectMemory(ctx, orgID, project.ID, codersdk.CreateChatProjectMemoryRequest{
		Name: "notes", Description: "Notes", Body: "Keep this.",
	})
	require.NoError(t, err)
	_, err = owner.CreateChatProjectSkill(ctx, orgID, project.ID, codersdk.CreateSkillRequest{
		Content: userSkillMarkdown("deploy", "Deploy", "Run make deploy."),
	})
	require.NoError(t, err)

	// The owner's chat is running, with an idle subagent chat. The sharee's
	// chat is idle. An archived chat stays archived after the unarchive.
	running := createChatInProject(t, owner, orgID, &project.ID)
	subagent := createSubagentChat(t, api, running, database.ChatStatusWaiting)
	shareeChat := createChatInProject(t, sharedUse, orgID, &project.ID)
	driveChatToWaiting(ctx, t, api, shareeChat.ID)
	archivedEarlier := createChatInProject(t, owner, orgID, &project.ID)
	driveChatToWaiting(ctx, t, api, archivedEarlier.ID)
	require.NoError(t, owner.UpdateChat(ctx, archivedEarlier.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(true)}))
	outside := createIdleChat(ctx, t, owner, api, orgID)

	for _, sharee := range []*codersdk.ExperimentalClient{sharedAdmin, sharedUse} {
		got, err := sharee.GetChatProject(ctx, orgID, project.ID)
		require.NoError(t, err)
		require.False(t, got.Permissions.Archive)
		_, err = sharee.ChatProjectArchivePreview(ctx, orgID, project.ID)
		requireSDKError(t, err, http.StatusForbidden)
		_, err = sharee.ArchiveChatProject(ctx, orgID, project.ID, codersdk.ArchiveChatProjectRequest{StopActiveChats: true})
		requireSDKError(t, err, http.StatusForbidden)
	}
	require.True(t, project.Permissions.Archive)

	preview, err := owner.ChatProjectArchivePreview(ctx, orgID, project.ID)
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatProjectArchivePreview{ChatCount: 2, ActiveChatCount: 1}, preview)

	_, err = owner.ArchiveChatProject(ctx, orgID, project.ID, codersdk.ArchiveChatProjectRequest{})
	sdkErr := requireSDKError(t, err, http.StatusConflict)
	require.Equal(t, "Chat project has 1 active chat. Set stop_active_chats to stop and archive it.", sdkErr.Message)
	// Requests without a body, or with an empty chunked body, are
	// refused like {}.
	archivePath := fmt.Sprintf("/api/experimental/organizations/%s/chats/projects/%s/archive", orgID, project.ID)
	for name, body := range map[string]any{
		"none": nil,
		// A reader of unknown length makes the client send a chunked body.
		"empty chunked": io.MultiReader(),
	} {
		res, err := owner.Request(ctx, http.MethodPost, archivePath, body)
		require.NoError(t, err, name)
		requireSDKError(t, codersdk.ReadBodyAsError(res), http.StatusConflict)
		_ = res.Body.Close()
	}
	requireChatArchived(ctx, t, owner, running.ID, false)

	archived, err := owner.ArchiveChatProject(ctx, orgID, project.ID, codersdk.ArchiveChatProjectRequest{StopActiveChats: true})
	require.NoError(t, err)
	require.NotNil(t, archived.ArchivedAt)
	requireChatArchived(ctx, t, owner, running.ID, true)
	requireChatArchived(ctx, t, owner, subagent.ID, true)
	requireChatArchived(ctx, t, sharedUse, shareeChat.ID, true)
	_, err = owner.ArchiveChatProject(ctx, orgID, project.ID, codersdk.ArchiveChatProjectRequest{StopActiveChats: true})
	requireSDKError(t, err, http.StatusConflict)

	active, err := owner.ListChatProjects(ctx)
	require.NoError(t, err)
	require.Empty(t, active)
	listedArchived, err := owner.ListChatProjects(ctx, codersdk.ListChatProjectsOptions{Archived: true})
	require.NoError(t, err)
	require.Len(t, listedArchived, 1)
	require.Equal(t, project.ID, listedArchived[0].ID)

	// An archived project is read-only, and its chats stay archived.
	requireArchivedProjectConflict := func(err error) {
		t.Helper()
		sdkErr := requireSDKError(t, err, http.StatusConflict)
		require.Equal(t, "Chat project is archived.", sdkErr.Message)
	}
	_, err = sharedUse.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: orgID,
		ProjectID:      &project.ID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "refused"}},
	})
	requireArchivedProjectConflict(err)
	requireArchivedProjectConflict(owner.UpdateChat(ctx, outside.ID, codersdk.UpdateChatRequest{ProjectID: &project.ID}))
	requireChatProjectID(ctx, t, owner, outside.ID, nil)
	err = sharedUse.UpdateChat(ctx, shareeChat.ID, codersdk.UpdateChatRequest{Archived: ptr.Ref(false)})
	requireSDKError(t, err, http.StatusConflict)
	requireChatArchived(ctx, t, sharedUse, shareeChat.ID, true)
	_, err = owner.UpdateChatProject(ctx, orgID, project.ID, codersdk.UpdateChatProjectRequest{Name: ptr.Ref("Renamed")})
	requireArchivedProjectConflict(err)
	requireArchivedProjectConflict(owner.UpdateChatProjectACL(ctx, orgID, project.ID, codersdk.UpdateChatProjectACL{
		UserRoles: map[string]codersdk.ChatProjectRole{sharedUseUser.ID.String(): codersdk.ChatProjectRoleDeleted},
	}))
	_, err = owner.UpdateChatProjectInstructions(ctx, orgID, project.ID, codersdk.UpdateChatProjectInstructionsRequest{Instructions: "refused"})
	requireArchivedProjectConflict(err)
	requireArchivedProjectConflict(owner.DeleteChatProjectInstructions(ctx, orgID, project.ID))
	_, err = owner.CreateChatProjectSkill(ctx, orgID, project.ID, codersdk.CreateSkillRequest{
		Content: userSkillMarkdown("refused", "Refused", "Refused."),
	})
	requireArchivedProjectConflict(err)
	_, err = owner.UpdateChatProjectSkill(ctx, orgID, project.ID, "deploy", codersdk.UpdateSkillRequest{Enabled: ptr.Ref(false)})
	requireArchivedProjectConflict(err)
	requireArchivedProjectConflict(owner.DeleteChatProjectSkill(ctx, orgID, project.ID, "deploy"))
	_, err = owner.CreateChatProjectMemory(ctx, orgID, project.ID, codersdk.CreateChatProjectMemoryRequest{
		Name: "refused", Description: "Refused", Body: "Refused.",
	})
	requireArchivedProjectConflict(err)
	requireArchivedProjectConflict(owner.DeleteChatProjectMemory(ctx, orgID, project.ID, memory.ID))

	// Unarchiving needs the delete permission too, so project ACL grants
	// do not allow it. An org admin can unarchive; only chats archived
	// with the project come back.
	for _, sharee := range []*codersdk.ExperimentalClient{sharedAdmin, sharedUse} {
		_, err = sharee.UnarchiveChatProject(ctx, orgID, project.ID)
		requireSDKError(t, err, http.StatusForbidden)
	}
	unarchived, err := orgAdmin.UnarchiveChatProject(ctx, orgID, project.ID)
	require.NoError(t, err)
	require.Nil(t, unarchived.ArchivedAt)
	requireChatArchived(ctx, t, owner, running.ID, false)
	requireChatArchived(ctx, t, owner, subagent.ID, false)
	requireChatArchived(ctx, t, sharedUse, shareeChat.ID, false)
	requireChatArchived(ctx, t, owner, archivedEarlier.ID, true)
	_, err = orgAdmin.UnarchiveChatProject(ctx, orgID, project.ID)
	requireSDKError(t, err, http.StatusConflict)

	// The site owner can archive too, and an archived project can still be
	// deleted, which detaches its chats and leaves them archived.
	_, err = siteOwner.ArchiveChatProject(ctx, orgID, project.ID, codersdk.ArchiveChatProjectRequest{StopActiveChats: true})
	require.NoError(t, err)
	require.NoError(t, owner.DeleteChatProject(ctx, orgID, project.ID))
	requireChatArchived(ctx, t, owner, running.ID, true)
	requireChatProjectID(ctx, t, owner, running.ID, nil)
}

func requireChatArchived(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, chatID uuid.UUID, want bool) {
	t.Helper()
	chat, err := client.GetChat(ctx, chatID)
	require.NoError(t, err)
	require.Equal(t, want, chat.Archived)
}

// TestChatProjectWriteRacingArchive starts each project write while an
// archive holds the project row lock, then archives the project. The write
// passed the early archived check, so it must wait for the lock and then
// reject the archived project.
func TestChatProjectWriteRacingArchive(t *testing.T) {
	t.Parallel()

	db, ps, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	client, _ := newChatProjectClient(t, func(opts *coderdtest.Options) {
		opts.Database = db
		opts.Pubsub = ps
	})
	orgID := coderdtest.CreateFirstUser(t, client.Client).OrganizationID
	_, member := newChatProjectMember(t, client, orgID)

	for _, tc := range []struct {
		name  string
		write func(ctx context.Context, projectID, memoryID uuid.UUID) error
	}{
		{name: "UpdateProject", write: func(ctx context.Context, projectID, _ uuid.UUID) error {
			_, err := client.UpdateChatProject(ctx, orgID, projectID, codersdk.UpdateChatProjectRequest{Name: ptr.Ref("Renamed")})
			return err
		}},
		{name: "UpdateACL", write: func(ctx context.Context, projectID, _ uuid.UUID) error {
			return client.UpdateChatProjectACL(ctx, orgID, projectID, codersdk.UpdateChatProjectACL{
				UserRoles: map[string]codersdk.ChatProjectRole{member.ID.String(): codersdk.ChatProjectRoleUse},
			})
		}},
		{name: "UpdateInstructions", write: func(ctx context.Context, projectID, _ uuid.UUID) error {
			_, err := client.UpdateChatProjectInstructions(ctx, orgID, projectID, codersdk.UpdateChatProjectInstructionsRequest{Instructions: "Changed."})
			return err
		}},
		{name: "DeleteInstructions", write: func(ctx context.Context, projectID, _ uuid.UUID) error {
			return client.DeleteChatProjectInstructions(ctx, orgID, projectID)
		}},
		{name: "CreateSkill", write: func(ctx context.Context, projectID, _ uuid.UUID) error {
			_, err := client.CreateChatProjectSkill(ctx, orgID, projectID, codersdk.CreateSkillRequest{
				Content: userSkillMarkdown("other", "Other", "Other."),
			})
			return err
		}},
		{name: "UpdateSkill", write: func(ctx context.Context, projectID, _ uuid.UUID) error {
			_, err := client.UpdateChatProjectSkill(ctx, orgID, projectID, "deploy", codersdk.UpdateSkillRequest{Enabled: ptr.Ref(false)})
			return err
		}},
		{name: "DeleteSkill", write: func(ctx context.Context, projectID, _ uuid.UUID) error {
			return client.DeleteChatProjectSkill(ctx, orgID, projectID, "deploy")
		}},
		{name: "CreateMemory", write: func(ctx context.Context, projectID, _ uuid.UUID) error {
			_, err := client.CreateChatProjectMemory(ctx, orgID, projectID, codersdk.CreateChatProjectMemoryRequest{
				Name: "other", Description: "Other", Body: "Other.",
			})
			return err
		}},
		{name: "DeleteMemory", write: func(ctx context.Context, projectID, memoryID uuid.UUID) error {
			return client.DeleteChatProjectMemory(ctx, orgID, projectID, memoryID)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			project := createChatProject(t, client, orgID, "Race "+tc.name)
			memory, err := client.CreateChatProjectMemory(ctx, orgID, project.ID, codersdk.CreateChatProjectMemoryRequest{
				Name: "notes", Description: "Notes", Body: "Keep this.",
			})
			require.NoError(t, err)
			_, err = client.CreateChatProjectSkill(ctx, orgID, project.ID, codersdk.CreateSkillRequest{
				Content: userSkillMarkdown("deploy", "Deploy", "Run make deploy."),
			})
			require.NoError(t, err)
			_, err = client.UpdateChatProjectInstructions(ctx, orgID, project.ID, codersdk.UpdateChatProjectInstructionsRequest{Instructions: "Be brief."})
			require.NoError(t, err)

			archive, err := sqlDB.BeginTx(ctx, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = archive.Rollback() })
			var archivePID int
			require.NoError(t, archive.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&archivePID))
			_, err = archive.ExecContext(ctx, `SELECT id FROM chat_projects WHERE id = $1 FOR UPDATE`, project.ID)
			require.NoError(t, err)

			writeErr := make(chan error, 1)
			go func() { writeErr <- tc.write(ctx, project.ID, memory.ID) }()
			testutil.Eventually(ctx, t, func(ctx context.Context) bool {
				var waiting bool
				err := sqlDB.QueryRowContext(ctx,
					`SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE $1::int = ANY(pg_blocking_pids(pid)))`,
					archivePID).Scan(&waiting)
				return err == nil && waiting
			}, testutil.IntervalFast, "wait for the write to block on the project lock")
			_, err = archive.ExecContext(ctx, `UPDATE chat_projects SET archived_at = now() WHERE id = $1`, project.ID)
			require.NoError(t, err)
			require.NoError(t, archive.Commit())

			sdkErr := requireSDKError(t, testutil.RequireReceive(ctx, t, writeErr), http.StatusConflict)
			require.Equal(t, "Chat project is archived.", sdkErr.Message)
		})
	}
}

func TestChatProjectArchivePins(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID

	a := createChatProject(t, client, orgID, "Archive pin A")
	b := createChatProject(t, client, orgID, "Archive pin B")
	c := createChatProject(t, client, orgID, "Archive pin C")
	_, err := client.ArchiveChatProject(ctx, orgID, b.ID, codersdk.ArchiveChatProjectRequest{})
	require.NoError(t, err)

	// An archived project leaves the summaries, and its pin takes no
	// position, either in summaries or in requested positions.
	require.NotContains(t, chatProjectSummariesByID(ctx, t, client), b.ID)
	requirePinOrders(ctx, t, client, map[uuid.UUID]int32{a.ID: 1, c.ID: 2})
	require.NoError(t, client.PinChatProject(ctx, orgID, c.ID, codersdk.PinChatProjectRequest{PinOrder: ptr.Ref(int32(1))}))
	requirePinOrders(ctx, t, client, map[uuid.UUID]int32{c.ID: 1, a.ID: 2})
	sdkErr := requireSDKError(t, client.PinChatProject(ctx, orgID, b.ID, codersdk.PinChatProjectRequest{}), http.StatusConflict)
	require.Equal(t, "Chat project is archived.", sdkErr.Message)

	// Unarchiving brings the pin back after the visible ones.
	_, err = client.UnarchiveChatProject(ctx, orgID, b.ID)
	require.NoError(t, err)
	requirePinOrders(ctx, t, client, map[uuid.UUID]int32{c.ID: 1, a.ID: 2, b.ID: 3})
}

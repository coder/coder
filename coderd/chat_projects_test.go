package coderd_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestChatProjectsCRUDListAndDeleteDetaches(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModel(t, client)

	project := createChatProject(t, client, firstUser.OrganizationID, "Project One")
	otherOrganization := dbgen.Organization(t, db, database.Organization{IsDefault: false})
	otherOrganizationProject := dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID: otherOrganization.ID,
		OwnerID:        firstUser.UserID,
		Name:           "Other Organization Project",
	})

	// The list spans organizations so clients can load it in one request.
	projects, err := client.ListChatProjects(ctx)
	require.NoError(t, err)
	require.ElementsMatch(t,
		[]uuid.UUID{project.ID, otherOrganizationProject.ID},
		[]uuid.UUID{projects[0].ID, projects[1].ID},
	)

	chat := createChatInProject(t, client, firstUser.OrganizationID, &project.ID)

	fetched, err := client.GetChatProject(ctx, firstUser.OrganizationID, project.ID)
	require.NoError(t, err)
	require.Equal(t, project.ID, fetched.ID)

	// The project exists and is readable, but not under this organization.
	_, err = client.GetChatProject(ctx, otherOrganization.ID, project.ID)
	require.Equal(t, 404, coderdtest.SDKError(t, err).StatusCode())

	updatedName := "Renamed Project"
	updatedDescription := "Updated description"
	updatedIcon := " /emojis/1f680.png "
	updated, err := client.UpdateChatProject(ctx, firstUser.OrganizationID, project.ID, codersdk.UpdateChatProjectRequest{
		Name:        &updatedName,
		Description: &updatedDescription,
		Icon:        &updatedIcon,
	})
	require.NoError(t, err)
	require.Equal(t, updatedName, updated.Name)
	require.Equal(t, updatedDescription, updated.Description)
	require.Equal(t, "/emojis/1f680.png", updated.Icon)

	// Omitted fields keep their stored values.
	keptName := "Renamed Again"
	updated, err = client.UpdateChatProject(ctx, firstUser.OrganizationID, project.ID, codersdk.UpdateChatProjectRequest{Name: &keptName})
	require.NoError(t, err)
	require.Equal(t, "/emojis/1f680.png", updated.Icon)

	longIcon := strings.Repeat("x", 257)
	_, err = client.UpdateChatProject(ctx, firstUser.OrganizationID, project.ID, codersdk.UpdateChatProjectRequest{Icon: &longIcon})
	requireChatProjectFieldError(t, err, "icon", "Icon must be at most 256 characters.")

	blankName := "   "
	_, err = client.UpdateChatProject(ctx, firstUser.OrganizationID, project.ID, codersdk.UpdateChatProjectRequest{Name: &blankName})
	requireChatProjectFieldError(t, err, "name", "Name must not be blank.")

	// Names are labels, not identifiers: the same owner may reuse one
	// through create or rename, and each project stays addressable by ID.
	duplicate := createChatProject(t, client, firstUser.OrganizationID, keptName)
	require.Equal(t, keptName, duplicate.Name)
	require.NotEqual(t, project.ID, duplicate.ID)
	duplicateAgain := createChatProject(t, client, firstUser.OrganizationID, keptName)
	require.NotEqual(t, duplicate.ID, duplicateAgain.ID)
	projects, err = client.ListChatProjects(ctx)
	require.NoError(t, err)
	var sameName []uuid.UUID
	for _, listed := range projects {
		if listed.Name == keptName {
			sameName = append(sameName, listed.ID)
		}
	}
	require.ElementsMatch(t, []uuid.UUID{project.ID, duplicate.ID, duplicateAgain.ID}, sameName)
	fetched, err = client.GetChatProject(ctx, firstUser.OrganizationID, duplicate.ID)
	require.NoError(t, err)
	require.Equal(t, duplicate.ID, fetched.ID)

	require.NoError(t, client.DeleteChatProject(ctx, firstUser.OrganizationID, project.ID))
	storedChat, err := client.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Nil(t, storedChat.ProjectID)
}

func TestChatProjectsAuthorizationAndCrossOrganizationBinding(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModel(t, client)
	project := createChatProject(t, client, firstUser.OrganizationID, "Protected Project")

	// Projects are private to their creator until shared: another member
	// neither sees nor can bind chats to it.
	memberRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
	member := codersdk.NewExperimentalClient(memberRaw)
	_, err := member.GetChatProject(ctx, firstUser.OrganizationID, project.ID)
	require.Equal(t, 404, coderdtest.SDKError(t, err).StatusCode())
	projects, err := member.ListChatProjects(ctx)
	require.NoError(t, err)
	require.Empty(t, projects)
	_, err = member.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		ProjectID:      &project.ID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "reject private project",
		}},
	})
	requireChatProjectNotFound(t, err)

	_, err = member.UpdateChatProject(ctx, firstUser.OrganizationID, project.ID, codersdk.UpdateChatProjectRequest{})
	require.Equal(t, 404, coderdtest.SDKError(t, err).StatusCode())
	err = member.DeleteChatProject(ctx, firstUser.OrganizationID, project.ID)
	require.Equal(t, 404, coderdtest.SDKError(t, err).StatusCode())

	// Members still create their own projects, which only they see.
	memberProject := createChatProject(t, member, firstUser.OrganizationID, "Member Project")
	projects, err = member.ListChatProjects(ctx)
	require.NoError(t, err)
	require.Len(t, projects, 1)
	require.Equal(t, memberProject.ID, projects[0].ID)
	// The list holds projects the caller owns or holds a grant on, not every
	// project a site owner can read by ID.
	projects, err = client.ListChatProjects(ctx)
	require.NoError(t, err)
	require.Len(t, projects, 1)
	require.Equal(t, project.ID, projects[0].ID)
	_, err = client.GetChatProject(ctx, firstUser.OrganizationID, memberProject.ID)
	require.NoError(t, err)

	otherOrganization := dbgen.Organization(t, db, database.Organization{IsDefault: false})
	otherProject := dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID: otherOrganization.ID,
		OwnerID:        firstUser.UserID,
		Name:           "Other Project",
	})
	otherMemberRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, otherOrganization.ID)
	otherMember := codersdk.NewExperimentalClient(otherMemberRaw)
	_, err = otherMember.GetChatProject(ctx, otherOrganization.ID, project.ID)
	require.Equal(t, 404, coderdtest.SDKError(t, err).StatusCode())

	_, err = client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		ProjectID:      &otherProject.ID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "reject cross-organization project",
		}},
	})
	sdkErr := coderdtest.SDKError(t, err)
	require.Equal(t, 400, sdkErr.StatusCode())
	require.Equal(t, "Chat project does not belong to this chat's organization.", sdkErr.Message)

	adminRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID,
		rbac.ScopedRoleOrgAdmin(firstUser.OrganizationID))
	admin := codersdk.NewExperimentalClient(adminRaw)
	adminName := "Updated by admin"
	_, err = admin.UpdateChatProject(ctx, firstUser.OrganizationID, project.ID, codersdk.UpdateChatProjectRequest{Name: &adminName})
	require.NoError(t, err)

	_, memberUser := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
	_, err = client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		OwnerID:        &memberUser.ID,
		ProjectID:      &project.ID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "reject binding another user's chat",
		}},
	})
	requireChatProjectNotFound(t, err)

	missingProjectID := uuid.New()
	_, err = client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		ProjectID:      &missingProjectID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "reject missing project",
		}},
	})
	requireChatProjectNotFound(t, err)
	require.NoError(t, admin.DeleteChatProject(ctx, firstUser.OrganizationID, project.ID))
}

func TestChatProjectFieldLimits(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)

	_, err := client.CreateChatProject(ctx, firstUser.OrganizationID, codersdk.CreateChatProjectRequest{
		Name: strings.Repeat("n", 65),
	})
	requireChatProjectFieldError(t, err, "name", "Name must be at most 64 characters.")
	_, err = client.CreateChatProject(ctx, firstUser.OrganizationID, codersdk.CreateChatProjectRequest{
		Name: "   ",
	})
	requireChatProjectFieldError(t, err, "name", "Name is required.")
	// Surrounding whitespace is trimmed before the length check.
	project := createChatProject(t, client, firstUser.OrganizationID, " "+strings.Repeat("n", 64)+" ")
	require.Equal(t, strings.Repeat("n", 64), project.Name)
	longDescription := strings.Repeat("d", 1025)
	_, err = client.UpdateChatProject(ctx, firstUser.OrganizationID, project.ID, codersdk.UpdateChatProjectRequest{Description: &longDescription})
	requireChatProjectFieldError(t, err, "description", "Description must be at most 1024 characters.")
}

func TestChatProjectLimit(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	// The cap counts the owner's projects in every organization.
	otherOrganization := dbgen.Organization(t, db, database.Organization{})
	var last database.ChatProject
	for i := range 100 {
		organizationID := firstUser.OrganizationID
		if i%2 == 0 {
			organizationID = otherOrganization.ID
		}
		last = dbgen.ChatProject(t, db, database.ChatProject{OrganizationID: organizationID, OwnerID: firstUser.UserID})
	}

	_, err := client.CreateChatProject(ctx, firstUser.OrganizationID, codersdk.CreateChatProjectRequest{Name: "One too many"})
	require.Equal(t, http.StatusConflict, coderdtest.SDKError(t, err).StatusCode())

	// Another user's projects do not count toward the cap.
	memberRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
	_ = createChatProject(t, codersdk.NewExperimentalClient(memberRaw), firstUser.OrganizationID, "Member project")

	require.NoError(t, client.DeleteChatProject(ctx, last.OrganizationID, last.ID))
	_ = createChatProject(t, client, firstUser.OrganizationID, "Fits after a delete")
}

func TestChatProjectListFilter(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, api := newChatProjectClientWithAPI(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModel(t, client)
	projectA := createChatProject(t, client, firstUser.OrganizationID, "Project A")
	projectB := createChatProject(t, client, firstUser.OrganizationID, "Project B")

	chat := createChatInProject(t, client, firstUser.OrganizationID, &projectA.ID)
	otherChat := createChatInProject(t, client, firstUser.OrganizationID, &projectB.ID)
	require.Equal(t, &projectA.ID, chat.ProjectID)

	chats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{ProjectID: &projectA.ID})
	require.NoError(t, err)
	require.Len(t, chats, 1)
	require.Equal(t, chat.ID, chats[0].ID)
	require.NotEqual(t, otherChat.ID, chats[0].ID)

	driveChatToWaiting(ctx, t, api, chat.ID)
	require.NoError(t, client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ProjectID: &projectB.ID}))
	chats, err = client.ListChats(ctx, &codersdk.ListChatsOptions{ProjectID: &projectB.ID})
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{chat.ID, otherChat.ID}, chatIDs(chats))
}

func TestPatchChatProject(t *testing.T) {
	t.Parallel()

	t.Run("MovesIntoBetweenAndOut", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, api := newChatProjectClientWithAPI(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)
		projectA := createChatProject(t, client, firstUser.OrganizationID, "Project A")
		projectB := createChatProject(t, client, firstUser.OrganizationID, "Project B")
		chat := createIdleChat(ctx, t, client, api, firstUser.OrganizationID)
		child := createSubagentChat(t, api, chat, database.ChatStatusWaiting)

		requireProjectChats := func(project codersdk.ChatProject, want ...uuid.UUID) {
			t.Helper()
			chats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{ProjectID: &project.ID})
			require.NoError(t, err)
			require.ElementsMatch(t, want, chatIDs(chats), "chats of %s", project.Name)
		}

		require.NoError(t, client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ProjectID: &projectA.ID}))
		requireChatProjectID(ctx, t, client, chat.ID, &projectA.ID)
		requireProjectChats(projectA, chat.ID)

		require.NoError(t, client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ProjectID: &projectB.ID}))
		requireChatProjectID(ctx, t, client, chat.ID, &projectB.ID)
		requireProjectChats(projectA)
		requireProjectChats(projectB, chat.ID)

		require.NoError(t, client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ProjectID: new(uuid.Nil)}))
		requireChatProjectID(ctx, t, client, chat.ID, nil)
		requireProjectChats(projectB)

		// Subagent chats have no project and use the root's.
		requireChatProjectID(ctx, t, client, child.ID, nil)
	})

	t.Run("MoveBackWaitsForAPendingMove", func(t *testing.T) {
		t.Parallel()

		// A move back to the current project compares with the locked row,
		// so it cannot succeed while an earlier move commits after it.
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
		client, api := newChatProjectClientWithAPI(t, func(options *coderdtest.Options) {
			options.Database = db
		})
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)
		projectA := createChatProject(t, client, firstUser.OrganizationID, "Project A")
		projectB := createChatProject(t, client, firstUser.OrganizationID, "Project B")
		chat := createIdleChat(ctx, t, client, api, firstUser.OrganizationID)
		require.NoError(t, client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ProjectID: &projectA.ID}))

		pendingMove, err := sqlDB.BeginTx(ctx, nil)
		require.NoError(t, err)
		t.Cleanup(func() {
			if err := pendingMove.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				t.Errorf("roll back the pending move: %v", err)
			}
		})
		_, err = pendingMove.ExecContext(ctx, `UPDATE chats SET project_id = $1 WHERE id = $2`, projectB.ID, chat.ID)
		require.NoError(t, err)

		moveBack := make(chan error, 1)
		go func() {
			moveBack <- client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ProjectID: &projectA.ID})
		}()
		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			if len(moveBack) > 0 {
				return true
			}
			var waiting int
			err := sqlDB.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM pg_stat_activity
WHERE datname = current_database()
	AND pid <> pg_backend_pid()
	AND wait_event_type = 'Lock'
	AND query LIKE '%-- name: GetChatByIDForUpdate%'
`).Scan(&waiting)
			return err == nil && waiting == 1
		}, testutil.IntervalFast, "the move back must answer or wait for the chat row")
		require.NoError(t, pendingMove.Commit())

		require.NoError(t, testutil.RequireReceive(ctx, t, moveBack))
		requireChatProjectID(ctx, t, client, chat.ID, &projectA.ID)
	})

	t.Run("AnotherOwnersProject", func(t *testing.T) {
		t.Parallel()

		// The site owner can read every project and update every chat, so
		// only the access check keeps a member's chat out of the site
		// owner's project and its memory until the project is shared with
		// the member.
		ctx := testutil.Context(t, testutil.WaitLong)
		client, api := newChatProjectClientWithAPI(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)
		project := createChatProject(t, client, firstUser.OrganizationID, "Owner Project")
		member, memberUser := newChatProjectMember(t, client, firstUser.OrganizationID)
		chat := createIdleChat(ctx, t, member, api, firstUser.OrganizationID)

		err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ProjectID: &project.ID})
		requireChatProjectNotFound(t, err)
		requireChatProjectID(ctx, t, member, chat.ID, nil)

		err = client.UpdateChatProjectACL(ctx, project.OrganizationID, project.ID, codersdk.UpdateChatProjectACL{
			UserRoles: map[string]codersdk.ChatProjectRole{memberUser.ID.String(): codersdk.ChatProjectRoleUse},
		})
		require.NoError(t, err)
		require.NoError(t, member.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ProjectID: &project.ID}))
		requireChatProjectID(ctx, t, member, chat.ID, &project.ID)
	})

	t.Run("AnotherOrganization", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, api := newChatProjectClientWithAPI(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)
		otherOrganization := dbgen.Organization(t, api.Database, database.Organization{})
		otherProject := dbgen.ChatProject(t, api.Database, database.ChatProject{
			OrganizationID: otherOrganization.ID,
			OwnerID:        firstUser.UserID,
		})
		chat := createIdleChat(ctx, t, client, api, firstUser.OrganizationID)

		err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ProjectID: &otherProject.ID})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Chat project does not belong to this chat's organization.", sdkErr.Message)
		requireChatProjectID(ctx, t, client, chat.ID, nil)
	})

	t.Run("ChildChat", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, api := newChatProjectClientWithAPI(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)
		project := createChatProject(t, client, firstUser.OrganizationID, "Project")
		chat := createIdleChat(ctx, t, client, api, firstUser.OrganizationID)
		child := createSubagentChat(t, api, chat, database.ChatStatusWaiting)

		err := client.UpdateChat(ctx, child.ID, codersdk.UpdateChatRequest{ProjectID: &project.ID})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Cannot move a child chat.", sdkErr.Message)
		requireChatProjectID(ctx, t, client, child.ID, nil)
	})

	t.Run("ArchivedChat", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, api := newChatProjectClientWithAPI(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)
		project := createChatProject(t, client, firstUser.OrganizationID, "Project")
		chat := createIdleChat(ctx, t, client, api, firstUser.OrganizationID)

		// Archiving in the same request is refused before any write.
		err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{Archived: new(true), ProjectID: &project.ID})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Cannot move an archived chat.", sdkErr.Message)
		got, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.False(t, got.Archived)

		require.NoError(t, client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{Archived: new(true)}))
		err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ProjectID: &project.ID})
		sdkErr = requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Cannot move an archived chat.", sdkErr.Message)
		requireChatProjectID(ctx, t, client, chat.ID, nil)

		// The unarchive runs before the move, so both apply.
		require.NoError(t, client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{Archived: new(false), ProjectID: &project.ID}))
		requireChatProjectID(ctx, t, client, chat.ID, &project.ID)
	})

	t.Run("ActiveFamilyReturnsConflict", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name string
			// active is the status of the family member that blocks the move.
			active database.ChatStatus
			// activate returns the root chat after it puts the family in a
			// state that is not idle.
			activate func(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, api *coderd.API, organizationID uuid.UUID) codersdk.Chat
		}{
			{
				name:   "RunningRoot",
				active: database.ChatStatusRunning,
				activate: func(_ context.Context, t *testing.T, client *codersdk.ExperimentalClient, _ *coderd.API, organizationID uuid.UUID) codersdk.Chat {
					return createChatInProject(t, client, organizationID, nil)
				},
			},
			{
				name:   "InterruptingRoot",
				active: database.ChatStatusInterrupting,
				activate: func(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, api *coderd.API, organizationID uuid.UUID) codersdk.Chat {
					chat := createChatInProject(t, client, organizationID, nil)
					chatdCtx := dbauthz.AsChatd(ctx) //nolint:gocritic // Test fixture mirrors a worker that owns the chat.
					machine := chatstate.NewChatMachine(api.Database, api.Pubsub, chat.ID)
					require.NoError(t, machine.Update(chatdCtx, func(tx *chatstate.Tx, _ database.Store) error {
						_, err := tx.Acquire(chatstate.AcquireInput{WorkerID: uuid.New(), RunnerID: uuid.New()})
						return err
					}))
					_, err := client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
						Content:      []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "stop"}},
						BusyBehavior: codersdk.ChatBusyBehaviorInterrupt,
					})
					require.NoError(t, err)
					return chat
				},
			},
			{
				name:   "RequiresActionRoot",
				active: database.ChatStatusRequiresAction,
				activate: func(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, api *coderd.API, organizationID uuid.UUID) codersdk.Chat {
					chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
						OrganizationID: organizationID,
						Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
						UnsafeDynamicTools: []codersdk.DynamicTool{{
							Name:        "echo",
							Description: "test echo tool",
							InputSchema: json.RawMessage(`{"type":"object"}`),
						}},
					})
					require.NoError(t, err)
					_ = driveChatToRequiresAction(ctx, t, api, chat, "echo")
					return chat
				},
			},
			{
				name:   "RunningSubagent",
				active: database.ChatStatusRunning,
				activate: func(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, api *coderd.API, organizationID uuid.UUID) codersdk.Chat {
					chat := createIdleChat(ctx, t, client, api, organizationID)
					_ = createSubagentChat(t, api, chat, database.ChatStatusRunning)
					return chat
				},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitLong)
				client, api := newChatProjectClientWithAPI(t)
				firstUser := coderdtest.CreateFirstUser(t, client.Client)
				_ = createChatModel(t, client)
				project := createChatProject(t, client, firstUser.OrganizationID, "Project")
				chat := tc.activate(ctx, t, client, api, firstUser.OrganizationID)
				before := chatRowsOfFamily(ctx, t, api, chat.ID)
				require.True(t, slices.ContainsFunc(before, func(member database.Chat) bool {
					return member.Status == tc.active
				}), "the family must hold a %s member", tc.active)

				err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ProjectID: &project.ID})
				sdkErr := requireSDKError(t, err, http.StatusConflict)
				require.Equal(t, "Cannot move an active chat. Interrupt or wait for the chat to finish first.", sdkErr.Message)
				require.Equal(t, before, chatRowsOfFamily(ctx, t, api, chat.ID), "a refused move changes no row")

				// The chat has no project, so removing it from its project
				// moves nothing and is not refused.
				require.NoError(t, client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ProjectID: new(uuid.Nil)}))
				require.Equal(t, before, chatRowsOfFamily(ctx, t, api, chat.ID), "a move to the current project changes no row")
			})
		}
	})

	t.Run("ExperimentDisabled", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, api := newChatClientWithAPI(t, withChatWorkerDisabled)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)
		project := dbgen.ChatProject(t, api.Database, database.ChatProject{
			OrganizationID: firstUser.OrganizationID,
			OwnerID:        firstUser.UserID,
		})
		chat := createIdleChat(ctx, t, client, api, firstUser.OrganizationID)

		err := client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ProjectID: &project.ID})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "chat projects experiment is not enabled", sdkErr.Message)
		requireChatProjectID(ctx, t, client, chat.ID, nil)
	})

	t.Run("PublishesProjectChange", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, api := newChatProjectClientWithAPI(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)
		project := createChatProject(t, client, firstUser.OrganizationID, "Project")
		chat := createIdleChat(ctx, t, client, api, firstUser.OrganizationID)
		child := createSubagentChat(t, api, chat, database.ChatStatusWaiting)

		events, closer, err := client.WatchChats(ctx)
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, closer.Close()) })

		require.NoError(t, client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ProjectID: &project.ID}))

		// Each family member gets one event, so every tab can update the
		// rows it shows.
		pending := map[uuid.UUID]*uuid.UUID{chat.ID: &project.ID, child.ID: nil}
		for len(pending) > 0 {
			event := testutil.RequireReceive(ctx, t, events)
			if event.Kind != codersdk.ChatWatchEventKindProjectChange {
				continue
			}
			want, ok := pending[event.Chat.ID]
			require.True(t, ok, "unexpected project_change event for chat %s", event.Chat.ID)
			require.Equal(t, want, event.Chat.ProjectID)
			delete(pending, event.Chat.ID)
		}
	})

	t.Run("AuditsTheMove", func(t *testing.T) {
		t.Parallel()

		mAudit := audit.NewMockWithDiffFn(func(old, newVal any) audit.Map {
			oldChat, oldIsChat := old.(database.Chat)
			newChat, newIsChat := newVal.(database.Chat)
			if !oldIsChat || !newIsChat {
				return audit.Map{}
			}
			return audit.Map{"project_id": {Old: oldChat.ProjectID.UUID, New: newChat.ProjectID.UUID}}
		})
		ctx := testutil.Context(t, testutil.WaitLong)
		client, api := newChatProjectClientWithAPI(t, func(options *coderdtest.Options) {
			options.Auditor = mAudit
		})
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)
		projectA := createChatProject(t, client, firstUser.OrganizationID, "Project A")
		projectB := createChatProject(t, client, firstUser.OrganizationID, "Project B")
		chat := createChatInProject(t, client, firstUser.OrganizationID, &projectA.ID)
		driveChatToWaiting(ctx, t, api, chat.ID)
		mAudit.ResetLogs()

		require.NoError(t, client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ProjectID: &projectB.ID}))

		logs := mAudit.AuditLogs()
		require.Len(t, logs, 1)
		require.Equal(t, database.ResourceTypeChat, logs[0].ResourceType)
		require.Equal(t, chat.ID, logs[0].ResourceID)
		var diff map[string]codersdk.AuditDiffField
		require.NoError(t, json.Unmarshal(logs[0].Diff, &diff))
		require.Equal(t, map[string]codersdk.AuditDiffField{
			"project_id": {Old: projectA.ID.String(), New: projectB.ID.String()},
		}, diff)
	})
}

// createIdleChat creates a chat outside any project and moves it to
// waiting, where a move is allowed.
func createIdleChat(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, api *coderd.API, organizationID uuid.UUID) codersdk.Chat {
	t.Helper()
	chat := createChatInProject(t, client, organizationID, nil)
	driveChatToWaiting(ctx, t, api, chat.ID)
	return chat
}

func createSubagentChat(t *testing.T, api *coderd.API, root codersdk.Chat, status database.ChatStatus) database.Chat {
	t.Helper()
	return dbgen.Chat(t, api.Database, database.Chat{
		OrganizationID:    root.OrganizationID,
		OwnerID:           root.OwnerID,
		LastModelConfigID: root.LastModelConfigID,
		Title:             "subagent",
		Status:            status,
		ParentChatID:      uuid.NullUUID{UUID: root.ID, Valid: true},
		RootChatID:        uuid.NullUUID{UUID: root.ID, Valid: true},
	})
}

func requireChatProjectID(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, chatID uuid.UUID, want *uuid.UUID) {
	t.Helper()
	chat, err := client.GetChat(ctx, chatID)
	require.NoError(t, err)
	require.Equal(t, want, chat.ProjectID)
}

func chatRowsOfFamily(ctx context.Context, t *testing.T, api *coderd.API, rootID uuid.UUID) []database.Chat {
	t.Helper()
	sysCtx := dbauthz.AsSystemRestricted(ctx) //nolint:gocritic // Test reads rows the chat owner cannot see as one family.
	ids, err := api.Database.GetChatFamilyIDsByRootID(sysCtx, rootID)
	require.NoError(t, err)
	chats := make([]database.Chat, 0, len(ids))
	for _, id := range ids {
		chat, err := api.Database.GetChatByID(sysCtx, id)
		require.NoError(t, err)
		chats = append(chats, chat)
	}
	return chats
}

func chatIDs(chats []codersdk.Chat) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(chats))
	for _, chat := range chats {
		ids = append(ids, chat.ID)
	}
	return ids
}

func TestChatProjectsExperimentDisabled(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatClientWithDatabase(t, withChatWorkerDisabled)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModel(t, client)
	// The project routes bypass the experiment in development builds, so
	// seed the row directly and exercise chat creation, which does not.
	project := dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID: firstUser.OrganizationID,
		OwnerID:        firstUser.UserID,
		Name:           "Off",
	})

	_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		ProjectID:      &project.ID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "experiment off"}},
	})
	require.Equal(t, 400, coderdtest.SDKError(t, err).StatusCode())

	_, err = client.ListChats(ctx, &codersdk.ListChatsOptions{ProjectID: &project.ID})
	require.Equal(t, 400, coderdtest.SDKError(t, err).StatusCode())
}

func newChatProjectClient(t testing.TB, overrides ...func(*coderdtest.Options)) (*codersdk.ExperimentalClient, database.Store) {
	t.Helper()
	client, api := newChatProjectClientWithAPI(t, overrides...)
	return client, api.Database
}

func newChatProjectClientWithAPI(t testing.TB, overrides ...func(*coderdtest.Options)) (*codersdk.ExperimentalClient, *coderd.API) {
	t.Helper()
	return newChatClientWithAPI(t, append([]func(*coderdtest.Options){
		func(options *coderdtest.Options) {
			options.DeploymentValues.Experiments = serpent.StringArray{
				string(codersdk.ExperimentChatProjects),
			}
		},
		withChatWorkerDisabled,
	}, overrides...)...)
}

func createChatProject(t testing.TB, client *codersdk.ExperimentalClient, organizationID uuid.UUID, name string) codersdk.ChatProject {
	t.Helper()

	project, err := client.CreateChatProject(testutil.Context(t, testutil.WaitLong), organizationID, codersdk.CreateChatProjectRequest{
		Name: name,
	})
	require.NoError(t, err)
	return project
}

func requireChatProjectFieldError(t testing.TB, err error, field, detail string) {
	t.Helper()

	sdkErr := coderdtest.SDKError(t, err)
	require.Equal(t, 400, sdkErr.StatusCode())
	require.Equal(t, detail, sdkErr.Message)
	require.Equal(t, []codersdk.ValidationError{{Field: field, Detail: detail}}, sdkErr.Validations)
}

func requireChatProjectNotFound(t testing.TB, err error) {
	t.Helper()

	sdkErr := coderdtest.SDKError(t, err)
	require.Equal(t, 404, sdkErr.StatusCode())
	require.Equal(t, "Chat project not found.", sdkErr.Message)
}

func createChatInProject(t testing.TB, client *codersdk.ExperimentalClient, organizationID uuid.UUID, projectID *uuid.UUID) codersdk.Chat {
	t.Helper()

	chat, err := client.CreateChat(testutil.Context(t, testutil.WaitLong), codersdk.CreateChatRequest{
		OrganizationID: organizationID,
		ProjectID:      projectID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "chat project coverage",
		}},
	})
	require.NoError(t, err)
	return chat
}

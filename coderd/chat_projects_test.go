package coderd_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
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
	// Like chats, the list holds only the caller's own projects, even for a
	// site owner who can read others' projects by ID.
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

	// The owner may create chats for other users, but binding one to the
	// owner's project would expose its memory to someone who cannot read
	// the project, so the chat owner must own the project. The response
	// matches an unknown project so the check does not reveal existence.
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

func TestChatProjectListFilter(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := newChatProjectClient(t)
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

	// Membership is fixed at creation; the update endpoint ignores project_id.
	require.NoError(t, client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{Title: new("renamed")}))
	updated, err := client.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, &projectA.ID, updated.ProjectID)
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

func newChatProjectClient(t testing.TB) (*codersdk.ExperimentalClient, database.Store) {
	t.Helper()
	client, db := newChatClientWithDatabase(t,
		func(options *coderdtest.Options) {
			options.DeploymentValues.Experiments = serpent.StringArray{
				string(codersdk.ExperimentChatProjects),
			}
		},
		withChatWorkerDisabled,
	)
	return client, db
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

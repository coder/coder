package coderd_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
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
	_ = dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID: otherOrganization.ID,
		OwnerID:        firstUser.UserID,
		Name:           "Other Organization Project",
	})

	projects, err := client.ListChatProjects(ctx, firstUser.OrganizationID)
	require.NoError(t, err)
	require.Len(t, projects, 1)
	require.Equal(t, project.ID, projects[0].ID)

	chat := createChatInProject(t, client, firstUser.OrganizationID, &project.ID)

	fetched, err := client.GetChatProject(ctx, project.ID)
	require.NoError(t, err)
	require.Equal(t, project.ID, fetched.ID)

	updatedName := "Renamed Project"
	updatedDescription := "Updated description"
	updated, err := client.UpdateChatProject(ctx, project.ID, codersdk.UpdateChatProjectRequest{
		Name:        &updatedName,
		Description: &updatedDescription,
	})
	require.NoError(t, err)
	require.Equal(t, updatedName, updated.Name)
	require.Equal(t, updatedDescription, updated.Description)

	_, err = client.CreateChatProject(ctx, codersdk.CreateChatProjectRequest{
		OrganizationID: firstUser.OrganizationID,
		Name:           updatedName,
	})
	require.Equal(t, 409, coderdtest.SDKError(t, err).StatusCode())

	// Names are unique per creator, so another member's private project can
	// reuse one without learning that it exists elsewhere.
	otherRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
	other := codersdk.NewExperimentalClient(otherRaw)
	otherProject := createChatProject(t, other, firstUser.OrganizationID, updatedName)
	require.Equal(t, updatedName, otherProject.Name)

	require.NoError(t, client.DeleteChatProject(ctx, project.ID))
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
	_, err := member.GetChatProject(ctx, project.ID)
	require.Equal(t, 404, coderdtest.SDKError(t, err).StatusCode())
	projects, err := member.ListChatProjects(ctx, firstUser.OrganizationID)
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
	require.Equal(t, 400, coderdtest.SDKError(t, err).StatusCode())

	_, err = member.UpdateChatProject(ctx, project.ID, codersdk.UpdateChatProjectRequest{})
	require.Equal(t, 404, coderdtest.SDKError(t, err).StatusCode())
	err = member.DeleteChatProject(ctx, project.ID)
	require.Equal(t, 404, coderdtest.SDKError(t, err).StatusCode())

	// Members still create their own projects, which only they see.
	memberProject := createChatProject(t, member, firstUser.OrganizationID, "Member Project")
	projects, err = member.ListChatProjects(ctx, firstUser.OrganizationID)
	require.NoError(t, err)
	require.Len(t, projects, 1)
	require.Equal(t, memberProject.ID, projects[0].ID)
	// The first user holds the site owner role and therefore sees both.
	projects, err = client.ListChatProjects(ctx, firstUser.OrganizationID)
	require.NoError(t, err)
	require.Len(t, projects, 2)

	otherOrganization := dbgen.Organization(t, db, database.Organization{IsDefault: false})
	otherProject := dbgen.ChatProject(t, db, database.ChatProject{
		OrganizationID: otherOrganization.ID,
		OwnerID:        firstUser.UserID,
		Name:           "Other Project",
	})
	otherMemberRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, otherOrganization.ID)
	otherMember := codersdk.NewExperimentalClient(otherMemberRaw)
	_, err = otherMember.GetChatProject(ctx, project.ID)
	require.Equal(t, 404, coderdtest.SDKError(t, err).StatusCode())

	_, err = client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		ProjectID:      &otherProject.ID,
		Content: []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: "reject cross-organization project",
		}},
	})
	require.Equal(t, 400, coderdtest.SDKError(t, err).StatusCode())

	adminRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID,
		rbac.ScopedRoleOrgAdmin(firstUser.OrganizationID))
	admin := codersdk.NewExperimentalClient(adminRaw)
	adminName := "Updated by admin"
	_, err = admin.UpdateChatProject(ctx, project.ID, codersdk.UpdateChatProjectRequest{Name: &adminName})
	require.NoError(t, err)

	// The owner may create chats for other users, but binding one to the
	// owner's project would expose its memory to someone who cannot read
	// the project, so the chat owner must own the project.
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
	require.Equal(t, 400, coderdtest.SDKError(t, err).StatusCode())
	memberChat := createChatInProject(t, member, firstUser.OrganizationID, nil)
	err = client.UpdateChat(ctx, memberChat.ID, codersdk.UpdateChatRequest{ProjectID: &project.ID})
	require.Equal(t, 400, coderdtest.SDKError(t, err).StatusCode())

	require.NoError(t, admin.DeleteChatProject(ctx, project.ID))
}

func TestChatProjectFieldLimits(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, _ := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)

	_, err := client.CreateChatProject(ctx, codersdk.CreateChatProjectRequest{
		OrganizationID: firstUser.OrganizationID,
		Name:           strings.Repeat("n", 65),
	})
	require.Equal(t, 400, coderdtest.SDKError(t, err).StatusCode())
	project := createChatProject(t, client, firstUser.OrganizationID, strings.Repeat("n", 64))
	longDescription := strings.Repeat("d", 1025)
	_, err = client.UpdateChatProject(ctx, project.ID, codersdk.UpdateChatProjectRequest{Description: &longDescription})
	require.Equal(t, 400, coderdtest.SDKError(t, err).StatusCode())
}

func TestChatProjectBindingPatchClearAndListFilter(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	model := createChatModel(t, client)
	projectA := createChatProject(t, client, firstUser.OrganizationID, "Project A")
	projectB := createChatProject(t, client, firstUser.OrganizationID, "Project B")

	chat := createChatInProject(t, client, firstUser.OrganizationID, nil)
	otherChat := createChatInProject(t, client, firstUser.OrganizationID, &projectB.ID)

	require.NoError(t, client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ProjectID: &projectA.ID}))
	updated, err := client.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, &projectA.ID, updated.ProjectID)

	chats, err := client.ListChats(ctx, &codersdk.ListChatsOptions{ProjectID: &projectA.ID})
	require.NoError(t, err)
	require.Len(t, chats, 1)
	require.Equal(t, chat.ID, chats[0].ID)
	require.NotEqual(t, otherChat.ID, chats[0].ID)

	clearProject := uuid.Nil
	require.NoError(t, client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{ProjectID: &clearProject}))
	updated, err = client.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Nil(t, updated.ProjectID)

	chats, err = client.ListChats(ctx, &codersdk.ListChatsOptions{ProjectID: &projectA.ID})
	require.NoError(t, err)
	require.Empty(t, chats)

	// A rejected project fails the whole PATCH; fields listed before it in
	// the request are not applied.
	missing := uuid.New()
	err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{Title: new("should not apply"), ProjectID: &missing})
	require.Equal(t, 400, coderdtest.SDKError(t, err).StatusCode())
	unchanged, err := client.GetChat(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, updated.Title, unchanged.Title)
	require.Nil(t, unchanged.ProjectID)

	// Child chats are created by chatd, not the public API. Seed one to verify
	// the public PATCH endpoint still enforces the root-chat invariant.
	child, err := db.InsertChat(dbauthz.AsSystemRestricted(ctx), database.InsertChatParams{
		OrganizationID:    firstUser.OrganizationID,
		OwnerID:           firstUser.UserID,
		ParentChatID:      uuid.NullUUID{UUID: chat.ID, Valid: true},
		LastModelConfigID: model.ID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		Title:             "child chat",
	})
	require.NoError(t, err)
	err = client.UpdateChat(ctx, child.ID, codersdk.UpdateChatRequest{ProjectID: &projectA.ID})
	require.Equal(t, 400, coderdtest.SDKError(t, err).StatusCode())
}

func TestChatProjectsExperimentDisabled(t *testing.T) {
	t.Parallel()

	// RequireExperimentWithDevBypass intentionally bypasses disabled experiments
	// in development builds, which is how this integration suite runs.
	t.Skip("experiment-disabled route behavior is not testable in development builds")
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

	project, err := client.CreateChatProject(testutil.Context(t, testutil.WaitLong), codersdk.CreateChatProjectRequest{
		OrganizationID: organizationID,
		Name:           name,
	})
	require.NoError(t, err)
	return project
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

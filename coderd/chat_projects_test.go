package coderd_test

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
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

func TestChatProjectTemplates(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	orgID := firstUser.OrganizationID

	first, firstPreset := createAgentTemplate(t, db, orgID, firstUser.UserID, true, false)
	second, secondPreset := createAgentTemplate(t, db, orgID, firstUser.UserID, true, true)
	notAllowed, _ := createAgentTemplate(t, db, orgID, firstUser.UserID, false, false)
	otherOrganization := dbgen.Organization(t, db, database.Organization{})
	otherOrgTemplate, _ := createAgentTemplate(t, db, otherOrganization.ID, firstUser.UserID, true, false)

	// A new project has no templates.
	project := createChatProject(t, client, orgID, "No Templates")
	require.Equal(t, codersdk.ChatProjectTemplates{TemplateIDs: []uuid.UUID{}}, project.Templates)

	withPreset := codersdk.ChatProjectTemplates{
		TemplateIDs:       []uuid.UUID{first.ID, second.ID, first.ID},
		DefaultTemplateID: &first.ID,
		DefaultPresetID:   &firstPreset.ID,
	}
	project, err := client.CreateChatProject(ctx, orgID, codersdk.CreateChatProjectRequest{Name: "Templates", Templates: &withPreset})
	require.NoError(t, err)
	withPreset.TemplateIDs = []uuid.UUID{first.ID, second.ID}
	require.Equal(t, withPreset, project.Templates)
	fetched, err := client.GetChatProject(ctx, orgID, project.ID)
	require.NoError(t, err)
	require.Equal(t, withPreset, fetched.Templates)

	// Updating other fields keeps the templates.
	name := "Renamed"
	updated, err := client.UpdateChatProject(ctx, orgID, project.ID, codersdk.UpdateChatProjectRequest{Name: &name})
	require.NoError(t, err)
	require.Equal(t, withPreset, updated.Templates)

	// Templates are replaced as a whole.
	withParameters := codersdk.ChatProjectTemplates{
		TemplateIDs:       []uuid.UUID{second.ID},
		DefaultTemplateID: &second.ID,
		DefaultParameters: map[string]string{"region": "eu"},
	}
	updated, err = client.UpdateChatProject(ctx, orgID, project.ID, codersdk.UpdateChatProjectRequest{Templates: &withParameters})
	require.NoError(t, err)
	require.Equal(t, withParameters, updated.Templates)

	updated, err = client.UpdateChatProject(ctx, orgID, project.ID, codersdk.UpdateChatProjectRequest{Templates: &codersdk.ChatProjectTemplates{}})
	require.NoError(t, err)
	require.Equal(t, codersdk.ChatProjectTemplates{TemplateIDs: []uuid.UUID{}}, updated.Templates)

	unknownID := uuid.New()
	for _, tc := range []struct {
		name      string
		templates codersdk.ChatProjectTemplates
		field     string
		detail    string
	}{
		{"NotAllowed", codersdk.ChatProjectTemplates{TemplateIDs: []uuid.UUID{notAllowed.ID}}, "templates.template_ids", "Template " + notAllowed.ID.String() + " is not available to agents in this organization."},
		{"OtherOrganization", codersdk.ChatProjectTemplates{TemplateIDs: []uuid.UUID{otherOrgTemplate.ID}}, "templates.template_ids", "Template " + otherOrgTemplate.ID.String() + " is not available to agents in this organization."},
		{"Unknown", codersdk.ChatProjectTemplates{TemplateIDs: []uuid.UUID{unknownID}}, "templates.template_ids", "Template " + unknownID.String() + " is not available to agents in this organization."},
		{"DefaultNotListed", codersdk.ChatProjectTemplates{TemplateIDs: []uuid.UUID{first.ID}, DefaultTemplateID: &second.ID}, "templates.default_template_id", "The default template must be one of the project's templates."},
		{"PresetWithoutDefault", codersdk.ChatProjectTemplates{TemplateIDs: []uuid.UUID{first.ID}, DefaultPresetID: &firstPreset.ID}, "templates.default_template_id", "Default preset and parameters require a default template."},
		{"PresetAndParameters", codersdk.ChatProjectTemplates{TemplateIDs: []uuid.UUID{first.ID}, DefaultTemplateID: &first.ID, DefaultPresetID: &firstPreset.ID, DefaultParameters: map[string]string{"region": "eu"}}, "templates.default_preset_id", "Choose a default preset or default parameters, not both."},
		{"PresetOfOtherTemplate", codersdk.ChatProjectTemplates{TemplateIDs: []uuid.UUID{first.ID}, DefaultTemplateID: &first.ID, DefaultPresetID: &secondPreset.ID}, "templates.default_preset_id", "The default preset must belong to the default template's active version."},
		{"UnknownParameter", codersdk.ChatProjectTemplates{TemplateIDs: []uuid.UUID{first.ID}, DefaultTemplateID: &first.ID, DefaultParameters: map[string]string{"size": "xl"}}, "templates.default_parameters", `Parameter "size" does not exist on the default template's active version.`},
	} {
		_, err := client.UpdateChatProject(ctx, orgID, project.ID, codersdk.UpdateChatProjectRequest{Templates: &tc.templates})
		requireChatProjectFieldError(t, err, tc.field, tc.detail)
	}

	// Classic parameter values are checked against the parameter's rules.
	_, err = client.UpdateChatProject(ctx, orgID, project.ID, codersdk.UpdateChatProjectRequest{Templates: &codersdk.ChatProjectTemplates{
		TemplateIDs:       []uuid.UUID{second.ID},
		DefaultTemplateID: &second.ID,
		DefaultParameters: map[string]string{"cpu": "99"},
	}})
	sdkErr := coderdtest.SDKError(t, err)
	require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	require.Len(t, sdkErr.Validations, 1)
	require.Equal(t, "templates.default_parameters", sdkErr.Validations[0].Field)
	require.True(t, strings.HasPrefix(sdkErr.Message, `Parameter "cpu" has an invalid value: `), sdkErr.Message)

	// Dynamic parameter values are left for the build to validate.
	_, err = client.UpdateChatProject(ctx, orgID, project.ID, codersdk.UpdateChatProjectRequest{Templates: &codersdk.ChatProjectTemplates{
		TemplateIDs:       []uuid.UUID{first.ID},
		DefaultTemplateID: &first.ID,
		DefaultParameters: map[string]string{"cpu": "99"},
	}})
	require.NoError(t, err)
}

// createAgentTemplate creates a template whose active version has a "region"
// parameter and one preset.
func createAgentTemplate(t testing.TB, db database.Store, orgID, userID uuid.UUID, agentsAllowed, classicParameters bool) (database.Template, database.TemplateVersionPreset) {
	t.Helper()

	versionID := uuid.New()
	template := dbgen.Template(t, db, database.Template{
		OrganizationID:  orgID,
		CreatedBy:       userID,
		ActiveVersionID: versionID,
		AgentsAllowed:   agentsAllowed,

		UseClassicParameterFlow: classicParameters,
	})
	preset := database.TemplateVersionPreset{ID: uuid.New(), Name: "default"}
	dbfake.TemplateVersion(t, db).
		Seed(database.TemplateVersion{
			ID:             versionID,
			TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
			OrganizationID: orgID,
			CreatedBy:      userID,
		}).
		Params(
			database.TemplateVersionParameter{Name: "region"},
			database.TemplateVersionParameter{
				Name:          "cpu",
				Type:          "number",
				DefaultValue:  "2",
				ValidationMin: sql.NullInt32{Int32: 1, Valid: true},
				ValidationMax: sql.NullInt32{Int32: 8, Valid: true},
			},
		).
		Preset(preset).
		Do()
	return template, preset
}

func TestChatProjectWorkspaceTemplates(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client, db := newChatProjectClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModel(t, client)
	orgID := firstUser.OrganizationID

	allowed, _ := createAgentTemplate(t, db, orgID, firstUser.UserID, true, false)
	other, _ := createAgentTemplate(t, db, orgID, firstUser.UserID, true, false)
	project, err := client.CreateChatProject(ctx, orgID, codersdk.CreateChatProjectRequest{
		Name:      "Workspace Templates",
		Templates: &codersdk.ChatProjectTemplates{TemplateIDs: []uuid.UUID{allowed.ID}},
	})
	require.NoError(t, err)
	workspace := func(templateID uuid.UUID) uuid.UUID {
		return dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
			OrganizationID: orgID,
			OwnerID:        firstUser.UserID,
			TemplateID:     templateID,
		}).Do().Workspace.ID
	}
	allowedWorkspace, otherWorkspace := workspace(allowed.ID), workspace(other.ID)
	createChat := func(workspaceID uuid.UUID) (codersdk.Chat, error) {
		return client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: orgID,
			ProjectID:      &project.ID,
			WorkspaceID:    &workspaceID,
			Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "attach"}},
		})
	}
	const notAllowed = "This workspace's template is not allowed in the chat's project."

	_, err = createChat(otherWorkspace)
	sdkErr := coderdtest.SDKError(t, err)
	require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	require.Equal(t, notAllowed, sdkErr.Message)

	chat, err := createChat(allowedWorkspace)
	require.NoError(t, err)

	err = client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{WorkspaceID: &otherWorkspace})
	sdkErr = coderdtest.SDKError(t, err)
	require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	require.Equal(t, notAllowed, sdkErr.Message)
	require.NoError(t, client.UpdateChat(ctx, chat.ID, codersdk.UpdateChatRequest{WorkspaceID: &allowedWorkspace}))
}

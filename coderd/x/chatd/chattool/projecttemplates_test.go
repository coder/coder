package chattool_test

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const (
	projectTemplateNotAvailable = "template not available in this chat's project; use list_templates to find the project's templates"
	projectTemplatesLimitedNote = "This chat's project limits workspaces to the templates listed here. Do not use any other template."
	projectTemplatesHiddenNote  = "Some templates attached to this chat's project are not listed because the user does not have access to them. " +
		"A template admin or organization admin can give the user access. Tell the user if none of the listed templates fit."
)

func TestProjectTemplateEnforcement(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	model := seedModelConfig(t, db)

	inProject, preset := projectTestTemplate(t, db, org.ID, user.ID)
	outside, otherPreset := projectTestTemplate(t, db, org.ID, user.ID)
	// newProject creates a project that limits its chats to the attached
	// templates, unless seed is changed afterwards.
	newProject := func(seed database.ChatProject, attached ...uuid.UUID) uuid.NullUUID {
		seed.OrganizationID, seed.OwnerID = org.ID, user.ID
		seed.LimitTemplates = true
		return attachedProject(t, db, seed, attached...)
	}
	defaultTemplate := uuid.NullUUID{UUID: inProject.ID, Valid: true}
	withParameters := newProject(database.ChatProject{
		DefaultTemplateID: defaultTemplate,
		DefaultParameters: json.RawMessage(`{"region":"eu","size":"m"}`),
	}, inProject.ID)
	empty := newProject(database.ChatProject{})
	// Without the project, the name would sort this template last.
	attachedLast := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "zzz-" + uuid.NewString()[:8],
		AgentsAllowed:  true,
	})
	unlimited := attachedProject(t, db, database.ChatProject{OrganizationID: org.ID, OwnerID: user.ID}, attachedLast.ID)

	runTool := func(t *testing.T, tool fantasy.AgentTool, input string) fantasy.ToolResponse {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: uuid.NewString(), Name: tool.Info().Name, Input: input})
		require.NoError(t, err)
		return resp
	}
	// createWorkspace runs create_workspace in a new chat and returns the
	// request passed to CreateFn, or nil when it was not called.
	createWorkspace := func(t *testing.T, projectID uuid.NullUUID, input string) (fantasy.ToolResponse, *codersdk.CreateWorkspaceRequest) {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		chat, err := db.InsertChat(ctx, database.InsertChatParams{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: model.ID,
			Title:             "project-create",
			Status:            database.ChatStatusWaiting,
			ClientType:        database.ChatClientTypeApi,
		})
		require.NoError(t, err)
		var captured *codersdk.CreateWorkspaceRequest
		tool := chattool.CreateWorkspace(db, org.ID, chat.ID, chattool.CreateWorkspaceOptions{
			OwnerID:   user.ID,
			ProjectID: projectID,
			CreateFn: func(_ context.Context, _, _ uuid.UUID, req codersdk.CreateWorkspaceRequest) (codersdk.Workspace, database.Chat, error) {
				captured = &req
				return codersdk.Workspace{}, database.Chat{}, nil
			},
		})
		return runTool(t, tool, input), captured
	}
	parameters := func(req *codersdk.CreateWorkspaceRequest) map[string]string {
		values := map[string]string{}
		for _, p := range req.RichParameterValues {
			values[p.Name] = p.Value
		}
		return values
	}

	t.Run("ListTemplates", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		tool := chattool.ListTemplates(db, org.ID, chattool.ListTemplatesOptions{OwnerID: user.ID, ProjectID: withParameters})
		result := runListTemplates(ctx, t, tool, "{}")
		items := listTemplateItems(t, result)
		require.Len(t, items, 1)
		require.Equal(t, inProject.ID.String(), items[0]["id"])
		require.Equal(t, inProject.ID.String(), result["recommended_template_id"])
		require.Equal(t, chattool.NextStepUseRecommended, result[chattool.NextStepField])

		require.Equal(t, projectTemplatesLimitedNote, result["project_templates_note"])
		require.NotContains(t, result, "hidden_project_template_count")

		// A project that limits its chats and has nothing attached has no
		// templates, even though the org does.
		tool = chattool.ListTemplates(db, org.ID, chattool.ListTemplatesOptions{OwnerID: user.ID, ProjectID: empty})
		result = runListTemplates(ctx, t, tool, "{}")
		require.Empty(t, listTemplateItems(t, result))
		require.Equal(t, chattool.NextStepNoTemplates, result[chattool.NextStepField])
	})

	t.Run("Unlimited", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		// Without the limit the org's other templates stay available, and
		// the attached template is listed ahead of them.
		tool := chattool.ListTemplates(db, org.ID, chattool.ListTemplatesOptions{OwnerID: user.ID, ProjectID: unlimited})
		result := runListTemplates(ctx, t, tool, "{}")
		items := listTemplateItems(t, result)
		require.Greater(t, len(items), 1)
		require.Equal(t, attachedLast.ID.String(), items[0]["id"])
		require.Equal(t, true, items[0]["attached_to_project"])
		require.NotContains(t, items[1], "attached_to_project")
		require.NotContains(t, result, "project_templates_note")

		resp, req := createWorkspace(t, unlimited, `{"template_id":"`+outside.ID.String()+`"}`)
		require.False(t, resp.IsError, resp.Content)
		require.NotNil(t, req)
	})

	t.Run("ListTemplatesQueryOverDefault", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		named := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
			Name:           "query-" + uuid.NewString()[:8],
			AgentsAllowed:  true,
		})
		// The default only mentions the name in its description, a
		// weaker match than the other template's exact name.
		weakDefault := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
			Description:    "Similar to " + named.Name,
			AgentsAllowed:  true,
		})
		project := newProject(database.ChatProject{
			DefaultTemplateID: uuid.NullUUID{UUID: weakDefault.ID, Valid: true},
		}, named.ID, weakDefault.ID)
		tool := chattool.ListTemplates(db, org.ID, chattool.ListTemplatesOptions{OwnerID: user.ID, ProjectID: project})

		result := runListTemplates(ctx, t, tool, `{"query":"`+named.Name+`"}`)
		require.Len(t, listTemplateItems(t, result), 2)
		require.Equal(t, named.ID.String(), result["recommended_template_id"])

		// Without a decisive match, the default is recommended.
		result = runListTemplates(ctx, t, tool, "{}")
		require.Equal(t, weakDefault.ID.String(), result["recommended_template_id"])
		require.Equal(t, chattool.NextStepUseRecommended, result[chattool.NextStepField])
	})

	t.Run("ReadTemplate", func(t *testing.T) {
		t.Parallel()
		tool := chattool.ReadTemplate(db, org.ID, chattool.ReadTemplateOptions{OwnerID: user.ID, ProjectID: withParameters})
		resp := runTool(t, tool, `{"template_id":"`+outside.ID.String()+`"}`)
		require.True(t, resp.IsError)
		require.Equal(t, projectTemplateNotAvailable, resp.Content)

		resp = runTool(t, tool, `{"template_id":"`+inProject.ID.String()+`"}`)
		require.False(t, resp.IsError, resp.Content)
		var result struct {
			ProjectDefaults struct {
				Parameters map[string]string `json:"parameters"`
			} `json:"project_defaults"`
		}
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		require.Equal(t, map[string]string{"region": "eu", "size": "m"}, result.ProjectDefaults.Parameters)

		// A default preset outside the active version is not reported,
		// because create_workspace skips it.
		stale := newProject(database.ChatProject{
			DefaultTemplateID:              defaultTemplate,
			DefaultTemplateVersionPresetID: uuid.NullUUID{UUID: otherPreset.ID, Valid: true},
		}, inProject.ID)
		tool = chattool.ReadTemplate(db, org.ID, chattool.ReadTemplateOptions{OwnerID: user.ID, ProjectID: stale})
		resp = runTool(t, tool, `{"template_id":"`+inProject.ID.String()+`"}`)
		require.False(t, resp.IsError, resp.Content)
		var staleResult map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &staleResult))
		require.NotContains(t, staleResult, "project_defaults")
	})

	t.Run("CreateWorkspaceOutsideProject", func(t *testing.T) {
		t.Parallel()
		resp, req := createWorkspace(t, empty, `{"template_id":"`+inProject.ID.String()+`"}`)
		require.True(t, resp.IsError)
		require.Equal(t, projectTemplateNotAvailable, resp.Content)
		require.Nil(t, req)
	})

	t.Run("CreateWorkspaceDefaultParameters", func(t *testing.T) {
		t.Parallel()
		_, req := createWorkspace(t, withParameters, `{"template_id":"`+inProject.ID.String()+`","parameters":{"region":"us"}}`)
		require.NotNil(t, req)
		require.Equal(t, map[string]string{"region": "us", "size": "m"}, parameters(req))

		// A preset the model chooses replaces the project's defaults.
		_, req = createWorkspace(t, withParameters, `{"template_id":"`+inProject.ID.String()+`","preset_id":"`+preset.ID.String()+`"}`)
		require.NotNil(t, req)
		require.Empty(t, parameters(req))
	})

	t.Run("CreateWorkspaceDefaultPreset", func(t *testing.T) {
		t.Parallel()
		withPreset := newProject(database.ChatProject{
			DefaultTemplateID:              defaultTemplate,
			DefaultTemplateVersionPresetID: uuid.NullUUID{UUID: preset.ID, Valid: true},
		}, inProject.ID, outside.ID)
		_, req := createWorkspace(t, withPreset, `{"template_id":"`+inProject.ID.String()+`"}`)
		require.NotNil(t, req)
		require.Equal(t, preset.ID, req.TemplateVersionPresetID)

		// Defaults apply only to the default template.
		_, req = createWorkspace(t, withPreset, `{"template_id":"`+outside.ID.String()+`"}`)
		require.NotNil(t, req)
		require.Equal(t, uuid.Nil, req.TemplateVersionPresetID)

		// A parameter the model passes would be overridden by the default
		// preset's value for it, so the preset is skipped.
		_, req = createWorkspace(t, withPreset, `{"template_id":"`+inProject.ID.String()+`","parameters":{"region":"us"}}`)
		require.NotNil(t, req)
		require.Equal(t, uuid.Nil, req.TemplateVersionPresetID)

		// Parameters the preset does not set leave it applied.
		_, req = createWorkspace(t, withPreset, `{"template_id":"`+inProject.ID.String()+`","parameters":{"size":"xl"}}`)
		require.NotNil(t, req)
		require.Equal(t, preset.ID, req.TemplateVersionPresetID)

		// A preset the model chooses wins over the default.
		chosen := dbgen.Preset(t, db, database.InsertPresetParams{TemplateVersionID: inProject.ActiveVersionID})
		_, req = createWorkspace(t, withPreset, `{"template_id":"`+inProject.ID.String()+`","preset_id":"`+chosen.ID.String()+`"}`)
		require.NotNil(t, req)
		require.Equal(t, chosen.ID, req.TemplateVersionPresetID)
	})

	t.Run("CreateWorkspacePresetFromOtherTemplate", func(t *testing.T) {
		t.Parallel()
		// Prebuilds are claimed by preset ID alone, so a preset from a
		// template outside the project must not reach CreateFn.
		resp, req := createWorkspace(t, withParameters, `{"template_id":"`+inProject.ID.String()+`","preset_id":"`+otherPreset.ID.String()+`"}`)
		require.True(t, resp.IsError)
		require.Contains(t, resp.Content, "preset_id does not belong")
		require.Nil(t, req)
	})

	t.Run("CreateWorkspaceStaleDefaultPreset", func(t *testing.T) {
		t.Parallel()
		// The preset no longer belongs to the default template's active
		// version, so it is skipped rather than failing the build.
		stale := newProject(database.ChatProject{
			DefaultTemplateID:              defaultTemplate,
			DefaultTemplateVersionPresetID: uuid.NullUUID{UUID: otherPreset.ID, Valid: true},
		}, inProject.ID)
		_, req := createWorkspace(t, stale, `{"template_id":"`+inProject.ID.String()+`"}`)
		require.NotNil(t, req)
		require.Equal(t, uuid.Nil, req.TemplateVersionPresetID)
	})
}

// A template attached to the chat's project that the chat owner cannot read
// is only counted, and asking for it by ID looks the same as asking for a
// template that does not exist.
func TestProjectTemplatesHiddenFromOwner(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	model := seedModelConfig(t, db)
	visible := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID, AgentsAllowed: true})
	// Without the Everyone group, the member cannot read the template.
	hidden := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		AgentsAllowed:  true,
		GroupACL:       database.TemplateACL{},
	})
	project := attachedProject(t, db, database.ChatProject{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LimitTemplates:    true,
		DefaultTemplateID: uuid.NullUUID{UUID: hidden.ID, Valid: true},
	}, visible.ID, hidden.ID)
	// The tools run under real RBAC as the member, not with the raw
	// store's implicit system access.
	authzDB := dbauthz.New(
		db,
		rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()),
		slogtest.Make(t, nil),
		testAccessControlStorePointer(),
	)

	list := chattool.ListTemplates(authzDB, org.ID, chattool.ListTemplatesOptions{OwnerID: user.ID, ProjectID: project})
	result := runListTemplates(ctx, t, list, "{}")
	items := listTemplateItems(t, result)
	require.Len(t, items, 1)
	require.Equal(t, visible.ID.String(), items[0]["id"])
	require.EqualValues(t, 1, result["hidden_project_template_count"])
	require.Equal(t, projectTemplatesHiddenNote, result["hidden_project_templates_note"])
	// The hidden template is the project's default, which must not be
	// recommended to an owner who cannot use it.
	require.Equal(t, visible.ID.String(), result["recommended_template_id"])

	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "hidden-template",
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeApi,
	})
	require.NoError(t, err)
	create := chattool.CreateWorkspace(authzDB, org.ID, chat.ID, chattool.CreateWorkspaceOptions{
		OwnerID:   user.ID,
		ProjectID: project,
		CreateFn: func(context.Context, uuid.UUID, uuid.UUID, codersdk.CreateWorkspaceRequest) (codersdk.Workspace, database.Chat, error) {
			t.Error("CreateFn must not be called for a template the owner cannot read")
			return codersdk.Workspace{}, database.Chat{}, nil
		},
	})
	run := func(templateID uuid.UUID) fantasy.ToolResponse {
		//nolint:gocritic // The tool runs as chatd and switches to the chat owner.
		resp, err := create.Run(dbauthz.AsChatd(ctx), fantasy.ToolCall{ID: uuid.NewString(), Name: "create_workspace", Input: `{"template_id":"` + templateID.String() + `"}`})
		require.NoError(t, err)
		require.True(t, resp.IsError)
		return resp
	}
	const notFound = "template not found; use list_templates to find available templates"
	require.Equal(t, notFound, run(uuid.New()).Content)
	require.Equal(t, notFound, run(hidden.ID).Content)
}

// attachedProject creates a chat project with the given templates attached.
func attachedProject(t *testing.T, db database.Store, seed database.ChatProject, templateIDs ...uuid.UUID) uuid.NullUUID {
	t.Helper()
	project := dbgen.ChatProject(t, db, seed)
	for _, templateID := range templateIDs {
		dbgen.ChatProjectTemplate(t, db, database.ChatProjectTemplate{ProjectID: project.ID, TemplateID: templateID, CreatedBy: seed.OwnerID})
	}
	return uuid.NullUUID{UUID: project.ID, Valid: true}
}

// projectTestTemplate creates an agent-allowed template whose active version
// has one preset that sets region to "eu".
func projectTestTemplate(t *testing.T, db database.Store, orgID, userID uuid.UUID) (database.Template, database.TemplateVersionPreset) {
	t.Helper()

	versionID := uuid.New()
	template := dbgen.Template(t, db, database.Template{
		OrganizationID:  orgID,
		CreatedBy:       userID,
		ActiveVersionID: versionID,
		AgentsAllowed:   true,
	})
	preset := database.TemplateVersionPreset{ID: uuid.New(), Name: "default"}
	presetRegion := database.TemplateVersionPresetParameter{TemplateVersionPresetID: preset.ID, Name: "region", Value: "eu"}
	dbfake.TemplateVersion(t, db).
		Seed(database.TemplateVersion{
			ID:             versionID,
			TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
			OrganizationID: orgID,
			CreatedBy:      userID,
		}).
		Preset(preset, presetRegion).
		Do()
	return template, preset
}

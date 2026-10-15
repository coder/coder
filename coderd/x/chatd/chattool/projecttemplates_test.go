package chattool_test

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

const projectTemplateNotAvailable = "template not available in this chat's project; use list_templates to find the project's templates"

func TestProjectTemplateEnforcement(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: user.ID, OrganizationID: org.ID})
	model := seedModelConfig(t, db)

	inProject, preset := projectTestTemplate(t, db, org.ID, user.ID)
	outside, otherPreset := projectTestTemplate(t, db, org.ID, user.ID)
	newProject := func(seed database.ChatProject) uuid.NullUUID {
		seed.OrganizationID, seed.OwnerID = org.ID, user.ID
		return uuid.NullUUID{UUID: dbgen.ChatProject(t, db, seed).ID, Valid: true}
	}
	defaultTemplate := uuid.NullUUID{UUID: inProject.ID, Valid: true}
	withParameters := newProject(database.ChatProject{
		TemplateIDs:       []uuid.UUID{inProject.ID},
		DefaultTemplateID: defaultTemplate,
		DefaultParameters: json.RawMessage(`{"region":"eu","size":"m"}`),
	})
	empty := newProject(database.ChatProject{})

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
			CreateFn: func(_ context.Context, _ uuid.UUID, req codersdk.CreateWorkspaceRequest) (codersdk.Workspace, error) {
				captured = &req
				return codersdk.Workspace{}, nil
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

		// An empty project has no templates, even though the org does.
		tool = chattool.ListTemplates(db, org.ID, chattool.ListTemplatesOptions{OwnerID: user.ID, ProjectID: empty})
		result = runListTemplates(ctx, t, tool, "{}")
		require.Empty(t, listTemplateItems(t, result))
		require.Equal(t, chattool.NextStepNoTemplates, result[chattool.NextStepField])
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
			TemplateIDs:                    []uuid.UUID{inProject.ID, outside.ID},
			DefaultTemplateID:              defaultTemplate,
			DefaultTemplateVersionPresetID: uuid.NullUUID{UUID: preset.ID, Valid: true},
		})
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
		_, req = createWorkspace(t, withPreset, `{"template_id":"`+inProject.ID.String()+`","preset_id":"`+otherPreset.ID.String()+`"}`)
		require.NotNil(t, req)
		require.Equal(t, otherPreset.ID, req.TemplateVersionPresetID)
	})

	t.Run("CreateWorkspaceStaleDefaultPreset", func(t *testing.T) {
		t.Parallel()
		// The preset no longer belongs to the default template's active
		// version, so it is skipped rather than failing the build.
		stale := newProject(database.ChatProject{
			TemplateIDs:                    []uuid.UUID{inProject.ID},
			DefaultTemplateID:              defaultTemplate,
			DefaultTemplateVersionPresetID: uuid.NullUUID{UUID: otherPreset.ID, Valid: true},
		})
		_, req := createWorkspace(t, stale, `{"template_id":"`+inProject.ID.String()+`"}`)
		require.NotNil(t, req)
		require.Equal(t, uuid.Nil, req.TemplateVersionPresetID)
	})
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

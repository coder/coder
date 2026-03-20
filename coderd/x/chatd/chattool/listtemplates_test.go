package chattool_test

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestListTemplates(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)

	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})

	t1 := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "template-alpha",
	})
	t2 := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "template-beta",
	})

	// No allowlist — returns all templates.
	tool := chattool.ListTemplates(chattool.ListTemplatesOptions{
		DB:      db,
		OwnerID: user.ID,
	})
	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "c1", Name: "list_templates", Input: "{}"})
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	templates := result["templates"].([]any)
	require.Len(t, templates, 2)

	// Empty allowlist — same as no allowlist.
	tool = chattool.ListTemplates(chattool.ListTemplatesOptions{
		DB:                 db,
		OwnerID:            user.ID,
		AllowedTemplateIDs: map[uuid.UUID]bool{},
	})
	resp, err = tool.Run(ctx, fantasy.ToolCall{ID: "c2", Name: "list_templates", Input: "{}"})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	templates = result["templates"].([]any)
	require.Len(t, templates, 2)

	// Allowlist with one match — returns only that template.
	tool = chattool.ListTemplates(chattool.ListTemplatesOptions{
		DB:                 db,
		OwnerID:            user.ID,
		AllowedTemplateIDs: map[uuid.UUID]bool{t1.ID: true},
	})
	resp, err = tool.Run(ctx, fantasy.ToolCall{ID: "c3", Name: "list_templates", Input: "{}"})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	templates = result["templates"].([]any)
	require.Len(t, templates, 1)
	m := templates[0].(map[string]any)
	require.Equal(t, t1.ID.String(), m["id"].(string))

	// Allowlist with no matches — returns empty.
	tool = chattool.ListTemplates(chattool.ListTemplatesOptions{
		DB:                 db,
		OwnerID:            user.ID,
		AllowedTemplateIDs: map[uuid.UUID]bool{uuid.New(): true},
	})
	resp, err = tool.Run(ctx, fantasy.ToolCall{ID: "c4", Name: "list_templates", Input: "{}"})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	templates = result["templates"].([]any)
	require.Empty(t, templates)

	// ReadTemplate: allowed template succeeds.
	readTool := chattool.ReadTemplate(chattool.ReadTemplateOptions{
		DB:                 db,
		OwnerID:            user.ID,
		AllowedTemplateIDs: map[uuid.UUID]bool{t1.ID: true},
	})
	input := `{"template_id":"` + t1.ID.String() + `"}`
	resp, err = readTool.Run(ctx, fantasy.ToolCall{ID: "c5", Name: "read_template", Input: input})
	require.NoError(t, err)
	require.False(t, resp.IsError)
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	tmplInfo := result["template"].(map[string]any)
	require.Equal(t, t1.ID.String(), tmplInfo["id"].(string))

	// ReadTemplate: disallowed template returns "not found".
	readTool = chattool.ReadTemplate(chattool.ReadTemplateOptions{
		DB:                 db,
		OwnerID:            user.ID,
		AllowedTemplateIDs: map[uuid.UUID]bool{uuid.New(): true},
	})
	input = `{"template_id":"` + t2.ID.String() + `"}`
	resp, err = readTool.Run(ctx, fantasy.ToolCall{ID: "c6", Name: "read_template", Input: input})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "not found")

	// ReadTemplate: no allowlist allows any template.
	readTool = chattool.ReadTemplate(chattool.ReadTemplateOptions{
		DB:      db,
		OwnerID: user.ID,
	})
	input = `{"template_id":"` + t2.ID.String() + `"}`
	resp, err = readTool.Run(ctx, fantasy.ToolCall{ID: "c7", Name: "read_template", Input: input})
	require.NoError(t, err)
	require.False(t, resp.IsError)

	// CreateWorkspace: disallowed template returns "not found"
	// without invoking the create function.
	createCalled := false
	createTool := chattool.CreateWorkspace(chattool.CreateWorkspaceOptions{
		DB:                 db,
		OwnerID:            user.ID,
		AllowedTemplateIDs: map[uuid.UUID]bool{uuid.New(): true},
		CreateFn: func(_ context.Context, _ uuid.UUID, _ codersdk.CreateWorkspaceRequest) (codersdk.Workspace, error) {
			createCalled = true
			t.Fatal("CreateFn should not be called for blocked template")
			return codersdk.Workspace{}, nil
		},
	})
	input = `{"template_id":"` + t1.ID.String() + `"}`
	resp, err = createTool.Run(ctx, fantasy.ToolCall{ID: "c8", Name: "create_workspace", Input: input})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "not found")
	require.False(t, createCalled, "CreateFn should not be called for blocked template")
}

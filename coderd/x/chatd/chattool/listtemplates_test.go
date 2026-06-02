package chattool_test

import (
	"context"
	"encoding/json"
	"strings"
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

func TestListTemplates_OrganizationFilter(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})

	orgA := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: orgA.ID,
	})
	orgB := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: orgB.ID,
	})

	tAlpha := dbgen.Template(t, db, database.Template{
		OrganizationID: orgA.ID,
		CreatedBy:      user.ID,
		Name:           "alpha",
	})
	tBeta := dbgen.Template(t, db, database.Template{
		OrganizationID: orgB.ID,
		CreatedBy:      user.ID,
		Name:           "beta",
	})

	t.Run("ScopedToOrgA", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		tool := chattool.ListTemplates(db, orgA.ID, chattool.ListTemplatesOptions{
			OwnerID: user.ID,
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "org-a", Name: "list_templates", Input: "{}"})
		require.NoError(t, err)
		require.False(t, resp.IsError)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		templates := result["templates"].([]any)
		require.Len(t, templates, 1)
		m := templates[0].(map[string]any)
		require.Equal(t, tAlpha.ID.String(), m["id"].(string))
	})

	t.Run("NilOrgReturnsBoth", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		tool := chattool.ListTemplates(db, uuid.Nil, chattool.ListTemplatesOptions{
			OwnerID: user.ID,
			// Pass uuid.Nil to skip org filtering.
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "nil-org", Name: "list_templates", Input: "{}"})
		require.NoError(t, err)
		require.False(t, resp.IsError)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		templates := result["templates"].([]any)
		require.Len(t, templates, 2)
	})

	t.Run("ReadTemplate_CrossOrgRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		// Tool scoped to orgA, but requesting a template in orgB.
		tool := chattool.ReadTemplate(db, orgA.ID, chattool.ReadTemplateOptions{
			OwnerID: user.ID,
		})

		input := `{"template_id":"` + tBeta.ID.String() + `"}`
		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "cross-org", Name: "read_template", Input: input})
		require.NoError(t, err)
		require.True(t, resp.IsError)
		require.Contains(t, resp.Content, "not found")
	})

	t.Run("ReadTemplate_SameOrgAllowed", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		// Tool scoped to orgA, requesting a template in orgA.
		tool := chattool.ReadTemplate(db, orgA.ID, chattool.ReadTemplateOptions{
			OwnerID: user.ID,
		})

		input := `{"template_id":"` + tAlpha.ID.String() + `"}`
		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "same-org", Name: "read_template", Input: input})
		require.NoError(t, err)
		require.False(t, resp.IsError)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		tmplInfo := result["template"].(map[string]any)
		require.Equal(t, tAlpha.ID.String(), tmplInfo["id"].(string))
	})
}

func TestListTemplates_Abstract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		template    database.Template
		want        string
		wantPresent bool
	}{
		{
			name: "LongAbstract",
			template: database.Template{
				Name:        "with-abstract",
				Description: "short description",
				Abstract:    strings.Repeat("a", 1000),
			},
			want:        strings.Repeat("a", 1000),
			wantPresent: true,
		},
		{
			name: "ShortAbstractUntouched",
			template: database.Template{
				Name:        "short-abstract",
				Description: "short description",
				Abstract:    "a concise abstract",
			},
			want:        "a concise abstract",
			wantPresent: true,
		},
		{
			name: "EmptyAbstractOmitted",
			template: database.Template{
				Name:        "no-abstract",
				Description: "short description",
				Abstract:    "",
			},
		},
		{
			name: "WhitespaceOnlyAbstractOmitted",
			template: database.Template{
				Name:        "whitespace-abstract",
				Description: "short description",
				Abstract:    "   \t  \n  ",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fixture := newListTemplatesFixture(t)
			template := tc.template
			template.OrganizationID = fixture.org.ID
			template.CreatedBy = fixture.user.ID
			tpl := dbgen.Template(t, fixture.db, template)

			items := fixture.listTemplates(t, "{}")
			require.Len(t, items, 1)
			require.Equal(t, tpl.ID.String(), items[0]["id"].(string))
			require.Equal(t, "short description", items[0]["description"].(string))

			got, ok := items[0]["abstract"]
			require.Equal(t, tc.wantPresent, ok)
			if tc.wantPresent {
				require.Equal(t, tc.want, got.(string))
			}
		})
	}
}

type listTemplatesFixture struct {
	db   database.Store
	user database.User
	org  database.Organization
}

func newListTemplatesFixture(t *testing.T) listTemplatesFixture {
	t.Helper()

	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	return listTemplatesFixture{db: db, user: user, org: org}
}

func (f listTemplatesFixture) listTemplates(t *testing.T, input string) []map[string]any {
	t.Helper()

	tool := chattool.ListTemplates(f.db, f.org.ID, chattool.ListTemplatesOptions{
		OwnerID: f.user.ID,
	})
	resp, err := tool.Run(testutil.Context(t, testutil.WaitShort), fantasy.ToolCall{
		ID:    "list-templates",
		Name:  "list_templates",
		Input: input,
	})
	require.NoError(t, err)
	require.False(t, resp.IsError)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	templates := result["templates"].([]any)
	items := make([]map[string]any, 0, len(templates))
	for _, template := range templates {
		items = append(items, template.(map[string]any))
	}
	return items
}

//nolint:tparallel,paralleltest // Subtests share a single DB and run sequentially.
func TestTemplateAllowlistEnforcement(t *testing.T) {
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

	t.Run("ListTemplates", func(t *testing.T) {
		t.Run("NoAllowlist", func(t *testing.T) {
			tool := chattool.ListTemplates(db, uuid.Nil, chattool.ListTemplatesOptions{
				OwnerID: user.ID,
			})

			resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "c1", Name: "list_templates", Input: "{}"})
			require.NoError(t, err)
			var result map[string]any
			require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
			templates := result["templates"].([]any)
			require.Len(t, templates, 2)
		})

		t.Run("EmptyAllowlist", func(t *testing.T) {
			tool := chattool.ListTemplates(db, uuid.Nil, chattool.ListTemplatesOptions{
				OwnerID:            user.ID,
				AllowedTemplateIDs: func() map[uuid.UUID]bool { return map[uuid.UUID]bool{} },
			})

			resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "c2", Name: "list_templates", Input: "{}"})
			require.NoError(t, err)
			var result map[string]any
			require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
			templates := result["templates"].([]any)
			require.Len(t, templates, 2)
		})

		t.Run("OneMatch", func(t *testing.T) {
			tool := chattool.ListTemplates(db, uuid.Nil, chattool.ListTemplatesOptions{
				OwnerID:            user.ID,
				AllowedTemplateIDs: func() map[uuid.UUID]bool { return map[uuid.UUID]bool{t1.ID: true} },
			})

			resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "c3", Name: "list_templates", Input: "{}"})
			require.NoError(t, err)
			var result map[string]any
			require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
			templates := result["templates"].([]any)
			require.Len(t, templates, 1)
			m := templates[0].(map[string]any)
			require.Equal(t, t1.ID.String(), m["id"].(string))
		})

		t.Run("NoMatches", func(t *testing.T) {
			tool := chattool.ListTemplates(db, uuid.Nil, chattool.ListTemplatesOptions{
				OwnerID:            user.ID,
				AllowedTemplateIDs: func() map[uuid.UUID]bool { return map[uuid.UUID]bool{uuid.New(): true} },
			})

			resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "c4", Name: "list_templates", Input: "{}"})
			require.NoError(t, err)
			var result map[string]any
			require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
			templates := result["templates"].([]any)
			require.Empty(t, templates)
		})
	})

	t.Run("ReadTemplate", func(t *testing.T) {
		t.Run("Allowed", func(t *testing.T) {
			tool := chattool.ReadTemplate(db, org.ID, chattool.ReadTemplateOptions{
				OwnerID:            user.ID,
				AllowedTemplateIDs: func() map[uuid.UUID]bool { return map[uuid.UUID]bool{t1.ID: true} },
			})
			input := `{"template_id":"` + t1.ID.String() + `"}`
			resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "c5", Name: "read_template", Input: input})
			require.NoError(t, err)
			require.False(t, resp.IsError)
			var result map[string]any
			require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
			tmplInfo := result["template"].(map[string]any)
			require.Equal(t, t1.ID.String(), tmplInfo["id"].(string))
		})

		t.Run("Disallowed", func(t *testing.T) {
			tool := chattool.ReadTemplate(db, org.ID, chattool.ReadTemplateOptions{
				OwnerID:            user.ID,
				AllowedTemplateIDs: func() map[uuid.UUID]bool { return map[uuid.UUID]bool{uuid.New(): true} },
			})
			input := `{"template_id":"` + t2.ID.String() + `"}`
			resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "c6", Name: "read_template", Input: input})
			require.NoError(t, err)
			require.True(t, resp.IsError)
			require.Contains(t, resp.Content, "not found")
		})

		t.Run("NoAllowlist", func(t *testing.T) {
			tool := chattool.ReadTemplate(db, org.ID, chattool.ReadTemplateOptions{
				OwnerID: user.ID,
			})
			input := `{"template_id":"` + t2.ID.String() + `"}`
			resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "c7", Name: "read_template", Input: input})
			require.NoError(t, err)
			require.False(t, resp.IsError)
		})
	})

	t.Run("CreateWorkspace", func(t *testing.T) {
		t.Run("Allowed", func(t *testing.T) {
			// CreateWorkspace requires a real chat row so the existing
			// workspace lookup can fall through to creation.
			model := seedModelConfig(t, db)
			chat, err := db.InsertChat(ctx, database.InsertChatParams{
				OrganizationID:    org.ID,
				OwnerID:           user.ID,
				LastModelConfigID: model.ID,
				Title:             "allowed-create",
				Status:            database.ChatStatusWaiting,
				ClientType:        database.ChatClientTypeApi,
			})
			require.NoError(t, err)

			createCalled := false
			tool := chattool.CreateWorkspace(db, org.ID, chat.ID, chattool.CreateWorkspaceOptions{
				OwnerID:            user.ID,
				AllowedTemplateIDs: func() map[uuid.UUID]bool { return map[uuid.UUID]bool{t1.ID: true} },

				CreateFn: func(_ context.Context, _ uuid.UUID, _ codersdk.CreateWorkspaceRequest) (codersdk.Workspace, error) {
					createCalled = true
					return codersdk.Workspace{}, nil
				},
			})

			input := `{"template_id":"` + t1.ID.String() + `"}`
			resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "c8a", Name: "create_workspace", Input: input})
			require.NoError(t, err)
			require.True(t, createCalled, "CreateFn should be called for allowed template")
			// We don't assert resp.IsError here because CreateWorkspace
			// does additional work (asOwner, workspace lookup) that
			// depends on full RBAC setup. The key assertion is that
			// the allowlist gate passed and CreateFn was invoked.
			_ = resp
		})

		t.Run("Disallowed", func(t *testing.T) {
			var createCalled bool
			tool := chattool.CreateWorkspace(db, org.ID, uuid.New(), chattool.CreateWorkspaceOptions{
				OwnerID:            user.ID,
				AllowedTemplateIDs: func() map[uuid.UUID]bool { return map[uuid.UUID]bool{t2.ID: true} },
				CreateFn: func(_ context.Context, _ uuid.UUID, _ codersdk.CreateWorkspaceRequest) (codersdk.Workspace, error) {
					createCalled = true
					t.Fatal("CreateFn should not be called for blocked template")
					return codersdk.Workspace{}, nil
				},
			})

			input := `{"template_id":"` + t1.ID.String() + `"}`
			resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "c8", Name: "create_workspace", Input: input})
			require.NoError(t, err)
			require.True(t, resp.IsError)
			require.Contains(t, resp.Content, "template not available for chat workspaces")
			require.False(t, createCalled, "CreateFn should not be called for blocked template")
		})
	})
}

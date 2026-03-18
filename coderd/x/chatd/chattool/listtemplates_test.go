package chattool_test

import (
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/chatd/chattool"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

func TestListTemplates(t *testing.T) {
	t.Parallel()

	t.Run("ReturnsAllWhenNoAllowlist", func(t *testing.T) {
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
			Name:           "template-one",
		})
		t2 := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
			Name:           "template-two",
		})

		tool := chattool.ListTemplates(chattool.ListTemplatesOptions{
			DB:      db,
			OwnerID: user.ID,
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "list_templates", Input: "{}"})
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		templates := result["templates"].([]any)
		require.Len(t, templates, 2)

		// Collect returned IDs.
		returnedIDs := make(map[string]bool)
		for _, tmpl := range templates {
			m := tmpl.(map[string]any)
			returnedIDs[m["id"].(string)] = true
		}
		require.True(t, returnedIDs[t1.ID.String()])
		require.True(t, returnedIDs[t2.ID.String()])
	})

	t.Run("FiltersToAllowlist", func(t *testing.T) {
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
			Name:           "allowed-template",
		})
		_ = dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
			Name:           "blocked-template",
		})

		tool := chattool.ListTemplates(chattool.ListTemplatesOptions{
			DB:                 db,
			OwnerID:            user.ID,
			AllowedTemplateIDs: []uuid.UUID{t1.ID},
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "list_templates", Input: "{}"})
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		templates := result["templates"].([]any)
		require.Len(t, templates, 1)
		m := templates[0].(map[string]any)
		require.Equal(t, t1.ID.String(), m["id"].(string))
	})

	t.Run("EmptyAllowlistReturnsAll", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		_ = dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
			Name:           "any-template",
		})

		tool := chattool.ListTemplates(chattool.ListTemplatesOptions{
			DB:                 db,
			OwnerID:            user.ID,
			AllowedTemplateIDs: []uuid.UUID{}, // empty = all allowed
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "list_templates", Input: "{}"})
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		templates := result["templates"].([]any)
		require.Len(t, templates, 1)
	})

	t.Run("AllowlistWithNoMatchesReturnsEmpty", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		_ = dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
			Name:           "some-template",
		})

		tool := chattool.ListTemplates(chattool.ListTemplatesOptions{
			DB:                 db,
			OwnerID:            user.ID,
			AllowedTemplateIDs: []uuid.UUID{uuid.New()}, // no match
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "list_templates", Input: "{}"})
		require.NoError(t, err)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		templates := result["templates"].([]any)
		require.Empty(t, templates)
	})
}

func TestReadTemplateAllowlist(t *testing.T) {
	t.Parallel()

	t.Run("AllowedTemplateSucceeds", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		tmpl := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
			Name:           "allowed",
		})

		tool := chattool.ReadTemplate(chattool.ReadTemplateOptions{
			DB:                 db,
			OwnerID:            user.ID,
			AllowedTemplateIDs: []uuid.UUID{tmpl.ID},
		})

		input := `{"template_id":"` + tmpl.ID.String() + `"}`
		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "read_template", Input: input})
		require.NoError(t, err)
		require.False(t, resp.IsError)

		var result map[string]any
		require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
		tmplInfo := result["template"].(map[string]any)
		require.Equal(t, tmpl.ID.String(), tmplInfo["id"].(string))
	})

	t.Run("DisallowedTemplateBlocked", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		tmpl := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
			Name:           "blocked",
		})

		tool := chattool.ReadTemplate(chattool.ReadTemplateOptions{
			DB:                 db,
			OwnerID:            user.ID,
			AllowedTemplateIDs: []uuid.UUID{uuid.New()}, // different ID
		})

		input := `{"template_id":"` + tmpl.ID.String() + `"}`
		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "read_template", Input: input})
		require.NoError(t, err)
		require.True(t, resp.IsError)
		require.Contains(t, resp.Content, "not available")
	})

	t.Run("NoAllowlistAllowsAny", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db, _ := dbtestutil.NewDB(t)

		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		tmpl := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
			Name:           "any",
		})

		tool := chattool.ReadTemplate(chattool.ReadTemplateOptions{
			DB:      db,
			OwnerID: user.ID,
			// AllowedTemplateIDs not set = nil = all allowed
		})

		input := `{"template_id":"` + tmpl.ID.String() + `"}`
		resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "read_template", Input: input})
		require.NoError(t, err)
		require.False(t, resp.IsError)
	})
}

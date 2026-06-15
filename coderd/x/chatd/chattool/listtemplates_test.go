package chattool_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
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

func TestListTemplates_ReadmeExcerpt(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})

	longReadme := strings.TrimSpace(strings.Repeat("Go with Docker. ", 90))
	withReadme := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Readme:         longReadme + "\n",
	})
	tWith := dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		CreatedBy:       user.ID,
		Name:            "with-readme",
		ActiveVersionID: withReadme.ID,
	})

	// A README that opens with a frontmatter block: the excerpt must skip the
	// metadata and surface the body prose instead.
	frontmatterReadme := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Readme:         "---\ndisplay_name: With Frontmatter\ntags: [a, b]\n---\nRouting prose for the agent.\n",
	})
	tFrontmatter := dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		CreatedBy:       user.ID,
		Name:            "frontmatter-readme",
		ActiveVersionID: frontmatterReadme.ID,
	})

	emptyReadme := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Readme:         " \n\t\n",
	})
	_ = dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		CreatedBy:       user.ID,
		Name:            "empty-readme",
		ActiveVersionID: emptyReadme.ID,
	})

	// A frontmatter-only README has no body, so the excerpt is omitted.
	frontmatterOnlyReadme := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Readme:         "---\ndisplay_name: Only Frontmatter\n---\n",
	})
	_ = dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		CreatedBy:       user.ID,
		Name:            "frontmatter-only",
		ActiveVersionID: frontmatterOnlyReadme.ID,
	})

	_ = dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		CreatedBy:       user.ID,
		Name:            "missing-version",
		ActiveVersionID: uuid.New(),
	})

	ctx := testutil.Context(t, testutil.WaitShort)
	tool := chattool.ListTemplates(db, org.ID, chattool.ListTemplatesOptions{OwnerID: user.ID})
	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "list", Name: "list_templates", Input: "{}"})
	require.NoError(t, err)
	require.False(t, resp.IsError, "unexpected error: %s", resp.Content)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	items := result["templates"].([]any)

	byID := make(map[string]map[string]any, len(items))
	gotHasExcerpt := make(map[string]bool, len(items))
	for _, it := range items {
		m := it.(map[string]any)
		byID[m["id"].(string)] = m
		_, ok := m["readme_excerpt"]
		gotHasExcerpt[m["name"].(string)] = ok
	}

	// Assert which templates surface a readme_excerpt in a single structural
	// diff: present for real prose (with or without frontmatter), omitted when
	// the body is blank, frontmatter-only, or the active version is missing.
	wantHasExcerpt := map[string]bool{
		"with-readme":        true,
		"frontmatter-readme": true,
		"empty-readme":       false,
		"frontmatter-only":   false,
		"missing-version":    false,
	}
	if diff := cmp.Diff(wantHasExcerpt, gotHasExcerpt); diff != "" {
		t.Fatalf("readme_excerpt presence mismatch (-want +got):\n%s", diff)
	}

	// The long README is truncated to the cap and ends with an ellipsis so the
	// agent can tell a clipped excerpt from a complete one.
	excerpt, ok := byID[tWith.ID.String()]["readme_excerpt"].(string)
	require.True(t, ok)
	excerptRunes := []rune(excerpt)
	require.Len(t, excerptRunes, chattool.ListTemplatesReadmeExcerptMaxRunes)
	require.Equal(t, '…', excerptRunes[len(excerptRunes)-1])
	require.Equal(t, string([]rune(longReadme)[:chattool.ListTemplatesReadmeExcerptMaxRunes-1]), string(excerptRunes[:len(excerptRunes)-1]))

	// Frontmatter is skipped so the body prose fills the excerpt.
	require.Equal(t, "Routing prose for the agent.", byID[tFrontmatter.ID.String()]["readme_excerpt"],
		"readme_excerpt should skip frontmatter and surface the body")
}

// TestListTemplates_ReadmeExcerpt_NonOwnerRBAC runs list_templates under a
// real dbauthz wrapper as an ordinary org member (not the site owner) and
// asserts readme_excerpt is still surfaced. This guards against regressing to
// a system-scoped version query that only the owner role can run.
func TestListTemplates_ReadmeExcerpt_NonOwnerRBAC(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	member := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         member.ID,
		OrganizationID: org.ID,
	})
	ctx := testutil.Context(t, testutil.WaitShort)
	readme := "# Title\n\nMember-visible routing context.\n"
	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      member.ID,
		Readme:         readme,
	})
	tmpl := dbgen.Template(t, db, database.Template{
		OrganizationID:  org.ID,
		CreatedBy:       member.ID,
		Name:            "with-readme",
		ActiveVersionID: tv.ID,
	})
	// Link the version to the template so GetTemplateVersionByID authorizes via
	// the parent template (the production path), not the broader org-level
	// fallback used for unlinked versions.
	require.NoError(t, db.UpdateTemplateVersionByID(ctx, database.UpdateTemplateVersionByIDParams{
		ID:         tv.ID,
		TemplateID: uuid.NullUUID{UUID: tmpl.ID, Valid: true},
		UpdatedAt:  tv.UpdatedAt,
		Name:       tv.Name,
		Message:    tv.Message,
	}))

	// Seed with the raw store, but run the tool through a dbauthz-wrapped store
	// so the tool executes under real RBAC as the member.
	authzDB := dbauthz.New(
		db,
		rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()),
		slogtest.Make(t, nil),
		testAccessControlStorePointer(),
	)

	tool := chattool.ListTemplates(authzDB, org.ID, chattool.ListTemplatesOptions{OwnerID: member.ID})
	resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "list", Name: "list_templates", Input: "{}"})
	require.NoError(t, err)
	require.False(t, resp.IsError, "unexpected error: %s", resp.Content)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	items := result["templates"].([]any)
	require.Len(t, items, 1)
	m := items[0].(map[string]any)
	require.Equal(t, tmpl.ID.String(), m["id"])
	require.Equal(t, "Title Member-visible routing context.", m["readme_excerpt"],
		"a non-owner member must still receive readme_excerpt")
}

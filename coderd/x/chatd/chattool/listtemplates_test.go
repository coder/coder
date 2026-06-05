package chattool_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
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
		require.Equal(t, "no_confident_match", result["selection_hint"])
		require.Equal(t, "no_ranking_signal", result["recommendation_reason"])
		_, ok := result["recommended_template_id"]
		require.False(t, ok)
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

func TestListTemplates_QueryMatchesDisplayNameAndDescription(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})

	displayTemplate := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "tpl-42",
		DisplayName:    "Data Science Lab",
	})
	descriptionTemplate := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "node-general",
		Description:    "A JavaScript and TypeScript workspace.",
	})
	_ = dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "unrelated",
		Description:    "A plain Linux workspace.",
	})

	tool := chattool.ListTemplates(db, org.ID, chattool.ListTemplatesOptions{
		OwnerID: user.ID,
	})

	result := runListTemplates(ctx, t, tool, `{"query":"Data Science"}`)
	templates := listTemplateItems(t, result)
	require.Len(t, templates, 1)
	require.Equal(t, displayTemplate.ID.String(), templates[0]["id"])
	require.Equal(t, "high_confidence_recommendation", result["selection_hint"])
	require.Equal(t, false, result["user_selection_required"])
	require.Equal(t, displayTemplate.ID.String(), result["recommended_template_id"])
	require.Equal(t, "matches_query", templates[0]["relevance_signals"])

	result = runListTemplates(ctx, t, tool, `{"query":"TypeScript"}`)
	templates = listTemplateItems(t, result)
	require.Len(t, templates, 1)
	require.Equal(t, descriptionTemplate.ID.String(), templates[0]["id"])
	require.Equal(t, "high_confidence_recommendation", result["selection_hint"])
	require.Equal(t, descriptionTemplate.ID.String(), result["recommended_template_id"])

	result = runListTemplates(ctx, t, tool, `{"query":"-"}`)
	templates = listTemplateItems(t, result)
	require.Empty(t, templates)
	require.Equal(t, float64(0), result["total_count"])
	require.Equal(t, float64(3), result["available_template_count"])
	require.Equal(t, "no_confident_match", result["selection_hint"])
	require.Equal(t, true, result["user_selection_required"])
	require.Equal(t, "no_matching_templates", result["recommendation_reason"])

	result = runListTemplates(ctx, t, tool, `{"query":"does-not-exist"}`)
	templates = listTemplateItems(t, result)
	require.Empty(t, templates)
	require.Equal(t, float64(0), result["total_count"])
	require.Equal(t, float64(3), result["available_template_count"])
	require.Equal(t, "no_confident_match", result["selection_hint"])
	require.Equal(t, true, result["user_selection_required"])
	require.Equal(t, "no_matching_templates", result["recommendation_reason"])
}

func TestListTemplates_QueryScoreTiers(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})

	exact := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "python",
	})
	prefix := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "python-alpha",
	})
	contains := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "go-python",
	})
	description := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "generic-dev",
		Description:    "Python-capable general environment.",
	})

	tool := chattool.ListTemplates(db, org.ID, chattool.ListTemplatesOptions{
		OwnerID: user.ID,
	})
	result := runListTemplates(ctx, t, tool, `{"query":"python"}`)
	templates := listTemplateItems(t, result)
	require.Len(t, templates, 4)
	require.Equal(t, exact.ID.String(), templates[0]["id"])
	require.Equal(t, prefix.ID.String(), templates[1]["id"])
	require.Equal(t, contains.ID.String(), templates[2]["id"])
	require.Equal(t, description.ID.String(), templates[3]["id"])

	hyphenated := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "python-gpu",
	})
	result = runListTemplates(ctx, t, tool, `{"query":"python gpu"}`)
	templates = listTemplateItems(t, result)
	require.Len(t, templates, 1)
	require.Equal(t, hyphenated.ID.String(), templates[0]["id"])

	descriptionHyphenated := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "ml-tools",
		Description:    "Includes machine-learning libraries.",
	})
	result = runListTemplates(ctx, t, tool, `{"query":"machine learning"}`)
	templates = listTemplateItems(t, result)
	require.Len(t, templates, 1)
	require.Equal(t, descriptionHyphenated.ID.String(), templates[0]["id"])
}

func TestListTemplates_RanksAllCandidatesBeforePagination(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})

	var target database.Template
	for i := range 11 {
		tpl := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      user.ID,
			Name:           fmt.Sprintf("template-%02d", i),
		})
		if i == 10 {
			target = tpl
		}
	}
	dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     target.ID,
		LastUsedAt:     time.Now().Add(-time.Hour),
	})

	tool := chattool.ListTemplates(db, org.ID, chattool.ListTemplatesOptions{
		OwnerID: user.ID,
	})
	result := runListTemplates(ctx, t, tool, `{}`)
	templates := listTemplateItems(t, result)
	require.Len(t, templates, 10)
	require.Equal(t, float64(11), result["total_count"])
	require.Equal(t, float64(2), result["total_pages"])
	require.Equal(t, target.ID.String(), templates[0]["id"])
	require.Equal(t, float64(1), templates[0]["rank"])
	require.Equal(t, float64(1), templates[0]["your_workspace_count"])
	require.NotEmpty(t, templates[0]["last_used_by_you"])
	require.Equal(t, true, templates[0]["recommended"])
	require.Equal(t, "used_by_you", templates[0]["relevance_signals"])
	require.Equal(t, "high_confidence_recommendation", result["selection_hint"])
	require.Equal(t, target.ID.String(), result["recommended_template_id"])

	result = runListTemplates(ctx, t, tool, `{"page":2}`)
	templates = listTemplateItems(t, result)
	require.Len(t, templates, 1)
	require.Equal(t, float64(2), result["page"])
	require.Equal(t, float64(11), templates[0]["rank"])
}

func TestListTemplates_QueryRelevanceOutranksPersonalUsage(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})

	target := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "python-gpu",
		Description:    "GPU workspace.",
	})
	used := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "generic-dev",
		Description:    "Python-capable general environment.",
	})
	dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     used.ID,
		LastUsedAt:     time.Now().Add(-14 * 24 * time.Hour),
	})

	tool := chattool.ListTemplates(db, org.ID, chattool.ListTemplatesOptions{
		OwnerID: user.ID,
	})
	result := runListTemplates(ctx, t, tool, `{"query":"python"}`)
	templates := listTemplateItems(t, result)
	require.Len(t, templates, 2)
	require.Equal(t, target.ID.String(), templates[0]["id"])
	require.Equal(t, used.ID.String(), templates[1]["id"])
	require.Equal(t, "matches_query", templates[0]["relevance_signals"])
	require.Equal(t, "matches_query_and_used_by_you", templates[1]["relevance_signals"])
	require.Equal(t, "high_confidence_recommendation", result["selection_hint"])
	require.Equal(t, target.ID.String(), result["recommended_template_id"])
}

func TestListTemplates_PersonalUsageBreaksEqualQueryScoreTie(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})

	unused := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "python-alpha",
	})
	used := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "python-beta",
	})
	dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     used.ID,
		LastUsedAt:     time.Now().Add(-time.Hour),
	})

	tool := chattool.ListTemplates(db, org.ID, chattool.ListTemplatesOptions{
		OwnerID: user.ID,
	})
	result := runListTemplates(ctx, t, tool, `{"query":"python"}`)
	templates := listTemplateItems(t, result)
	require.Len(t, templates, 2)
	require.Equal(t, used.ID.String(), templates[0]["id"])
	require.Equal(t, unused.ID.String(), templates[1]["id"])
	require.Equal(t, "matches_query_and_used_by_you", templates[0]["relevance_signals"])
	require.Equal(t, "high_confidence_recommendation", result["selection_hint"])
	require.Equal(t, used.ID.String(), result["recommended_template_id"])
}

func TestListTemplates_OrgPopularityFallback(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})

	popular := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "popular-template",
	})
	lessPopular := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "less-popular-template",
	})
	for range 2 {
		otherUser := dbgen.User(t, db, database.User{})
		dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        otherUser.ID,
			OrganizationID: org.ID,
			TemplateID:     popular.ID,
		})
	}
	otherUser := dbgen.User(t, db, database.User{})
	dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        otherUser.ID,
		OrganizationID: org.ID,
		TemplateID:     lessPopular.ID,
	})

	tool := chattool.ListTemplates(db, org.ID, chattool.ListTemplatesOptions{
		OwnerID: user.ID,
	})
	result := runListTemplates(ctx, t, tool, `{}`)
	templates := listTemplateItems(t, result)
	require.Len(t, templates, 2)
	require.Equal(t, popular.ID.String(), templates[0]["id"])
	require.Equal(t, float64(2), templates[0]["active_developers"])
	require.Equal(t, "popular_in_org", templates[0]["relevance_signals"])
	require.Equal(t, "high_confidence_recommendation", result["selection_hint"])
	require.Equal(t, popular.ID.String(), result["recommended_template_id"])
}

func TestListTemplates_WeakOrgPopularityDoesNotRecommend(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})

	usedByOne := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "used-by-one",
	})
	unused := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "unused",
	})
	otherUser := dbgen.User(t, db, database.User{})
	dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        otherUser.ID,
		OrganizationID: org.ID,
		TemplateID:     usedByOne.ID,
	})

	tool := chattool.ListTemplates(db, org.ID, chattool.ListTemplatesOptions{
		OwnerID: user.ID,
	})
	result := runListTemplates(ctx, t, tool, `{}`)
	templates := listTemplateItems(t, result)
	require.Len(t, templates, 2)
	require.Equal(t, usedByOne.ID.String(), templates[0]["id"])
	require.Equal(t, unused.ID.String(), templates[1]["id"])
	require.Equal(t, float64(1), templates[0]["active_developers"])
	require.Equal(t, "no_confident_match", result["selection_hint"])
	require.Equal(t, "weak_ranking_signal", result["recommendation_reason"])
	_, ok := result["recommended_template_id"]
	require.False(t, ok)
}

func TestListTemplates_StalePersonalUsageDoesNotRecommend(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	clock.Set(now).MustWait(ctx)
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})

	oldUsage := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "old-usage",
	})
	unused := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "unused",
	})
	dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     oldUsage.ID,
		LastUsedAt:     now.Add(-180 * 24 * time.Hour),
	})

	tool := chattool.ListTemplates(db, org.ID, chattool.ListTemplatesOptions{
		OwnerID: user.ID,
		Clock:   clock,
	})
	result := runListTemplates(ctx, t, tool, `{}`)
	templates := listTemplateItems(t, result)
	require.Len(t, templates, 2)
	require.Equal(t, oldUsage.ID.String(), templates[0]["id"])
	require.Equal(t, unused.ID.String(), templates[1]["id"])
	// The 180-day-old workspace is outside the 60-day lookback window, so it no
	// longer counts as in-window personal usage.
	_, hasCount := templates[0]["your_workspace_count"]
	require.False(t, hasCount)
	require.Equal(t, "no_confident_match", result["selection_hint"])
	require.Equal(t, "weak_ranking_signal", result["recommendation_reason"])
	_, ok := result["recommended_template_id"]
	require.False(t, ok)
}

func TestListTemplates_StaleFrequentPersonalUsageDoesNotRecommend(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	clock.Set(now).MustWait(ctx)
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})

	staleUsage := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "stale-usage",
	})
	unused := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "unused",
	})
	// Two workspaces used 180 days ago. Frequency no longer dominates recency:
	// usage outside the lookback window decays out of the personal signal, so a
	// frequently-but-stalely-used template is no longer a confident match.
	for range 2 {
		dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
			TemplateID:     staleUsage.ID,
			LastUsedAt:     now.Add(-180 * 24 * time.Hour),
		})
	}

	tool := chattool.ListTemplates(db, org.ID, chattool.ListTemplatesOptions{
		OwnerID: user.ID,
		Clock:   clock,
	})
	result := runListTemplates(ctx, t, tool, `{}`)
	templates := listTemplateItems(t, result)
	require.Len(t, templates, 2)
	require.Equal(t, staleUsage.ID.String(), templates[0]["id"])
	require.Equal(t, unused.ID.String(), templates[1]["id"])
	require.Equal(t, "no_confident_match", result["selection_hint"])
	require.Equal(t, "weak_ranking_signal", result["recommendation_reason"])
	_, ok := result["recommended_template_id"]
	require.False(t, ok)
	// The stale workspaces fall outside the lookback window, so no in-window
	// personal count is surfaced.
	_, hasCount := templates[0]["your_workspace_count"]
	require.False(t, hasCount)
}

func TestListTemplates_RecentPersonalUsageRecommends(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	clock.Set(now).MustWait(ctx)
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})

	recentUsage := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "recent-usage",
	})
	unused := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "unused",
	})
	// Two workspaces used two days ago. Recent, in-window usage is a confident
	// signal: this is the frecency improvement over the old count-only ranking.
	for range 2 {
		dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        user.ID,
			OrganizationID: org.ID,
			TemplateID:     recentUsage.ID,
			LastUsedAt:     now.Add(-2 * 24 * time.Hour),
		})
	}

	tool := chattool.ListTemplates(db, org.ID, chattool.ListTemplatesOptions{
		OwnerID: user.ID,
		Clock:   clock,
	})
	result := runListTemplates(ctx, t, tool, `{}`)
	templates := listTemplateItems(t, result)
	require.Len(t, templates, 2)
	require.Equal(t, recentUsage.ID.String(), templates[0]["id"])
	require.Equal(t, unused.ID.String(), templates[1]["id"])
	require.Equal(t, float64(2), templates[0]["your_workspace_count"])
	require.Equal(t, "used_by_you", templates[0]["relevance_signals"])
	require.Equal(t, "high_confidence_recommendation", result["selection_hint"])
	require.Equal(t, recentUsage.ID.String(), result["recommended_template_id"])
}

func TestListTemplates_DeletedRecentPersonalUsageShowsEvidence(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	clock.Set(now).MustWait(ctx)
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})

	deletedUsage := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "deleted-usage",
	})
	unused := dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "unused",
	})
	dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID:        user.ID,
		OrganizationID: org.ID,
		TemplateID:     deletedUsage.ID,
		LastUsedAt:     now.Add(-2 * 24 * time.Hour),
		Deleted:        true,
	})

	tool := chattool.ListTemplates(db, org.ID, chattool.ListTemplatesOptions{
		OwnerID: user.ID,
		Clock:   clock,
	})
	result := runListTemplates(ctx, t, tool, `{}`)
	templates := listTemplateItems(t, result)
	require.Len(t, templates, 2)
	require.Equal(t, deletedUsage.ID.String(), templates[0]["id"])
	require.Equal(t, unused.ID.String(), templates[1]["id"])
	require.Equal(t, "used_by_you", templates[0]["relevance_signals"])
	require.Equal(t, float64(1), templates[0]["your_recently_deleted_workspace_count"])
	require.NotEmpty(t, templates[0]["last_used_by_you"])
	_, hasActiveCount := templates[0]["your_workspace_count"]
	require.False(t, hasActiveCount)
}

func TestListTemplates_AmbiguousTopMatches(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
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
		Name:           "go-alpha",
	})
	_ = dbgen.Template(t, db, database.Template{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
		Name:           "go-beta",
	})

	tool := chattool.ListTemplates(db, org.ID, chattool.ListTemplatesOptions{
		OwnerID: user.ID,
	})
	result := runListTemplates(ctx, t, tool, `{"query":"go"}`)
	templates := listTemplateItems(t, result)
	require.Len(t, templates, 2)
	require.Equal(t, "ambiguous_top_matches", result["selection_hint"])
	_, ok := result["recommended_template_id"]
	require.False(t, ok)
	_, ok = templates[0]["recommended"]
	require.False(t, ok)
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
			require.Equal(t, "only_available_template", result["selection_hint"])
			require.Equal(t, t1.ID.String(), result["recommended_template_id"])
			require.Equal(t, true, m["recommended"])
			require.Equal(t, float64(1), m["rank"])
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
			require.Equal(t, "no_confident_match", result["selection_hint"])
			require.Equal(t, "no_matching_templates", result["recommendation_reason"])
			_, ok := result["recommended_template_id"]
			require.False(t, ok)
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

// TestGetTemplateRankingSignalsByOwnerID exercises the raw SQL signals query:
// the lookback window, the active/deleted split, and excluding the prebuilds
// system user from the organization developer count.
func TestGetTemplateRankingSignalsByOwnerID(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	db, _ := dbtestutil.NewDB(t)

	now := time.Now()
	lookbackCutoff := now.Add(-60 * 24 * time.Hour)

	user := dbgen.User(t, db, database.User{})
	otherUser := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	for _, u := range []uuid.UUID{user.ID, otherUser.ID} {
		_ = dbgen.OrganizationMember(t, db, database.OrganizationMember{UserID: u, OrganizationID: org.ID})
	}

	used := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID, Name: "used"})
	unused := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID, Name: "unused"})

	activeLastUsedAt := now.Add(-2 * 24 * time.Hour)
	deletedLastUsedAt := now.Add(-3 * 24 * time.Hour)
	// Active, in-window workspace for the requesting user.
	dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID: user.ID, OrganizationID: org.ID, TemplateID: used.ID,
		LastUsedAt: activeLastUsedAt,
	})
	// Recently-deleted, in-window workspace for the requesting user.
	dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID: user.ID, OrganizationID: org.ID, TemplateID: used.ID,
		LastUsedAt: deletedLastUsedAt, Deleted: true,
	})
	// Non-deleted but outside the lookback window: it must not count toward the
	// in-window active count, though it still keeps the user in the org count.
	dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID: user.ID, OrganizationID: org.ID, TemplateID: used.ID,
		LastUsedAt: now.Add(-90 * 24 * time.Hour),
	})
	// Another developer's active workspace contributes to org popularity.
	dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID: otherUser.ID, OrganizationID: org.ID, TemplateID: used.ID,
		LastUsedAt: now.Add(-1 * 24 * time.Hour),
	})
	// The prebuilds system user must be excluded from the org developer count.
	dbgen.Workspace(t, db, database.WorkspaceTable{
		OwnerID: database.PrebuildsSystemUserID, OrganizationID: org.ID, TemplateID: used.ID,
		LastUsedAt: now.Add(-1 * 24 * time.Hour),
	})

	rows, err := db.GetTemplateRankingSignalsByOwnerID(ctx, database.GetTemplateRankingSignalsByOwnerIDParams{
		TemplateIDs:     []uuid.UUID{used.ID, unused.ID},
		OwnerID:         user.ID,
		OrganizationID:  org.ID,
		PrebuildsUserID: database.PrebuildsSystemUserID,
		LookbackCutoff:  lookbackCutoff,
	})
	require.NoError(t, err)

	byTemplate := make(map[uuid.UUID]database.GetTemplateRankingSignalsByOwnerIDRow, len(rows))
	for _, row := range rows {
		byTemplate[row.TemplateID] = row
	}
	// The unnest LEFT JOIN returns a row for every requested template.
	require.Len(t, byTemplate, 2)

	usedRow := byTemplate[used.ID]
	require.Equal(t, int64(1), usedRow.ActiveCount, "only the in-window active workspace counts")
	require.Equal(t, int64(1), usedRow.DeletedRecentCount, "the in-window deleted workspace counts")
	require.Equal(t, int64(2), usedRow.OrgDevs, "user and otherUser count; prebuilds user is excluded")
	require.True(t, usedRow.LastUsedAt.Valid)
	require.WithinDuration(t, activeLastUsedAt, usedRow.LastUsedAt.Time, time.Microsecond)

	unusedRow := byTemplate[unused.ID]
	require.Equal(t, int64(0), unusedRow.ActiveCount)
	require.Equal(t, int64(0), unusedRow.DeletedRecentCount)
	require.Equal(t, int64(0), unusedRow.OrgDevs)
	require.False(t, unusedRow.LastUsedAt.Valid)
}

func runListTemplates(
	ctx context.Context,
	t *testing.T,
	tool fantasy.AgentTool,
	input string,
) map[string]any {
	t.Helper()

	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    uuid.NewString(),
		Name:  "list_templates",
		Input: input,
	})
	require.NoError(t, err)
	require.False(t, resp.IsError)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &result))
	return result
}

func listTemplateItems(t *testing.T, result map[string]any) []map[string]any {
	t.Helper()

	rawTemplates, ok := result["templates"].([]any)
	require.True(t, ok)
	templates := make([]map[string]any, 0, len(rawTemplates))
	for _, raw := range rawTemplates {
		template, ok := raw.(map[string]any)
		require.True(t, ok)
		templates = append(templates, template)
	}
	return templates
}

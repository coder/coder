package chatstate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestReplaceInlineMCPServers(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	ctx := dbauthz.AsSystemRestricted(testutil.Context(t, testutil.WaitShort))
	user, org, model := seedFamilyDeps(t, db)
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
	})

	err := chatstate.ReplaceInlineMCPServers(ctx, db, chat.ID, []codersdk.InlineMCPServerRequest{
		{Slug: "a", URL: "https://a.example.com/mcp", Headers: map[string]string{"Authorization": "Bearer a"}},
		{Slug: "b", URL: "https://b.example.com/mcp"},
	})
	require.NoError(t, err)

	rows, err := db.GetChatMCPServersByChatID(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	bySlug := map[string]database.ChatMCPServer{}
	for _, row := range rows {
		bySlug[row.Slug] = row
	}
	require.JSONEq(t, `{"Authorization":"Bearer a"}`, bySlug["a"].Headers)
	require.Equal(t, "{}", bySlug["b"].Headers)
	require.NotNil(t, bySlug["b"].ToolAllowList)
	originalAID := bySlug["a"].ID

	err = chatstate.ReplaceInlineMCPServers(ctx, db, chat.ID, []codersdk.InlineMCPServerRequest{
		{Slug: "a", URL: "https://a2.example.com/mcp"},
		{Slug: "c", URL: "https://c.example.com/mcp", ToolAllowList: []string{"lookup"}},
	})
	require.NoError(t, err)

	rows, err = db.GetChatMCPServersByChatID(ctx, chat.ID)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	bySlug = map[string]database.ChatMCPServer{}
	for _, row := range rows {
		bySlug[row.Slug] = row
	}
	require.NotContains(t, bySlug, "b")
	require.Equal(t, originalAID, bySlug["a"].ID)
	require.Equal(t, "https://a2.example.com/mcp", bySlug["a"].Url)
	require.Equal(t, "{}", bySlug["a"].Headers)
	require.Equal(t, []string{"lookup"}, bySlug["c"].ToolAllowList)

	err = chatstate.ReplaceInlineMCPServers(ctx, db, chat.ID, []codersdk.InlineMCPServerRequest{})
	require.NoError(t, err)

	rows, err = db.GetChatMCPServersByChatID(ctx, chat.ID)
	require.NoError(t, err)
	require.Empty(t, rows)
}

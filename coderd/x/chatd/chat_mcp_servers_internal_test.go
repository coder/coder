package chatd

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

func TestLoadChatMCPServers(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	root := createInternalParentChat(ctx, t, server, db, org.ID, user.ID, model.ID, "chat-mcp-root")

	shared := dbgen.ChatMCPServer(t, db, database.ChatMCPServer{
		ChatID:           root.ID,
		Slug:             "shared",
		Url:              "https://shared.example.com/mcp",
		Headers:          `{"X-Bot-Key":"secret","Authorization":"Bearer token"}`,
		ToolAllowList:    []string{"echo"},
		AllowInPlanMode:  true,
		AllowInSubagents: true,
	})
	rootOnly := dbgen.ChatMCPServer(t, db, database.ChatMCPServer{
		ChatID:              root.ID,
		Slug:                "root-only",
		Url:                 "https://root.example.com/mcp",
		ForwardCoderHeaders: true,
	})

	t.Run("Root", func(t *testing.T) {
		t.Parallel()

		ctx := chatdTestContext(t)
		configs, sensitive, err := server.loadChatMCPServers(ctx, root)
		require.NoError(t, err)
		require.Len(t, configs, 2)

		bySlug := make(map[string]database.MCPServerConfig, len(configs))
		for _, cfg := range configs {
			bySlug[cfg.Slug] = cfg
			require.Equal(t, "streamable_http", cfg.Transport)
			require.True(t, cfg.Enabled)
			require.Equal(t, org.ID, cfg.OrganizationID)
			require.Equal(t, cfg.Slug, cfg.DisplayName)
		}
		require.Equal(t, shared.ID, bySlug["shared"].ID)
		require.Equal(t, "custom_headers", bySlug["shared"].AuthType)
		require.Equal(t, shared.Headers, bySlug["shared"].CustomHeaders)
		require.Equal(t, []string{"echo"}, bySlug["shared"].ToolAllowList)
		require.True(t, bySlug["shared"].AllowInPlanMode)
		require.Equal(t, rootOnly.ID, bySlug["root-only"].ID)
		require.Equal(t, "none", bySlug["root-only"].AuthType)
		require.True(t, bySlug["root-only"].ForwardCoderHeaders)

		require.Equal(t, map[uuid.UUID][]string{
			shared.ID:   {"https://shared.example.com/mcp", "Authorization", "Bearer token", "X-Bot-Key", "secret"},
			rootOnly.ID: {"https://root.example.com/mcp"},
		}, sensitive)
	})

	t.Run("ChildKeepsAllowInSubagents", func(t *testing.T) {
		t.Parallel()

		ctx := chatdTestContext(t)
		child, err := server.createChildSubagentChatWithOptions(ctx, root, "delegate work", "", childSubagentChatOptions{})
		require.NoError(t, err)

		configs, sensitive, err := server.loadChatMCPServers(ctx, child)
		require.NoError(t, err)
		require.Len(t, configs, 1)
		require.Equal(t, shared.ID, configs[0].ID)
		require.Len(t, sensitive, 1)
		require.Contains(t, sensitive, shared.ID)
	})

	t.Run("ExploreChildLoadsNone", func(t *testing.T) {
		t.Parallel()

		ctx := chatdTestContext(t)
		child, err := server.createChildSubagentChatWithOptions(ctx, root, "inspect", "", childSubagentChatOptions{
			chatMode: database.NullChatMode{ChatMode: database.ChatModeExplore, Valid: true},
		})
		require.NoError(t, err)

		configs, sensitive, err := server.loadChatMCPServers(ctx, child)
		require.NoError(t, err)
		require.Empty(t, configs)
		require.Empty(t, sensitive)
	})
}

package chatd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/codersdk"
)

// TestInlineMCPServerHeaderValueMinimum pins the API minimum to the
// redactor minimum so every accepted header value is redacted.
func TestInlineMCPServerHeaderValueMinimum(t *testing.T) {
	t.Parallel()
	require.GreaterOrEqual(t, codersdk.MinInlineMCPServerHeaderValueBytes, mcpclient.MinSensitiveValueBytes)
}

func TestLoadInlineMCPServers(t *testing.T) {
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
		Headers:          `{"X-Bot-Key":"bot-secret","Authorization":"Bearer token"}`,
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
	// The API never stores this shape; it stands in for a corrupted row.
	badHeaders := dbgen.ChatMCPServer(t, db, database.ChatMCPServer{
		ChatID:  root.ID,
		Slug:    "bad-headers",
		Url:     "https://bad.example.com/mcp",
		Headers: "not json",
	})

	t.Run("Root", func(t *testing.T) {
		t.Parallel()

		ctx := chatdTestContext(t)
		servers, failures := server.loadInlineMCPServers(ctx, root)
		require.Len(t, servers, 2)
		require.Equal(t, []mcpclient.ConnectSummary{{
			ConfigID: badHeaders.ID,
			Slug:     "bad-headers",
			Outcome:  mcpclient.ConnectOutcomeError,
			Error:    "invalid stored headers",
		}}, failures)

		bySlug := make(map[string]inlineMCPServer, len(servers))
		for _, srv := range servers {
			bySlug[srv.Server.Slug] = srv
			require.Equal(t, mcpclient.TransportStreamableHTTP, srv.Server.Transport)
			require.Equal(t, mcpclient.UserAuthNone, srv.Server.UserAuth)
		}
		require.Equal(t, shared.ID, bySlug["shared"].Server.ID)
		require.Equal(t, "https://shared.example.com/mcp", bySlug["shared"].Server.URL)
		require.Equal(t, map[string]string{
			"X-Bot-Key":     "bot-secret",
			"Authorization": "Bearer token",
		}, bySlug["shared"].Server.Headers)
		require.Equal(t, []string{"echo"}, bySlug["shared"].Server.ToolAllowList)
		require.True(t, bySlug["shared"].AllowInPlanMode)

		require.Equal(t, rootOnly.ID, bySlug["root-only"].Server.ID)
		require.Empty(t, bySlug["root-only"].Server.Headers)
		require.True(t, bySlug["root-only"].Server.ForwardCoderHeaders)
		require.False(t, bySlug["root-only"].AllowInPlanMode)
	})

	t.Run("ChildKeepsAllowInSubagents", func(t *testing.T) {
		t.Parallel()

		ctx := chatdTestContext(t)
		child, err := server.createChildSubagentChatWithOptions(ctx, root, "delegate work", "", childSubagentChatOptions{})
		require.NoError(t, err)

		servers, failures := server.loadInlineMCPServers(ctx, child)
		require.Empty(t, failures, "the bad row is root-only, so a child never reads it")
		require.Len(t, servers, 1)
		require.Equal(t, shared.ID, servers[0].Server.ID)
	})

	t.Run("ExploreChildLoadsNone", func(t *testing.T) {
		t.Parallel()

		ctx := chatdTestContext(t)
		child, err := server.createChildSubagentChatWithOptions(ctx, root, "inspect", "", childSubagentChatOptions{
			chatMode: database.NullChatMode{ChatMode: database.ChatModeExplore, Valid: true},
		})
		require.NoError(t, err)

		servers, failures := server.loadInlineMCPServers(ctx, child)
		require.Empty(t, failures)
		require.Empty(t, servers)
	})
}

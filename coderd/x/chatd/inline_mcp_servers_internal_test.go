package chatd

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/codersdk"
)

func TestLoadInlineMCPServers(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	server := &Server{
		db:          db,
		logger:      slogtest.Make(t, nil),
		experiments: codersdk.Experiments{codersdk.ExperimentChatInlineMCPServers},
	}
	user, org, model := seedInternalChatDeps(t, db)
	newChat := func(chat database.Chat) database.Chat {
		chat.OrganizationID = org.ID
		chat.OwnerID = user.ID
		chat.LastModelConfigID = model.ID
		return dbgen.Chat(t, db, chat)
	}
	root := newChat(database.Chat{})
	rootRef := uuid.NullUUID{UUID: root.ID, Valid: true}
	child := newChat(database.Chat{ParentChatID: rootRef, RootChatID: rootRef})
	exploreChild := newChat(database.Chat{
		ParentChatID: rootRef,
		RootChatID:   rootRef,
		Mode:         database.NullChatMode{ChatMode: database.ChatModeExplore, Valid: true},
	})

	shared := dbgen.ChatMCPServer(t, db, database.ChatMCPServer{
		ChatID:           root.ID,
		Slug:             "shared",
		Url:              "https://shared.example.com/mcp",
		Headers:          `{"X-Bot-Key":"bot-secret","Authorization":"Bearer token"}`,
		ToolAllowList:    []string{"echo"},
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

		bySlug := make(map[string]mcpclient.Server, len(servers))
		for _, srv := range servers {
			bySlug[srv.Slug] = srv
			require.Equal(t, mcpclient.TransportStreamableHTTP, srv.Transport)
			require.Equal(t, mcpclient.UserAuthNone, srv.UserAuth)
		}
		require.Equal(t, shared.ID, bySlug["shared"].ID)
		require.Equal(t, "https://shared.example.com/mcp", bySlug["shared"].URL)
		require.Equal(t, map[string]string{
			"X-Bot-Key":     "bot-secret",
			"Authorization": "Bearer token",
		}, bySlug["shared"].Headers)
		require.Equal(t, []string{"echo"}, bySlug["shared"].ToolAllowList)

		require.Equal(t, rootOnly.ID, bySlug["root-only"].ID)
		require.Empty(t, bySlug["root-only"].Headers)
		require.True(t, bySlug["root-only"].ForwardCoderHeaders)
	})

	t.Run("ChildKeepsAllowInSubagents", func(t *testing.T) {
		t.Parallel()

		ctx := chatdTestContext(t)
		for _, chat := range []database.Chat{child, exploreChild} {
			servers, failures := server.loadInlineMCPServers(ctx, chat)
			require.Empty(t, failures, "the bad row is root-only, so a child never reads it")
			require.Len(t, servers, 1, "chat mode %q", chat.Mode.ChatMode)
			require.Equal(t, shared.ID, servers[0].ID)
		}
	})
}

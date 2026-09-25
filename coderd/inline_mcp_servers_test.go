package coderd_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func chatTextContent(text string) []codersdk.ChatInputPart {
	return []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: text}}
}

func TestInlineMCPServers(t *testing.T) {
	t.Parallel()

	t.Run("CreateAndGet", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t, withChatWorkerDisabled)
		user := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content:        chatTextContent("hello"),
			InlineMCPServers: []codersdk.InlineMCPServerRequest{{
				Slug:                "orders",
				URL:                 "https://mcp.example.com/v1",
				Headers:             map[string]string{"Authorization": "Bearer secret-value-123"},
				ToolAllowList:       []string{"lookup"},
				AllowInSubagents:    true,
				ForwardCoderHeaders: true,
			}},
		})
		require.NoError(t, err)

		rows, err := db.GetChatMCPServersByChatID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, "orders", rows[0].Slug)
		require.JSONEq(t, `{"Authorization":"Bearer secret-value-123"}`, rows[0].Headers)

		got, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		servers := got.InlineMCPServers
		require.Len(t, servers, 1)
		require.Equal(t, rows[0].ID, servers[0].ID)
		require.Equal(t, "orders", servers[0].Slug)
		require.Equal(t, "https://mcp.example.com/v1", servers[0].URL)
		require.True(t, servers[0].HasCustomHeaders)
		require.Equal(t, []string{"lookup"}, servers[0].ToolAllowList)
		require.Equal(t, []string{}, servers[0].ToolDenyList)
		require.True(t, servers[0].AllowInSubagents)
		require.True(t, servers[0].ForwardCoderHeaders)

		raw, err := json.Marshal(got)
		require.NoError(t, err)
		require.NotContains(t, string(raw), "secret-value-123")
	})

	t.Run("ExperimentDisabled", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t, func(o *coderdtest.Options) {
			o.DeploymentValues.Experiments = serpent.StringArray{string(codersdk.ExperimentAgentLifecycleHooks)}
		})
		user := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID:   user.OrganizationID,
			Content:          chatTextContent("hello"),
			InlineMCPServers: []codersdk.InlineMCPServerRequest{{Slug: "orders", URL: "https://mcp.example.com/v1"}},
		})
		sdkErr := requireSDKError(t, err, http.StatusForbidden)
		require.Equal(t, "Inline MCP servers are not enabled on this deployment.", sdkErr.Message)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content:        chatTextContent("hello"),
		})
		require.NoError(t, err)

		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content:          chatTextContent("again"),
			InlineMCPServers: &[]codersdk.InlineMCPServerRequest{{Slug: "orders", URL: "https://mcp.example.com/v1"}},
		})
		sdkErr = requireSDKError(t, err, http.StatusForbidden)
		require.Equal(t, "Inline MCP servers are not enabled on this deployment.", sdkErr.Message)

		// A stored row is still reported, because it can still load (an
		// internal server) or be detached.
		dbgen.ChatMCPServer(t, db, database.ChatMCPServer{ChatID: chat.ID, Slug: "stored"})
		got, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, got.InlineMCPServers, 1)
		require.Equal(t, "stored", got.InlineMCPServers[0].Slug)
	})

	t.Run("CallerSuppliedToolsDisabled", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t, withChatWorkerDisabled, func(o *coderdtest.Options) {
			o.DeploymentValues.DisableChatCallerSuppliedTools = serpent.Bool(true)
		})
		user := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID:   user.OrganizationID,
			Content:          chatTextContent("hello"),
			InlineMCPServers: []codersdk.InlineMCPServerRequest{{Slug: "orders", URL: "https://mcp.example.com/v1"}},
		})
		sdkErr := requireSDKError(t, err, http.StatusForbidden)
		require.Equal(t, "Caller-supplied tools are disabled on this deployment.", sdkErr.Message)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content:        chatTextContent("hello"),
		})
		require.NoError(t, err)

		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content:          chatTextContent("again"),
			InlineMCPServers: &[]codersdk.InlineMCPServerRequest{{Slug: "orders", URL: "https://mcp.example.com/v1"}},
		})
		sdkErr = requireSDKError(t, err, http.StatusForbidden)
		require.Equal(t, "Caller-supplied tools are disabled on this deployment.", sdkErr.Message)

		dbgen.ChatMCPServer(t, db, database.ChatMCPServer{ChatID: chat.ID, Slug: "stale"})
		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content:          chatTextContent("detach"),
			InlineMCPServers: &[]codersdk.InlineMCPServerRequest{},
		})
		require.NoError(t, err)

		rows, err := db.GetChatMCPServersByChatID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.Empty(t, rows)
	})

	t.Run("Validation", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t, withChatWorkerDisabled)
		user := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID:   user.OrganizationID,
			Content:          chatTextContent("hello"),
			InlineMCPServers: []codersdk.InlineMCPServerRequest{{Slug: "bad slug", URL: "https://mcp.example.com/v1"}},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "Invalid inline_mcp_servers.", sdkErr.Message)
		require.NotEmpty(t, sdkErr.Validations)
		require.Equal(t, "inline_mcp_servers[0].slug", sdkErr.Validations[0].Field)

		_, err = client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content:        chatTextContent("hello"),
			InlineMCPServers: []codersdk.InlineMCPServerRequest{{
				Slug:    "local",
				URL:     "http://127.0.0.1:1/mcp",
				Headers: map[string]string{"Authorization": "Bearer x"},
			}},
		})
		require.NoError(t, err)
	})

	t.Run("ReplaceKeepClear", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t, withChatWorkerDisabled)
		user := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)
		dbCtx := dbauthz.AsSystemRestricted(ctx)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content:        chatTextContent("hello"),
			InlineMCPServers: []codersdk.InlineMCPServerRequest{
				{Slug: "a", URL: "https://a.example.com/mcp"},
				{Slug: "b", URL: "https://b.example.com/mcp"},
			},
		})
		require.NoError(t, err)

		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: chatTextContent("keep"),
		})
		require.NoError(t, err)
		rows, err := db.GetChatMCPServersByChatID(dbCtx, chat.ID)
		require.NoError(t, err)
		require.Len(t, rows, 2)

		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content:          chatTextContent("replace"),
			InlineMCPServers: &[]codersdk.InlineMCPServerRequest{{Slug: "c", URL: "https://c.example.com/mcp"}},
		})
		require.NoError(t, err)
		rows, err = db.GetChatMCPServersByChatID(dbCtx, chat.ID)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, "c", rows[0].Slug)

		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content:          chatTextContent("clear"),
			InlineMCPServers: &[]codersdk.InlineMCPServerRequest{},
		})
		require.NoError(t, err)
		rows, err = db.GetChatMCPServersByChatID(dbCtx, chat.ID)
		require.NoError(t, err)
		require.Empty(t, rows)
	})

	t.Run("ChildChatRejected", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t, withChatWorkerDisabled)
		user := coderdtest.CreateFirstUser(t, client.Client)
		model := createChatModel(t, client)

		parent, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content:        chatTextContent("hello"),
		})
		require.NoError(t, err)
		child := dbgen.Chat(t, db, database.Chat{
			OrganizationID:    user.OrganizationID,
			OwnerID:           user.UserID,
			LastModelConfigID: model.ID,
			ParentChatID:      uuid.NullUUID{UUID: parent.ID, Valid: true},
			RootChatID:        uuid.NullUUID{UUID: parent.ID, Valid: true},
		})

		_, err = client.CreateChatMessage(ctx, child.ID, codersdk.CreateChatMessageRequest{
			Content:          chatTextContent("child"),
			InlineMCPServers: &[]codersdk.InlineMCPServerRequest{{Slug: "orders", URL: "https://mcp.example.com/v1"}},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "inline_mcp_servers can only be declared on a root chat.", sdkErr.Message)

		// An empty declaration is still a declaration on a child chat.
		_, err = client.CreateChatMessage(ctx, child.ID, codersdk.CreateChatMessageRequest{
			Content:          chatTextContent("child again"),
			InlineMCPServers: &[]codersdk.InlineMCPServerRequest{},
		})
		sdkErr = requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "inline_mcp_servers can only be declared on a root chat.", sdkErr.Message)
	})

	t.Run("NonOwnerURLHidden", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t, withChatWorkerDisabled)
		user := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID:   user.OrganizationID,
			Content:          chatTextContent("hello"),
			InlineMCPServers: []codersdk.InlineMCPServerRequest{{Slug: "orders", URL: "https://mcp.example.com/v1"}},
		})
		require.NoError(t, err)

		got, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, got.InlineMCPServers, 1)
		require.Equal(t, "https://mcp.example.com/v1", got.InlineMCPServers[0].URL)

		reader, readerUser := coderdtest.CreateAnotherUser(t, client.Client, user.OrganizationID)
		err = client.UpdateChatACL(ctx, chat.ID, codersdk.UpdateChatACL{
			UserRoles: map[string]codersdk.ChatRole{readerUser.ID.String(): codersdk.ChatRoleRead},
		})
		require.NoError(t, err)
		got, err = reader.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, got.InlineMCPServers, 1)
		require.Equal(t, "orders", got.InlineMCPServers[0].Slug)
		require.Empty(t, got.InlineMCPServers[0].URL)
	})

	t.Run("LoadFailureNonFatal", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t, withChatWorkerDisabled)
		user := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content:        chatTextContent("hello"),
		})
		require.NoError(t, err)
		dbgen.ChatMCPServer(t, db, database.ChatMCPServer{ChatID: chat.ID, Slug: "broken", Headers: "not json"})

		got, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.Equal(t, chat.ID, got.ID)
		require.Empty(t, got.InlineMCPServers)
	})
}

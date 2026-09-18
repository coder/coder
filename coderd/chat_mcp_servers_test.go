package coderd_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridgedtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func chatMCPServerRequest(slug, url string) codersdk.ChatMCPServerRequest {
	return codersdk.ChatMCPServerRequest{Slug: slug, URL: url}
}

func chatTextContent(text string) []codersdk.ChatInputPart {
	return []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: text}}
}

func chatMCPServerSlugs(rows []database.ChatMCPServer) map[string]database.ChatMCPServer {
	bySlug := make(map[string]database.ChatMCPServer, len(rows))
	for _, row := range rows {
		bySlug[row.Slug] = row
	}
	return bySlug
}

func TestChatMCPServers(t *testing.T) {
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
			MCPServers: []codersdk.ChatMCPServerRequest{{
				Slug:                "orders",
				URL:                 "https://mcp.example.com/v1",
				Headers:             map[string]string{"Authorization": "Bearer secret-value-123"},
				ToolAllowList:       []string{"lookup"},
				AllowInPlanMode:     true,
				ForwardCoderHeaders: true,
			}},
		})
		require.NoError(t, err)

		rows, err := db.GetChatMCPServersByChatID(dbauthz.AsSystemRestricted(ctx), chat.ID)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, "orders", rows[0].Slug)
		require.JSONEq(t, `{"Authorization":"Bearer secret-value-123"}`, rows[0].Headers)

		servers, err := client.GetChatMCPServers(ctx, chat.ID)
		require.NoError(t, err)
		require.Len(t, servers, 1)
		require.Equal(t, rows[0].ID, servers[0].ID)
		require.Equal(t, "orders", servers[0].Slug)
		require.Equal(t, "https://mcp.example.com/v1", servers[0].URL)
		require.Equal(t, []string{"Authorization"}, servers[0].HeaderNames)
		require.Equal(t, []string{"lookup"}, servers[0].ToolAllowList)
		require.Equal(t, []string{}, servers[0].ToolDenyList)
		require.True(t, servers[0].AllowInPlanMode)
		require.False(t, servers[0].AllowInSubagents)
		require.True(t, servers[0].ForwardCoderHeaders)

		raw, err := json.Marshal(servers)
		require.NoError(t, err)
		require.NotContains(t, string(raw), "secret-value-123")
	})

	t.Run("ExperimentDisabled", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		values := coderdtest.DeploymentValues(t)
		values.Experiments = serpent.StringArray{string(codersdk.ExperimentAgentLifecycleHooks)}
		client := newChatClientWithDeploymentValues(t, values)
		user := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content:        chatTextContent("hello"),
			MCPServers:     []codersdk.ChatMCPServerRequest{chatMCPServerRequest("orders", "https://mcp.example.com/v1")},
		})
		sdkErr := requireSDKError(t, err, http.StatusForbidden)
		require.Equal(t, "Chat-attached MCP servers are not enabled on this deployment.", sdkErr.Message)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content:        chatTextContent("hello"),
		})
		require.NoError(t, err)

		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content:    chatTextContent("again"),
			MCPServers: &[]codersdk.ChatMCPServerRequest{chatMCPServerRequest("orders", "https://mcp.example.com/v1")},
		})
		sdkErr = requireSDKError(t, err, http.StatusForbidden)
		require.Equal(t, "Chat-attached MCP servers are not enabled on this deployment.", sdkErr.Message)

		_, err = client.GetChatMCPServers(ctx, chat.ID)
		requireSDKError(t, err, http.StatusForbidden)
	})

	t.Run("CallerSuppliedToolsDisabled", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		values := coderdtest.DeploymentValues(t)
		values.DisableChatCallerSuppliedTools = serpent.Bool(true)
		rawClient, _, api := coderdtest.NewWithAPI(t, newChatTestOptions(t, values, withChatWorkerDisabled))
		aibridgedtest.StartTestAIBridgeDaemon(t.Context(), t, api, nil)
		client := codersdk.NewExperimentalClient(rawClient)
		db := api.Database
		user := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content:        chatTextContent("hello"),
			MCPServers:     []codersdk.ChatMCPServerRequest{chatMCPServerRequest("orders", "https://mcp.example.com/v1")},
		})
		sdkErr := requireSDKError(t, err, http.StatusForbidden)
		require.Equal(t, "Caller-supplied tools are disabled on this deployment.", sdkErr.Message)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content:        chatTextContent("hello"),
		})
		require.NoError(t, err)

		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content:    chatTextContent("again"),
			MCPServers: &[]codersdk.ChatMCPServerRequest{chatMCPServerRequest("orders", "https://mcp.example.com/v1")},
		})
		sdkErr = requireSDKError(t, err, http.StatusForbidden)
		require.Equal(t, "Caller-supplied tools are disabled on this deployment.", sdkErr.Message)

		dbgen.ChatMCPServer(t, db, database.ChatMCPServer{ChatID: chat.ID, Slug: "stale"})
		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content:    chatTextContent("detach"),
			MCPServers: &[]codersdk.ChatMCPServerRequest{},
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

		for _, tc := range []struct {
			name      string
			server    codersdk.ChatMCPServerRequest
			wantField string
		}{
			{
				name:      "BadSlug",
				server:    chatMCPServerRequest("bad slug", "https://mcp.example.com/v1"),
				wantField: "mcp_servers[0].slug",
			},
			{
				name:      "QueryString",
				server:    chatMCPServerRequest("orders", "https://mcp.example.com/v1?token=x"),
				wantField: "mcp_servers[0].url",
			},
			{
				name:      "PrivateIPLiteral",
				server:    chatMCPServerRequest("orders", "http://10.0.0.1/mcp"),
				wantField: "mcp_servers[0].url",
			},
			{
				name: "HeadersOverHTTP",
				server: codersdk.ChatMCPServerRequest{
					Slug:    "orders",
					URL:     "http://mcp.example.com/v1",
					Headers: map[string]string{"Authorization": "Bearer x"},
				},
				wantField: "mcp_servers[0].headers",
			},
			{
				name: "ReservedHeader",
				server: codersdk.ChatMCPServerRequest{
					Slug:    "orders",
					URL:     "https://mcp.example.com/v1",
					Headers: map[string]string{"X-Coder-Owner-Id": "x"},
				},
				wantField: "mcp_servers[0].headers[X-Coder-Owner-Id]",
			},
			{
				name: "AllowAndDeny",
				server: codersdk.ChatMCPServerRequest{
					Slug:          "orders",
					URL:           "https://mcp.example.com/v1",
					ToolAllowList: []string{"a"},
					ToolDenyList:  []string{"b"},
				},
				wantField: "mcp_servers[0].tool_deny_list",
			},
		} {
			_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
				OrganizationID: user.OrganizationID,
				Content:        chatTextContent("hello"),
				MCPServers:     []codersdk.ChatMCPServerRequest{tc.server},
			})
			sdkErr := requireSDKError(t, err, http.StatusBadRequest)
			require.Equal(t, "Invalid mcp_servers.", sdkErr.Message, tc.name)
			require.NotEmpty(t, sdkErr.Validations, tc.name)
			require.Equal(t, tc.wantField, sdkErr.Validations[0].Field, tc.name)
		}

		_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content:        chatTextContent("hello"),
			MCPServers: []codersdk.ChatMCPServerRequest{{
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
			MCPServers: []codersdk.ChatMCPServerRequest{
				chatMCPServerRequest("a", "https://a.example.com/mcp"),
				chatMCPServerRequest("b", "https://b.example.com/mcp"),
			},
		})
		require.NoError(t, err)

		rows, err := db.GetChatMCPServersByChatID(dbCtx, chat.ID)
		require.NoError(t, err)
		require.Len(t, rows, 2)
		originalAID := chatMCPServerSlugs(rows)["a"].ID

		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: chatTextContent("keep"),
		})
		require.NoError(t, err)
		rows, err = db.GetChatMCPServersByChatID(dbCtx, chat.ID)
		require.NoError(t, err)
		require.Len(t, rows, 2)

		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content: chatTextContent("replace"),
			MCPServers: &[]codersdk.ChatMCPServerRequest{
				chatMCPServerRequest("a", "https://a2.example.com/mcp"),
				chatMCPServerRequest("c", "https://c.example.com/mcp"),
			},
		})
		require.NoError(t, err)
		rows, err = db.GetChatMCPServersByChatID(dbCtx, chat.ID)
		require.NoError(t, err)
		bySlug := chatMCPServerSlugs(rows)
		require.Len(t, bySlug, 2)
		require.NotContains(t, bySlug, "b")
		require.Equal(t, originalAID, bySlug["a"].ID)
		require.Equal(t, "https://a2.example.com/mcp", bySlug["a"].Url)
		require.Contains(t, bySlug, "c")

		_, err = client.CreateChatMessage(ctx, chat.ID, codersdk.CreateChatMessageRequest{
			Content:    chatTextContent("clear"),
			MCPServers: &[]codersdk.ChatMCPServerRequest{},
		})
		require.NoError(t, err)
		rows, err = db.GetChatMCPServersByChatID(dbCtx, chat.ID)
		require.NoError(t, err)
		require.Empty(t, rows)

		servers, err := client.GetChatMCPServers(ctx, chat.ID)
		require.NoError(t, err)
		require.NotNil(t, servers)
		require.Empty(t, servers)
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
			Content:    chatTextContent("child"),
			MCPServers: &[]codersdk.ChatMCPServerRequest{chatMCPServerRequest("orders", "https://mcp.example.com/v1")},
		})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Equal(t, "mcp_servers can only be declared on a root chat.", sdkErr.Message)
	})

	t.Run("NonOwnerForbidden", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client := newChatClient(t, withChatWorkerDisabled)
		user := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModel(t, client)

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: user.OrganizationID,
			Content:        chatTextContent("hello"),
			MCPServers:     []codersdk.ChatMCPServerRequest{chatMCPServerRequest("orders", "https://mcp.example.com/v1")},
		})
		require.NoError(t, err)

		memberRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, user.OrganizationID)
		member := codersdk.NewExperimentalClient(memberRaw)
		_, err = member.GetChatMCPServers(ctx, chat.ID)
		requireSDKError(t, err, http.StatusNotFound)

		ownerRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, user.OrganizationID, rbac.RoleOwner())
		owner := codersdk.NewExperimentalClient(ownerRaw)
		_, err = owner.GetChatMCPServers(ctx, chat.ID)
		sdkErr := requireSDKError(t, err, http.StatusForbidden)
		require.Equal(t, "Only the chat owner may view chat MCP servers.", sdkErr.Message)
	})
}

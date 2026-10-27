package coderd_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

const mcpAppTestViewURI = "ui://dice/board.html"

// newMCPAppTestServer serves one view tool with a thread entry point,
// one app-only tool, one model-only tool and the view resource.
func newMCPAppTestServer(t *testing.T) string {
	t.Helper()
	srv := mcp.NewServer(&mcp.Implementation{Name: "dice", Version: "1.0.0"}, nil)
	roll := func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: "rolled 4"}},
			StructuredContent: map[string]any{"rolls": []int{4}},
		}, nil
	}
	srv.AddTool(&mcp.Tool{
		Name:        "show_board",
		Title:       "Dice board",
		InputSchema: map[string]any{"type": "object"},
		Meta: mcp.Meta{
			"ui":        map[string]any{"resourceUri": mcpAppTestViewURI},
			"openai/ui": map[string]any{"entrypoints": []any{map[string]any{"type": "thread"}}},
		},
	}, roll)
	srv.AddTool(&mcp.Tool{
		Name:        "roll_again",
		InputSchema: map[string]any{"type": "object"},
		Meta:        mcp.Meta{"ui": map[string]any{"resourceUri": mcpAppTestViewURI, "visibility": []any{"app"}}},
	}, roll)
	srv.AddTool(&mcp.Tool{
		Name:        "server_time",
		InputSchema: map[string]any{"type": "object"},
		Meta:        mcp.Meta{"ui": map[string]any{"visibility": []any{"model"}}},
	}, roll)
	srv.AddResource(&mcp.Resource{URI: mcpAppTestViewURI, Name: "board", MIMEType: "text/html;profile=mcp-app"},
		func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
				URI:      req.Params.URI,
				MIMEType: "text/html;profile=mcp-app",
				Text:     "<!doctype html><p>board</p>",
				Meta: mcp.Meta{"ui": map[string]any{"csp": map[string]any{
					"connectDomains": []any{"https://api.example.com", "https://evil.example.com; script-src *"},
				}}},
			}}}, nil
		})
	ts := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))
	t.Cleanup(ts.Close)
	return ts.URL
}

func deploymentValuesWithExperiments(t *testing.T, experiments ...codersdk.Experiment) *codersdk.DeploymentValues {
	t.Helper()
	values := coderdtest.DeploymentValues(t)
	// A non-empty list keeps newChatTestOptions from adding its defaults.
	values.Experiments = serpent.StringArray{string(codersdk.ExperimentChatAdvisor)}
	for _, experiment := range experiments {
		values.Experiments = append(values.Experiments, string(experiment))
	}
	return values
}

func TestChatMCPApps(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T, experiments ...codersdk.Experiment) (owner, member *codersdk.ExperimentalClient, chat database.Chat, server database.MCPServerConfig) {
		t.Helper()
		opts := newChatTestOptions(t, deploymentValuesWithExperiments(t, experiments...))
		rawClient, _, api := coderdtest.NewWithAPI(t, opts)
		owner = codersdk.NewExperimentalClient(rawClient)
		firstUser := coderdtest.CreateFirstUser(t, rawClient)
		model := createChatModel(t, owner)
		memberRaw, memberUser := coderdtest.CreateAnotherUser(t, rawClient, firstUser.OrganizationID)
		member = codersdk.NewExperimentalClient(memberRaw)

		server = dbgen.MCPServerConfig(t, api.Database, database.MCPServerConfig{
			OrganizationID: firstUser.OrganizationID,
			DisplayName:    "Dice",
			Url:            newMCPAppTestServer(t),
			Enabled:        true,
		})
		chat = dbgen.Chat(t, api.Database, database.Chat{
			OrganizationID:    firstUser.OrganizationID,
			OwnerID:           memberUser.ID,
			LastModelConfigID: model.ID,
		})
		return owner, member, chat, server
	}

	get := func(ctx context.Context, t *testing.T, client *codersdk.ExperimentalClient, path string) (int, http.Header, []byte) {
		t.Helper()
		res, err := client.Request(ctx, http.MethodGet, path, nil)
		require.NoError(t, err)
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		return res.StatusCode, res.Header, body
	}

	t.Run("ExperimentOff", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		_, member, chat, server := setup(t)

		for _, path := range []string{
			fmt.Sprintf("/api/experimental/chats/%s/mcp-apps/entrypoints", chat.ID),
			fmt.Sprintf("/api/experimental/chats/%s/mcp-apps/%s/view?uri=%s", chat.ID, server.ID, mcpAppTestViewURI),
		} {
			status, _, _ := get(ctx, t, member, path)
			assert.Equal(t, http.StatusForbidden, status, path)
		}
	})

	t.Run("Owner", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		_, member, chat, server := setup(t, codersdk.ExperimentMCPApps)

		// The view page has the sandbox CSP. A declared domain with a
		// directive in it is dropped.
		status, header, html := get(ctx, t, member,
			fmt.Sprintf("/api/experimental/chats/%s/mcp-apps/%s/view?uri=%s", chat.ID, server.ID, mcpAppTestViewURI))
		require.Equal(t, http.StatusOK, status)
		assert.Equal(t, "<!doctype html><p>board</p>", string(html))
		assert.Equal(t, "text/html; charset=utf-8", header.Get("Content-Type"))
		csp := header.Get("Content-Security-Policy")
		assert.Contains(t, csp, "sandbox allow-scripts allow-forms;")
		assert.Contains(t, csp, "connect-src https://api.example.com;")
		assert.NotContains(t, csp, "evil")

		// Entry points: only the tool with a thread entry point.
		entrypoints, err := member.GetChatMCPAppEntrypoints(ctx, chat.ID, []uuid.UUID{server.ID, uuid.New()})
		require.NoError(t, err)
		assert.Equal(t, []codersdk.ChatMCPAppEntrypoint{{
			MCPServerConfigID: server.ID,
			ServerDisplayName: "Dice",
			ToolName:          "show_board",
			Title:             "Dice board",
			ResourceURI:       mcpAppTestViewURI,
		}}, entrypoints)

		// Open the entry point: a call with {}.
		app, err := member.OpenChatMCPApp(ctx, chat.ID, server.ID, codersdk.ChatMCPAppOpenRequest{ToolName: "show_board"})
		require.NoError(t, err)
		assert.Equal(t, map[string]any{}, app.ToolInput)
		assert.Contains(t, app.ToolResult, "structuredContent")
		_, err = member.OpenChatMCPApp(ctx, chat.ID, server.ID, codersdk.ChatMCPAppOpenRequest{ToolName: "server_time"})
		requireSDKError(t, err, http.StatusNotFound)

		// View tool calls: app tools only.
		result, err := member.CallChatMCPAppTool(ctx, chat.ID, server.ID, codersdk.ChatMCPAppToolCallRequest{Name: "roll_again"})
		require.NoError(t, err)
		assert.Contains(t, result, "structuredContent")
		_, err = member.CallChatMCPAppTool(ctx, chat.ID, server.ID, codersdk.ChatMCPAppToolCallRequest{Name: "server_time"})
		requireSDKError(t, err, http.StatusForbidden)
		_, err = member.CallChatMCPAppTool(ctx, chat.ID, server.ID, codersdk.ChatMCPAppToolCallRequest{Name: "missing"})
		requireSDKError(t, err, http.StatusNotFound)

		// A server that does not exist in the org is not found.
		_, err = member.CallChatMCPAppTool(ctx, chat.ID, uuid.New(), codersdk.ChatMCPAppToolCallRequest{Name: "roll_again"})
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("NotOwner", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		owner, _, chat, server := setup(t, codersdk.ExperimentMCPApps)

		// The deployment owner can read the chat, but the MCP session
		// would use the chat owner's credentials.
		status, _, _ := get(ctx, t, owner,
			fmt.Sprintf("/api/experimental/chats/%s/mcp-apps/%s/view?uri=%s", chat.ID, server.ID, mcpAppTestViewURI))
		assert.Equal(t, http.StatusForbidden, status)
		_, err := owner.CallChatMCPAppTool(ctx, chat.ID, server.ID, codersdk.ChatMCPAppToolCallRequest{Name: "roll_again"})
		requireSDKError(t, err, http.StatusForbidden)
	})
}

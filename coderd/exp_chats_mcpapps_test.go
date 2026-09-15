package coderd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// newMCPAppTestServer serves an MCP server with one model-visible UI
// tool, one app-only tool, one tool denied by the config's deny list,
// and one ui:// resource.
func newMCPAppTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := mcp.NewServer(&mcp.Implementation{Name: "board", Version: "1.0.0"}, nil)
	boardMeta := map[string]any{"ui": map[string]any{"resourceUri": "ui://board/main"}}
	srv.AddTool(&mcp.Tool{
		Name: "list_tasks", InputSchema: map[string]any{"type": "object"}, Meta: boardMeta,
	}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]any
		_ = json.Unmarshal(req.Params.Arguments, &args)
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: "tasks"}},
			StructuredContent: map[string]any{"tasks": []any{"milk"}, "args": args},
		}, nil
	})
	srv.AddTool(&mcp.Tool{
		Name: "refresh", InputSchema: map[string]any{"type": "object"},
		Meta: map[string]any{"ui": map[string]any{"resourceUri": "ui://board/main", "visibility": []any{"app"}}},
	}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "refreshed"}}}, nil
	})
	srv.AddTool(&mcp.Tool{
		Name: "model_only", InputSchema: map[string]any{"type": "object"},
		Meta: map[string]any{"ui": map[string]any{"visibility": []any{"model"}}},
	}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "nope"}}}, nil
	})
	srv.AddTool(&mcp.Tool{
		Name: "denied", InputSchema: map[string]any{"type": "object"},
	}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "nope"}}}, nil
	})
	srv.AddResource(&mcp.Resource{
		URI: "ui://board/main", Name: "board", MIMEType: "text/html;profile=mcp-app",
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI: req.Params.URI, MIMEType: "text/html;profile=mcp-app", Text: "<!doctype html><p>board</p>",
		}}}, nil
	})
	srv.AddResource(&mcp.Resource{
		URI: "file:///listed.txt", Name: "listed", MIMEType: "text/plain",
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI: req.Params.URI, MIMEType: "text/plain", Text: "listed",
		}}}, nil
	})
	ts := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true},
	))
	t.Cleanup(ts.Close)
	return ts
}

func TestChatMCPAppProxy(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	client := newChatClient(t)
	firstUser := coderdtest.CreateFirstUser(t, client.Client)
	_ = createChatModel(t, client)
	mcpTS := newMCPAppTestServer(t)

	attached, err := client.CreateMCPServerConfig(ctx, firstUser.OrganizationID, codersdk.CreateMCPServerConfigRequest{
		DisplayName:   "Board",
		Slug:          "board",
		Transport:     "streamable_http",
		URL:           mcpTS.URL,
		AuthType:      "none",
		Availability:  "default_off",
		Enabled:       true,
		ToolAllowList: []string{},
		ToolDenyList:  []string{"denied"},
	})
	require.NoError(t, err)
	detached, err := client.CreateMCPServerConfig(ctx, firstUser.OrganizationID, codersdk.CreateMCPServerConfigRequest{
		DisplayName:   "Other",
		Slug:          "other",
		Transport:     "streamable_http",
		URL:           mcpTS.URL,
		AuthType:      "none",
		Availability:  "default_off",
		Enabled:       true,
		ToolAllowList: []string{},
		ToolDenyList:  []string{},
	})
	require.NoError(t, err)

	chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
		OrganizationID: firstUser.OrganizationID,
		Content:        []codersdk.ChatInputPart{{Type: codersdk.ChatInputPartTypeText, Text: "hello"}},
		MCPServerIDs:   []uuid.UUID{attached.ID},
	})
	require.NoError(t, err)

	t.Run("CallTool", func(t *testing.T) {
		t.Parallel()
		resp, err := client.CallChatMCPAppTool(ctx, chat.ID, attached.ID, codersdk.ChatMCPAppToolCallRequest{
			Name:      "list_tasks",
			Arguments: map[string]any{"filter": "open"},
		})
		require.NoError(t, err)
		var result map[string]any
		require.NoError(t, json.Unmarshal(resp.Result, &result))
		structured, ok := result["structuredContent"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, []any{"milk"}, structured["tasks"])
		require.Equal(t, map[string]any{"filter": "open"}, structured["args"])
	})

	t.Run("CallAppOnlyTool", func(t *testing.T) {
		t.Parallel()
		resp, err := client.CallChatMCPAppTool(ctx, chat.ID, attached.ID, codersdk.ChatMCPAppToolCallRequest{Name: "refresh"})
		require.NoError(t, err)
		require.Contains(t, string(resp.Result), "refreshed")
	})

	t.Run("RejectsModelOnlyTool", func(t *testing.T) {
		t.Parallel()
		_, err := client.CallChatMCPAppTool(ctx, chat.ID, attached.ID, codersdk.ChatMCPAppToolCallRequest{Name: "model_only"})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Contains(t, sdkErr.Message, "not callable by apps")
	})

	t.Run("RejectsDeniedTool", func(t *testing.T) {
		t.Parallel()
		_, err := client.CallChatMCPAppTool(ctx, chat.ID, attached.ID, codersdk.ChatMCPAppToolCallRequest{Name: "denied"})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Contains(t, sdkErr.Message, "not allowed")
	})

	t.Run("RejectsUnknownTool", func(t *testing.T) {
		t.Parallel()
		_, err := client.CallChatMCPAppTool(ctx, chat.ID, attached.ID, codersdk.ChatMCPAppToolCallRequest{Name: "missing"})
		requireSDKError(t, err, http.StatusBadRequest)
	})

	t.Run("ServerNotAttached", func(t *testing.T) {
		t.Parallel()
		_, err := client.CallChatMCPAppTool(ctx, chat.ID, detached.ID, codersdk.ChatMCPAppToolCallRequest{Name: "list_tasks"})
		requireSDKError(t, err, http.StatusNotFound)
	})

	t.Run("ReadUIResource", func(t *testing.T) {
		t.Parallel()
		resp, err := client.ReadChatMCPAppResource(ctx, chat.ID, attached.ID, codersdk.ChatMCPAppResourceReadRequest{URI: "ui://board/main"})
		require.NoError(t, err)
		var result struct {
			Contents []struct {
				URI      string `json:"uri"`
				MIMEType string `json:"mimeType"`
				Text     string `json:"text"`
			} `json:"contents"`
		}
		require.NoError(t, json.Unmarshal(resp.Result, &result))
		require.Len(t, result.Contents, 1)
		require.Equal(t, "text/html;profile=mcp-app", result.Contents[0].MIMEType)
		require.Contains(t, result.Contents[0].Text, "board")
	})

	t.Run("ReadListedResource", func(t *testing.T) {
		t.Parallel()
		resp, err := client.ReadChatMCPAppResource(ctx, chat.ID, attached.ID, codersdk.ChatMCPAppResourceReadRequest{URI: "file:///listed.txt"})
		require.NoError(t, err)
		require.Contains(t, string(resp.Result), "listed")
	})

	t.Run("RejectsUnlistedResource", func(t *testing.T) {
		t.Parallel()
		_, err := client.ReadChatMCPAppResource(ctx, chat.ID, attached.ID, codersdk.ChatMCPAppResourceReadRequest{URI: "file:///etc/passwd"})
		sdkErr := requireSDKError(t, err, http.StatusBadRequest)
		require.Contains(t, sdkErr.Message, "does not list")
	})

	t.Run("NonOwnerForbidden", func(t *testing.T) {
		t.Parallel()
		otherRaw, _ := coderdtest.CreateAnotherUser(t, client.Client, firstUser.OrganizationID)
		other := codersdk.NewExperimentalClient(otherRaw)
		_, err := other.CallChatMCPAppTool(ctx, chat.ID, attached.ID, codersdk.ChatMCPAppToolCallRequest{Name: "list_tasks"})
		// Members without access to the chat see 404 from RBAC.
		requireSDKError(t, err, http.StatusNotFound)
	})
}

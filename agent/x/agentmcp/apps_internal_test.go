package agentmcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestMCPApps(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	meta := mcp.Meta{"ui": map[string]any{"resourceUri": "ui://view"}}
	resourceMeta := mcp.Meta{"ui": map[string]any{"prefersBorder": true}}
	server := mcp.NewServer(&mcp.Implementation{Name: "apps", Version: "1"}, nil)
	handler := func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		settings, ok := req.Session.InitializeParams().Capabilities.Extensions["io.modelcontextprotocol/ui"]
		if !ok {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "MCP Apps capability required"}}}, nil
		}
		assert.Equal(t, map[string]any{"mimeTypes": []any{"text/html;profile=mcp-app"}}, settings)
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: "view ready"}},
			StructuredContent: map[string]any{"items": []any{"one", "two"}},
		}, nil
	}
	tool := &mcp.Tool{Name: "view", InputSchema: map[string]any{"type": "object"}, Meta: meta}
	server.AddTool(tool, handler)
	server.AddResource(&mcp.Resource{URI: "ui://view", Name: "view"}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{
			{URI: "ui://view", MIMEType: "text/html;profile=mcp-app", Text: "<html>hello</html>", Meta: resourceMeta},
			{URI: "ui://ignored", Text: "second"},
		}}, nil
	})
	server.AddResource(&mcp.Resource{URI: "ui://blob", Name: "blob"}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{
			{URI: "ui://blob", MIMEType: "text/html;profile=mcp-app", Blob: []byte("<html>binary</html>")},
		}}, nil
	})
	server.AddResource(&mcp.Resource{URI: "ui://empty", Name: "empty"}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{}, nil
	})
	oversized := bytes.Repeat([]byte("x"), workspacesdk.MaxMCPResourceContentBytes+1)
	server.AddResource(&mcp.Resource{URI: "ui://large-text", Name: "large-text"}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{
			{URI: "ui://large-text", MIMEType: "text/html;profile=mcp-app", Text: string(oversized)},
		}}, nil
	})
	server.AddResource(&mcp.Resource{URI: "ui://large-blob", Name: "large-blob"}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{
			{URI: "ui://large-blob", MIMEType: "text/html;profile=mcp-app", Blob: oversized},
		}}, nil
	})
	server.AddResource(&mcp.Resource{URI: "ui://large-meta", Name: "large-meta"}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{
			{URI: "ui://large-meta", MIMEType: "text/html;profile=mcp-app", Text: "<html>small</html>", Meta: mcp.Meta{"padding": string(oversized)}},
		}}, nil
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
	t.Cleanup(httpServer.Close)
	manager := &Manager{logger: testutil.Logger(t)}
	manager.SetMCPAppsEnabled(true)
	client, err := manager.connectServer(ctx, ServerConfig{Name: "apps", Transport: "http", URL: httpServer.URL})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	manager.servers = map[string]*serverEntry{"apps": {client: client}}
	wanted := map[string]ServerConfig{"apps": {Name: "apps"}}
	require.True(t, manager.refreshCatalog(ctx, wanted))
	catalog := manager.Catalog()
	require.Len(t, catalog, 1)
	require.Len(t, catalog[0].Tools, 1)
	require.Equal(t, map[string]any(meta), catalog[0].Tools[0].Meta)
	updatedTool := *tool
	updatedTool.Meta = mcp.Meta{"ui": map[string]any{"resourceUri": "ui://changed"}}
	server.AddTool(&updatedTool, handler)
	require.True(t, manager.refreshCatalog(ctx, wanted))
	require.Equal(t, map[string]any(updatedTool.Meta), manager.Catalog()[0].Tools[0].Meta)

	api := NewAPI(manager).Routes()
	for _, tc := range []struct {
		name, server, uri string
		status            int
	}{
		{"text", "apps", "ui://view", http.StatusOK},
		{"blob", "apps", "ui://blob", http.StatusOK},
		{"unknown server", "missing", "ui://view", http.StatusNotFound},
		{"unknown resource", "apps", "ui://missing", http.StatusBadGateway},
		{"empty", "apps", "ui://empty", http.StatusBadGateway},
		{"large text", "apps", "ui://large-text", http.StatusRequestEntityTooLarge},
		{"large blob", "apps", "ui://large-blob", http.StatusRequestEntityTooLarge},
		{"large meta", "apps", "ui://large-meta", http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			body, err := json.Marshal(workspacesdk.ReadMCPResourceRequest{ServerName: tc.server, URI: tc.uri})
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/read-resource", bytes.NewReader(body)).WithContext(ctx)
			rr := httptest.NewRecorder()
			api.ServeHTTP(rr, req)
			require.Equal(t, tc.status, rr.Code, rr.Body.String())
			if tc.status != http.StatusOK {
				return
			}
			var got workspacesdk.ReadMCPResourceResponse
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
			require.Equal(t, tc.uri, got.URI)
			require.Equal(t, "text/html;profile=mcp-app", got.MimeType)
			if tc.name == "text" {
				require.Equal(t, "<html>hello</html>", got.Text)
				require.JSONEq(t, `{"ui":{"prefersBorder":true}}`, string(got.Meta))
			} else {
				require.Equal(t, base64.StdEncoding.EncodeToString([]byte("<html>binary</html>")), got.Blob)
			}
		})
	}
	rr := httptest.NewRecorder()
	api.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/read-resource", bytes.NewBufferString("{")))
	require.Equal(t, http.StatusBadRequest, rr.Code)
	for _, includeResult := range []bool{false, true} {
		callBody := bytes.NewBufferString(`{"tool_name":"apps__view","arguments":{},"include_result":` + strconv.FormatBool(includeResult) + `}`)
		rr = httptest.NewRecorder()
		api.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/call-tool", callBody).WithContext(ctx))
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		var result workspacesdk.CallMCPToolResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &result))
		require.False(t, result.IsError)
		require.Equal(t, []workspacesdk.MCPToolContent{{Type: "text", Text: "view ready"}}, result.Content)
		if includeResult {
			require.JSONEq(t, `{"content":[{"type":"text","text":"view ready"}],"structuredContent":{"items":["one","two"]}}`, string(result.Result))
		} else {
			require.Empty(t, result.Result)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = manager.ReadResource(canceled, workspacesdk.ReadMCPResourceRequest{ServerName: "apps", URI: "ui://view"})
	require.ErrorIs(t, err, context.Canceled)

	// Without the deployment flag the UI extension is not advertised.
	disabled := &Manager{logger: testutil.Logger(t)}
	disabledClient, err := disabled.connectServer(ctx, ServerConfig{Name: "apps", Transport: "http", URL: httpServer.URL})
	require.NoError(t, err)
	t.Cleanup(func() { _ = disabledClient.Close() })
	disabled.servers = map[string]*serverEntry{"apps": {client: disabledClient}}
	result, err := disabled.CallTool(ctx, workspacesdk.CallMCPToolRequest{ToolName: "apps__view"})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Equal(t, []workspacesdk.MCPToolContent{{Type: "text", Text: "MCP Apps capability required"}}, result.Content)
}

func TestConvertResultEmbeddedResource(t *testing.T) {
	t.Parallel()
	for _, binary := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "blob"}[binary], func(t *testing.T) {
			t.Parallel()
			resource := &mcp.ResourceContents{URI: "ui://view", MIMEType: "text/html;profile=mcp-app", Meta: mcp.Meta{"ui": map[string]any{"prefersBorder": true}}}
			if binary {
				resource.Blob = []byte("html")
			} else {
				resource.Text = "html"
			}
			original := &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.EmbeddedResource{Resource: resource, Annotations: &mcp.Annotations{Audience: []mcp.Role{"user"}}},
					&mcp.ResourceLink{URI: "https://example.com/report", Name: "Sales report", MIMEType: "text/html"},
				},
				IsError: true, StructuredContent: map[string]any{"items": []any{"one"}},
				Meta: mcp.Meta{"display": "chart"},
			}
			got := convertResult(original)
			require.Empty(t, got.Result)
			got.Result = rawResult(original)
			data, err := json.Marshal(got)
			require.NoError(t, err)
			var roundtrip workspacesdk.CallMCPToolResponse
			require.NoError(t, json.Unmarshal(data, &roundtrip))
			require.True(t, roundtrip.IsError)
			require.Equal(t, []workspacesdk.MCPToolContent{
				{Type: "resource", Text: "[embedded resource: *mcp.ResourceContents]"},
				{Type: "resource", Text: "[resource link: https://example.com/report]"},
			}, roundtrip.Content)
			expected, err := json.Marshal(original)
			require.NoError(t, err)
			require.JSONEq(t, string(expected), string(roundtrip.Result))
			var mcpResult mcp.CallToolResult
			require.NoError(t, json.Unmarshal(roundtrip.Result, &mcpResult))
			require.Equal(t, original, &mcpResult)
		})
	}
}

func TestRawResultOversized(t *testing.T) {
	t.Parallel()
	for _, isError := range []bool{false, true} {
		t.Run(strconv.FormatBool(isError), func(t *testing.T) {
			t.Parallel()
			original := &mcp.CallToolResult{
				Content:           []mcp.Content{&mcp.TextContent{Text: "ok"}},
				StructuredContent: map[string]any{"blob": strings.Repeat("x", workspacesdk.MaxMCPToolResultBytes)},
				IsError:           isError,
			}
			require.JSONEq(t, `{"content":[{"type":"text","text":"[result omitted: too large]"}],"isError":`+strconv.FormatBool(isError)+`}`, string(rawResult(original)))
		})
	}
}

func TestConvertResultNilResource(t *testing.T) {
	t.Parallel()
	got := convertResult(&mcp.CallToolResult{Content: []mcp.Content{&mcp.EmbeddedResource{}}})
	require.Len(t, got.Content, 1)
	require.Equal(t, "resource", got.Content[0].Type)
	require.Equal(t, "[embedded resource: *mcp.ResourceContents]", got.Content[0].Text)
}

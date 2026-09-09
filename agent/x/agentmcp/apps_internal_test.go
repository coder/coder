package agentmcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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
	handler := func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	session, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	client, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	manager := &Manager{logger: testutil.Logger(t), servers: map[string]*serverEntry{"apps": {client: client}}}
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
	callBody := bytes.NewBufferString(`{"tool_name":"apps__view","arguments":{}}`)
	rr = httptest.NewRecorder()
	api.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/call-tool", callBody).WithContext(ctx))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var result workspacesdk.CallMCPToolResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &result))
	require.JSONEq(t, `{"items":["one","two"]}`, string(result.StructuredContent))
	require.Equal(t, []workspacesdk.MCPToolContent{{Type: "text", Text: "view ready"}}, result.Content)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = manager.ReadResource(canceled, workspacesdk.ReadMCPResourceRequest{ServerName: "apps", URI: "ui://view"})
	require.ErrorIs(t, err, context.Canceled)
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
			got := convertResult(&mcp.CallToolResult{Content: []mcp.Content{&mcp.EmbeddedResource{Resource: resource}}, IsError: true, StructuredContent: map[string]any{}})
			data, err := json.Marshal(got)
			require.NoError(t, err)
			var roundtrip workspacesdk.CallMCPToolResponse
			require.NoError(t, json.Unmarshal(data, &roundtrip))
			require.True(t, roundtrip.IsError)
			require.JSONEq(t, "{}", string(roundtrip.StructuredContent))
			require.Len(t, roundtrip.Content, 1)
			item := roundtrip.Content[0]
			require.Equal(t, "resource", item.Type)
			require.NotNil(t, item.Resource)
			require.Equal(t, resource.URI, item.Resource.URI)
			require.Equal(t, resource.MIMEType, item.Resource.MimeType)
			require.Equal(t, resource.Text, item.Resource.Text)
			require.Equal(t, base64.StdEncoding.EncodeToString(resource.Blob), item.Resource.Blob)
			require.JSONEq(t, `{"ui":{"prefersBorder":true}}`, string(item.Resource.Meta))
		})
	}
}

func TestConvertResultNilResource(t *testing.T) {
	t.Parallel()
	got := convertResult(&mcp.CallToolResult{Content: []mcp.Content{&mcp.EmbeddedResource{}}})
	require.Len(t, got.Content, 1)
	require.Equal(t, "resource", got.Content[0].Type)
	require.Equal(t, &workspacesdk.ReadMCPResourceResponse{}, got.Content[0].Resource)
}

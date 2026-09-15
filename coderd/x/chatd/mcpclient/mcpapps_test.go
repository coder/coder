package mcpclient_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
)

func uiTool(name, resourceURI string, visibility []any) testTool {
	ui := map[string]any{"resourceUri": resourceURI}
	if visibility != nil {
		ui["visibility"] = visibility
	}
	return testTool{
		tool: &mcp.Tool{
			Name:        name,
			InputSchema: map[string]any{"type": "object"},
			Meta:        map[string]any{"ui": ui},
		},
		handler: func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content:           []mcp.Content{&mcp.TextContent{Text: "board updated"}},
				StructuredContent: map[string]any{"tasks": []any{"a"}},
				Meta:              map[string]any{"version": float64(2)},
			}, nil
		},
	}
}

func TestParseToolUIMeta(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		meta         map[string]any
		wantURI      string
		wantModel    bool
		wantApp      bool
		wantVisiblen int
	}{
		{name: "nil", meta: nil, wantModel: true, wantApp: true},
		{
			name:      "nested",
			meta:      map[string]any{"ui": map[string]any{"resourceUri": "ui://srv/board"}},
			wantURI:   "ui://srv/board",
			wantModel: true, wantApp: true,
		},
		{
			name:      "deprecated flat key",
			meta:      map[string]any{"ui/resourceUri": "ui://srv/flat"},
			wantURI:   "ui://srv/flat",
			wantModel: true, wantApp: true,
		},
		{
			name: "nested wins over flat",
			meta: map[string]any{
				"ui":             map[string]any{"resourceUri": "ui://srv/nested"},
				"ui/resourceUri": "ui://srv/flat",
			},
			wantURI:   "ui://srv/nested",
			wantModel: true, wantApp: true,
		},
		{
			name:      "non ui scheme ignored",
			meta:      map[string]any{"ui": map[string]any{"resourceUri": "https://evil.example/x"}},
			wantModel: true, wantApp: true,
		},
		{
			name: "app only",
			meta: map[string]any{"ui": map[string]any{
				"resourceUri": "ui://srv/board",
				"visibility":  []any{"app"},
			}},
			wantURI:   "ui://srv/board",
			wantModel: false, wantApp: true,
		},
		{
			name: "model only",
			meta: map[string]any{"ui": map[string]any{
				"visibility": []any{"model"},
			}},
			wantModel: true, wantApp: false,
		},
		{
			name: "empty visibility means nobody",
			meta: map[string]any{"ui": map[string]any{
				"visibility": []any{},
			}},
			wantModel: false, wantApp: false,
		},
		{
			name: "malformed values ignored",
			meta: map[string]any{"ui": map[string]any{
				"resourceUri": 42,
				"visibility":  "model",
			}},
			wantModel: true, wantApp: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mcpclient.ParseToolUIMeta(tt.meta)
			assert.Equal(t, tt.wantURI, got.ResourceURI)
			assert.Equal(t, tt.wantModel, got.ModelVisible(), "model visible")
			assert.Equal(t, tt.wantApp, got.AppVisible(), "app visible")
		})
	}
}

func TestConnectAll_MCPApps(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t,
		uiTool("board", "ui://srv/board", nil),
		uiTool("refresh", "ui://srv/board", []any{"app"}),
		echoTool(),
	)
	cfg := makeConfig("srv", ts.URL)

	t.Run("disabled keeps app-only tools and no metadata", func(t *testing.T) {
		t.Parallel()
		tools, _, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil, uuid.Nil, nil, nil, testMCPHTTPClient(nil), mcpclient.ConnectOptions{})
		t.Cleanup(cleanup)
		require.Len(t, tools, 3)
		for _, tool := range tools {
			appTool, ok := tool.(mcpclient.MCPAppToolIdentifier)
			require.True(t, ok)
			assert.Empty(t, appTool.MCPAppResourceURI())
		}
		board := requireTool(t, tools, "srv__board")
		resp, err := board.Run(ctx, fantasy.ToolCall{ID: "1", Name: "srv__board", Input: "{}"})
		require.NoError(t, err)
		assert.Empty(t, resp.Metadata)
	})

	t.Run("enabled hides app-only tools and attaches result", func(t *testing.T) {
		t.Parallel()
		tools, _, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil, uuid.Nil, nil, nil, testMCPHTTPClient(nil), mcpclient.ConnectOptions{MCPApps: true})
		t.Cleanup(cleanup)
		require.Len(t, tools, 2)
		names := []string{tools[0].Info().Name, tools[1].Info().Name}
		assert.ElementsMatch(t, []string{"srv__board", "srv__echo"}, names)

		board := requireTool(t, tools, "srv__board")
		appTool, ok := board.(mcpclient.MCPAppToolIdentifier)
		require.True(t, ok)
		assert.Equal(t, "ui://srv/board", appTool.MCPAppResourceURI())

		echo := requireTool(t, tools, "srv__echo")
		echoApp, ok := echo.(mcpclient.MCPAppToolIdentifier)
		require.True(t, ok)
		assert.Empty(t, echoApp.MCPAppResourceURI())

		resp, err := board.Run(ctx, fantasy.ToolCall{ID: "1", Name: "srv__board", Input: "{}"})
		require.NoError(t, err)
		// The model-facing content is unchanged by the envelope.
		assert.Contains(t, resp.Content, "board updated")
		assert.Contains(t, resp.Content, `{"tasks":["a"]}`)

		appResult, ok, err := mcpclient.AppResultFromMetadata(resp.Metadata)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "ui://srv/board", appResult.ResourceURI)
		assert.False(t, appResult.Truncated)
		var raw map[string]any
		require.NoError(t, json.Unmarshal(appResult.Result, &raw))
		assert.Equal(t, map[string]any{"tasks": []any{"a"}}, raw["structuredContent"])
		meta, ok := raw["_meta"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, float64(2), meta["version"])
		content, ok := raw["content"].([]any)
		require.True(t, ok)
		require.Len(t, content, 1)

		resp, err = echo.Run(ctx, fantasy.ToolCall{ID: "2", Name: "srv__echo", Input: `{"input":"x"}`})
		require.NoError(t, err)
		assert.Empty(t, resp.Metadata)
	})
}

func TestConnectAll_MCPAppsTruncatesLargeResult(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	big := strings.Repeat("x", mcpclient.MaxAppResultBytes+1)
	ts := newTestMCPServer(t, testTool{
		tool: &mcp.Tool{
			Name:        "big",
			InputSchema: map[string]any{"type": "object"},
			Meta:        map[string]any{"ui": map[string]any{"resourceUri": "ui://srv/big"}},
		},
		handler: func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return textToolResult(big), nil
		},
	})
	cfg := makeConfig("srv", ts.URL)
	tools, _, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil, uuid.Nil, nil, nil, testMCPHTTPClient(nil), mcpclient.ConnectOptions{MCPApps: true})
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)

	resp, err := tools[0].Run(ctx, fantasy.ToolCall{ID: "1", Name: "srv__big", Input: "{}"})
	require.NoError(t, err)
	appResult, ok, err := mcpclient.AppResultFromMetadata(resp.Metadata)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "ui://srv/big", appResult.ResourceURI)
	assert.True(t, appResult.Truncated)
	assert.Empty(t, appResult.Result)
}

func TestAppResultFromMetadata(t *testing.T) {
	t.Parallel()

	_, ok, err := mcpclient.AppResultFromMetadata("")
	require.NoError(t, err)
	assert.False(t, ok)

	_, ok, err = mcpclient.AppResultFromMetadata(`{"attachments":[]}`)
	require.NoError(t, err)
	assert.False(t, ok)

	_, _, err = mcpclient.AppResultFromMetadata(`{not json`)
	require.Error(t, err)
}

func TestConnectAll_MCPAppsAdvertisesExtension(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	var (
		mu   sync.Mutex
		caps []*mcp.ClientCapabilities
	)
	srv := mcp.NewServer(&mcp.Implementation{Name: "caps", Version: "1.0.0"}, nil)
	srv.AddTool(echoTool().tool, echoTool().handler)
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true},
	)
	// Capture the client capabilities from the handshake. Depending
	// on the negotiated protocol revision they travel either in the
	// initialize params or in the request _meta of the first call.
	ts := newHTTPTestServer(t, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		r.Body = io.NopCloser(bytes.NewReader(body))
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Capabilities *mcp.ClientCapabilities `json:"capabilities"`
				Meta         struct {
					Capabilities *mcp.ClientCapabilities `json:"io.modelcontextprotocol/clientCapabilities"`
				} `json:"_meta"`
			} `json:"params"`
		}
		if json.Unmarshal(body, &msg) == nil && string(msg.ID) == "1" {
			mu.Lock()
			if msg.Params.Capabilities != nil {
				caps = append(caps, msg.Params.Capabilities)
			} else {
				caps = append(caps, msg.Params.Meta.Capabilities)
			}
			mu.Unlock()
		}
		mcpHandler.ServeHTTP(rw, r)
	}))
	cfg := makeConfig("caps", ts.URL)

	_, _, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil, uuid.Nil, nil, nil, testMCPHTTPClient(nil), mcpclient.ConnectOptions{})
	t.Cleanup(cleanup)
	_, _, cleanup = mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil, uuid.Nil, nil, nil, testMCPHTTPClient(nil), mcpclient.ConnectOptions{MCPApps: true})
	t.Cleanup(cleanup)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, caps, 2)

	require.NotNil(t, caps[0])
	assert.NotContains(t, caps[0].Extensions, mcpclient.UIExtensionID)

	require.NotNil(t, caps[1])
	ext, ok := caps[1].Extensions[mcpclient.UIExtensionID].(map[string]any)
	require.True(t, ok, "ui extension advertised")
	assert.Equal(t, []any{mcpclient.UIResourceMIMEType}, ext["mimeTypes"])
	assert.True(t, caps[1].Roots.ListChanged, "roots capability preserved")
}

func TestDialSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	srv := mcp.NewServer(&mcp.Implementation{Name: "dial", Version: "1.0.0"}, nil)
	board := uiTool("board", "ui://srv/board", nil)
	srv.AddTool(board.tool, board.handler)
	srv.AddResource(&mcp.Resource{
		URI:      "ui://srv/board",
		Name:     "board",
		MIMEType: mcpclient.UIResourceMIMEType,
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI:      req.Params.URI,
			MIMEType: mcpclient.UIResourceMIMEType,
			Text:     "<!doctype html><html></html>",
		}}}, nil
	})
	ts := newHTTPTestServer(t, mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true},
	))
	cfg := makeConfig("srv", ts.URL)

	session, err := mcpclient.DialSession(ctx, logger, cfg, nil, uuid.Nil, nil, nil, testMCPHTTPClient(nil), mcpclient.ConnectOptions{MCPApps: true})
	require.NoError(t, err)
	t.Cleanup(session.Close)

	require.Len(t, session.Tools(), 1)
	assert.Equal(t, "board", session.Tools()[0].Name)

	result, err := session.CallTool(ctx, "board", map[string]any{})
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	read, err := session.ReadResource(ctx, "ui://srv/board")
	require.NoError(t, err)
	require.Len(t, read.Contents, 1)
	assert.Equal(t, mcpclient.UIResourceMIMEType, read.Contents[0].MIMEType)

	resources, err := session.ListResources(ctx)
	require.NoError(t, err)
	require.Len(t, resources, 1)
	assert.Equal(t, "ui://srv/board", resources[0].URI)

	_, err = session.CallTool(ctx, "missing", nil)
	require.Error(t, err)
}

func newHTTPTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

func requireTool(t *testing.T, tools []fantasy.AgentTool, name string) fantasy.AgentTool {
	t.Helper()
	tool := findTool(tools, name)
	require.NotNil(t, tool, "tool %q not found", name)
	return tool
}

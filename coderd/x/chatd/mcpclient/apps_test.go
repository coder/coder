package mcpclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/testutil"
)

const testViewURI = "ui://dice/board.html"

// appsTestServer is an MCP server with one view tool (with a thread
// entry point), one app-only tool, one model-only tool and the view.
type appsTestServer struct {
	URL string

	mu         sync.Mutex
	extensions []map[string]any
}

func (s *appsTestServer) seenExtensions() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.extensions
}

func newAppsTestServer(t *testing.T) *appsTestServer {
	t.Helper()
	out := &appsTestServer{}
	srv := mcp.NewServer(&mcp.Implementation{Name: "apps-test", Version: "1.0.0"}, &mcp.ServerOptions{
		InitializedHandler: func(_ context.Context, req *mcp.InitializedRequest) {
			params := req.Session.InitializeParams()
			out.mu.Lock()
			defer out.mu.Unlock()
			if params != nil && params.Capabilities != nil {
				out.extensions = append(out.extensions, params.Capabilities.Extensions)
			} else {
				out.extensions = append(out.extensions, nil)
			}
		},
	})
	roll := func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]any
		if len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return nil, err
			}
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: "rolled 4"}},
			StructuredContent: map[string]any{"rolls": []int{4}, "args": args},
		}, nil
	}
	srv.AddTool(&mcp.Tool{
		Name:        "show_board",
		Title:       "Dice board",
		InputSchema: map[string]any{"type": "object"},
		Meta: mcp.Meta{
			"ui":        map[string]any{"resourceUri": testViewURI},
			"openai/ui": map[string]any{"entrypoints": []any{map[string]any{"type": "thread"}}},
		},
	}, roll)
	srv.AddTool(&mcp.Tool{
		Name:        "roll_again",
		InputSchema: map[string]any{"type": "object"},
		Meta:        mcp.Meta{"ui": map[string]any{"resourceUri": testViewURI, "visibility": []any{"app"}}},
	}, roll)
	srv.AddTool(&mcp.Tool{
		Name:        "server_time",
		InputSchema: map[string]any{"type": "object"},
		Meta:        mcp.Meta{"ui": map[string]any{"visibility": []any{"model"}}},
	}, roll)
	srv.AddResource(&mcp.Resource{URI: testViewURI, Name: "board", MIMEType: mcpclient.AppMIMEType},
		func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
				URI:      req.Params.URI,
				MIMEType: mcpclient.AppMIMEType,
				Text:     "<!doctype html><p>board</p>",
				Meta: mcp.Meta{"ui": map[string]any{"csp": map[string]any{
					"connectDomains": []any{"https://api.example.com"},
				}}},
			}}}, nil
		})
	srv.AddResource(&mcp.Resource{URI: "ui://dice/data.json", Name: "data", MIMEType: "application/json"},
		func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
				URI: req.Params.URI, MIMEType: "application/json", Text: "{}",
			}}}, nil
		})
	ts := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))
	t.Cleanup(ts.Close)
	out.URL = ts.URL
	return out
}

func TestParseUIMeta(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		meta mcp.Meta
		want mcpclient.UIMeta
	}{
		{
			name: "NoMeta",
			want: mcpclient.UIMeta{Visibility: []string{"model", "app"}},
		},
		{
			name: "NestedURI",
			meta: mcp.Meta{"ui": map[string]any{"resourceUri": "ui://a/b"}},
			want: mcpclient.UIMeta{ResourceURI: "ui://a/b", Visibility: []string{"model", "app"}},
		},
		{
			name: "DeprecatedFlatURI",
			meta: mcp.Meta{"ui/resourceUri": "ui://a/b"},
			want: mcpclient.UIMeta{ResourceURI: "ui://a/b", Visibility: []string{"model", "app"}},
		},
		{
			name: "NonUIScheme",
			meta: mcp.Meta{"ui": map[string]any{"resourceUri": "https://a/b"}},
			want: mcpclient.UIMeta{Visibility: []string{"model", "app"}},
		},
		{
			name: "AppOnly",
			meta: mcp.Meta{"ui": map[string]any{"resourceUri": "ui://a/b", "visibility": []any{"app"}}},
			want: mcpclient.UIMeta{ResourceURI: "ui://a/b", Visibility: []string{"app"}},
		},
		{
			name: "ThreadEntrypoint",
			meta: mcp.Meta{
				"ui":        map[string]any{"resourceUri": "ui://a/b"},
				"openai/ui": map[string]any{"entrypoints": []any{map[string]any{"type": "global"}, map[string]any{"type": "thread"}}},
			},
			want: mcpclient.UIMeta{ResourceURI: "ui://a/b", Visibility: []string{"model", "app"}, Thread: true},
		},
		{
			name: "ThreadEntrypointWithoutView",
			meta: mcp.Meta{"openai/ui": map[string]any{"entrypoints": []any{map[string]any{"type": "thread"}}}},
			want: mcpclient.UIMeta{Visibility: []string{"model", "app"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, mcpclient.ParseUIMeta(tc.meta))
		})
	}
}

func TestConnectAll_MCPApps(t *testing.T) {
	t.Parallel()

	t.Run("Enabled", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		server := newAppsTestServer(t)

		cfg := makeConfig("dice", server.URL)
		cfg.MCPApps = true
		tools, _, cleanup := mcpclient.ConnectAll(ctx, logger, []mcpclient.Server{cfg}, nil, uuid.Nil, nil, nil, testMCPHTTPClient(nil))
		t.Cleanup(cleanup)

		// The model never gets the app-only tool.
		assert.ElementsMatch(t, []string{"dice__show_board", "dice__server_time"}, toolNames(tools))
		require.Len(t, server.seenExtensions(), 1)
		assert.Equal(t, map[string]any{
			mcpclient.AppsExtension: map[string]any{"mimeTypes": []any{mcpclient.AppMIMEType}},
		}, server.seenExtensions()[0])

		board := findTool(tools, "dice__show_board")
		require.NotNil(t, board)
		resp, err := board.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "dice__show_board", Input: `{"count":1}`})
		require.NoError(t, err)
		assert.Equal(t, "rolled 4", resp.Content[:len("rolled 4")])

		var meta struct {
			MCPApp map[string]any `json:"mcp_app"`
		}
		require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
		assert.Equal(t, cfg.ID.String(), meta.MCPApp["mcp_server_config_id"])
		assert.Equal(t, "show_board", meta.MCPApp["tool_name"])
		assert.Equal(t, "Dice board", meta.MCPApp["title"])
		assert.Equal(t, testViewURI, meta.MCPApp["resource_uri"])
		assert.Equal(t, map[string]any{"count": float64(1)}, meta.MCPApp["tool_input"])
		result, ok := meta.MCPApp["tool_result"].(map[string]any)
		require.True(t, ok, "tool_result: %v", meta.MCPApp["tool_result"])
		assert.Contains(t, result, "structuredContent")
	})

	t.Run("Disabled", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		server := newAppsTestServer(t)

		cfg := makeConfig("dice", server.URL)
		tools, _, cleanup := mcpclient.ConnectAll(ctx, logger, []mcpclient.Server{cfg}, nil, uuid.Nil, nil, nil, testMCPHTTPClient(nil))
		t.Cleanup(cleanup)

		// Nothing changes without the experiment: all tools, no extension.
		assert.ElementsMatch(t, []string{"dice__show_board", "dice__roll_again", "dice__server_time"}, toolNames(tools))
		require.Len(t, server.seenExtensions(), 1)
		assert.Empty(t, server.seenExtensions()[0])
		for _, tool := range tools {
			if tool.Info().Name != "dice__show_board" {
				continue
			}
			resp, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "dice__show_board", Input: `{}`})
			require.NoError(t, err)
			assert.Empty(t, resp.Metadata)
		}
	})
}

func TestAppSession(t *testing.T) {
	t.Parallel()

	connect := func(t *testing.T, mutate func(*mcpclient.Server)) *mcpclient.AppSession {
		t.Helper()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		server := newAppsTestServer(t)
		cfg := makeConfig("dice", server.URL)
		if mutate != nil {
			mutate(&cfg)
		}
		session, err := mcpclient.ConnectAppSession(ctx, logger, cfg, nil, uuid.Nil, nil, nil, testMCPHTTPClient(nil))
		require.NoError(t, err)
		t.Cleanup(session.Close)
		// The server records the extensions when the initialized
		// notification arrives, after Connect returns.
		testutil.Eventually(ctx, t, func(context.Context) bool {
			return len(server.seenExtensions()) == 1
		}, testutil.IntervalFast)
		assert.Contains(t, server.seenExtensions()[0], mcpclient.AppsExtension)
		return session
	}

	t.Run("ReadView", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		session := connect(t, nil)

		view, err := session.ReadView(ctx, testViewURI)
		require.NoError(t, err)
		assert.Equal(t, "<!doctype html><p>board</p>", string(view.HTML))
		assert.Equal(t, []string{"https://api.example.com"}, view.CSP.ConnectDomains)

		_, err = session.ReadView(ctx, "https://example.com/view.html")
		require.ErrorIs(t, err, mcpclient.ErrAppViewInvalid)
		_, err = session.ReadView(ctx, "ui://dice/data.json")
		require.ErrorIs(t, err, mcpclient.ErrAppViewInvalid)
	})

	t.Run("Entrypoints", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		session := connect(t, nil)

		entrypoints, err := session.Entrypoints(ctx)
		require.NoError(t, err)
		assert.Equal(t, []mcpclient.AppEntrypoint{{
			ToolName: "show_board", Title: "Dice board", ResourceURI: testViewURI,
		}}, entrypoints)
	})

	t.Run("CallAppTool", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		session := connect(t, nil)

		result, err := session.CallAppTool(ctx, "roll_again", map[string]any{"count": 2})
		require.NoError(t, err)
		assert.NotNil(t, result.StructuredContent)

		// A model-only tool is refused, an unknown tool is not found.
		_, err = session.CallAppTool(ctx, "server_time", nil)
		require.ErrorIs(t, err, mcpclient.ErrAppToolNotAllowed)
		_, err = session.CallAppTool(ctx, "missing", nil)
		require.ErrorIs(t, err, mcpclient.ErrAppToolNotFound)
	})

	t.Run("CallAppToolDenyList", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		session := connect(t, func(srv *mcpclient.Server) { srv.ToolDenyList = []string{"roll_again"} })

		_, err := session.CallAppTool(ctx, "roll_again", nil)
		require.ErrorIs(t, err, mcpclient.ErrAppToolNotFound)
	})

	t.Run("OpenEntrypoint", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		session := connect(t, nil)

		app, err := session.OpenEntrypoint(ctx, "show_board")
		require.NoError(t, err)
		assert.Equal(t, "show_board", app.ToolName)
		assert.Equal(t, testViewURI, app.ResourceURI)
		assert.Equal(t, map[string]any{}, app.ToolInput)
		assert.Contains(t, app.ToolResult, "structuredContent")

		// Only a tool with a thread entry point opens.
		_, err = session.OpenEntrypoint(ctx, "roll_again")
		require.ErrorIs(t, err, mcpclient.ErrAppToolNotFound)
	})
}

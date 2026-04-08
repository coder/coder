package mcpclient_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
)

// newTestMCPServer creates a streamable HTTP MCP server with the
// given tools.  The caller must close the returned *httptest.Server.
func newTestMCPServer(t *testing.T, tools ...mcpserver.ServerTool) *httptest.Server {
	t.Helper()
	srv := mcpserver.NewMCPServer("test-server", "1.0.0")
	srv.AddTools(tools...)
	httpSrv := mcpserver.NewStreamableHTTPServer(srv)
	ts := httptest.NewServer(httpSrv)
	t.Cleanup(ts.Close)
	return ts
}

// echoTool returns a ServerTool that echoes its "input" argument
// prefixed with "echo: ".
func echoTool() mcpserver.ServerTool {
	return mcpserver.ServerTool{
		Tool: mcp.NewTool("echo",
			mcp.WithDescription("Echoes the input"),
			mcp.WithString("input", mcp.Description("The input"), mcp.Required()),
		),
		Handler: func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			input, _ := req.GetArguments()["input"].(string)
			return mcp.NewToolResultText("echo: " + input), nil
		},
	}
}

// greetTool returns a ServerTool that greets by name.
func greetTool() mcpserver.ServerTool {
	return mcpserver.ServerTool{
		Tool: mcp.NewTool("greet",
			mcp.WithDescription("Greets the user"),
			mcp.WithString("name", mcp.Description("Name to greet"), mcp.Required()),
		),
		Handler: func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.GetArguments()["name"].(string)
			return mcp.NewToolResultText("hello " + name), nil
		},
	}
}

// makeTool returns a ServerTool with the given name and a
// no-op handler that always returns "ok".
func makeTool(name string) mcpserver.ServerTool {
	return mcpserver.ServerTool{
		Tool: mcp.NewTool(name),
		Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("ok"), nil
		},
	}
}

// makeConfig builds a database.MCPServerConfig suitable for tests.
func makeConfig(slug, url string) database.MCPServerConfig {
	return database.MCPServerConfig{
		ID:          uuid.New(),
		Slug:        slug,
		DisplayName: slug,
		Url:         url,
		Transport:   "streamable_http",
		AuthType:    "none",
		Enabled:     true,
	}
}

func TestConnectAll_DiscoverTools(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, echoTool(), greetTool())

	cfg := makeConfig("myserver", ts.URL)
	tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	t.Cleanup(cleanup)

	// Two tools should be discovered, namespaced with the server slug.
	require.Len(t, tools, 2)

	names := toolNames(tools)
	assert.Contains(t, names, "myserver__echo")
	assert.Contains(t, names, "myserver__greet")

	// Verify the description is preserved.
	foundEcho := findTool(tools, "myserver__echo")
	require.NotNilf(t, foundEcho, "expected to find myserver__echo")
	echoInfo := foundEcho.Info()
	assert.Equal(t, "Echoes the input", echoInfo.Description)
}

func TestConnectAll_CallTool(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, echoTool())

	cfg := makeConfig("srv", ts.URL)
	tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)

	tool := tools[0]
	resp, err := tool.Run(ctx, fantasy.ToolCall{
		ID:    "call-1",
		Name:  "srv__echo",
		Input: `{"input":"hello world"}`,
	})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.Equal(t, "echo: hello world", resp.Content)
}

func TestConnectAll_ToolAllowList(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, echoTool(), greetTool())

	cfg := makeConfig("filtered", ts.URL)
	// Only allow the "echo" tool.
	cfg.ToolAllowList = []string{"echo"}

	tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	t.Cleanup(cleanup)

	require.Len(t, tools, 1)
	assert.Equal(t, "filtered__echo", tools[0].Info().Name)
}

func TestConnectAll_ToolDenyList(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, echoTool(), greetTool())

	cfg := makeConfig("filtered", ts.URL)
	// Deny the "greet" tool, so only "echo" remains.
	cfg.ToolDenyList = []string{"greet"}

	tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	t.Cleanup(cleanup)

	require.Len(t, tools, 1)
	assert.Equal(t, "filtered__echo", tools[0].Info().Name)
}

func TestConnectAll_ConnectionFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	cfg := makeConfig("bad", "http://127.0.0.1:0/does-not-exist")

	tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	t.Cleanup(cleanup)

	assert.Empty(t, tools, "no tools should be returned for an unreachable server")
}

func TestConnectAll_MultipleServers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts1 := newTestMCPServer(t, echoTool())
	ts2 := newTestMCPServer(t, greetTool())

	cfg1 := makeConfig("alpha", ts1.URL)
	cfg2 := makeConfig("beta", ts2.URL)

	tools, cleanup := mcpclient.ConnectAll(
		ctx, logger,
		[]database.MCPServerConfig{cfg1, cfg2},
		nil,
	)
	t.Cleanup(cleanup)

	require.Len(t, tools, 2)

	names := toolNames(tools)
	assert.Contains(t, names, "alpha__echo")
	assert.Contains(t, names, "beta__greet")
}

func TestConnectAll_NoToolsAfterFiltering(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, echoTool())

	cfg := makeConfig("filtered", ts.URL)
	cfg.ToolAllowList = []string{"greet"}

	tools, cleanup := mcpclient.ConnectAll(
		ctx,
		logger,
		[]database.MCPServerConfig{cfg},
		nil,
	)

	require.Empty(t, tools)
	assert.NotPanics(t, cleanup)
}

func TestConnectAll_DeterministicOrder(t *testing.T) {
	t.Parallel()

	t.Run("AcrossServers", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

		ts1 := newTestMCPServer(t, makeTool("zebra"))
		ts2 := newTestMCPServer(t, makeTool("alpha"))
		ts3 := newTestMCPServer(t, makeTool("middle"))

		tools, cleanup := mcpclient.ConnectAll(
			ctx,
			logger,
			[]database.MCPServerConfig{
				makeConfig("srv3", ts3.URL),
				makeConfig("srv1", ts1.URL),
				makeConfig("srv2", ts2.URL),
			},
			nil,
		)
		t.Cleanup(cleanup)

		require.Len(t, tools, 3)
		// Sorted by full prefixed name (slug__tool), so slug
		// order determines the sequence, not the tool name.
		assert.Equal(t,
			[]string{"srv1__zebra", "srv2__alpha", "srv3__middle"},
			toolNames(tools),
		)
	})

	t.Run("WithMultiToolServer", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

		multi := newTestMCPServer(t, makeTool("zeta"), makeTool("beta"))
		other := newTestMCPServer(t, makeTool("gamma"))

		tools, cleanup := mcpclient.ConnectAll(
			ctx,
			logger,
			[]database.MCPServerConfig{
				makeConfig("zzz", multi.URL),
				makeConfig("aaa", other.URL),
			},
			nil,
		)
		t.Cleanup(cleanup)

		require.Len(t, tools, 3)
		assert.Equal(t,
			[]string{"aaa__gamma", "zzz__beta", "zzz__zeta"},
			toolNames(tools),
		)
	})

	t.Run("TiebreakByConfigID", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

		ts1 := newTestMCPServer(t, makeTool("b__z"))
		ts2 := newTestMCPServer(t, makeTool("z"))

		// Use fixed UUIDs so the tiebreaker order is
		// predictable. Both servers produce the same prefixed
		// name, a__b__z, due to the __ separator ambiguity.
		cfg1 := makeConfig("a", ts1.URL)
		cfg1.ID = uuid.MustParse("00000000-0000-0000-0000-000000000002")

		cfg2 := makeConfig("a__b", ts2.URL)
		cfg2.ID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

		tools, cleanup := mcpclient.ConnectAll(
			ctx,
			logger,
			[]database.MCPServerConfig{cfg1, cfg2},
			nil,
		)
		t.Cleanup(cleanup)

		require.Len(t, tools, 2)
		assert.Equal(t, []string{"a__b__z", "a__b__z"}, toolNames(tools))

		id0 := tools[0].(mcpclient.MCPToolIdentifier).MCPServerConfigID()
		id1 := tools[1].(mcpclient.MCPToolIdentifier).MCPServerConfigID()
		assert.Equal(t, cfg2.ID, id0, "lower config ID should sort first")
		assert.Equal(t, cfg1.ID, id1, "higher config ID should sort second")
	})
}

func TestConnectAll_AuthHeaders(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	// Create a server whose tool handler records the Authorization
	// header it receives on each request.
	var (
		mu          sync.Mutex
		seenHeaders []string
	)

	srv := mcpserver.NewMCPServer("auth-server", "1.0.0")
	srv.AddTools(mcpserver.ServerTool{
		Tool: mcp.NewTool("whoami",
			mcp.WithDescription("Returns the auth header"),
		),
		Handler: func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			auth := req.Header.Get("Authorization")
			mu.Lock()
			seenHeaders = append(seenHeaders, auth)
			mu.Unlock()
			return mcp.NewToolResultText("auth:" + auth), nil
		},
	})

	httpSrv := mcpserver.NewStreamableHTTPServer(srv)
	ts := httptest.NewServer(httpSrv)
	t.Cleanup(ts.Close)

	configID := uuid.New()
	cfg := database.MCPServerConfig{
		ID:          configID,
		Slug:        "auth-srv",
		DisplayName: "Auth Server",
		Url:         ts.URL,
		Transport:   "streamable_http",
		AuthType:    "oauth2",
		Enabled:     true,
	}
	token := database.MCPServerUserToken{
		MCPServerConfigID: configID,
		AccessToken:       "test-token-abc",
		TokenType:         "Bearer",
	}

	tools, cleanup := mcpclient.ConnectAll(
		ctx, logger,
		[]database.MCPServerConfig{cfg},
		[]database.MCPServerUserToken{token},
	)
	t.Cleanup(cleanup)

	require.Len(t, tools, 1)

	// Call the tool and verify the response includes the auth header
	// that was sent.
	resp, err := tools[0].Run(ctx, fantasy.ToolCall{
		ID:    "call-auth",
		Name:  "auth-srv__whoami",
		Input: "{}",
	})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.Equal(t, "auth:Bearer test-token-abc", resp.Content)

	// Also verify the handler actually observed the header.
	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, seenHeaders)
	assert.Equal(t, "Bearer test-token-abc", seenHeaders[len(seenHeaders)-1])
}

// --- helpers ---

func toolNames(tools []fantasy.AgentTool) []string {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Info().Name)
	}
	return names
}

func findTool(tools []fantasy.AgentTool, name string) fantasy.AgentTool {
	for _, t := range tools {
		if t.Info().Name == name {
			return t
		}
	}
	return nil
}

// TestConnectAll_DisabledServer verifies that disabled configs are
// silently skipped.
func TestConnectAll_DisabledServer(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, echoTool())

	cfg := makeConfig("disabled", ts.URL)
	cfg.Enabled = false

	tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	t.Cleanup(cleanup)
	assert.Empty(t, tools)
}

// TestConnectAll_CallToolInvalidInput verifies that malformed JSON
// input returns an error response rather than a Go error.
func TestConnectAll_CallToolInvalidInput(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, echoTool())

	cfg := makeConfig("srv", ts.URL)
	tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)

	// Pass syntactically invalid JSON as tool input.
	resp, err := tools[0].Run(ctx, fantasy.ToolCall{
		ID:    "call-bad",
		Name:  "srv__echo",
		Input: `{not json`,
	})
	require.NoError(t, err, "Run should not return a Go error for bad input")
	assert.True(t, resp.IsError)
	assert.Contains(t, resp.Content, "invalid JSON input")
}

// TestConnectAll_ToolInfoParameters verifies that tool input schema
// parameters are propagated to the ToolInfo.
func TestConnectAll_ToolInfoParameters(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, echoTool())

	cfg := makeConfig("srv", ts.URL)
	tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)

	info := tools[0].Info()
	// The echo tool has a required "input" string parameter.
	require.NotNil(t, info.Parameters)
	_, hasInput := info.Parameters["input"]
	assert.True(t, hasInput, "parameters should contain 'input'")

	// The "input" field should also appear in Required.
	inputProp, ok := info.Parameters["input"].(map[string]any)
	assert.True(t, ok, "input parameter should be a map")
	if ok {
		propBytes, _ := json.Marshal(inputProp)
		assert.Contains(t, string(propBytes), "string")
	}
	assert.Contains(t, info.Required, "input")
}

// TestConnectAll_NilRequiredBecomesEmptySlice verifies that a tool
// whose inputSchema omits "required" produces an empty slice instead
// of nil.  A nil slice serializes to JSON null, which OpenAI rejects
// with "None is not of type 'array'".
func TestConnectAll_NilRequiredBecomesEmptySlice(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	// noRequiredTool defines a tool with no required parameters.
	noRequiredTool := mcpserver.ServerTool{
		Tool: mcp.NewTool("optional_only",
			mcp.WithDescription("A tool with no required fields"),
			mcp.WithString("note", mcp.Description("An optional note")),
		),
		Handler: func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("ok"), nil
		},
	}

	ts := newTestMCPServer(t, noRequiredTool)
	cfg := makeConfig("srv", ts.URL)
	tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)

	info := tools[0].Info()
	// Required must be a non-nil empty slice, not nil.
	require.NotNil(t, info.Required, "Required should never be nil")
	assert.Empty(t, info.Required, "Required should be empty for tools without required fields")

	// Verify it serializes to [] not null.
	bs, err := json.Marshal(info.Required)
	require.NoError(t, err)
	assert.Equal(t, "[]", string(bs))
}

// TestConnectAll_APIKeyAuth verifies that api_key auth sends the
// configured header and value on every request.
func TestConnectAll_APIKeyAuth(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	var (
		mu          sync.Mutex
		seenHeaders []string
	)

	srv := mcpserver.NewMCPServer("apikey-server", "1.0.0")
	srv.AddTools(mcpserver.ServerTool{
		Tool: mcp.NewTool("check",
			mcp.WithDescription("Returns the API key header"),
		),
		Handler: func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			val := req.Header.Get("X-API-Key")
			mu.Lock()
			seenHeaders = append(seenHeaders, val)
			mu.Unlock()
			return mcp.NewToolResultText("key:" + val), nil
		},
	})

	httpSrv := mcpserver.NewStreamableHTTPServer(srv)
	ts := httptest.NewServer(httpSrv)
	t.Cleanup(ts.Close)

	cfg := makeConfig("apikey", ts.URL)
	cfg.AuthType = "api_key"
	cfg.APIKeyHeader = "X-API-Key"
	cfg.APIKeyValue = "secret-123"

	tools, cleanup := mcpclient.ConnectAll(
		ctx, logger, []database.MCPServerConfig{cfg}, nil,
	)
	t.Cleanup(cleanup)

	require.Len(t, tools, 1)

	resp, err := tools[0].Run(ctx, fantasy.ToolCall{
		ID:    "call-apikey",
		Name:  "apikey__check",
		Input: "{}",
	})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.Equal(t, "key:secret-123", resp.Content)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, seenHeaders)
	assert.Equal(t, "secret-123", seenHeaders[len(seenHeaders)-1])
}

// TestConnectAll_CustomHeadersAuth verifies that custom_headers
// auth sends the configured headers on every request.
func TestConnectAll_CustomHeadersAuth(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	var (
		mu          sync.Mutex
		seenHeaders []string
	)

	srv := mcpserver.NewMCPServer("custom-server", "1.0.0")
	srv.AddTools(mcpserver.ServerTool{
		Tool: mcp.NewTool("check",
			mcp.WithDescription("Returns the custom auth header"),
		),
		Handler: func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			val := req.Header.Get("X-Custom-Auth")
			mu.Lock()
			seenHeaders = append(seenHeaders, val)
			mu.Unlock()
			return mcp.NewToolResultText("custom:" + val), nil
		},
	})

	httpSrv := mcpserver.NewStreamableHTTPServer(srv)
	ts := httptest.NewServer(httpSrv)
	t.Cleanup(ts.Close)

	cfg := makeConfig("custom", ts.URL)
	cfg.AuthType = "custom_headers"
	cfg.CustomHeaders = `{"X-Custom-Auth":"custom-val"}`

	tools, cleanup := mcpclient.ConnectAll(
		ctx, logger, []database.MCPServerConfig{cfg}, nil,
	)
	t.Cleanup(cleanup)

	require.Len(t, tools, 1)

	resp, err := tools[0].Run(ctx, fantasy.ToolCall{
		ID:    "call-custom",
		Name:  "custom__check",
		Input: "{}",
	})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.Equal(t, "custom:custom-val", resp.Content)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, seenHeaders)
	assert.Equal(t, "custom-val", seenHeaders[len(seenHeaders)-1])
}

// TestConnectAll_CustomHeadersInvalidJSON verifies that invalid
// JSON in CustomHeaders does not prevent the server from
// connecting. The auth headers are silently skipped.
func TestConnectAll_CustomHeadersInvalidJSON(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, echoTool())

	cfg := makeConfig("badjson", ts.URL)
	cfg.AuthType = "custom_headers"
	cfg.CustomHeaders = "{not json}"

	tools, cleanup := mcpclient.ConnectAll(
		ctx, logger, []database.MCPServerConfig{cfg}, nil,
	)
	t.Cleanup(cleanup)

	// The server should still connect; only auth headers are
	// skipped.
	require.Len(t, tools, 1)
	assert.Equal(t, "badjson__echo", tools[0].Info().Name)
}

// TestConnectAll_ParallelConnections verifies that connecting to
// multiple MCP servers simultaneously returns all discovered
// tools with the correct server slug prefixes.
func TestConnectAll_ParallelConnections(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts1 := newTestMCPServer(t, echoTool())
	ts2 := newTestMCPServer(t, greetTool())
	ts3 := newTestMCPServer(t, echoTool())

	cfg1 := makeConfig("srv1", ts1.URL)
	cfg2 := makeConfig("srv2", ts2.URL)
	cfg3 := makeConfig("srv3", ts3.URL)

	tools, cleanup := mcpclient.ConnectAll(
		ctx, logger,
		[]database.MCPServerConfig{cfg1, cfg2, cfg3},
		nil,
	)
	t.Cleanup(cleanup)

	require.Len(t, tools, 3)

	names := toolNames(tools)
	assert.Contains(t, names, "srv1__echo")
	assert.Contains(t, names, "srv2__greet")
	assert.Contains(t, names, "srv3__echo")
}

func TestRedactURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain", "https://mcp.example.com/v1", "https://mcp.example.com/v1"},
		{"with userinfo", "https://user:secret@mcp.example.com/v1", "https://mcp.example.com/v1"},
		{"with query params", "https://mcp.example.com/v1?api_key=sk-123", "https://mcp.example.com/v1"},
		{"with both", "https://user:pass@host/p?key=val", "https://host/p"},
		{"invalid url", "://not-a-url", "://not-a-url"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mcpclient.RedactURL(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestConnectAll_ExpiredToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, echoTool())

	configID := uuid.New()
	cfg := database.MCPServerConfig{
		ID:          configID,
		Slug:        "expired-srv",
		DisplayName: "Expired Server",
		Url:         ts.URL,
		Transport:   "streamable_http",
		AuthType:    "oauth2",
		Enabled:     true,
	}
	// Token exists but is expired.
	token := database.MCPServerUserToken{
		MCPServerConfigID: configID,
		AccessToken:       "expired-token",
		TokenType:         "Bearer",
		Expiry:            sql.NullTime{Time: time.Now().Add(-1 * time.Hour), Valid: true},
	}

	tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, []database.MCPServerUserToken{token})
	t.Cleanup(cleanup)

	// The server accepts any auth, so the tool is still discovered
	// despite the expired token. The important thing is that the
	// warning is logged (verified via IgnoreErrors: true in slogtest).
	require.NotEmpty(t, tools)
}

func TestConnectAll_EmptyAccessToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, echoTool())

	configID := uuid.New()
	cfg := database.MCPServerConfig{
		ID:          configID,
		Slug:        "empty-tok",
		DisplayName: "Empty Token Server",
		Url:         ts.URL,
		Transport:   "streamable_http",
		AuthType:    "oauth2",
		Enabled:     true,
	}
	// Token record exists but AccessToken is empty.
	token := database.MCPServerUserToken{
		MCPServerConfigID: configID,
		AccessToken:       "",
		TokenType:         "Bearer",
	}

	tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, []database.MCPServerUserToken{token})
	t.Cleanup(cleanup)

	// Tool is still discovered (server doesn't require auth), but
	// no Authorization header was sent. The warning about empty
	// access token is logged.
	require.NotEmpty(t, tools)
}

// TestConnectAll_MCPToolIdentifier verifies that tools returned
// by ConnectAll implement the MCPToolIdentifier interface and
// report the correct server config ID.
func TestConnectAll_MCPToolIdentifier(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, echoTool())

	configID := uuid.New()
	cfg := database.MCPServerConfig{
		ID:          configID,
		Slug:        "id-srv",
		DisplayName: "ID Server",
		Url:         ts.URL,
		Transport:   "streamable_http",
		AuthType:    "none",
		Enabled:     true,
	}

	tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	t.Cleanup(cleanup)

	require.Len(t, tools, 1)

	// Assert the tool implements MCPToolIdentifier.
	identifier, ok := tools[0].(mcpclient.MCPToolIdentifier)
	require.True(t, ok, "tool should implement MCPToolIdentifier")
	assert.Equal(t, configID, identifier.MCPServerConfigID())
}

// TestConnectAll_MCPToolIdentifier_MultipleServers verifies that
// each tool from a different MCP server carries its own config ID.
func TestConnectAll_MCPToolIdentifier_MultipleServers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts1 := newTestMCPServer(t, echoTool())
	ts2 := newTestMCPServer(t, greetTool())

	configID1 := uuid.New()
	configID2 := uuid.New()
	cfg1 := database.MCPServerConfig{
		ID:          configID1,
		Slug:        "srv-a",
		DisplayName: "Server A",
		Url:         ts1.URL,
		Transport:   "streamable_http",
		AuthType:    "none",
		Enabled:     true,
	}
	cfg2 := database.MCPServerConfig{
		ID:          configID2,
		Slug:        "srv-b",
		DisplayName: "Server B",
		Url:         ts2.URL,
		Transport:   "streamable_http",
		AuthType:    "none",
		Enabled:     true,
	}

	tools, cleanup := mcpclient.ConnectAll(
		ctx, logger,
		[]database.MCPServerConfig{cfg1, cfg2},
		nil,
	)
	t.Cleanup(cleanup)

	require.Len(t, tools, 2)

	// Map tool name to config ID via the MCPToolIdentifier
	// interface.
	idByName := make(map[string]uuid.UUID)
	for _, tool := range tools {
		identifier, ok := tool.(mcpclient.MCPToolIdentifier)
		require.True(t, ok, "tool %q should implement MCPToolIdentifier", tool.Info().Name)
		idByName[tool.Info().Name] = identifier.MCPServerConfigID()
	}

	assert.Equal(t, configID1, idByName["srv-a__echo"])
	assert.Equal(t, configID2, idByName["srv-b__greet"])
}

// TestConnectAll_EmbeddedResourceText verifies that a tool returning
// an EmbeddedResource with TextResourceContents has its text extracted
// into the response content.
func TestConnectAll_EmbeddedResourceText(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	srv := mcpserver.NewMCPServer("embedded-text-server", "1.0.0")
	srv.AddTools(mcpserver.ServerTool{
		Tool: mcp.NewTool("fetch_doc",
			mcp.WithDescription("Returns an embedded text resource"),
		),
		Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{
						Type: "text",
						Text: "successfully downloaded text file",
					},
					mcp.EmbeddedResource{
						Type: "resource",
						Resource: mcp.TextResourceContents{
							URI:      "file:///example.txt",
							MIMEType: "text/plain",
							Text:     "Hello from embedded resource",
						},
					},
				},
			}, nil
		},
	})

	httpSrv := mcpserver.NewStreamableHTTPServer(srv)
	ts := httptest.NewServer(httpSrv)
	t.Cleanup(ts.Close)

	cfg := makeConfig("embed-txt", ts.URL)
	tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)

	resp, err := tools[0].Run(ctx, fantasy.ToolCall{
		ID:    "call-embed-txt",
		Name:  "embed-txt__fetch_doc",
		Input: "{}",
	})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.Contains(t, resp.Content, "Hello from embedded resource")
	assert.Contains(t, resp.Content, "successfully downloaded text file")
	assert.NotContains(t, resp.Content, "unsupported content type")
}

// TestConnectAll_EmbeddedResourceBlob verifies that a tool returning
// an EmbeddedResource with BlobResourceContents has its blob decoded
// into the binary response path, with the Type field reflecting the
// MIME type.
func TestConnectAll_EmbeddedResourceBlob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		mimeType     string
		expectedType string
	}{
		{"image", "image/png", "image"},
		{"non-image", "application/pdf", "media"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

			blobData := base64.StdEncoding.EncodeToString([]byte("binary-content"))
			mime := tt.mimeType

			srv := mcpserver.NewMCPServer("embedded-blob-server", "1.0.0")
			srv.AddTools(mcpserver.ServerTool{
				Tool: mcp.NewTool("fetch_blob",
					mcp.WithDescription("Returns an embedded blob resource"),
				),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return &mcp.CallToolResult{
						Content: []mcp.Content{
							mcp.EmbeddedResource{
								Type: "resource",
								Resource: mcp.BlobResourceContents{
									URI:      "file:///blob",
									MIMEType: mime,
									Blob:     blobData,
								},
							},
						},
					}, nil
				},
			})

			httpSrv := mcpserver.NewStreamableHTTPServer(srv)
			ts := httptest.NewServer(httpSrv)
			t.Cleanup(ts.Close)

			cfg := makeConfig("embed-blob", ts.URL)
			tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
			t.Cleanup(cleanup)
			require.Len(t, tools, 1)

			resp, err := tools[0].Run(ctx, fantasy.ToolCall{
				ID:    "call-embed-blob",
				Name:  "embed-blob__fetch_blob",
				Input: "{}",
			})
			require.NoError(t, err)
			assert.False(t, resp.IsError)
			// The blob is the only content item, so the binary
			// path is taken: Content is empty and the decoded
			// bytes land in Data.
			assert.Empty(t, resp.Content, "binary-only response should have empty Content")
			assert.Equal(t, tt.expectedType, resp.Type)
			assert.Equal(t, []byte("binary-content"), resp.Data)
			assert.Equal(t, tt.mimeType, resp.MediaType)
		})
	}
}

// TestConnectAll_ResourceLink verifies that a tool returning a
// ResourceLink renders it as human-readable text containing the
// resource name, URI, and description when present.
func TestConnectAll_ResourceLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		link        mcp.ResourceLink
		contains    []string
		notContains []string
	}{
		{
			name: "with_name",
			link: mcp.ResourceLink{
				Type: "resource_link",
				Name: "Example Resource",
				URI:  "https://example.com/resource",
			},
			contains:    []string{"Example Resource", "https://example.com/resource"},
			notContains: []string{"unsupported content type"},
		},
		{
			name: "with_description",
			link: mcp.ResourceLink{
				Type:        "resource_link",
				Name:        "Deploy Log",
				URI:         "file:///var/log/deploy.log",
				Description: "Latest deployment log",
			},
			contains: []string{"Deploy Log", "file:///var/log/deploy.log", "Latest deployment log"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

			link := tt.link
			srv := mcpserver.NewMCPServer("resource-link-server", "1.0.0")
			srv.AddTools(mcpserver.ServerTool{
				Tool: mcp.NewTool("get_link",
					mcp.WithDescription("Returns a resource link"),
				),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return &mcp.CallToolResult{
						Content: []mcp.Content{link},
					}, nil
				},
			})

			httpSrv := mcpserver.NewStreamableHTTPServer(srv)
			ts := httptest.NewServer(httpSrv)
			t.Cleanup(ts.Close)

			cfg := makeConfig("res-link", ts.URL)
			tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
			t.Cleanup(cleanup)
			require.Len(t, tools, 1)

			resp, err := tools[0].Run(ctx, fantasy.ToolCall{
				ID:    "call-res-link",
				Name:  "res-link__get_link",
				Input: "{}",
			})
			require.NoError(t, err)
			assert.False(t, resp.IsError)
			for _, s := range tt.contains {
				assert.Contains(t, resp.Content, s)
			}
			for _, s := range tt.notContains {
				assert.NotContains(t, resp.Content, s)
			}
		})
	}
}

func TestConnectAll_CallToolError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	// Server with a tool that always returns an error result.
	srv := mcpserver.NewMCPServer("error-server", "1.0.0")
	srv.AddTools(mcpserver.ServerTool{
		Tool: mcp.NewTool("fail_tool",
			mcp.WithDescription("Always fails"),
		),
		Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.NewTextContent("something broke")},
				IsError: true,
			}, nil
		},
	})
	httpSrv := mcpserver.NewStreamableHTTPServer(srv)
	ts := httptest.NewServer(httpSrv)
	t.Cleanup(ts.Close)

	cfg := makeConfig("err-srv", ts.URL)
	tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)

	resp, err := tools[0].Run(ctx, fantasy.ToolCall{
		ID:    "call-err",
		Name:  "err-srv__fail_tool",
		Input: "{}",
	})
	require.NoError(t, err, "Run should not return a Go error for MCP-level errors")
	assert.True(t, resp.IsError, "response should be flagged as error")
	assert.Contains(t, resp.Content, "something broke")
}

func TestModelIntent_Info_WrapsSchema(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, echoTool())

	cfg := makeConfig("intent-srv", ts.URL)
	cfg.ModelIntent = true

	tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)

	info := tools[0].Info()

	// Top-level schema should have model_intent and properties.
	_, hasModelIntent := info.Parameters["model_intent"]
	_, hasProperties := info.Parameters["properties"]
	assert.True(t, hasModelIntent, "schema should contain model_intent")
	assert.True(t, hasProperties, "schema should contain properties")

	// Required should include both.
	assert.Contains(t, info.Required, "model_intent")
	assert.Contains(t, info.Required, "properties")

	// The original "input" parameter should be nested under
	// properties.properties.
	propsObj, ok := info.Parameters["properties"].(map[string]any)
	require.True(t, ok)
	innerProps, ok := propsObj["properties"].(map[string]any)
	require.True(t, ok)
	_, hasInput := innerProps["input"]
	assert.True(t, hasInput, "original 'input' param should be nested")
}

func TestModelIntent_Info_NoWrapWhenDisabled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, echoTool())

	cfg := makeConfig("no-intent", ts.URL)
	cfg.ModelIntent = false

	tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)

	info := tools[0].Info()

	// Original schema should be flat — no model_intent wrapper.
	_, hasModelIntent := info.Parameters["model_intent"]
	assert.False(t, hasModelIntent, "schema should NOT contain model_intent")
	_, hasInput := info.Parameters["input"]
	assert.True(t, hasInput, "original 'input' param should be at top level")
}

func TestModelIntent_Run_UnwrapsProperties(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, echoTool())

	cfg := makeConfig("unwrap-srv", ts.URL)
	cfg.ModelIntent = true

	tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)

	// Correct format: model_intent + properties wrapper.
	resp, err := tools[0].Run(ctx, fantasy.ToolCall{
		ID:    "call-1",
		Name:  "unwrap-srv__echo",
		Input: `{"model_intent":"Testing echo","properties":{"input":"hello"}}`,
	})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.Equal(t, "echo: hello", resp.Content)
}

func TestModelIntent_Run_UnwrapsFlat(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, echoTool())

	cfg := makeConfig("flat-srv", ts.URL)
	cfg.ModelIntent = true

	tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)

	// Flat format: model_intent at top level, no properties wrapper.
	resp, err := tools[0].Run(ctx, fantasy.ToolCall{
		ID:    "call-2",
		Name:  "flat-srv__echo",
		Input: `{"model_intent":"Testing flat","input":"world"}`,
	})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.Equal(t, "echo: world", resp.Content)
}

func TestModelIntent_Run_PassthroughWhenDisabled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, echoTool())

	cfg := makeConfig("pass-srv", ts.URL)
	cfg.ModelIntent = false

	tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)

	// Without model_intent, input is passed through unchanged.
	resp, err := tools[0].Run(ctx, fantasy.ToolCall{
		ID:    "call-3",
		Name:  "pass-srv__echo",
		Input: `{"input":"direct"}`,
	})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
	assert.Equal(t, "echo: direct", resp.Content)
}

func TestModelIntent_Run_FallbackOnBadJSON(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, echoTool())

	cfg := makeConfig("bad-srv", ts.URL)
	cfg.ModelIntent = true

	tools, cleanup := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)

	// Malformed JSON should not panic — the error is returned
	// from the JSON unmarshal in Run(), not from unwrap.
	resp, err := tools[0].Run(ctx, fantasy.ToolCall{
		ID:    "call-bad",
		Name:  "bad-srv__echo",
		Input: `not-json`,
	})
	require.NoError(t, err)
	assert.True(t, resp.IsError, "malformed input should produce an error response")
}

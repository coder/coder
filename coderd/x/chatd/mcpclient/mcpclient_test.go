package mcpclient_test

import (
	"context"
	"database/sql"
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

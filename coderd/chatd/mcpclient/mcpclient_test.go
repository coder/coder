package mcpclient_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/chatd/mcpclient"
	"github.com/coder/coder/v2/coderd/database"
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

// secretTool returns a ServerTool that only succeeds if the request
// has the expected auth header.
func secretTool() mcpserver.ServerTool {
	return mcpserver.ServerTool{
		Tool: mcp.NewTool("secret",
			mcp.WithDescription("Returns a secret"),
		),
		Handler: func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			auth := req.Header.Get("Authorization")
			if auth == "" {
				return mcp.NewToolResultText("no-auth"), nil
			}
			return mcp.NewToolResultText("auth:" + auth), nil
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
	tools, cleanup, err := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	// Two tools should be discovered, namespaced with the server slug.
	require.Len(t, tools, 2)

	names := toolNames(tools)
	assert.Contains(t, names, "myserver__echo")
	assert.Contains(t, names, "myserver__greet")

	// Verify the description is preserved.
	echoInfo := findTool(tools, "myserver__echo").Info()
	assert.Equal(t, "Echoes the input", echoInfo.Description)
}

func TestConnectAll_CallTool(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, echoTool())

	cfg := makeConfig("srv", ts.URL)
	tools, cleanup, err := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	require.NoError(t, err)
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

	tools, cleanup, err := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	require.NoError(t, err)
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

	tools, cleanup, err := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	require.Len(t, tools, 1)
	assert.Equal(t, "filtered__echo", tools[0].Info().Name)
}

func TestConnectAll_ConnectionFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	cfg := makeConfig("bad", "http://127.0.0.1:0/does-not-exist")

	tools, cleanup, err := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	require.NoError(t, err, "ConnectAll should not return an error for unreachable servers")
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

	tools, cleanup, err := mcpclient.ConnectAll(
		ctx, logger,
		[]database.MCPServerConfig{cfg1, cfg2},
		nil,
	)
	require.NoError(t, err)
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

	tools, cleanup, err := mcpclient.ConnectAll(
		ctx, logger,
		[]database.MCPServerConfig{cfg},
		[]database.MCPServerUserToken{token},
	)
	require.NoError(t, err)
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

	tools, cleanup, err := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	require.NoError(t, err)
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
	tools, cleanup, err := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	require.NoError(t, err)
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
	tools, cleanup, err := mcpclient.ConnectAll(ctx, logger, []database.MCPServerConfig{cfg}, nil)
	require.NoError(t, err)
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

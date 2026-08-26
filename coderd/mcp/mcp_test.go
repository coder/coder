package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/aisdk-go"
	mcpserver "github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestMCPServer_Creation(t *testing.T) {
	t.Parallel()

	logger := testutil.Logger(t)

	server, err := mcpserver.NewServer(logger)
	require.NoError(t, err)
	require.NotNil(t, server)
}

func TestMCPServer_Handler(t *testing.T) {
	t.Parallel()

	logger := testutil.Logger(t)

	server, err := mcpserver.NewServer(logger)
	require.NoError(t, err)

	// Test that server implements http.Handler interface
	var handler http.Handler = server
	require.NotNil(t, handler)
}

func TestMCPHTTP_InitializeRequest(t *testing.T) {
	t.Parallel()

	logger := testutil.Logger(t)

	server, err := mcpserver.NewServer(logger)
	require.NoError(t, err)

	// Use server directly as http.Handler
	handler := server

	// Create initialize request
	initRequest := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	}

	body, err := json.Marshal(initRequest)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json,text/event-stream")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Logf("Response body: %s", recorder.Body.String())
	}
	assert.Equal(t, http.StatusOK, recorder.Code)

	sessionID := recorder.Header().Get("Mcp-Session-Id")
	assert.Empty(t, sessionID)

	// Parse response
	var response map[string]any
	err = json.Unmarshal(recorder.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "2.0", response["jsonrpc"])
	assert.Equal(t, float64(1), response["id"])

	result, ok := response["result"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "2025-06-18", result["protocolVersion"])
	assert.Contains(t, result, "capabilities")
	assert.Contains(t, result, "serverInfo")
}

func TestMCPHTTP_ToolRegistration(t *testing.T) {
	t.Parallel()

	logger := testutil.Logger(t)

	server, err := mcpserver.NewServer(logger)
	require.NoError(t, err)

	// Test registering tools with nil client should return error
	err = server.RegisterTools(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "client cannot be nil", "Should reject nil client with appropriate error message")

	// Test registering tools with valid client should succeed
	client := codersdk.New(testutil.MustURL(t, "http://not-used"))
	err = server.RegisterTools(client)
	require.NoError(t, err)

	// Verify that all expected tools are available in the toolsdk
	expectedToolCount := len(toolsdk.All)
	require.Greater(t, expectedToolCount, 0, "Should have some tools available")

	// Verify specific tools are present by checking tool names
	toolNames := make([]string, len(toolsdk.All))
	for i, tool := range toolsdk.All {
		toolNames[i] = tool.Name
	}
	require.Contains(t, toolNames, toolsdk.ToolNameReportTask, "Should include ReportTask (UserClientOptional)")
	require.Contains(t, toolNames, toolsdk.ToolNameGetAuthenticatedUser, "Should include GetAuthenticatedUser (requires auth)")
}

func TestMCPHTTP_ModernProtocol(t *testing.T) {
	t.Parallel()

	logger := testutil.Logger(t)

	server, err := mcpserver.NewServer(logger)
	require.NoError(t, err)
	client := codersdk.New(testutil.MustURL(t, "http://not-used"))
	err = server.RegisterTools(client)
	require.NoError(t, err)

	ts := httptest.NewServer(server)
	defer ts.Close()

	ctx := testutil.Context(t, testutil.WaitShort)
	mcpClient := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := mcpClient.Connect(ctx, &sdkmcp.StreamableClientTransport{
		Endpoint: ts.URL,
	}, nil)
	require.NoError(t, err)
	defer session.Close()

	init := session.InitializeResult()
	require.Equal(t, "2026-07-28", init.ProtocolVersion)
	require.Equal(t, mcpserver.MCPServerName, init.ServerInfo.Name)
	require.Equal(t, mcpserver.MCPServerInstructions, init.Instructions)

	tools, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	require.NotEmpty(t, tools.Tools)
}

func TestMCPHTTP_ToolArgumentValidation(t *testing.T) {
	t.Parallel()

	server, err := mcpserver.NewServer(testutil.Logger(t))
	require.NoError(t, err)
	require.NoError(t, server.RegisterTools(codersdk.New(testutil.MustURL(t, "http://not-used"))))

	ts := httptest.NewServer(server)
	t.Cleanup(ts.Close)

	connectCtx := testutil.Context(t, testutil.WaitShort)
	mcpClient := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := mcpClient.Connect(connectCtx, &sdkmcp.StreamableClientTransport{Endpoint: ts.URL}, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, session.Close())
	})

	tests := []struct {
		name      string
		arguments map[string]any
		keyword   string
		property  string
	}{
		{
			name: "MissingRequiredProperty",
			arguments: map[string]any{
				"user":            "me",
				"rich_parameters": map[string]any{},
			},
			keyword:  "required:",
			property: "name",
		},
		{
			name: "IncorrectPropertyType",
			arguments: map[string]any{
				"user":            false,
				"name":            "example",
				"rich_parameters": map[string]any{},
			},
			keyword:  "type:",
			property: "user",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)

			result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
				Name:      toolsdk.ToolNameCreateWorkspace,
				Arguments: test.arguments,
			})
			require.NoError(t, err)
			require.True(t, result.IsError)
			require.Len(t, result.Content, 1)
			content, ok := result.Content[0].(*sdkmcp.TextContent)
			require.True(t, ok, "expected TextContent, got %T", result.Content[0])
			var payload struct {
				Error          string         `json:"error"`
				ExpectedSchema map[string]any `json:"expectedSchema"`
			}
			require.NoError(t, json.Unmarshal([]byte(content.Text), &payload))
			require.Contains(t, payload.Error, "invalid arguments: $:")
			require.Contains(t, payload.Error, test.keyword)
			require.Contains(t, payload.Error, test.property)
			require.Equal(t, "object", payload.ExpectedSchema["type"])
			require.Equal(t, false, payload.ExpectedSchema["additionalProperties"])
			properties, ok := payload.ExpectedSchema["properties"].(map[string]any)
			require.True(t, ok)
			require.Contains(t, properties, test.property)
		})
	}
}

func TestRegisterSDKTool(t *testing.T) {
	t.Parallel()

	type arguments struct {
		Fail  bool   `json:"fail"`
		Value string `json:"value"`
	}
	type response struct {
		Value string `json:"value"`
	}
	tool := toolsdk.Tool[arguments, response]{
		Tool: aisdk.Tool{
			Name: "test_typed_tool",
			Schema: aisdk.Schema{
				Properties: map[string]any{
					"fail":  map[string]any{"type": "boolean"},
					"value": map[string]any{"type": "string"},
				},
				Required: []string{"value"},
			},
		},
		Handler: func(_ context.Context, _ toolsdk.Deps, args arguments) (response, error) {
			if args.Fail {
				return response{}, assert.AnError
			}
			return response{Value: args.Value}, nil
		},
	}.Generic()

	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	mcpserver.RegisterSDKTool(server, tool, toolsdk.Deps{})
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	ctx := testutil.Context(t, testutil.WaitShort)
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, serverSession.Close())
	})
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, clientSession.Close())
	})

	result, err := clientSession.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      tool.Name,
		Arguments: map[string]any{"value": "hello"},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	content, ok := result.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok)
	require.JSONEq(t, `{"value":"hello"}`, content.Text)

	_, err = clientSession.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      tool.Name,
		Arguments: map[string]any{"fail": true, "value": "hello"},
	})
	require.ErrorContains(t, err, assert.AnError.Error())
}

func TestMCPHTTP_UnsupportedProtocolVersion(t *testing.T) {
	t.Parallel()

	logger := testutil.Logger(t)

	server, err := mcpserver.NewServer(logger)
	require.NoError(t, err)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{` +
		`"io.modelcontextprotocol/protocolVersion":"2099-01-01",` +
		`"io.modelcontextprotocol/clientInfo":{"name":"test","version":"1.0"},` +
		`"io.modelcontextprotocol/clientCapabilities":{}}}}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json,text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2099-01-01")
	req.Header.Set("Mcp-Method", "tools/list")

	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, req)

	var response struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response), "body: %s", recorder.Body.String())
	require.Equal(t, -32022, response.Error.Code)
}

func TestMCPHTTP_TransportMethods(t *testing.T) {
	t.Parallel()

	logger := testutil.Logger(t)

	server, err := mcpserver.NewServer(logger)
	require.NoError(t, err)

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		req := httptest.NewRequest(method, "/", nil)
		if method == http.MethodGet {
			req.Header.Set("Accept", "text/event-stream")
		}
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)
		require.Equal(t, http.StatusMethodNotAllowed, recorder.Code, "method %s", method)
	}
}

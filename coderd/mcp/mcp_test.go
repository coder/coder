package mcp_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
			"protocolVersion": mcp.LATEST_PROTOCOL_VERSION,
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

	assert.Equal(t, mcp.LATEST_PROTOCOL_VERSION, result["protocolVersion"])
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

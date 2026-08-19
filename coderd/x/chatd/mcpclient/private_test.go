package mcpclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
)

type countingRoundTripper struct {
	base  http.RoundTripper
	calls atomic.Int64
}

func (r *countingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r.calls.Add(1)
	return r.base.RoundTrip(req)
}

func TestConnectPrivate(t *testing.T) {
	t.Parallel()

	const (
		headerName  = "X-Private-Canary"
		headerValue = "private-canary-value-12345"
	)
	ctx := t.Context()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	server := newTestMCPServer(t, testTool{
		tool: &mcp.Tool{
			Name:        "private_echo",
			Description: "description contains " + headerName + " and " + headerValue,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input": map[string]any{
						"type":        "string",
						"description": "schema contains " + headerValue,
					},
				},
			},
		},
		handler: func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			require.Equal(t, headerValue, req.Extra.Header.Get(headerName))
			return textToolResult("result contains " + headerName + " " + headerValue), nil
		},
	})

	customHeaders, err := json.Marshal(map[string]string{headerName: headerValue})
	require.NoError(t, err)
	configID := uuid.New()
	cfg := database.MCPServerConfig{
		ID:            configID,
		DisplayName:   "private",
		Slug:          "private",
		Url:           server.URL,
		Transport:     "streamable_http",
		AuthType:      "custom_headers",
		CustomHeaders: string(customHeaders),
		Enabled:       true,
	}
	transport := &countingRoundTripper{base: http.DefaultTransport}
	httpClient := &http.Client{Transport: transport}
	sensitiveValues := map[uuid.UUID][]string{
		configID: {server.URL, headerName, headerValue},
	}

	tools, cleanup := mcpclient.ConnectPrivate(ctx, logger, []database.MCPServerConfig{cfg}, httpClient, sensitiveValues)
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)
	require.Greater(t, transport.calls.Load(), int64(0), "private MCP must use the supplied guarded HTTP client")
	_, registered := tools[0].(mcpclient.MCPToolIdentifier)
	require.False(t, registered, "private MCP tools must not look like shared MCP registrations")

	infoJSON, err := json.Marshal(tools[0].Info())
	require.NoError(t, err)
	for _, secret := range sensitiveValues[configID] {
		require.NotContains(t, string(infoJSON), secret)
	}

	response, err := tools[0].Run(ctx, fantasy.ToolCall{
		ID:    "private-call",
		Name:  "private__private_echo",
		Input: `{"input":"hello"}`,
	})
	require.NoError(t, err)
	for _, secret := range sensitiveValues[configID] {
		require.NotContains(t, response.Content, secret)
		require.NotContains(t, response.Metadata, secret)
		require.NotContains(t, response.MediaType, secret)
	}
}

func TestConnectPrivateDoesNotForwardHeadersAcrossOrigins(t *testing.T) {
	t.Parallel()

	const (
		headerName  = "X-Private-Canary"
		headerValue = "private-canary-value-12345"
	)
	ctx := t.Context()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	var leaked atomic.Bool
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "redirect-target", Version: "1.0.0"}, nil)
	mcpServer.AddTool(&mcp.Tool{
		Name:        "ping",
		Description: "ping",
		InputSchema: map[string]any{"type": "object"},
	}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return textToolResult("ok"), nil
	})
	targetHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return mcpServer },
		&mcp.StreamableHTTPOptions{Stateless: true},
	)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(headerName) != "" {
			leaked.Store(true)
		}
		targetHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(target.Close)

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirect.Close)

	customHeaders, err := json.Marshal(map[string]string{headerName: headerValue})
	require.NoError(t, err)
	configID := uuid.New()
	cfg := database.MCPServerConfig{
		ID:            configID,
		DisplayName:   "redirect",
		Slug:          "redirect",
		Url:           redirect.URL,
		Transport:     "streamable_http",
		AuthType:      "custom_headers",
		CustomHeaders: string(customHeaders),
		Enabled:       true,
	}
	tools, cleanup := mcpclient.ConnectPrivate(
		ctx,
		logger,
		[]database.MCPServerConfig{cfg},
		http.DefaultClient,
		map[uuid.UUID][]string{configID: {redirect.URL, headerName, headerValue}},
	)
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)
	require.False(t, leaked.Load(), "private headers must not be sent to a redirected origin")
}

package mcpclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/testutil"
)

// newHeaderRecordingServer creates a streamable HTTP MCP server with a
// single "ping" tool. Every request's headers are appended to the
// returned slice so tests can assert which headers were forwarded.
func newHeaderRecordingServer(t *testing.T) (*httptest.Server, *sync.Mutex, *[]http.Header) {
	t.Helper()
	var (
		mu      sync.Mutex
		headers []http.Header
	)
	srv := mcpserver.NewMCPServer("hdr-server", "1.0.0")
	srv.AddTools(mcpserver.ServerTool{
		Tool: mcp.NewTool("ping", mcp.WithDescription("records the request headers")),
		Handler: func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			mu.Lock()
			headers = append(headers, req.Header.Clone())
			mu.Unlock()
			return mcp.NewToolResultText("ok"), nil
		},
	})
	httpSrv := mcpserver.NewStreamableHTTPServer(srv)
	ts := httptest.NewServer(httpSrv)
	t.Cleanup(ts.Close)
	return ts, &mu, &headers
}

// TestConnectAll_ForwardCoderHeaders_DefaultOff is a regression guard
// that the Coder identity headers are NOT sent when the option is
// left at its default (false).
func TestConnectAll_ForwardCoderHeaders_DefaultOff(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	logger := testutil.Logger(t, testutil.WithIgnoreErrors())

	ts, mu, recorded := newHeaderRecordingServer(t)

	cfg := makeConfig("no-hdr", ts.URL)
	assert.False(t, cfg.ForwardCoderHeaders, "default must be false")

	coderHeaders := map[string]string{
		chatprovider.HeaderCoderOwnerID:     uuid.NewString(),
		chatprovider.HeaderCoderChatID:      uuid.NewString(),
		chatprovider.HeaderCoderWorkspaceID: uuid.NewString(),
	}

	tools, cleanup := mcpclient.ConnectAll(
		ctx, logger, []database.MCPServerConfig{cfg}, nil, uuid.Nil, nil,
		coderHeaders,
	)
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)

	_, err := tools[0].Run(ctx, fantasy.ToolCall{
		ID: "call-1", Name: "no-hdr__ping", Input: "{}",
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, *recorded)
	for _, h := range *recorded {
		assert.Empty(t, h.Get(chatprovider.HeaderCoderOwnerID))
		assert.Empty(t, h.Get(chatprovider.HeaderCoderChatID))
		assert.Empty(t, h.Get(chatprovider.HeaderCoderSubchatID))
		assert.Empty(t, h.Get(chatprovider.HeaderCoderWorkspaceID))
	}
}

// TestConnectAll_ForwardCoderHeaders_Enabled verifies that when the
// option is enabled, the Coder identity headers are forwarded on every
// outgoing MCP request, including the subchat and workspace headers.
func TestConnectAll_ForwardCoderHeaders_Enabled(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	logger := testutil.Logger(t, testutil.WithIgnoreErrors())

	ts, mu, recorded := newHeaderRecordingServer(t)

	ownerID := uuid.New()
	chatID := uuid.New()
	workspaceID := uuid.New()
	subchatID := uuid.New()

	cfg := makeConfig("hdr", ts.URL)
	cfg.ForwardCoderHeaders = true

	// Subchat headers: parent's chat ID lives in X-Coder-Chat-Id, the
	// subchat's own ID lives in X-Coder-Subchat-Id.
	coderHeaders := chatprovider.CoderHeaders(database.Chat{
		ID:           subchatID,
		OwnerID:      ownerID,
		ParentChatID: uuid.NullUUID{UUID: chatID, Valid: true},
		WorkspaceID:  uuid.NullUUID{UUID: workspaceID, Valid: true},
	})

	tools, cleanup := mcpclient.ConnectAll(
		ctx, logger, []database.MCPServerConfig{cfg}, nil, uuid.Nil, nil,
		coderHeaders,
	)
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)

	_, err := tools[0].Run(ctx, fantasy.ToolCall{
		ID: "call-1", Name: "hdr__ping", Input: "{}",
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, *recorded)
	last := (*recorded)[len(*recorded)-1]
	assert.Equal(t, ownerID.String(), last.Get(chatprovider.HeaderCoderOwnerID))
	assert.Equal(t, chatID.String(), last.Get(chatprovider.HeaderCoderChatID))
	assert.Equal(t, subchatID.String(), last.Get(chatprovider.HeaderCoderSubchatID))
	assert.Equal(t, workspaceID.String(), last.Get(chatprovider.HeaderCoderWorkspaceID))
}

// TestConnectAll_ForwardCoderHeaders_RootChat verifies that for a root
// chat (no parent), the chat's own ID is forwarded as
// X-Coder-Chat-Id and the X-Coder-Subchat-Id header is absent.
func TestConnectAll_ForwardCoderHeaders_RootChat(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	logger := testutil.Logger(t, testutil.WithIgnoreErrors())

	ts, mu, recorded := newHeaderRecordingServer(t)

	ownerID := uuid.New()
	chatID := uuid.New()

	cfg := makeConfig("hdr-root", ts.URL)
	cfg.ForwardCoderHeaders = true

	coderHeaders := chatprovider.CoderHeaders(database.Chat{
		ID:      chatID,
		OwnerID: ownerID,
	})

	tools, cleanup := mcpclient.ConnectAll(
		ctx, logger, []database.MCPServerConfig{cfg}, nil, uuid.Nil, nil,
		coderHeaders,
	)
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)

	_, err := tools[0].Run(ctx, fantasy.ToolCall{
		ID: "call-1", Name: "hdr-root__ping", Input: "{}",
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, *recorded)
	last := (*recorded)[len(*recorded)-1]
	assert.Equal(t, ownerID.String(), last.Get(chatprovider.HeaderCoderOwnerID))
	assert.Equal(t, chatID.String(), last.Get(chatprovider.HeaderCoderChatID))
	assert.Empty(t, last.Get(chatprovider.HeaderCoderSubchatID))
	assert.Empty(t, last.Get(chatprovider.HeaderCoderWorkspaceID))
}

// TestConnectAll_ForwardCoderHeaders_WithAPIKeyAuth verifies that the
// api_key auth header is preserved when Coder identity headers are
// forwarded alongside.
func TestConnectAll_ForwardCoderHeaders_WithAPIKeyAuth(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	logger := testutil.Logger(t, testutil.WithIgnoreErrors())

	ts, mu, recorded := newHeaderRecordingServer(t)

	ownerID := uuid.New()
	chatID := uuid.New()

	cfg := makeConfig("hdr-apikey", ts.URL)
	cfg.AuthType = "api_key"
	cfg.APIKeyHeader = "X-Api-Key"
	cfg.APIKeyValue = "sekret"
	cfg.ForwardCoderHeaders = true

	coderHeaders := chatprovider.CoderHeaders(database.Chat{
		ID:      chatID,
		OwnerID: ownerID,
	})

	tools, cleanup := mcpclient.ConnectAll(
		ctx, logger, []database.MCPServerConfig{cfg}, nil, uuid.Nil, nil,
		coderHeaders,
	)
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)

	_, err := tools[0].Run(ctx, fantasy.ToolCall{
		ID: "call-1", Name: "hdr-apikey__ping", Input: "{}",
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, *recorded)
	last := (*recorded)[len(*recorded)-1]
	assert.Equal(t, "sekret", last.Get("X-Api-Key"))
	assert.Equal(t, ownerID.String(), last.Get(chatprovider.HeaderCoderOwnerID))
	assert.Equal(t, chatID.String(), last.Get(chatprovider.HeaderCoderChatID))
}

// TestConnectAll_ForwardCoderHeaders_WithOAuth2 verifies that the
// oauth2 Authorization header is preserved when Coder identity
// headers are forwarded alongside, and that auth wins on a conflict.
func TestConnectAll_ForwardCoderHeaders_WithOAuth2(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	logger := testutil.Logger(t, testutil.WithIgnoreErrors())

	ts, mu, recorded := newHeaderRecordingServer(t)

	cfgID := uuid.New()
	cfg := makeConfig("hdr-oauth", ts.URL)
	cfg.ID = cfgID
	cfg.AuthType = "oauth2"
	cfg.ForwardCoderHeaders = true
	token := database.MCPServerUserToken{
		MCPServerConfigID: cfgID,
		AccessToken:       "oauth-token-xyz",
		TokenType:         "Bearer",
	}

	// Intentionally include an Authorization key to verify the auth
	// header wins on conflict.
	ownerID := uuid.NewString()
	coderHeaders := map[string]string{
		"Authorization":                 "Bearer should-be-overridden",
		chatprovider.HeaderCoderOwnerID: ownerID,
	}

	tools, cleanup := mcpclient.ConnectAll(
		ctx, logger,
		[]database.MCPServerConfig{cfg},
		[]database.MCPServerUserToken{token},
		uuid.Nil, nil,
		coderHeaders,
	)
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)

	_, err := tools[0].Run(ctx, fantasy.ToolCall{
		ID: "call-1", Name: "hdr-oauth__ping", Input: "{}",
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, *recorded)
	last := (*recorded)[len(*recorded)-1]
	assert.Equal(t, "Bearer oauth-token-xyz", last.Get("Authorization"))
	assert.Equal(t, ownerID, last.Get(chatprovider.HeaderCoderOwnerID))
}

// TestConnectAll_ForwardCoderHeaders_WithCustomHeaders verifies that
// custom_headers admin-configured values are preserved when Coder
// identity headers are forwarded alongside, including the case where
// the admin configures a custom header whose name only differs from a
// Coder identity header by case. Conflict detection is case-
// insensitive because http.Header.Set canonicalizes header names.
func TestConnectAll_ForwardCoderHeaders_WithCustomHeaders(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	logger := testutil.Logger(t, testutil.WithIgnoreErrors())

	ts, mu, recorded := newHeaderRecordingServer(t)

	ownerID := uuid.New()
	chatID := uuid.New()

	cfg := makeConfig("hdr-custom", ts.URL)
	cfg.AuthType = "custom_headers"
	// Include both an unrelated custom header AND a case-variant of
	// X-Coder-Owner-Id to exercise the case-insensitive conflict
	// check. The admin-configured value MUST win.
	cfg.CustomHeaders = `{"X-Tenant":"acme","x-coder-owner-id":"admin-controlled"}`
	cfg.ForwardCoderHeaders = true

	coderHeaders := chatprovider.CoderHeaders(database.Chat{
		ID:      chatID,
		OwnerID: ownerID,
	})

	tools, cleanup := mcpclient.ConnectAll(
		ctx, logger, []database.MCPServerConfig{cfg}, nil, uuid.Nil, nil,
		coderHeaders,
	)
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)

	_, err := tools[0].Run(ctx, fantasy.ToolCall{
		ID: "call-1", Name: "hdr-custom__ping", Input: "{}",
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, *recorded)
	last := (*recorded)[len(*recorded)-1]
	assert.Equal(t, "acme", last.Get("X-Tenant"))
	// The admin's case-variant header must win, because HTTP header
	// names are case-insensitive at the transport level.
	assert.Equal(t, "admin-controlled", last.Get(chatprovider.HeaderCoderOwnerID))
	assert.Equal(t, chatID.String(), last.Get(chatprovider.HeaderCoderChatID))
}

package mcpclient_test

import (
	"context"
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

func TestServerFromConfig_UserAPIKey(t *testing.T) {
	t.Parallel()

	srv, err := mcpclient.ServerFromConfig(database.MCPServerConfig{
		ID:           uuid.New(),
		Slug:         "github",
		Url:          "https://mcp.example.com",
		AuthType:     "user_api_key",
		APIKeyHeader: "X-Api-Key",
		APIKeyPrefix: "Token ",
		// A shared value is never sent for user_api_key servers.
		APIKeyValue: "shared",
	})
	require.NoError(t, err)
	require.Equal(t, mcpclient.UserAuthAPIKey, srv.UserAuth)
	require.Equal(t, "X-Api-Key", srv.UserTokenHeader)
	require.Equal(t, "Token ", srv.UserTokenPrefix)
	require.Empty(t, srv.Headers)
}

// TestConnectAll_UserAPIKey verifies that a user_api_key server receives
// only the chat owner's own token, and is not contacted at all when the
// owner has none, even when another user's token is passed in.
func TestConnectAll_UserAPIKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	ts := newTestMCPServer(t, testTool{
		tool: &mcp.Tool{
			Name:        "whoami",
			Description: "Returns the auth header",
			InputSchema: map[string]any{"type": "object"},
		},
		handler: func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return textToolResult("auth:" + req.Extra.Header.Get("X-Api-Key")), nil
		},
	})
	cfg := makeConfig("keyed", ts.URL)
	cfg.UserAuth = mcpclient.UserAuthAPIKey
	cfg.UserTokenHeader = "X-Api-Key"
	cfg.UserTokenPrefix = "Token "

	owner, other := uuid.New(), uuid.New()
	ownerToken := database.MCPServerUserToken{MCPServerConfigID: cfg.ID, UserID: owner, AccessToken: "owner-token"}
	otherToken := database.MCPServerUserToken{MCPServerConfigID: cfg.ID, UserID: other, AccessToken: "other-token"}

	tools, summaries, cleanup := mcpclient.ConnectAll(ctx, logger, []mcpclient.Server{cfg},
		[]database.MCPServerUserToken{ownerToken}, owner, nil, nil, testMCPHTTPClient(nil))
	t.Cleanup(cleanup)
	require.Len(t, tools, 1)
	require.Equal(t, mcpclient.ConnectOutcomeConnected, summaries[0].Outcome)
	resp, err := tools[0].Run(ctx, fantasy.ToolCall{ID: "call", Name: "keyed__whoami", Input: "{}"})
	require.NoError(t, err)
	require.Equal(t, "auth:Token owner-token", resp.Content)

	// Without the owner's own token the server is never contacted.
	var hits atomic.Int64
	unreached := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		rw.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(unreached.Close)
	missing := makeConfig("missing", unreached.URL)
	missing.UserAuth = mcpclient.UserAuthAPIKey
	missing.UserTokenHeader = "Authorization"
	otherForMissing := otherToken
	otherForMissing.MCPServerConfigID = missing.ID

	tools, summaries, cleanup = mcpclient.ConnectAll(ctx, logger, []mcpclient.Server{missing},
		[]database.MCPServerUserToken{otherForMissing}, owner, nil, nil, testMCPHTTPClient(nil))
	t.Cleanup(cleanup)
	require.Empty(t, tools)
	require.Len(t, summaries, 1)
	require.Equal(t, mcpclient.ConnectOutcomeMissingToken, summaries[0].Outcome)
	require.Zero(t, hits.Load())
}

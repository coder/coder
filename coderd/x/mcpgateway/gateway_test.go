package mcpgateway_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/x/mcpgateway"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/safedial"
)

// TestGateway proves the gateway aggregates the upstream MCP servers a user
// may read, hides tools on the deny list, and hides servers the user's ACL
// does not grant.
func TestGateway(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	db, _ := dbtestutil.NewDB(t)

	org := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{})
	other := dbgen.User(t, db, database.User{})
	for _, id := range []uuid.UUID{owner.ID, other.ID} {
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			OrganizationID: org.ID,
			UserID:         id,
		})
	}

	upstreamA := newTestMCPServer(t, textTool("echo"), textTool("secret"))
	upstreamB := newTestMCPServer(t, textTool("x"))

	dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		OrganizationID: org.ID,
		Slug:           "a",
		Url:            upstreamA.URL,
		Enabled:        true,
		ToolDenyList:   []string{"secret"},
		CreatedBy:      uuid.NullUUID{UUID: owner.ID, Valid: true},
	})
	// Only "other" may read this server.
	dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
		OrganizationID: org.ID,
		Slug:           "b",
		Url:            upstreamB.URL,
		Enabled:        true,
		GroupACL:       database.ChatACL{},
		UserACL: database.ChatACL{
			other.ID.String(): {Permissions: []policy.Action{policy.ActionRead}},
		},
		CreatedBy: uuid.NullUUID{UUID: owner.ID, Valid: true},
	})

	authzDB := dbauthz.New(
		db,
		rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()),
		logger,
		coderdtest.AccessControlStorePointer(),
	)
	gateway := mcpgateway.New(mcpgateway.Options{
		Logger:     logger,
		Database:   authzDB,
		HTTPClient: testHTTPClient(),
	})

	t.Run("Owner", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		session := connect(ctx, t, db, authzDB, gateway, org, owner.ID)

		tools, err := session.ListTools(ctx, nil)
		require.NoError(t, err)
		require.Equal(t, []string{"a__echo"}, toolNames(tools))

		res, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "a__echo",
			Arguments: map[string]any{"input": "hi"},
		})
		require.NoError(t, err)
		require.False(t, res.IsError)
		require.Equal(t, "echo: hi", textContent(t, res))

		// Denied tool and ACL-hidden server must not be callable.
		for _, name := range []string{"a__secret", "b__x"} {
			res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name})
			if err == nil {
				require.True(t, res.IsError, "expected %s to fail", name)
			}
		}
	})

	t.Run("Other", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		session := connect(ctx, t, db, authzDB, gateway, org, other.ID)

		tools, err := session.ListTools(ctx, nil)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"a__echo", "b__x"}, toolNames(tools))

		res, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "b__x",
			Arguments: map[string]any{"input": "yo"},
		})
		require.NoError(t, err)
		require.False(t, res.IsError)
		require.Equal(t, "x: yo", textContent(t, res))
	})
}

// connect serves the gateway over HTTP with the request context carrying
// userID's actor and the organization param, then dials it with an MCP client.
func connect(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	authzDB database.Store,
	gateway *mcpgateway.Gateway,
	org database.Organization,
	userID uuid.UUID,
) *mcp.ClientSession {
	t.Helper()

	subject, _, err := httpmw.UserRBACSubject(
		dbauthz.AsSystemRestricted(ctx), db, userID, rbac.ScopeAll,
	)
	require.NoError(t, err)

	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(dbauthz.As(r.Context(), subject)))
		})
	})
	router.Route("/organizations/{organization}", func(r chi.Router) {
		r.Use(httpmw.ExtractOrganizationParam(authzDB))
		r.Mount("/mcp-gateway", gateway.Handler())
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   srv.URL + "/organizations/" + org.ID.String() + "/mcp-gateway",
		HTTPClient: srv.Client(),
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func newTestMCPServer(t *testing.T, tools ...toolPair) *httptest.Server {
	t.Helper()
	srv := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	for _, tool := range tools {
		srv.AddTool(tool.tool, tool.handler)
	}
	ts := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true},
	))
	t.Cleanup(ts.Close)
	return ts
}

type toolPair struct {
	tool    *mcp.Tool
	handler mcp.ToolHandler
}

// textTool returns a tool that echoes "<name>: <input>".
func textTool(name string) toolPair {
	return toolPair{
		tool: &mcp.Tool{
			Name:        name,
			Description: name + " tool",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input": map[string]any{"type": "string"},
				},
			},
		},
		handler: func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var args map[string]any
			if len(req.Params.Arguments) > 0 {
				if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
					return nil, err
				}
			}
			input, _ := args["input"].(string)
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: name + ": " + input}},
			}, nil
		},
	}
}

func toolNames(res *mcp.ListToolsResult) []string {
	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func textContent(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	require.Len(t, res.Content, 1)
	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok, "expected text content, got %T", res.Content[0])
	return text.Text
}

// testHTTPClient allows loopback upstreams, which the production SSRF-safe
// client blocks.
func testHTTPClient() *http.Client {
	return safedial.NewHTTPClient(nil, safedial.WithAllowedPrefixes(
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("::1/128"),
	))
}

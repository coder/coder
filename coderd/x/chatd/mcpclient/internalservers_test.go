package mcpclient_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"charm.land/fantasy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/testutil"
)

func TestInternalServersRegister(t *testing.T) {
	t.Parallel()

	servers := mcpclient.NewInternalServers()
	require.Equal(t, "coder-internal://slack", servers.Register("slack", http.NotFoundHandler()))
	require.Panics(t, func() { servers.Register("slack", http.NotFoundHandler()) }, "duplicate host")
	require.Panics(t, func() { servers.Register("Slack", http.NotFoundHandler()) }, "invalid host")
}

func TestConnectInline_InternalServer(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		jsonResponse bool
	}{
		{name: "JSON", jsonResponse: true},
		{name: "SSE", jsonResponse: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

			srv := mcp.NewServer(&mcp.Implementation{Name: "internal", Version: "1.0.0"}, nil)
			echo := echoTool()
			srv.AddTool(echo.tool, echo.handler)
			servers := mcpclient.NewInternalServers()
			serverURL := servers.Register("bot", mcp.NewStreamableHTTPHandler(
				func(*http.Request) *mcp.Server { return srv },
				&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: tc.jsonResponse},
			))

			tools, summaries, cleanup := mcpclient.ConnectInline(
				ctx, logger, []mcpclient.Server{makeConfig("bot", serverURL)}, nil,
				mcpclient.NewHTTPClient(nil), servers,
			)
			t.Cleanup(cleanup)
			require.Len(t, summaries, 1)
			require.Equal(t, mcpclient.ConnectOutcomeConnected, summaries[0].Outcome, summaries[0].Error)
			require.Len(t, tools, 1)
			resp, err := tools[0].Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "bot__echo", Input: `{"input":"hi"}`})
			require.NoError(t, err)
			require.Equal(t, "echo: hi", resp.Content)
		})
	}
}

func TestConnectInline_UnregisteredInternalServer(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	tools, summaries, cleanup := mcpclient.ConnectInline(
		ctx, logger, []mcpclient.Server{makeConfig("bot", "coder-internal://missing")}, nil,
		mcpclient.NewHTTPClient(nil), mcpclient.NewInternalServers(),
	)
	t.Cleanup(cleanup)
	require.Empty(t, tools)
	require.Len(t, summaries, 1)
	require.Equal(t, mcpclient.ConnectOutcomeError, summaries[0].Outcome)
	require.Contains(t, summaries[0].Error, "no internal MCP server registered")
}

// A caller-supplied server must not reach an internal server by
// redirecting to its URL.
func TestConnectInline_RedirectCannotReachInternalServer(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	servers := mcpclient.NewInternalServers()
	serverURL := servers.Register("target", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the redirect reached the internal server")
	}))
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, serverURL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirector.Close)

	tools, summaries, cleanup := mcpclient.ConnectInline(
		ctx, logger, []mcpclient.Server{makeConfig("redirector", redirector.URL)}, nil,
		testMCPHTTPClient(nil), servers,
	)
	t.Cleanup(cleanup)
	require.Empty(t, tools)
	require.Len(t, summaries, 1)
	require.Contains(t, summaries[0].Error, "redirect must stay on origin")
}

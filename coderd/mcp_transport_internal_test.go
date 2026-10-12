package coderd

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

func TestMCPTransportAuthentication(t *testing.T) {
	t.Parallel()
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	app := dbgen.OAuth2ProviderApp(t, db, database.OAuth2ProviderApp{})
	origin, err := url.Parse("https://coder.example.com")
	require.NoError(t, err)
	handler := httpmw.PrecheckAPIKey(httpmw.ValidateAPIKeyConfig{DB: db})(
		httpmw.ExtractAPIKeyMW(httpmw.ExtractAPIKeyConfig{DB: db, AccessURL: origin, Logger: testutil.Logger(t)})(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, user.ID.String(), httpmw.UserAuthorization(r.Context()).ID)
				w.WriteHeader(http.StatusNoContent)
			})))

	for _, tc := range []struct {
		name       string
		audience   string
		status     int
		expired    bool
		deleted    bool
		wrongKeyID bool
	}{
		{name: "MCPAudience", audience: origin.String() + mcp.MCPEndpoint, status: http.StatusNoContent},
		{name: "TrailingSlash", audience: origin.String() + mcp.MCPEndpoint + "/", status: http.StatusNoContent},
		{name: "UnboundLegacyToken", status: http.StatusNoContent},
		{name: "RootAudience", audience: origin.String(), status: http.StatusForbidden},
		{name: "OtherDeployment", audience: "https://other.example.com" + mcp.MCPEndpoint, status: http.StatusForbidden},
		{name: "WrongKey", audience: origin.String() + mcp.MCPEndpoint, wrongKeyID: true, status: http.StatusUnauthorized},
		{name: "Expired", audience: origin.String() + mcp.MCPEndpoint, expired: true, status: http.StatusUnauthorized},
		{name: "Deleted", audience: origin.String() + mcp.MCPEndpoint, deleted: true, status: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			key, token := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID, LoginType: database.LoginTypeOAuth2ProviderApp})
			dbgen.OAuth2ProviderAppToken(t, db, database.OAuth2ProviderAppToken{
				AppID: app.ID, UserID: user.ID, APIKeyID: key.ID, HashPrefix: []byte(key.ID),
				Audience: sql.NullString{String: tc.audience, Valid: tc.audience != ""},
			})
			delegatedID := key.ID
			if tc.wrongKeyID {
				delegatedID = "other-key"
			}
			transport, closeTransport := newMCPDelegatedTransport(testutil.Logger(t), handler, origin, delegatedID)
			t.Cleanup(closeTransport)
			client := codersdk.New(origin, codersdk.WithSessionToken(token), codersdk.WithHTTPClient(&http.Client{Transport: transport}))

			if tc.expired || tc.deleted {
				// Validate once before mutation, then reuse the same private
				// transport. Internal calls must not cache authentication.
				resp, err := client.Request(ctx, http.MethodGet, "/api/v2/users/me", nil)
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
				require.Equal(t, http.StatusNoContent, resp.StatusCode)
				if tc.deleted {
					require.NoError(t, db.DeleteAPIKeyByID(ctx, key.ID))
				} else {
					require.NoError(t, db.UpdateAPIKeyByID(ctx, database.UpdateAPIKeyByIDParams{
						ID: key.ID, LastUsed: key.LastUsed, ExpiresAt: dbtime.Now().Add(-time.Hour), IPAddress: key.IPAddress,
					}))
				}
			}
			resp, err := client.Request(ctx, http.MethodGet, "/api/v2/users/me", nil)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, tc.status, resp.StatusCode)
		})
	}
}

func TestMCPTransportIsolation(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	origin, err := url.Parse("https://coder.example.com")
	require.NoError(t, err)
	type callerContextKey struct{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Nil(t, r.Context().Value(callerContextKey{}), "caller auth and routing state must not cross the connection")
		if r.URL.Path == "/api/v2/redirect" {
			http.Redirect(w, r, "https://other.example.com/api/v2/users/me", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	transport, closeTransport := newMCPDelegatedTransport(testutil.Logger(t), handler, origin, "test-key")
	t.Cleanup(closeTransport)
	client := &http.Client{Transport: transport}

	for _, target := range []string{
		"https://other.example.com/api/v2/users/me",
		"http://coder.example.com/api/v2/users/me",
		"https://coder.example.com/oauth2/tokens",
		"https://coder.example.com/api/experimental/mcp/http",
		"https://coder.example.com/api/experimental//mcp/http",
		"https://coder.example.com/api/v2/../../oauth2/tokens",
		"https://coder.example.com/api/v2/redirect",
	} {
		body := &mcpRequestBody{Reader: strings.NewReader("test")}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, body)
		require.NoError(t, err)
		resp, err := client.Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		require.Error(t, err, "untrusted destination must not be reachable")
		require.True(t, body.closed.Load(), "rejected request body must be closed")
	}
	req, err := http.NewRequestWithContext(context.WithValue(ctx, callerContextKey{}, "outer-auth"), http.MethodGet, origin.String()+"/api/v2/users/me", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	closeTransport()
	resp, err = client.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.Error(t, err, "transport must not outlive its MCP request")
}

type mcpRequestBody struct {
	io.Reader
	closed atomic.Bool
}

func (b *mcpRequestBody) Close() error {
	b.closed.Store(true)
	return nil
}

func TestMCPTransportStreaming(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	origin, err := url.Parse("https://coder.example.com")
	require.NoError(t, err)
	handlerDone := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		_, _ = io.WriteString(w, "first\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	transport, closeTransport := newMCPDelegatedTransport(testutil.Logger(t), handler, origin, "test-key")
	t.Cleanup(closeTransport)
	client := &http.Client{Transport: transport}
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, origin.String()+"/api/v2/logs", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	first := make([]byte, len("first\n"))
	_, err = io.ReadFull(resp.Body, first)
	require.NoError(t, err)
	require.Equal(t, "first\n", string(first))
	cancel()
	_, err = resp.Body.Read(first)
	require.Error(t, err)
	testutil.TryReceive(ctx, t, handlerDone)
}

func TestMCPTransportWebSocket(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	origin, err := url.Parse("https://coder.example.com")
	require.NoError(t, err)
	handlerDone := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		conn, err := websocket.Accept(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		defer conn.CloseNow()
		kind, data, err := conn.Read(ctx)
		if !assert.NoError(t, err) {
			return
		}
		assert.NoError(t, conn.Write(ctx, kind, data))
		_, _, err = conn.Read(ctx)
		assert.Error(t, err, "transport cleanup must close hijacked connections")
	})
	transport, closeTransport := newMCPDelegatedTransport(testutil.Logger(t), handler, origin, "test-key")
	t.Cleanup(closeTransport)
	conn, resp, err := websocket.Dial(ctx, "wss://coder.example.com/api/v2/chats/watch", &websocket.DialOptions{HTTPClient: &http.Client{Transport: transport}})
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	require.NoError(t, err)
	defer conn.CloseNow()
	require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte("ping")))
	_, data, err := conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, "ping", string(data))
	closeTransport()
	testutil.TryReceive(ctx, t, handlerDone)
}

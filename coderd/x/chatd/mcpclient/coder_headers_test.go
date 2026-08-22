package mcpclient_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
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
	ts := newTestMCPServer(t, testTool{
		tool: &mcp.Tool{
			Name:        "ping",
			Description: "records the request headers",
			InputSchema: map[string]any{"type": "object"},
		},
		handler: func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			mu.Lock()
			headers = append(headers, req.Extra.Header.Clone())
			mu.Unlock()
			return textToolResult("ok"), nil
		},
	})
	return ts, &mu, &headers
}

type recordedMCPRequest struct {
	method    string
	rpcMethod string
	err       error
}

func newSigningTestMCPServer(t *testing.T, signingSecret string) (*httptest.Server, <-chan recordedMCPRequest) {
	t.Helper()

	recorded := make(chan recordedMCPRequest, 16)
	handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err == nil {
			err = verifyMCPRequestSignature(r, body, signingSecret)
		}

		var rpcRequest struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if r.Method == http.MethodPost {
			if decodeErr := json.Unmarshal(body, &rpcRequest); err == nil && decodeErr != nil {
				err = decodeErr
			}
		}
		recorded <- recordedMCPRequest{method: r.Method, rpcMethod: rpcRequest.Method, err: err}
		if err != nil {
			http.Error(rw, err.Error(), http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case http.MethodGet:
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		case http.MethodPost:
		default:
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		rw.Header().Set("Content-Type", "application/json")
		switch rpcRequest.Method {
		case "server/discover":
			_, _ = fmt.Fprintf(rw, `{"jsonrpc":"2.0","id":%s,"error":{"code":-32601,"message":"method not found"}}`, rpcRequest.ID)
		case "initialize":
			rw.Header().Set("Mcp-Session-Id", "test-session")
			_, _ = fmt.Fprintf(rw, `{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-06-18","capabilities":{"tools":{}},"serverInfo":{"name":"test-server","version":"1.0.0"}}}`, rpcRequest.ID)
		case "notifications/initialized":
			rw.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_, _ = fmt.Fprintf(rw, `{"jsonrpc":"2.0","id":%s,"result":{"tools":[{"name":"ping","description":"test tool","inputSchema":{"type":"object"}}]}}`, rpcRequest.ID)
		case "tools/call":
			_, _ = fmt.Fprintf(rw, `{"jsonrpc":"2.0","id":%s,"result":{"content":[{"type":"text","text":"ok"}],"isError":false}}`, rpcRequest.ID)
		default:
			_, _ = fmt.Fprintf(rw, `{"jsonrpc":"2.0","id":%s,"error":{"code":-32601,"message":"method not found"}}`, rpcRequest.ID)
		}
	})

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server, recorded
}

func verifyMCPRequestSignature(r *http.Request, body []byte, signingSecret string) error {
	timestamp := r.Header.Get(mcpclient.HeaderCoderSignatureTimestamp)
	signature := r.Header.Get(mcpclient.HeaderCoderSignature)
	if signingSecret == "" {
		if timestamp != "" || signature != "" {
			return xerrors.New("unexpected MCP signature headers")
		}
		return nil
	}
	if timestamp == "" || signature == "" {
		return xerrors.New("missing MCP signature headers")
	}

	unixSeconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return xerrors.Errorf("parse MCP signature timestamp: %w", err)
	}
	if time.Since(time.Unix(unixSeconds, 0)).Abs() > 5*time.Minute {
		return xerrors.New("MCP signature timestamp is outside the allowed window")
	}

	bodyHash := sha256.Sum256(body)
	canonical := strings.Join([]string{
		"v1",
		timestamp,
		strings.ToUpper(r.Method),
		r.URL.RequestURI(),
		hex.EncodeToString(bodyHash[:]),
		"owner=" + r.Header.Get(chatprovider.HeaderCoderOwnerID),
		"chat=" + r.Header.Get(chatprovider.HeaderCoderChatID),
		"subchat=" + r.Header.Get(chatprovider.HeaderCoderSubchatID),
		"workspace=" + r.Header.Get(chatprovider.HeaderCoderWorkspaceID),
	}, "\n")
	mac := hmac.New(sha256.New, []byte(signingSecret))
	_, _ = mac.Write([]byte(canonical))
	expected := "v1=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return xerrors.New("invalid MCP request signature")
	}
	return nil
}

func TestConnectAll_RequestSigning(t *testing.T) {
	t.Parallel()

	const signingSecret = "0123456789abcdef0123456789abcdef"
	tests := []struct {
		name                  string
		forwardHeaders        bool
		signingSecret         string
		expectedSigningSecret string
	}{
		{name: "enabled", forwardHeaders: true, signingSecret: signingSecret, expectedSigningSecret: signingSecret},
		{name: "forwarding disabled", forwardHeaders: false, signingSecret: signingSecret},
		{name: "secret empty", forwardHeaders: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			server, recorded := newSigningTestMCPServer(t, tt.expectedSigningSecret)

			cfg := makeConfig("signed", server.URL+"/api/mcp?x=1")
			cfg.ForwardCoderHeaders = tt.forwardHeaders
			cfg.SigningSecret = tt.signingSecret
			coderHeaders := chatprovider.CoderHeaders(database.Chat{
				ID:      uuid.New(),
				OwnerID: uuid.New(),
			})

			tools, cleanup := mcpclient.ConnectAll(
				ctx, logger, []database.MCPServerConfig{cfg}, nil, uuid.Nil, nil,
				coderHeaders,
			)
			t.Cleanup(cleanup)
			require.Len(t, tools, 1)
			_, err := tools[0].Run(ctx, fantasy.ToolCall{
				ID: "call-1", Name: "signed__ping", Input: "{}",
			})
			require.NoError(t, err)

			expected := map[string]bool{
				"server/discover":           false,
				"initialize":                false,
				"notifications/initialized": false,
				"GET":                       false,
				"tools/list":                false,
				"tools/call":                false,
			}
			deadline := time.NewTimer(5 * time.Second)
			defer deadline.Stop()
			for {
				complete := true
				for _, seen := range expected {
					complete = complete && seen
				}
				if complete {
					break
				}

				select {
				case request := <-recorded:
					require.NoError(t, request.err)
					key := request.rpcMethod
					if request.method == http.MethodGet {
						key = http.MethodGet
					}
					_, ok := expected[key]
					require.Truef(t, ok, "unexpected MCP request %q", key)
					expected[key] = true
				case <-deadline.C:
					require.FailNow(t, "timed out waiting for MCP requests", "%v", expected)
				}
			}
		})
	}
}

// TestConnectAll_ForwardCoderHeaders_DefaultOff is a regression guard
// that the Coder identity headers are NOT sent when the option is
// left at its default (false).
func TestConnectAll_ForwardCoderHeaders_DefaultOff(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

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
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

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
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

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
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

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
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

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
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

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

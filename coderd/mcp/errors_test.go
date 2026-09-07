package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/aisdk-go"
	mcpserver "github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestRegisterSDKToolErrors(t *testing.T) {
	t.Parallel()
	const internalMessage = "An internal error occurred while running this tool. Check the Coder server logs for details."
	const secret = "private-error-sentinel"
	publicErr := &toolsdk.PublicError{Message: "Workspace must be started.", Cause: xerrors.New(secret)}
	apiErr := codersdk.NewTestError(http.StatusConflict, http.MethodPost, "https://coder.example/?token="+secret)
	apiErr.Message = "Workspace already exists."
	apiErr.Detail = secret
	apiErr.Helper = secret
	apiErr.Validations = []codersdk.ValidationError{{Field: "name", Detail: "Choose another name."}}
	apiMessage := "Coder API error (HTTP 409): Workspace already exists.\n- name: Choose another name."
	tests := []struct {
		name       string
		err        error
		panicValue any
		want       string
	}{
		{name: "Public", err: publicErr, want: publicErr.Message},
		{name: "WrappedPublic", err: xerrors.Errorf("%s: %w", secret, publicErr), want: publicErr.Message},
		{name: "EmptyPublic", err: &toolsdk.PublicError{Cause: xerrors.New(secret)}, want: internalMessage},
		{name: "API", err: apiErr, want: apiMessage},
		{name: "WrappedAPI", err: xerrors.Errorf("%s: %w", secret, apiErr), want: apiMessage},
		{name: "Canceled", err: xerrors.Errorf("%s: %w", secret, context.Canceled), want: "Tool execution was canceled."},
		{name: "Deadline", err: xerrors.Errorf("%s: %w", secret, context.DeadlineExceeded), want: "Tool execution timed out."},
		{name: "Internal", err: xerrors.New(secret), want: internalMessage},
		{name: "Panic", panicValue: secret, want: internalMessage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sink := testutil.NewFakeSink(t)
			tool := toolsdk.Tool[struct {
				Value string `json:"value"`
			}, string]{
				Tool: aisdk.Tool{Name: "test_error", Schema: aisdk.Schema{Properties: map[string]any{"value": map[string]any{"type": "string"}}}},
				Handler: func(context.Context, toolsdk.Deps, struct {
					Value string `json:"value"`
				}) (string, error) {
					if tt.panicValue != nil {
						panic(tt.panicValue)
					}
					return "", tt.err
				},
			}.Generic()
			server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test", Version: "1"}, nil)
			mcpserver.RegisterSDKTool(server, tool, toolsdk.Deps{}, sink.Logger())
			serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
			ctx := testutil.Context(t, testutil.WaitLong)
			serverSession, err := server.Connect(ctx, serverTransport, nil)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, serverSession.Close()) })
			client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "1"}, nil)
			session, err := client.Connect(ctx, clientTransport, nil)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, session.Close()) })

			result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: tool.Name, Arguments: map[string]any{"value": "private-argument-sentinel"}})
			require.NoError(t, err)
			require.True(t, result.IsError)
			require.Len(t, result.Content, 1)
			content, ok := result.Content[0].(*sdkmcp.TextContent)
			require.True(t, ok)
			require.Equal(t, tt.want, content.Text)
			require.NotContains(t, content.Text, secret)

			entries := sink.Entries()
			require.Len(t, entries, 1)
			require.Contains(t, fmt.Sprint(entries[0].Fields), tool.Name)
			require.NotContains(t, fmt.Sprint(entries[0].Fields), "private-argument-sentinel")
			loggedErr, ok := slogtest.FindFirstError(entries[0])
			require.True(t, ok)
			require.Contains(t, loggedErr.Error(), secret)
			if tt.err != nil {
				require.ErrorIs(t, loggedErr, tt.err)
			}
		})
	}
}

func TestMCPHTTP_APIError(t *testing.T) {
	t.Parallel()
	sink := testutil.NewFakeSink(t)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2/users/me", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(codersdk.Response{Message: "You do not have permission to view this user.", Detail: "private-api-detail"})
	}))
	t.Cleanup(api.Close)
	server, err := mcpserver.NewServer(sink.Logger())
	require.NoError(t, err)
	require.NoError(t, server.RegisterTools(codersdk.New(testutil.MustURL(t, api.URL))))
	ts := httptest.NewServer(server)
	t.Cleanup(ts.Close)
	ctx := testutil.Context(t, testutil.WaitLong)
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: ts.URL}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: toolsdk.ToolNameGetAuthenticatedUser, Arguments: map[string]any{}})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Len(t, result.Content, 1)
	content, ok := result.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok)
	require.Equal(t, "Coder API error (HTTP 403): You do not have permission to view this user.", content.Text)
	entries := sink.Entries()
	require.Len(t, entries, 1)
	loggedErr, ok := slogtest.FindFirstError(entries[0])
	require.True(t, ok)
	require.Contains(t, loggedErr.Error(), "private-api-detail")

	_, err = session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "nonexistent_tool", Arguments: map[string]any{}})
	require.ErrorContains(t, err, "nonexistent_tool")
}

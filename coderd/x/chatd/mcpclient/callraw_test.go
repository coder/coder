package mcpclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/testutil"
)

// TestCallRaw_OrgServer verifies that the raw result carries every content
// block, structured content, and the server's isError flag, and that a
// server-side failure is returned as an error whose cause is preserved.
func TestCallRaw_OrgServer(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	rich := testTool{
		tool: &mcp.Tool{Name: "rich", InputSchema: map[string]any{"type": "object"}},
		handler: func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			require.Contains(t, string(req.Params.Arguments), `"page":3`)
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "first"},
					&mcp.ImageContent{Data: []byte{1, 2, 3}, MIMEType: "image/png"},
					&mcp.ResourceLink{URI: "file:///a", Name: "a"},
				},
				StructuredContent: map[string]any{"items": []any{1.0, 2.0}},
				IsError:           true,
			}, nil
		},
	}
	failing := testTool{
		tool: &mcp.Tool{Name: "failing", InputSchema: map[string]any{"type": "object"}},
		handler: func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return nil, xerrors.New("upstream rejected")
		},
	}
	ts := newTestMCPServer(t, rich, failing)
	tools, _, cleanup := mcpclient.ConnectAllForTest(ctx, logger, []mcpclient.Server{makeConfig("srv", ts.URL)}, testutil.WaitLong, nil)
	t.Cleanup(cleanup)
	require.Len(t, tools, 2)

	byName := map[string]mcpclient.RawCaller{}
	for _, tool := range tools {
		caller, ok := tool.(mcpclient.RawCaller)
		require.True(t, ok)
		byName[tool.Info().Name] = caller
	}

	raw, err := byName["srv__rich"].CallRaw(ctx, map[string]any{"page": 3}, "box-1:1")
	require.NoError(t, err)
	require.True(t, raw.IsError)
	require.Equal(t, map[string]any{"items": []any{1.0, 2.0}}, raw.StructuredContent)
	require.Len(t, raw.Content, 3)
	require.Equal(t, map[string]any{"type": "text", "text": "first"}, raw.Content[0])
	require.Equal(t, "image", raw.Content[1]["type"])
	require.Equal(t, "AQID", raw.Content[1]["data"])
	require.Equal(t, "image/png", raw.Content[1]["mimeType"])
	require.Equal(t, "resource_link", raw.Content[2]["type"])
	require.Equal(t, "file:///a", raw.Content[2]["uri"])

	encoded, err := json.Marshal(raw)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"isError":true`)

	_, err = byName["srv__failing"].CallRaw(ctx, nil, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "upstream rejected")
}

// TestCallRaw_InlineRedactsAndCaps verifies that an inline server's
// secrets are redacted in every block field, in binary payloads, and in
// errors, and that the inline result cap applies to the raw result.
func TestCallRaw_InlineRedactsAndCaps(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	const secret = "super-secret-token-value"
	var serverURL string
	leaky := testTool{
		tool: &mcp.Tool{Name: "leaky", InputSchema: map[string]any{"type": "object"}},
		handler: func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "see " + secret},
					&mcp.ResourceLink{URI: serverURL + "/x", Name: "n", Description: "uses " + secret},
					&mcp.ImageContent{Data: []byte("img:" + secret), MIMEType: "image/png"},
				},
				StructuredContent: map[string]any{"token": secret},
			}, nil
		},
	}
	failing := testTool{
		tool: &mcp.Tool{Name: "failing", InputSchema: map[string]any{"type": "object"}},
		handler: func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return nil, xerrors.Errorf("rejected %s", secret)
		},
	}
	huge := testTool{
		tool: &mcp.Tool{Name: "huge", InputSchema: map[string]any{"type": "object"}},
		handler: func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return textToolResult(strings.Repeat("a", mcpclient.MaxInlineToolResultBytesForTest+1)), nil
		},
	}
	srv := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	ts := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true},
	))
	t.Cleanup(ts.Close)
	serverURL = ts.URL
	for _, tool := range []testTool{leaky, failing, huge} {
		srv.AddTool(tool.tool, tool.handler)
	}
	cfg := makeInlineConfig("bot", serverURL, map[string]string{"X-Key": secret})

	tools, _, cleanup := mcpclient.ConnectInlineForTest(ctx, logger, []mcpclient.Server{cfg}, nil, testutil.WaitLong)
	t.Cleanup(cleanup)
	require.Len(t, tools, 3)
	byName := map[string]mcpclient.RawCaller{}
	for _, tool := range tools {
		byName[tool.Info().Name] = tool.(mcpclient.RawCaller)
	}

	raw, err := byName["bot__leaky"].CallRaw(ctx, nil, "")
	require.NoError(t, err)
	encoded, err := json.Marshal(raw)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), secret)
	require.NotContains(t, string(encoded), ts.URL)
	require.Equal(t, "see [REDACTED]", raw.Content[0]["text"])
	require.Equal(t, "[REDACTED]/x", raw.Content[1]["uri"])
	require.Equal(t, "uses [REDACTED]", raw.Content[1]["description"])
	require.Equal(t, map[string]any{"token": "[REDACTED]"}, raw.StructuredContent)
	// Binary payloads are redacted before base64 encoding.
	require.Equal(t, "aW1nOltSRURBQ1RFRF0=", raw.Content[2]["data"])

	_, err = byName["bot__failing"].CallRaw(ctx, nil, "")
	require.Error(t, err)
	require.NotContains(t, err.Error(), secret)
	require.Contains(t, err.Error(), "[REDACTED]")

	_, err = byName["bot__huge"].CallRaw(ctx, nil, "")
	require.ErrorIs(t, err, mcpclient.ErrResultTooLarge)
}

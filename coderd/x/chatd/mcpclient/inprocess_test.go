package mcpclient_test

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
	"github.com/coder/coder/v2/testutil"
)

func TestConnectInProcess(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	srv := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	echo := echoTool()
	srv.AddTool(echo.tool, echo.handler)
	cfg := mcpclient.InProcessServer{ID: uuid.New(), Slug: "bot", Server: srv}

	tools, summaries, cleanup := mcpclient.ConnectInProcess(ctx, logger, []mcpclient.InProcessServer{cfg})

	require.Len(t, summaries, 1)
	require.Equal(t, mcpclient.ConnectOutcomeConnected, summaries[0].Outcome)
	require.Equal(t, cfg.ID, summaries[0].ConfigID)
	require.Equal(t, []string{"bot__echo"}, toolNames(tools))
	require.Len(t, slices.Collect(srv.Sessions()), 1)

	resp, err := tools[0].Run(ctx, fantasy.ToolCall{ID: "call-1", Name: "bot__echo", Input: `{"input":"hi"}`})
	require.NoError(t, err)
	require.Equal(t, "echo: hi", resp.Content)

	cleanup()
	require.Eventually(t, func() bool {
		return len(slices.Collect(srv.Sessions())) == 0
	}, testutil.WaitShort, testutil.IntervalFast, "closing the client must end the server session")
}

func TestConnectInProcess_RejectsTooManyTools(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	srv := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	for _, tool := range manyTools(65) {
		srv.AddTool(tool.tool, tool.handler)
	}
	cfg := mcpclient.InProcessServer{ID: uuid.New(), Slug: "bot", Server: srv}

	tools, summaries, cleanup := mcpclient.ConnectInProcess(ctx, logger, []mcpclient.InProcessServer{cfg})
	t.Cleanup(cleanup)

	require.Empty(t, tools)
	require.Len(t, summaries, 1)
	require.Equal(t, mcpclient.ConnectOutcomeError, summaries[0].Outcome)
	require.Contains(t, summaries[0].Error, "maximum is 64")
}

func TestConnectInProcess_ToolResultCap(t *testing.T) {
	t.Parallel()

	const maxBytes = mcpclient.MaxInProcessToolResultTextBytesForTest
	for _, tc := range []struct {
		name      string
		result    *mcp.CallToolResult
		wantError bool
	}{
		{name: "MediaOverCap", result: &mcp.CallToolResult{Content: []mcp.Content{
			&mcp.TextContent{Text: "report.pdf"},
			&mcp.EmbeddedResource{Resource: &mcp.ResourceContents{
				URI:      "file:///report.pdf",
				MIMEType: "application/pdf",
				Blob:     bytes.Repeat([]byte("a"), maxBytes),
			}},
		}}},
		{name: "TextOverCap", result: textToolResult(strings.Repeat("a", maxBytes+1)), wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

			srv := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
			srv.AddTool(
				&mcp.Tool{Name: "get_file", InputSchema: map[string]any{"type": "object"}},
				func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return tc.result, nil
				},
			)
			cfg := mcpclient.InProcessServer{ID: uuid.New(), Slug: "bot", Server: srv}

			tools, _, cleanup := mcpclient.ConnectInProcess(ctx, logger, []mcpclient.InProcessServer{cfg})
			t.Cleanup(cleanup)
			require.Len(t, tools, 1)

			resp, err := tools[0].Run(ctx, fantasy.ToolCall{ID: "call-1", Input: "{}"})
			require.NoError(t, err)
			require.Equal(t, tc.wantError, resp.IsError)
			if tc.wantError {
				require.Contains(t, resp.Content, "exceeded maximum size")
				return
			}
			require.Equal(t, "report.pdf", resp.Content)
			require.Equal(t, "application/pdf", resp.MediaType)
			require.Len(t, resp.Data, maxBytes)
		})
	}
}

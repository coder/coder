package chattool_test

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// fakeAgentConn implements just enough of workspacesdk.AgentConn
// for testing CallMCPTool.
type fakeAgentConn struct {
	workspacesdk.AgentConn
	callMCPToolFunc func(ctx context.Context, req workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error)
}

func (f *fakeAgentConn) CallMCPTool(ctx context.Context, req workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error) {
	return f.callMCPToolFunc(ctx, req)
}

func TestWorkspaceMCPTool_InvalidateOn404(t *testing.T) {
	t.Parallel()

	t.Run("404ErrorInvalidatesCache", func(t *testing.T) {
		t.Parallel()

		var invalidated atomic.Bool
		tool := chattool.NewWorkspaceMCPTool(
			workspacesdk.MCPToolInfo{
				Name:        "test__echo",
				Description: "test tool",
			},
			func(ctx context.Context) (workspacesdk.AgentConn, error) {
				return &fakeAgentConn{
					callMCPToolFunc: func(_ context.Context, _ workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error) {
						return workspacesdk.CallMCPToolResponse{}, codersdk.NewError(
							http.StatusNotFound,
							codersdk.Response{
								Message: "MCP tool call failed.",
								Detail:  `unknown MCP server: "test"`,
							},
						)
					},
				}, nil
			},
			func() { invalidated.Store(true) },
		)

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{})
		require.NoError(t, err)
		assert.True(t, resp.IsError, "response should be an error")
		assert.True(t, invalidated.Load(),
			"invalidateCache should fire on 404")
	})

	t.Run("Non404DoesNotInvalidate", func(t *testing.T) {
		t.Parallel()

		var invalidated atomic.Bool
		tool := chattool.NewWorkspaceMCPTool(
			workspacesdk.MCPToolInfo{
				Name:        "test__echo",
				Description: "test tool",
			},
			func(ctx context.Context) (workspacesdk.AgentConn, error) {
				return &fakeAgentConn{
					callMCPToolFunc: func(_ context.Context, _ workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error) {
						return workspacesdk.CallMCPToolResponse{}, codersdk.NewError(
							http.StatusBadGateway,
							codersdk.Response{
								Message: "Bad Gateway",
							},
						)
					},
				}, nil
			},
			func() { invalidated.Store(true) },
		)

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.False(t, invalidated.Load(),
			"invalidateCache should NOT fire on non-404 error")
	})

	t.Run("ToolLevelErrorNoInvalidation", func(t *testing.T) {
		t.Parallel()

		var invalidated atomic.Bool
		tool := chattool.NewWorkspaceMCPTool(
			workspacesdk.MCPToolInfo{
				Name:        "test__echo",
				Description: "test tool",
			},
			func(ctx context.Context) (workspacesdk.AgentConn, error) {
				return &fakeAgentConn{
					callMCPToolFunc: func(_ context.Context, _ workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error) {
						return workspacesdk.CallMCPToolResponse{
							IsError: true,
							Content: []workspacesdk.MCPToolContent{
								{Type: "text", Text: "tool error"},
							},
						}, nil
					},
				}, nil
			},
			func() { invalidated.Store(true) },
		)

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.False(t, invalidated.Load(),
			"invalidateCache should NOT fire on tool-level error (HTTP 200)")
	})

	t.Run("NilInvalidateCallbackSafe", func(t *testing.T) {
		t.Parallel()

		tool := chattool.NewWorkspaceMCPTool(
			workspacesdk.MCPToolInfo{
				Name:        "test__echo",
				Description: "test tool",
			},
			func(ctx context.Context) (workspacesdk.AgentConn, error) {
				return &fakeAgentConn{
					callMCPToolFunc: func(_ context.Context, _ workspacesdk.CallMCPToolRequest) (workspacesdk.CallMCPToolResponse, error) {
						return workspacesdk.CallMCPToolResponse{}, codersdk.NewError(
							http.StatusNotFound,
							codersdk.Response{
								Message: "MCP tool call failed.",
								Detail:  `unknown MCP server: "test"`,
							},
						)
					},
				}, nil
			},
			nil,
		)

		// Should not panic.
		resp, err := tool.Run(context.Background(), fantasy.ToolCall{})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
	})
}

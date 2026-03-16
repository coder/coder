package chattool_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"charm.land/fantasy"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

func newTestMCPServer(t *testing.T, tools ...mcpserver.ServerTool) *httptest.Server {
	t.Helper()
	srv := mcpserver.NewMCPServer("test-mcp-server", "1.0.0")
	srv.AddTools(tools...)
	return mcpserver.NewTestStreamableHTTPServer(srv)
}

func TestMCPServer(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)

	t.Run("DiscoverTools", func(t *testing.T) {
		t.Parallel()

		ts := newTestMCPServer(t,
			mcpserver.ServerTool{
				Tool: mcp.NewTool("greet",
					mcp.WithDescription("Say hello"),
					mcp.WithString("name", mcp.Required()),
				),
				Handler: func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					name := req.GetArguments()["name"].(string)
					return mcp.NewToolResultText("Hello, " + name + "!"), nil
				},
			},
			mcpserver.ServerTool{
				Tool: mcp.NewTool("add",
					mcp.WithDescription("Add two numbers"),
					mcp.WithNumber("a", mcp.Required()),
					mcp.WithNumber("b", mcp.Required()),
				),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return mcp.NewToolResultText("42"), nil
				},
			},
		)
		defer ts.Close()

		server := codersdk.ChatMCPServer{
			Slug:        "test-server",
			URL:         ts.URL,
			DisplayName: "Test Server",
		}

		tools, cleanup, err := chattool.MCPServer(ctx, server, logger)
		require.NoError(t, err)
		defer cleanup()

		require.Len(t, tools, 2)

		names := make([]string, len(tools))
		for i, tool := range tools {
			names[i] = tool.Info().Name
		}
		assert.Contains(t, names, "mcp__test-server__greet")
		assert.Contains(t, names, "mcp__test-server__add")

		for _, tool := range tools {
			info := tool.Info()
			assert.Contains(t, info.Description, "[MCP: Test Server]")
		}
	})

	t.Run("CallTool", func(t *testing.T) {
		t.Parallel()

		ts := newTestMCPServer(t,
			mcpserver.ServerTool{
				Tool: mcp.NewTool("echo",
					mcp.WithDescription("Echo input"),
					mcp.WithString("message", mcp.Required()),
				),
				Handler: func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					msg := req.GetArguments()["message"].(string)
					return mcp.NewToolResultText(msg), nil
				},
			},
		)
		defer ts.Close()

		server := codersdk.ChatMCPServer{
			Slug:        "echo-srv",
			URL:         ts.URL,
			DisplayName: "Echo",
		}

		tools, cleanup, err := chattool.MCPServer(ctx, server, logger)
		require.NoError(t, err)
		defer cleanup()
		require.Len(t, tools, 1)

		resp, err := tools[0].Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "mcp__echo-srv__echo",
			Input: `{"message":"hello world"}`,
		})
		require.NoError(t, err)
		assert.Equal(t, "hello world", resp.Content)
		assert.False(t, resp.IsError)
	})

	t.Run("ToolError", func(t *testing.T) {
		t.Parallel()

		ts := newTestMCPServer(t,
			mcpserver.ServerTool{
				Tool: mcp.NewTool("fail",
					mcp.WithDescription("Always fails"),
				),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return mcp.NewToolResultError("something went wrong"), nil
				},
			},
		)
		defer ts.Close()

		server := codersdk.ChatMCPServer{
			Slug:        "fail-srv",
			URL:         ts.URL,
			DisplayName: "Fail",
		}

		tools, cleanup, err := chattool.MCPServer(ctx, server, logger)
		require.NoError(t, err)
		defer cleanup()
		require.Len(t, tools, 1)

		resp, err := tools[0].Run(ctx, fantasy.ToolCall{
			ID:    "call-2",
			Name:  "mcp__fail-srv__fail",
			Input: `{}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "something went wrong")
	})

	t.Run("AllowRegex", func(t *testing.T) {
		t.Parallel()

		ts := newTestMCPServer(t,
			mcpserver.ServerTool{
				Tool: mcp.NewTool("read_file", mcp.WithDescription("Read a file")),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return mcp.NewToolResultText("ok"), nil
				},
			},
			mcpserver.ServerTool{
				Tool: mcp.NewTool("write_file", mcp.WithDescription("Write a file")),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return mcp.NewToolResultText("ok"), nil
				},
			},
			mcpserver.ServerTool{
				Tool: mcp.NewTool("delete_file", mcp.WithDescription("Delete a file")),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return mcp.NewToolResultText("ok"), nil
				},
			},
		)
		defer ts.Close()

		server := codersdk.ChatMCPServer{
			Slug:           "filtered",
			URL:            ts.URL,
			DisplayName:    "Filtered",
			ToolAllowRegex: "^read_",
		}

		tools, cleanup, err := chattool.MCPServer(ctx, server, logger)
		require.NoError(t, err)
		defer cleanup()

		require.Len(t, tools, 1)
		assert.Equal(t, "mcp__filtered__read_file", tools[0].Info().Name)
	})

	t.Run("DenyRegex", func(t *testing.T) {
		t.Parallel()

		ts := newTestMCPServer(t,
			mcpserver.ServerTool{
				Tool: mcp.NewTool("safe_tool", mcp.WithDescription("Safe")),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return mcp.NewToolResultText("ok"), nil
				},
			},
			mcpserver.ServerTool{
				Tool: mcp.NewTool("dangerous_tool", mcp.WithDescription("Dangerous")),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return mcp.NewToolResultText("ok"), nil
				},
			},
		)
		defer ts.Close()

		server := codersdk.ChatMCPServer{
			Slug:          "deny-test",
			URL:           ts.URL,
			DisplayName:   "Deny Test",
			ToolDenyRegex: "dangerous",
		}

		tools, cleanup, err := chattool.MCPServer(ctx, server, logger)
		require.NoError(t, err)
		defer cleanup()

		require.Len(t, tools, 1)
		assert.Equal(t, "mcp__deny-test__safe_tool", tools[0].Info().Name)
	})

	t.Run("InvalidURL", func(t *testing.T) {
		t.Parallel()

		server := codersdk.ChatMCPServer{
			Slug:        "bad-url",
			URL:         "http://localhost:1/nonexistent",
			DisplayName: "Bad URL",
		}

		_, _, err := chattool.MCPServer(ctx, server, logger)
		require.Error(t, err)
	})

	t.Run("NoTools", func(t *testing.T) {
		t.Parallel()

		ts := newTestMCPServer(t)
		defer ts.Close()

		server := codersdk.ChatMCPServer{
			Slug:        "empty",
			URL:         ts.URL,
			DisplayName: "Empty",
		}

		tools, cleanup, err := chattool.MCPServer(ctx, server, logger)
		require.NoError(t, err)
		defer cleanup()
		assert.Empty(t, tools)
	})

	t.Run("InvalidInput", func(t *testing.T) {
		t.Parallel()

		ts := newTestMCPServer(t,
			mcpserver.ServerTool{
				Tool: mcp.NewTool("test",
					mcp.WithDescription("Test"),
					mcp.WithString("name", mcp.Required()),
				),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return mcp.NewToolResultText("ok"), nil
				},
			},
		)
		defer ts.Close()

		server := codersdk.ChatMCPServer{
			Slug:        "input-test",
			URL:         ts.URL,
			DisplayName: "Input Test",
		}

		tools, cleanup, err := chattool.MCPServer(ctx, server, logger)
		require.NoError(t, err)
		defer cleanup()
		require.Len(t, tools, 1)

		resp, err := tools[0].Run(ctx, fantasy.ToolCall{
			ID:    "call-3",
			Name:  "mcp__input-test__test",
			Input: `not-json`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Contains(t, resp.Content, "invalid tool arguments")
	})
}

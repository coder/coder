package chattool_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/coderd/chatd/chattool"
	"github.com/coder/coder/v2/coderd/database"
)

// newTestMCPServer creates a streamable HTTP MCP server with the
// given tools, returning the httptest server. Caller must close
// the returned server.
func newTestMCPServer(t *testing.T, tools ...mcpserver.ServerTool) *httptest.Server {
	t.Helper()
	srv := mcpserver.NewMCPServer("test-mcp-server", "1.0.0")
	srv.AddTools(tools...)
	return mcpserver.NewTestStreamableHTTPServer(srv)
}

func TestDiscoverMCPServerTools(t *testing.T) {
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
				Handler: func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return mcp.NewToolResultText("42"), nil
				},
			},
		)
		defer ts.Close()

		server := database.ChatMCPServer{
			ID:          uuid.New(),
			Slug:        "test-server",
			Url:         ts.URL,
			DisplayName: "Test Server",
			AuthType:    "none",
			Enabled:     true,
		}

		tools, cleanup, err := chattool.DiscoverMCPServerTools(ctx, server, logger)
		require.NoError(t, err)
		defer cleanup()

		require.Len(t, tools, 2)

		// Verify tool names follow mcp__{slug}__{name} convention.
		names := make([]string, len(tools))
		for i, tool := range tools {
			names[i] = tool.Info().Name
		}
		assert.Contains(t, names, "mcp__test-server__greet")
		assert.Contains(t, names, "mcp__test-server__add")

		// Verify descriptions are prefixed.
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

		server := database.ChatMCPServer{
			ID:          uuid.New(),
			Slug:        "echo-srv",
			Url:         ts.URL,
			DisplayName: "Echo",
			AuthType:    "none",
			Enabled:     true,
		}

		tools, cleanup, err := chattool.DiscoverMCPServerTools(ctx, server, logger)
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

		server := database.ChatMCPServer{
			ID:          uuid.New(),
			Slug:        "fail-srv",
			Url:         ts.URL,
			DisplayName: "Fail",
			AuthType:    "none",
			Enabled:     true,
		}

		tools, cleanup, err := chattool.DiscoverMCPServerTools(ctx, server, logger)
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
				Tool:    mcp.NewTool("read_file", mcp.WithDescription("Read a file")),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) { return mcp.NewToolResultText("ok"), nil },
			},
			mcpserver.ServerTool{
				Tool:    mcp.NewTool("write_file", mcp.WithDescription("Write a file")),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) { return mcp.NewToolResultText("ok"), nil },
			},
			mcpserver.ServerTool{
				Tool:    mcp.NewTool("delete_file", mcp.WithDescription("Delete a file")),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) { return mcp.NewToolResultText("ok"), nil },
			},
		)
		defer ts.Close()

		server := database.ChatMCPServer{
			ID:             uuid.New(),
			Slug:           "filtered",
			Url:            ts.URL,
			DisplayName:    "Filtered",
			AuthType:       "none",
			ToolAllowRegex: "^read_",
			Enabled:        true,
		}

		tools, cleanup, err := chattool.DiscoverMCPServerTools(ctx, server, logger)
		require.NoError(t, err)
		defer cleanup()

		require.Len(t, tools, 1)
		assert.Equal(t, "mcp__filtered__read_file", tools[0].Info().Name)
	})

	t.Run("DenyRegex", func(t *testing.T) {
		t.Parallel()

		ts := newTestMCPServer(t,
			mcpserver.ServerTool{
				Tool:    mcp.NewTool("safe_tool", mcp.WithDescription("Safe")),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) { return mcp.NewToolResultText("ok"), nil },
			},
			mcpserver.ServerTool{
				Tool:    mcp.NewTool("dangerous_tool", mcp.WithDescription("Dangerous")),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) { return mcp.NewToolResultText("ok"), nil },
			},
		)
		defer ts.Close()

		server := database.ChatMCPServer{
			ID:            uuid.New(),
			Slug:          "deny-test",
			Url:           ts.URL,
			DisplayName:   "Deny Test",
			AuthType:      "none",
			ToolDenyRegex: "dangerous",
			Enabled:       true,
		}

		tools, cleanup, err := chattool.DiscoverMCPServerTools(ctx, server, logger)
		require.NoError(t, err)
		defer cleanup()

		require.Len(t, tools, 1)
		assert.Equal(t, "mcp__deny-test__safe_tool", tools[0].Info().Name)
	})

	t.Run("AllowAndDenyRegex", func(t *testing.T) {
		t.Parallel()

		ts := newTestMCPServer(t,
			mcpserver.ServerTool{
				Tool:    mcp.NewTool("file_read", mcp.WithDescription("Read")),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) { return mcp.NewToolResultText("ok"), nil },
			},
			mcpserver.ServerTool{
				Tool:    mcp.NewTool("file_write", mcp.WithDescription("Write")),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) { return mcp.NewToolResultText("ok"), nil },
			},
			mcpserver.ServerTool{
				Tool:    mcp.NewTool("file_delete", mcp.WithDescription("Delete")),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) { return mcp.NewToolResultText("ok"), nil },
			},
			mcpserver.ServerTool{
				Tool:    mcp.NewTool("network_read", mcp.WithDescription("Network read")),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) { return mcp.NewToolResultText("ok"), nil },
			},
		)
		defer ts.Close()

		// Allow only file_* tools, then deny file_delete.
		server := database.ChatMCPServer{
			ID:             uuid.New(),
			Slug:           "combo",
			Url:            ts.URL,
			DisplayName:    "Combo",
			AuthType:       "none",
			ToolAllowRegex: "^file_",
			ToolDenyRegex:  "delete$",
			Enabled:        true,
		}

		tools, cleanup, err := chattool.DiscoverMCPServerTools(ctx, server, logger)
		require.NoError(t, err)
		defer cleanup()

		require.Len(t, tools, 2)
		names := make([]string, len(tools))
		for i, tool := range tools {
			names[i] = tool.Info().Name
		}
		assert.Contains(t, names, "mcp__combo__file_read")
		assert.Contains(t, names, "mcp__combo__file_write")
	})

	t.Run("HeaderAuth", func(t *testing.T) {
		t.Parallel()

		ts := newTestMCPServer(t,
			mcpserver.ServerTool{
				Tool: mcp.NewTool("authed",
					mcp.WithDescription("Requires auth"),
				),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return mcp.NewToolResultText("authenticated"), nil
				},
			},
		)
		defer ts.Close()

		server := database.ChatMCPServer{
			ID:          uuid.New(),
			Slug:        "authed-srv",
			Url:         ts.URL,
			DisplayName: "Authed",
			AuthType:    "header",
			AuthHeaders: `{"Authorization":"Bearer test-token"}`,
			Enabled:     true,
		}

		tools, cleanup, err := chattool.DiscoverMCPServerTools(ctx, server, logger)
		require.NoError(t, err)
		defer cleanup()

		// We just verify it connected and discovered tools.
		// The test MCP server doesn't actually check headers,
		// but this validates the auth header parsing path.
		require.Len(t, tools, 1)
	})

	t.Run("InvalidURL", func(t *testing.T) {
		t.Parallel()

		server := database.ChatMCPServer{
			ID:          uuid.New(),
			Slug:        "bad-url",
			Url:         "http://localhost:1/nonexistent",
			DisplayName: "Bad URL",
			AuthType:    "none",
			Enabled:     true,
		}

		_, _, err := chattool.DiscoverMCPServerTools(ctx, server, logger)
		require.Error(t, err)
	})

	t.Run("InvalidAllowRegex", func(t *testing.T) {
		t.Parallel()

		ts := newTestMCPServer(t,
			mcpserver.ServerTool{
				Tool:    mcp.NewTool("test", mcp.WithDescription("Test")),
				Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) { return mcp.NewToolResultText("ok"), nil },
			},
		)
		defer ts.Close()

		server := database.ChatMCPServer{
			ID:             uuid.New(),
			Slug:           "bad-regex",
			Url:            ts.URL,
			DisplayName:    "Bad Regex",
			AuthType:       "none",
			ToolAllowRegex: "[invalid",
			Enabled:        true,
		}

		_, _, err := chattool.DiscoverMCPServerTools(ctx, server, logger)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tool_allow_regex")
	})

	t.Run("NoTools", func(t *testing.T) {
		t.Parallel()

		ts := newTestMCPServer(t) // No tools registered.
		defer ts.Close()

		server := database.ChatMCPServer{
			ID:          uuid.New(),
			Slug:        "empty",
			Url:         ts.URL,
			DisplayName: "Empty",
			AuthType:    "none",
			Enabled:     true,
		}

		tools, cleanup, err := chattool.DiscoverMCPServerTools(ctx, server, logger)
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

		server := database.ChatMCPServer{
			ID:          uuid.New(),
			Slug:        "input-test",
			Url:         ts.URL,
			DisplayName: "Input Test",
			AuthType:    "none",
			Enabled:     true,
		}

		tools, cleanup, err := chattool.DiscoverMCPServerTools(ctx, server, logger)
		require.NoError(t, err)
		defer cleanup()
		require.Len(t, tools, 1)

		// Send invalid JSON as input.
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

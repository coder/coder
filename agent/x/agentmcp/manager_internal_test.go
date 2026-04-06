package agentmcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestSplitToolName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantServer string
		wantTool   string
		wantErr    bool
	}{
		{
			name:       "Valid",
			input:      "server__tool",
			wantServer: "server",
			wantTool:   "tool",
		},
		{
			name:       "ValidWithUnderscoresInTool",
			input:      "server__my_tool",
			wantServer: "server",
			wantTool:   "my_tool",
		},
		{
			name:    "MissingSeparator",
			input:   "servertool",
			wantErr: true,
		},
		{
			name:    "EmptyServer",
			input:   "__tool",
			wantErr: true,
		},
		{
			name:    "EmptyTool",
			input:   "server__",
			wantErr: true,
		},
		{
			name:    "JustSeparator",
			input:   "__",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, tool, err := splitToolName(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidToolName)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantServer, server)
			assert.Equal(t, tt.wantTool, tool)
		})
	}
}

func TestConvertResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// input is a pointer so we can test nil.
		input *mcp.CallToolResult
		want  workspacesdk.CallMCPToolResponse
	}{
		{
			name:  "NilInput",
			input: nil,
			want:  workspacesdk.CallMCPToolResponse{},
		},
		{
			name: "TextContent",
			input: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{Type: "text", Text: "hello"},
				},
			},
			want: workspacesdk.CallMCPToolResponse{
				Content: []workspacesdk.MCPToolContent{
					{Type: "text", Text: "hello"},
				},
			},
		},
		{
			name: "ImageContent",
			input: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.ImageContent{
						Type:     "image",
						Data:     "base64data",
						MIMEType: "image/png",
					},
				},
			},
			want: workspacesdk.CallMCPToolResponse{
				Content: []workspacesdk.MCPToolContent{
					{Type: "image", Data: "base64data", MediaType: "image/png"},
				},
			},
		},
		{
			name: "AudioContent",
			input: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.AudioContent{
						Type:     "audio",
						Data:     "base64audio",
						MIMEType: "audio/mp3",
					},
				},
			},
			want: workspacesdk.CallMCPToolResponse{
				Content: []workspacesdk.MCPToolContent{
					{Type: "audio", Data: "base64audio", MediaType: "audio/mp3"},
				},
			},
		},
		{
			name: "IsErrorPropagation",
			input: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{Type: "text", Text: "fail"},
				},
				IsError: true,
			},
			want: workspacesdk.CallMCPToolResponse{
				Content: []workspacesdk.MCPToolContent{
					{Type: "text", Text: "fail"},
				},
				IsError: true,
			},
		},
		{
			name: "MultipleContentItems",
			input: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.TextContent{Type: "text", Text: "caption"},
					mcp.ImageContent{
						Type:     "image",
						Data:     "imgdata",
						MIMEType: "image/jpeg",
					},
				},
			},
			want: workspacesdk.CallMCPToolResponse{
				Content: []workspacesdk.MCPToolContent{
					{Type: "text", Text: "caption"},
					{Type: "image", Data: "imgdata", MediaType: "image/jpeg"},
				},
			},
		},
		{
			name: "ResourceLink",
			input: &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.ResourceLink{
						Type: "resource_link",
						URI:  "file:///tmp/test.txt",
					},
				},
			},
			want: workspacesdk.CallMCPToolResponse{
				Content: []workspacesdk.MCPToolContent{
					{Type: "resource", Text: "[resource link: file:///tmp/test.txt]"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := convertResult(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestConnectServer_StdioProcessSurvivesConnect verifies that a stdio MCP
// server subprocess remains alive after connectServer returns. This is a
// regression test for a bug where the subprocess was tied to a short-lived
// connectCtx and killed as soon as the context was canceled.
func TestConnectServer_StdioProcessSurvivesConnect(t *testing.T) {
	t.Parallel()

	if os.Getenv("TEST_MCP_FAKE_SERVER") == "1" {
		// Child process: act as a minimal MCP server over stdio.
		runFakeMCPServer()
		return
	}

	// Get the path to the test binary so we can re-exec ourselves
	// as a fake MCP server subprocess.
	testBin, err := os.Executable()
	require.NoError(t, err)

	cfg := ServerConfig{
		Name:      "fake",
		Transport: "stdio",
		Command:   testBin,
		Args:      []string{"-test.run=^TestConnectServer_StdioProcessSurvivesConnect$"},
		Env:       map[string]string{"TEST_MCP_FAKE_SERVER": "1"},
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	m := &Manager{}
	client, err := m.connectServer(ctx, cfg)
	require.NoError(t, err, "connectServer should succeed")
	t.Cleanup(func() { _ = client.Close() })

	// At this point connectServer has returned and its internal
	// connectCtx has been canceled. The subprocess must still be
	// alive. Verify by listing tools (requires a live server).
	listCtx, listCancel := context.WithTimeout(ctx, testutil.WaitShort)
	defer listCancel()
	result, err := client.ListTools(listCtx, mcp.ListToolsRequest{})
	require.NoError(t, err, "ListTools should succeed — server must be alive after connect")
	require.Len(t, result.Tools, 1)
	assert.Equal(t, "echo", result.Tools[0].Name)
}

// runFakeMCPServer implements a minimal JSON-RPC / MCP server over
// stdin/stdout, just enough for initialize + tools/list.
func runFakeMCPServer() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()

		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
		}
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		var resp any
		switch req.Method {
		case "initialize":
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"protocolVersion": "2025-03-26",
					"capabilities": map[string]any{
						"tools": map[string]any{},
					},
					"serverInfo": map[string]any{
						"name":    "fake-server",
						"version": "0.0.1",
					},
				},
			}
		case "notifications/initialized":
			// No response needed for notifications.
			continue
		case "tools/list":
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"tools": []map[string]any{
						{
							"name":        "echo",
							"description": "echoes input",
							"inputSchema": map[string]any{
								"type":       "object",
								"properties": map[string]any{},
							},
						},
					},
				},
			}
		default:
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"error": map[string]any{
					"code":    -32601,
					"message": "method not found",
				},
			}
		}

		out, err := json.Marshal(resp)
		if err != nil {
			continue
		}
		_, _ = fmt.Fprintf(os.Stdout, "%s\n", out)
	}
}

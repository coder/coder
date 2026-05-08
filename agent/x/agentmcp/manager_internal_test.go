package agentmcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentexec"
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
	m := &Manager{execer: agentexec.DefaultExecer}
	client, err := m.connectServer(ctx, cfg)
	require.NoError(t, err, "connectServer should succeed")
	t.Cleanup(func() { _ = client.Close() })

	// At this point connectServer has returned and its internal
	// connectCtx has been canceled. The subprocess must still be
	// alive. Verify by listing tools (requires a live server).
	listCtx, listCancel := context.WithTimeout(ctx, testutil.WaitShort)
	defer listCancel()
	result, err := client.ListTools(listCtx, mcp.ListToolsRequest{})
	require.NoError(t, err, "ListTools should succeed, server must be alive after connect")
	require.Len(t, result.Tools, 1)
	assert.Equal(t, "echo", result.Tools[0].Name)
}

func TestManager_ToolsStartupGate(t *testing.T) {
	t.Parallel()

	if os.Getenv("TEST_MCP_FAKE_SERVER") == "1" {
		runFakeMCPServer()
		return
	}

	t.Run("MissingBeforeStartupCanAppearBeforeSettlement", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()
		configPath := filepath.Join(dir, ".mcp.json")

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		t.Cleanup(func() { _ = m.Close() })

		type result struct {
			tools []workspacesdk.MCPToolInfo
			err   error
		}
		done := make(chan result, 1)
		go func() {
			tools, err := m.Tools(ctx, []string{configPath})
			done <- result{tools: tools, err: err}
		}()

		_, entry := fakeMCPServerConfig(t, "srv")
		writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})
		m.MarkStartupSettled()

		select {
		case got := <-done:
			require.NoError(t, got.err)
			require.Len(t, got.tools, 1)
			assert.Contains(t, got.tools[0].Name, "echo")
		case <-ctx.Done():
			t.Fatalf("Tools did not return after startup settled: %v", ctx.Err())
		}
	})

	t.Run("MissingAfterStartupReturnsEmptyAndMarksFirstSync", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()
		configPath := filepath.Join(dir, ".mcp.json")

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		m.MarkStartupSettled()
		t.Cleanup(func() { _ = m.Close() })

		tools, err := m.Tools(ctx, []string{configPath})
		require.NoError(t, err)
		assert.Empty(t, tools)

		m.mu.RLock()
		firstSyncSettled := m.firstSyncSettled
		m.mu.RUnlock()
		assert.True(t, firstSyncSettled)
	})

	t.Run("ConfigAppearsAfterEmptySyncReloads", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()
		configPath := filepath.Join(dir, ".mcp.json")

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		m.MarkStartupSettled()
		t.Cleanup(func() { _ = m.Close() })

		tools, err := m.Tools(ctx, []string{configPath})
		require.NoError(t, err)
		require.Empty(t, tools)

		_, entry := fakeMCPServerConfig(t, "srv")
		writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		tools, err = m.Tools(ctx, []string{configPath})
		require.NoError(t, err)
		require.Len(t, tools, 1)
		assert.Contains(t, tools[0].Name, "echo")
	})

	t.Run("ConcurrentFirstListToolsCallsAllSucceed", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()
		_, entry := fakeMCPServerConfig(t, "srv")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		m.MarkStartupSettled()
		t.Cleanup(func() { _ = m.Close() })

		const callers = 5
		var wg sync.WaitGroup
		errs := make([]error, callers)
		toolCounts := make([]int, callers)
		for i := range callers {
			wg.Go(func() {
				tools, err := m.Tools(ctx, []string{configPath})
				errs[i] = err
				toolCounts[i] = len(tools)
			})
		}
		wg.Wait()

		for i := range callers {
			assert.NoError(t, errs[i], "caller %d should not fail", i)
			assert.Equal(t, 1, toolCounts[i], "caller %d should see tools", i)
		}
	})

	t.Run("CloseUnblocksStartupWait", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()
		configPath := filepath.Join(dir, ".mcp.json")

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)

		done := make(chan error, 1)
		go func() {
			_, err := m.Tools(ctx, []string{configPath})
			done <- err
		}()
		require.NoError(t, m.Close())

		select {
		case err := <-done:
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrManagerClosed)
		case <-ctx.Done():
			t.Fatalf("Tools did not return after Close: %v", ctx.Err())
		}
	})

	t.Run("CallerCanceledBeforeStartupReturnsNoTools", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()
		configPath := filepath.Join(dir, ".mcp.json")

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		t.Cleanup(func() { _ = m.Close() })

		callerCtx, cancel := context.WithCancel(ctx)
		cancel()
		tools, err := m.Tools(callerCtx, []string{configPath})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, tools)
	})

	t.Run("ManagerCanceledBeforeStartupReturnsNoTools", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()
		configPath := filepath.Join(dir, ".mcp.json")

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		t.Cleanup(func() { _ = m.Close() })

		cancel()
		tools, err := m.Tools(testutil.Context(t, testutil.WaitLong), []string{configPath})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, tools)
	})

	t.Run("ClosedBeforeFirstSyncReturnsNoTools", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()
		configPath := filepath.Join(dir, ".mcp.json")

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		m.MarkStartupSettled()
		require.NoError(t, m.Close())

		tools, err := m.Tools(ctx, []string{configPath})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrManagerClosed)
		assert.Nil(t, tools)
	})

	t.Run("CanceledBeforeFirstSyncStillStartsReload", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()
		configPath := filepath.Join(dir, ".mcp.json")
		paths := []string{configPath}

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		m.MarkStartupSettled()
		t.Cleanup(func() { _ = m.Close() })

		callerCtx, cancel := context.WithCancel(ctx)
		cancel()
		tools, err := m.Tools(callerCtx, paths)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Empty(t, tools)

		testutil.Eventually(ctx, t, func(context.Context) bool {
			m.mu.RLock()
			firstSyncSettled := m.firstSyncSettled
			m.mu.RUnlock()
			return firstSyncSettled && !m.SnapshotChanged(paths)
		}, testutil.IntervalFast)

		tools, err = m.Tools(ctx, paths)
		require.NoError(t, err)
		assert.Empty(t, tools)
	})

	t.Run("CanceledAfterFirstSyncNoopReturnsCachedTools", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()
		_, entry := fakeMCPServerConfig(t, "srv")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		m.MarkStartupSettled()
		t.Cleanup(func() { _ = m.Close() })

		tools, err := m.Tools(ctx, []string{configPath})
		require.NoError(t, err)
		require.Len(t, tools, 1)

		callerCtx, cancel := context.WithCancel(ctx)
		cancel()
		tools, err = m.Tools(callerCtx, []string{configPath})
		require.NoError(t, err)
		require.Len(t, tools, 1)
		assert.Contains(t, tools[0].Name, "echo")
	})
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

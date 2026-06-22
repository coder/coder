package agentcontext_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/testutil"
)

// TestManager_MCPServerToolsInSnapshot exercises the MCP runner end to
// end against a real subprocess: a .mcp.json in the working directory is
// discovered by the resolver, the runner connects the declared stdio
// server, lists its tools, and they surface as a KindMCPServer resource
// in the manager's snapshot (the same snapshot that is pushed to coderd).
func TestManager_MCPServerToolsInSnapshot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeMCPConfig(t, dir, "fake", map[string]string{"TEST_MCP_FAKE_SERVER": "1"})

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return dir },
		MCPExecer:  agentexec.DefaultExecer,
	})
	ctx := testutil.Context(t, testutil.WaitLong)
	go func() { _ = m.Run(ctx) }()

	require.Eventually(t, func() bool {
		return findMCPServer(m.Snapshot(), "fake") != nil
	}, testutil.WaitLong, testutil.IntervalMedium,
		"the connected MCP server's tools should surface in the snapshot")

	got := findMCPServer(m.Snapshot(), "fake")
	require.NotNil(t, got)
	require.Equal(t, agentcontext.StatusOK, got.Status)
	require.Len(t, got.Tools, 1)
	require.Equal(t, "echo", got.Tools[0].Name)
	require.Equal(t, "echoes input", got.Tools[0].Description)
}

// TestManager_MCPServerHangingCloseDoesNotStall is a regression test for
// a server that ignores stdin-close. mcp-go's stdio Close() closes stdin
// and then blocks on cmd.Wait(); without a force-kill the runner's
// per-server connect (and thus the whole reload) would hang and the
// tools would never be published. The runner force-kills the subprocess,
// so the tool still surfaces in the snapshot.
func TestManager_MCPServerHangingCloseDoesNotStall(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeMCPConfig(t, dir, "hang", map[string]string{
		"TEST_MCP_FAKE_SERVER":     "1",
		"TEST_MCP_HANG_AFTER_LIST": "1",
	})

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return dir },
		MCPExecer:  agentexec.DefaultExecer,
	})
	ctx := testutil.Context(t, testutil.WaitLong)
	go func() { _ = m.Run(ctx) }()

	require.Eventually(t, func() bool {
		got := findMCPServer(m.Snapshot(), "hang")
		return got != nil && got.Status == agentcontext.StatusOK
	}, testutil.WaitLong, testutil.IntervalMedium,
		"a hanging MCP server must not stall the reload; its tool should still surface")
}

// findMCPServer returns the KindMCPServer resource for the named server,
// or nil if absent.
func findMCPServer(snap agentcontext.Snapshot, name string) *agentcontext.Resource {
	for i := range snap.Resources {
		if r := snap.Resources[i]; r.Kind == agentcontext.KindMCPServer && r.Source == name {
			return &r
		}
	}
	return nil
}

// writeMCPConfig writes a .mcp.json into dir declaring a single stdio MCP
// server that re-execs this test binary into serveFakeMCPServer (via the
// TEST_MCP_FAKE_SERVER env, which TestMain handles).
func writeMCPConfig(t *testing.T, dir, name string, env map[string]string) {
	t.Helper()
	testBin, err := os.Executable()
	require.NoError(t, err)
	cfg := map[string]any{
		"mcpServers": map[string]any{
			name: map[string]any{
				"command": testBin,
				"env":     env,
			},
		},
	}
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mcp.json"), data, 0o600))
}

// maybeServeFakeMCPServer serves the fake stdio MCP server when
// TEST_MCP_FAKE_SERVER=1 and reports whether it handled the process so
// the caller (TestMain) can exit. The runner re-execs the test binary
// into this, so it must run at the very top of TestMain. When
// TEST_MCP_HANG_AFTER_LIST=1 the server blocks after serving instead of
// returning, simulating a server that ignores stdin-close so a test can
// exercise the runner's force-kill (the process is then killed by the
// parent and never returns here).
func maybeServeFakeMCPServer() (served bool) {
	if os.Getenv("TEST_MCP_FAKE_SERVER") != "1" {
		return false
	}
	serveFakeMCPServer()
	if os.Getenv("TEST_MCP_HANG_AFTER_LIST") == "1" {
		select {}
	}
	return true
}

// serveFakeMCPServer serves a minimal MCP protocol over stdin/stdout: it
// answers initialize and advertises a single "echo" tool, then returns
// when the client closes stdin (EOF).
func serveFakeMCPServer() {
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
					"capabilities":    map[string]any{"tools": map[string]any{}},
					"serverInfo":      map[string]any{"name": "fake-server", "version": "0.0.1"},
				},
			}
		case "notifications/initialized":
			// Notifications take no response.
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
				"error":   map[string]any{"code": -32601, "message": "method not found"},
			}
		}

		out, err := json.Marshal(resp)
		if err != nil {
			continue
		}
		_, _ = fmt.Fprintf(os.Stdout, "%s\n", out)
	}
}

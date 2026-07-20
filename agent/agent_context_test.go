package agent_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agentcontextconfig"
	"github.com/coder/coder/v2/agent/agenttest"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

// TestAgent_ContextStatePushed verifies the agent pushes its workspace
// context over the v2.10 PushContextState RPC, and that the readiness
// gate (SetReady, wired to the lifecycle transition) holds the push
// until startup completes. The first push therefore already contains
// the seeded AGENTS.md with Initial=true and no "unreadable" issues.
func TestAgent_ContextStatePushed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t,
		os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("test rules"), 0o600))

	//nolint:dogsled // setupAgent returns a wide tuple; we only care about the client.
	_, client, _, _, _ := setupAgent(t,
		agentsdk.Manifest{Directory: dir},
		0,
		func(_ *agenttest.Client, opts *agent.Options) {
			opts.ContextConfig = agentcontextconfig.Config{}
		},
	)

	// The push is gated until the agent reaches lifecycle ready. Wait
	// for that first push to land.
	var pushes []*agentproto.PushContextStateRequest
	require.Eventually(t, func() bool {
		pushes = client.ContextStatePushes()
		return len(pushes) > 0
	}, testutil.WaitMedium, testutil.IntervalFast,
		"expected a context snapshot push after startup; got %d pushes", len(pushes))

	first := pushes[0]
	assert.True(t, first.GetInitial(), "first push must carry Initial=true")
	assert.NotEmpty(t, first.GetAggregateHash(), "aggregate_hash must be populated")

	// The first push must already reflect the ready workspace: the
	// seeded AGENTS.md is present and no resource is UNREADABLE.
	var foundAgents bool
	for _, r := range first.GetResources() {
		if r.GetInstructionFile() != nil &&
			filepath.Base(r.GetSource()) == "AGENTS.md" {
			foundAgents = true
		}
		assert.NotEqualf(t, agentproto.ContextResource_UNREADABLE, r.GetStatus(),
			"no resource should be UNREADABLE in the post-ready snapshot: %s", r.GetSource())
	}
	assert.True(t, foundAgents, "first push must already include the seeded AGENTS.md")

	// Subsequent pushes must not be Initial.
	for _, p := range pushes[1:] {
		assert.False(t, p.GetInitial(), "only the first push must be Initial")
	}
}

// TestAgent_ContextStateFirstPushIncludesMCP verifies the context gate
// waits for the MCP catalog to settle before the first push: the
// initial PushContextState (Initial=true) already contains the
// configured MCP server resource with its tools instead of relying on
// a follow-up push after the servers connect.
func TestAgent_ContextStateFirstPushIncludesMCP(t *testing.T) {
	t.Parallel()

	if os.Getenv("TEST_MCP_FAKE_SERVER") == "1" {
		// Child process: act as a minimal MCP server over stdio.
		runFakeMCPStdioServer()
		return
	}

	dir := t.TempDir()

	// Point the default .mcp.json at a fake MCP server using the
	// test binary re-exec pattern: the agent spawns this test
	// binary, which detects TEST_MCP_FAKE_SERVER and serves MCP
	// over stdio instead of running tests.
	testBin, err := os.Executable()
	require.NoError(t, err)
	mcpConfig := map[string]any{
		"mcpServers": map[string]any{
			"srv": map[string]any{
				"command": testBin,
				"args":    []string{"-test.run=^TestAgent_ContextStateFirstPushIncludesMCP$"},
				"env":     map[string]string{"TEST_MCP_FAKE_SERVER": "1"},
			},
		},
	}
	data, err := json.Marshal(mcpConfig)
	require.NoError(t, err)
	require.NoError(t,
		os.WriteFile(filepath.Join(dir, ".mcp.json"), data, 0o600))

	//nolint:dogsled // setupAgent returns a wide tuple; we only care about the client.
	_, client, _, _, _ := setupAgent(t,
		agentsdk.Manifest{Directory: dir},
		0,
		func(_ *agenttest.Client, opts *agent.Options) {
			opts.ContextConfig = agentcontextconfig.Config{}
		},
	)

	var pushes []*agentproto.PushContextStateRequest
	require.Eventually(t, func() bool {
		pushes = client.ContextStatePushes()
		return len(pushes) > 0
	}, testutil.WaitLong, testutil.IntervalFast,
		"expected a context snapshot push after startup; got %d pushes", len(pushes))

	first := pushes[0]
	require.True(t, first.GetInitial(), "first push must carry Initial=true")

	// The gate held until the MCP catalog settled, so the first
	// push must already contain the server and its tool list.
	var srv *agentproto.MCPServerBody
	for _, r := range first.GetResources() {
		if body := r.GetMcpServer(); body != nil && body.GetServerName() == "srv" {
			srv = body
		}
	}
	require.NotNil(t, srv,
		"first push must already contain the MCP server resource")
	require.Len(t, srv.GetTools(), 1,
		"first push must already contain the server's tools")
	assert.Equal(t, "echo", srv.GetTools()[0].GetName())
}

// runFakeMCPStdioServer implements a minimal JSON-RPC / MCP server
// over stdin/stdout, just enough for initialize + tools/list. It
// runs in a re-exec'd copy of the test binary spawned by the agent's
// MCP manager.
func runFakeMCPStdioServer() {
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

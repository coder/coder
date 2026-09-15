package agent_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
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

// TestAgent_PluginMCPServersLoaded verifies that a plugin discovered in
// the working directory has its mcp.json loaded through the MCP engine
// and that the resulting server resource is pushed with plugin
// attribution. The declared command does not exist, so the server is
// reported as unreadable; the attribution and the plugin-prefixed
// source are what this test checks.
func TestAgent_PluginMCPServersLoaded(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, ".agents", "plugins", "acme")
	require.NoError(t, os.MkdirAll(pluginDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(`{
		"$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
		"name": "acme"
	}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "mcp.json"), []byte(`{
		"$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
		"mcpServers": {
			"srv": {"type": "stdio", "command": "coder-test-missing-mcp-binary"}
		}
	}`), 0o600))

	//nolint:dogsled // setupAgent returns a wide tuple; we only care about the client.
	_, client, _, _, _ := setupAgent(t,
		agentsdk.Manifest{Directory: dir, PluginsSupported: true},
		0,
		func(_ *agenttest.Client, opts *agent.Options) {
			opts.ContextConfig = agentcontextconfig.Config{}
		},
	)

	var (
		plugin *agentproto.ContextResource
		server *agentproto.ContextResource
	)
	pushed := assert.Eventually(t, func() bool {
		plugin, server = nil, nil
		for _, push := range client.ContextStatePushes() {
			for _, r := range push.GetResources() {
				if r.GetPlugin() != nil {
					plugin = r
				}
				if r.GetMcpServer() != nil && r.GetMcpServer().GetPluginName() == "acme" {
					server = r
				}
			}
		}
		return plugin != nil && server != nil
	}, testutil.WaitSuperLong, testutil.IntervalFast)
	if !pushed {
		t.Fatalf("expected plugin and plugin MCP server resources to be pushed; got %s",
			describePushes(client.ContextStatePushes()))
	}

	assert.Equal(t, "acme", plugin.GetPlugin().GetName())
	assert.Equal(t, agentproto.ContextResource_OK, plugin.GetStatus())
	assert.Equal(t, "acme/srv", server.GetSource())
	assert.Equal(t, "srv", server.GetMcpServer().GetServerName())
	assert.Equal(t, agentproto.ContextResource_UNREADABLE, server.GetStatus())
}

// describePushes renders every pushed resource as "vN kind source status"
// so a failed wait shows which stage of the discovery chain stalled.
func describePushes(pushes []*agentproto.PushContextStateRequest) string {
	var sb strings.Builder
	for _, push := range pushes {
		for _, r := range push.GetResources() {
			kind := "other"
			switch {
			case r.GetPlugin() != nil:
				kind = "plugin"
			case r.GetMcpServer() != nil:
				kind = "mcp_server"
			case r.GetSkill() != nil:
				kind = "skill"
			case r.GetInstructionFile() != nil:
				kind = "instruction_file"
			}
			_, _ = fmt.Fprintf(&sb, "[v%d %s %s %s] ", push.GetVersion(), kind, r.GetSource(), r.GetStatus())
		}
	}
	return sb.String()
}

// logCapture records every message logged through it.
type logCapture struct {
	mu       sync.Mutex
	messages []string
}

func (c *logCapture) LogEntry(_ context.Context, e slog.SinkEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, e.Message)
}

func (*logCapture) Sync() {}

func (c *logCapture) has(msg string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range c.messages {
		if m == msg {
			return true
		}
	}
	return false
}

// TestAgent_MissingDirectoryNotice verifies that an agent whose manifest
// has no directory logs a warning naming the working-directory context it
// skips, since nothing else signals that <workdir>/.agents/plugins and
// its siblings are not scanned.
func TestAgent_MissingDirectoryNotice(t *testing.T) {
	t.Parallel()

	capture := &logCapture{}
	//nolint:dogsled // Only the side effect on the logger matters here.
	_, _, _, _, _ = setupAgent(t,
		agentsdk.Manifest{Directory: ""},
		0,
		func(_ *agenttest.Client, opts *agent.Options) {
			opts.Logger = opts.Logger.AppendSinks(capture)
		},
	)

	require.Eventually(t, func() bool {
		return capture.has("agent directory is not set; skipping working-directory context")
	}, testutil.WaitLong, testutil.IntervalFast)
}

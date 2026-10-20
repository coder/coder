package agent_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agentcontext"
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

// hasPluginData reports whether req carries a plugin resource.
func hasPluginData(req *agentproto.PushContextStateRequest) bool {
	return slices.ContainsFunc(req.GetResources(), func(r *agentproto.ContextResource) bool {
		return r.GetPlugin() != nil
	})
}

func writeTestPlugin(t *testing.T, dir string) {
	t.Helper()
	pluginDir := filepath.Join(dir, "plugins", "p")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "plugin.json"),
		[]byte(`{"$schema":"`+agentcontext.PluginSchemaV1+`","name":"p"}`), 0o600))
}

func TestAgent_ContextStatePushedPlugins(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestPlugin(t, dir)

	//nolint:dogsled // setupAgent returns a wide tuple; we only care about the client.
	_, client, _, _, _ := setupAgent(t,
		agentsdk.Manifest{Directory: dir},
		0,
		func(_ *agenttest.Client, opts *agent.Options) {
			opts.ContextConfig = agentcontextconfig.Config{}
		},
	)

	require.Eventually(t, func() bool {
		return slices.ContainsFunc(client.ContextStatePushes(), hasPluginData)
	}, testutil.WaitMedium, testutil.IntervalFast, "expected a push with plugin data")
}

// When coderd rejects pushes that carry plugin data, the agent still
// syncs the rest of its context and reports why plugins are missing.
func TestAgent_ContextStatePushedWithoutRejectedPlugins(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestPlugin(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("rules"), 0o600))

	var rejected atomic.Int32
	//nolint:dogsled // setupAgent returns a wide tuple; we only care about the client.
	_, client, _, _, _ := setupAgent(t,
		agentsdk.Manifest{Directory: dir},
		0,
		func(c *agenttest.Client, opts *agent.Options) {
			opts.ContextConfig = agentcontextconfig.Config{}
			c.SetPushContextStateFunc(func(req *agentproto.PushContextStateRequest) error {
				if hasPluginData(req) {
					rejected.Add(1)
					return xerrors.New("unknown resource body")
				}
				return nil
			})
		},
	)

	hasAgentsMD := func(p *agentproto.PushContextStateRequest) bool {
		return slices.ContainsFunc(p.GetResources(), func(r *agentproto.ContextResource) bool {
			return r.GetInstructionFile() != nil && filepath.Base(r.GetSource()) == "AGENTS.md"
		})
	}
	var pushes []*agentproto.PushContextStateRequest
	require.Eventually(t, func() bool {
		pushes = client.ContextStatePushes()
		return slices.ContainsFunc(pushes, hasAgentsMD)
	}, testutil.WaitMedium, testutil.IntervalFast, "expected an accepted push from the working directory")
	require.Positive(t, rejected.Load())
	first := pushes[0]
	require.True(t, first.GetInitial())
	require.Contains(t, first.GetSnapshotError(), "Agent Plugins could not be loaded")
	require.NotContains(t, first.GetSnapshotError(), "unknown resource body")
}

// The declared command does not exist, so the server is unreadable;
// only the nesting and plugin-prefixed source are checked.
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
		agentsdk.Manifest{Directory: dir},
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
				if r.GetPlugin().GetName() != "acme" {
					continue
				}
				plugin = r
				if servers := r.GetPlugin().GetMcpServers(); len(servers) > 0 {
					server = servers[0]
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

func TestAgent_PluginMCPServersFollowPluginChanges(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pluginsDir := filepath.Join(dir, ".agents", "plugins")
	require.NoError(t, os.MkdirAll(pluginsDir, 0o700))

	//nolint:dogsled // setupAgent returns a wide tuple; we only care about the client.
	_, client, _, _, _ := setupAgent(t,
		agentsdk.Manifest{Directory: dir},
		0,
		func(_ *agenttest.Client, opts *agent.Options) {
			opts.ContextConfig = agentcontextconfig.Config{}
		},
	)

	latestHasServer := func() (pushed, found bool) {
		pushes := client.ContextStatePushes()
		if len(pushes) == 0 {
			return false, false
		}
		for _, r := range pushes[len(pushes)-1].GetResources() {
			for _, server := range r.GetPlugin().GetMcpServers() {
				if server.GetMcpServer() != nil && server.GetSource() == "acme/srv" {
					return true, true
				}
			}
		}
		return true, false
	}
	require.Eventually(t, func() bool {
		pushed, _ := latestHasServer()
		return pushed
	}, testutil.WaitSuperLong, testutil.IntervalFast)

	pluginDir := filepath.Join(pluginsDir, "acme")
	require.NoError(t, os.Mkdir(pluginDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(`{
		"$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
		"name": "acme"
	}`), 0o600))
	require.Eventually(t, func() bool {
		pushes := client.ContextStatePushes()
		for _, r := range pushes[len(pushes)-1].GetResources() {
			if r.GetPlugin().GetName() == "acme" {
				return true
			}
		}
		return false
	}, testutil.WaitSuperLong, testutil.IntervalFast)

	// The plugin's mcp.json arrives after the plugin itself.
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "mcp.json"), []byte(`{
		"$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
		"mcpServers": {
			"srv": {"type": "stdio", "command": "coder-test-missing-mcp-binary"}
		}
	}`), 0o600))

	if !assert.Eventually(t, func() bool {
		_, found := latestHasServer()
		return found
	}, testutil.WaitSuperLong, testutil.IntervalFast) {
		t.Fatalf("expected the added plugin's MCP server to be pushed; got %s",
			describePushes(client.ContextStatePushes()))
	}

	require.NoError(t, os.RemoveAll(pluginDir))
	if !assert.Eventually(t, func() bool {
		_, found := latestHasServer()
		return !found
	}, testutil.WaitSuperLong, testutil.IntervalFast) {
		t.Fatalf("expected the removed plugin's MCP server to leave the push; got %s",
			describePushes(client.ContextStatePushes()))
	}
}

func describePushes(pushes []*agentproto.PushContextStateRequest) string {
	var sb strings.Builder
	for _, push := range pushes {
		var resources []*agentproto.ContextResource
		for _, r := range push.GetResources() {
			resources = append(resources, r)
			resources = append(resources, r.GetPlugin().GetSkills()...)
			resources = append(resources, r.GetPlugin().GetMcpServers()...)
		}
		for _, r := range resources {
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

// The notice is logged only when the manifest has no directory.
func TestAgent_MissingDirectoryNotice(t *testing.T) {
	t.Parallel()

	const notice = "agent directory is not set; skipping working-directory context"
	hasNotice := func(e slog.SinkEntry) bool { return e.Message == notice }

	t.Run("Unset", func(t *testing.T) {
		t.Parallel()
		sink := testutil.NewFakeSink(t)
		//nolint:dogsled // Only the side effect on the logger matters here.
		_, _, _, _, _ = setupAgent(t,
			agentsdk.Manifest{Directory: ""},
			0,
			func(_ *agenttest.Client, opts *agent.Options) {
				opts.Logger = opts.Logger.AppendSinks(sink)
			},
		)

		var entries []slog.SinkEntry
		require.Eventually(t, func() bool {
			entries = sink.Entries(hasNotice)
			return len(entries) > 0
		}, testutil.WaitLong, testutil.IntervalFast)
		require.Equal(t, slog.LevelInfo, entries[0].Level)

		fields := map[string]any{}
		for _, f := range entries[0].Fields {
			fields[f.Name] = f.Value
		}
		notScanned, ok := fields["not_scanned"].([]string)
		require.True(t, ok, "not_scanned must be a []string, got %T", fields["not_scanned"])
		hint, ok := fields["hint"].(string)
		require.True(t, ok, "hint must be a string, got %T", fields["hint"])
		require.Contains(t, notScanned, "AGENTS.md")
		require.Contains(t, notScanned, "plugin.json")
		require.Contains(t, notScanned, filepath.Join(".agents", "plugins"))
		require.Contains(t, hint, "coder_agent.dir")
		require.Contains(t, hint, "~/.coder/.agents/plugins")
	})

	t.Run("OncePerAgent", func(t *testing.T) {
		t.Parallel()
		sink := testutil.NewFakeSink(t)
		//nolint:dogsled // setupAgent returns a wide tuple; we only care about the client.
		_, client, _, _, _ := setupAgent(t,
			agentsdk.Manifest{Directory: ""},
			0,
			func(_ *agenttest.Client, opts *agent.Options) {
				opts.Logger = opts.Logger.AppendSinks(sink)
			},
		)
		// handleManifest returns after the notice point, so each
		// "routine exited" entry for it marks a handled manifest.
		manifestHandled := func(e slog.SinkEntry) bool {
			if e.Message != "routine exited" {
				return false
			}
			return slices.ContainsFunc(e.Fields, func(f slog.Field) bool {
				return f.Name == "name" && f.Value == "handle manifest"
			})
		}
		require.Eventually(t, func() bool {
			return len(sink.Entries(manifestHandled)) == 1
		}, testutil.WaitLong, testutil.IntervalFast)
		require.Len(t, sink.Entries(hasNotice), 1)

		// A reconnect delivers a new manifest without logging again.
		client.LastWorkspaceAgent()
		require.Eventually(t, func() bool {
			return len(sink.Entries(manifestHandled)) == 2
		}, testutil.WaitLong, testutil.IntervalFast, "expected the manifest to be handled on the new connection")
		require.Len(t, sink.Entries(hasNotice), 1)
	})

	t.Run("Set", func(t *testing.T) {
		t.Parallel()
		sink := testutil.NewFakeSink(t)
		//nolint:dogsled // setupAgent returns a wide tuple; we only care about the client.
		_, client, _, _, _ := setupAgent(t,
			agentsdk.Manifest{Directory: t.TempDir()},
			0,
			func(_ *agenttest.Client, opts *agent.Options) {
				opts.Logger = opts.Logger.AppendSinks(sink)
			},
		)

		// The manifest is handled before the first context push.
		require.Eventually(t, func() bool {
			return len(client.ContextStatePushes()) > 0
		}, testutil.WaitLong, testutil.IntervalFast)
		require.Empty(t, sink.Entries(hasNotice))
	})
}

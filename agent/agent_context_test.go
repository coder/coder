package agent_test

import (
	"os"
	"path/filepath"
	"slices"
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

// hasPluginData reports whether req carries a plugin resource or a
// resource attributed to a plugin.
func hasPluginData(req *agentproto.PushContextStateRequest) bool {
	return slices.ContainsFunc(req.GetResources(), func(r *agentproto.ContextResource) bool {
		return r.GetPlugin() != nil || r.GetSkill().GetPluginName() != "" || r.GetMcpServer().GetPluginName() != ""
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

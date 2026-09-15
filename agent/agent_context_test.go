package agent_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// TestAgent_ContextStatePushedPlugins verifies that plugin data reaches
// coderd only when the manifest advertises plugin support.
func TestAgent_ContextStatePushedPlugins(t *testing.T) {
	t.Parallel()

	for _, supported := range []bool{true, false} {
		t.Run(fmt.Sprintf("supported=%t", supported), func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			pluginDir := filepath.Join(dir, "plugins", "p")
			require.NoError(t, os.MkdirAll(pluginDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "plugin.json"),
				[]byte(`{"$schema":"`+agentcontext.PluginSchemaV1+`","name":"p"}`), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("rules"), 0o600))

			//nolint:dogsled // setupAgent returns a wide tuple; we only care about the client.
			_, client, _, _, _ := setupAgent(t,
				agentsdk.Manifest{Directory: dir, PluginsSupported: supported},
				0,
				func(_ *agenttest.Client, opts *agent.Options) {
					opts.ContextConfig = agentcontextconfig.Config{}
				},
			)

			hasPlugin := func(p *agentproto.PushContextStateRequest) bool {
				return slices.ContainsFunc(p.GetResources(), func(r *agentproto.ContextResource) bool {
					return r.GetPlugin() != nil
				})
			}
			hasAgentsMD := func(p *agentproto.PushContextStateRequest) bool {
				return slices.ContainsFunc(p.GetResources(), func(r *agentproto.ContextResource) bool {
					return r.GetInstructionFile() != nil && filepath.Base(r.GetSource()) == "AGENTS.md"
				})
			}

			var pushes []*agentproto.PushContextStateRequest
			require.Eventually(t, func() bool {
				pushes = client.ContextStatePushes()
				if supported {
					return slices.ContainsFunc(pushes, hasPlugin)
				}
				return slices.ContainsFunc(pushes, hasAgentsMD)
			}, testutil.WaitMedium, testutil.IntervalFast, "expected a context push from the working directory")
			if !supported {
				require.False(t, slices.ContainsFunc(pushes, hasPlugin), "plugin data must not be pushed without plugin support")
			}
		})
	}
}

// TestAgent_ContextStatePluginsDroppedAfterReconnect verifies that a
// connection whose manifest does not support plugins never receives
// plugin data, even when the snapshot was resolved while the previous
// connection supported them.
func TestAgent_ContextStatePluginsDroppedAfterReconnect(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins", "p")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "plugin.json"),
		[]byte(`{"$schema":"`+agentcontext.PluginSchemaV1+`","name":"p"}`), 0o600))

	//nolint:dogsled // setupAgent returns a wide tuple; we only care about the client.
	_, client, _, _, _ := setupAgent(t,
		agentsdk.Manifest{Directory: dir, PluginsSupported: true},
		0,
		func(_ *agenttest.Client, opts *agent.Options) {
			opts.ContextConfig = agentcontextconfig.Config{}
		},
	)

	hasPlugin := func(p *agentproto.PushContextStateRequest) bool {
		return slices.ContainsFunc(p.GetResources(), func(r *agentproto.ContextResource) bool {
			return r.GetPlugin() != nil
		})
	}
	require.Eventually(t, func() bool {
		return slices.ContainsFunc(client.ContextStatePushes(), hasPlugin)
	}, testutil.WaitMedium, testutil.IntervalFast, "expected a push with plugin data")

	client.SetPluginsSupported(false)
	client.LastWorkspaceAgent()

	// Each connection's first push carries Initial=true, so the second
	// Initial push is the first one on the new connection.
	var reconnected []*agentproto.PushContextStateRequest
	require.Eventually(t, func() bool {
		pushes := client.ContextStatePushes()
		initials := 0
		for i, p := range pushes {
			if p.GetInitial() {
				initials++
			}
			if initials == 2 {
				reconnected = pushes[i:]
				return true
			}
		}
		return false
	}, testutil.WaitMedium, testutil.IntervalFast, "expected a push on the new connection")
	for _, p := range reconnected {
		require.False(t, hasPlugin(p), "plugin data must not be pushed after plugin support is withdrawn")
	}
}

// TestAgent_MissingDirectoryNotice verifies that an agent logs a warning
// naming the working-directory context it skips only when its manifest
// has no directory.
func TestAgent_MissingDirectoryNotice(t *testing.T) {
	t.Parallel()

	const notice = "agent directory is not set; skipping working-directory context"
	hasNotice := func(e slog.SinkEntry) bool { return e.Message == notice }

	for _, supported := range []bool{false, true} {
		t.Run(fmt.Sprintf("Unset/supported=%t", supported), func(t *testing.T) {
			t.Parallel()
			sink := testutil.NewFakeSink(t)
			//nolint:dogsled // Only the side effect on the logger matters here.
			_, _, _, _, _ = setupAgent(t,
				agentsdk.Manifest{Directory: "", PluginsSupported: supported},
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

			fields := map[string]any{}
			for _, f := range entries[0].Fields {
				fields[f.Name] = f.Value
			}
			notScanned, ok := fields["not_scanned"].([]string)
			require.True(t, ok, "not_scanned must be a []string, got %T", fields["not_scanned"])
			hint, ok := fields["hint"].(string)
			require.True(t, ok, "hint must be a string, got %T", fields["hint"])
			require.Contains(t, notScanned, "AGENTS.md")
			require.Equal(t, supported, slices.Contains(notScanned, "plugin.json"), notScanned)
			require.Equal(t, supported, slices.Contains(notScanned, filepath.Join(".agents", "plugins")), notScanned)
			require.Equal(t, supported, strings.Contains(hint, "~/.coder/.agents/plugins"), hint)
		})
	}

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

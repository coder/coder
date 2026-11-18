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

// writeTestPlugin writes plugin p with skill s under dir.
func writeTestPlugin(t *testing.T, dir string) {
	t.Helper()
	pluginDir := filepath.Join(dir, "plugins", "p")
	require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, "skills", "s"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "plugin.json"),
		[]byte(`{"$schema":"`+agentcontext.PluginSchemaV1+`","name":"p"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "skills", "s", "SKILL.md"),
		[]byte("---\nname: s\ndescription: Plugin skill\n---\n"), 0o600))
}

func hasPluginData(req *agentproto.PushContextStateRequest) bool {
	return slices.ContainsFunc(req.GetResources(), func(r *agentproto.ContextResource) bool {
		return r.GetPlugin() != nil
	})
}

// Plugin data is pushed only when the manifest enables Agent Plugins.
func TestAgent_ContextStatePushedPlugins(t *testing.T) {
	t.Parallel()

	hasNestedSkill := func(req *agentproto.PushContextStateRequest) bool {
		return slices.ContainsFunc(req.GetResources(), func(r *agentproto.ContextResource) bool {
			plugin := r.GetPlugin()
			return plugin.GetName() == "p" && slices.ContainsFunc(plugin.GetSkills(), func(s *agentproto.ContextResource) bool {
				return s.GetSkill().GetName() == "s"
			})
		})
	}
	hasAgentsMD := func(req *agentproto.PushContextStateRequest) bool {
		return slices.ContainsFunc(req.GetResources(), func(r *agentproto.ContextResource) bool {
			return r.GetInstructionFile() != nil && filepath.Base(r.GetSource()) == "AGENTS.md"
		})
	}

	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("Enabled=%t", enabled), func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			writeTestPlugin(t, dir)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("rules"), 0o600))

			//nolint:dogsled // setupAgent returns a wide tuple; we only care about the client.
			_, client, _, _, _ := setupAgent(t,
				agentsdk.Manifest{Directory: dir, AgentPluginsEnabled: enabled},
				0,
				func(_ *agenttest.Client, opts *agent.Options) {
					opts.ContextConfig = agentcontextconfig.Config{}
				},
			)

			if enabled {
				require.Eventually(t, func() bool {
					return slices.ContainsFunc(client.ContextStatePushes(), hasNestedSkill)
				}, testutil.WaitMedium, testutil.IntervalFast, "expected a push with plugin p nesting skill s")
				return
			}
			var pushes []*agentproto.PushContextStateRequest
			require.Eventually(t, func() bool {
				pushes = client.ContextStatePushes()
				return slices.ContainsFunc(pushes, hasAgentsMD)
			}, testutil.WaitMedium, testutil.IntervalFast, "expected a push with AGENTS.md")
			require.False(t, slices.ContainsFunc(pushes, hasPluginData), "plugin data must not be pushed with plugins disabled")
		})
	}
}

// A later manifest that disables Agent Plugins removes plugin data from
// the pushed context.
func TestAgent_ContextStatePluginsDroppedAfterReconnect(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestPlugin(t, dir)

	//nolint:dogsled // setupAgent returns a wide tuple; we only care about the client.
	_, client, _, _, _ := setupAgent(t,
		agentsdk.Manifest{Directory: dir, AgentPluginsEnabled: true},
		0,
		func(_ *agenttest.Client, opts *agent.Options) {
			opts.ContextConfig = agentcontextconfig.Config{}
		},
	)
	require.Eventually(t, func() bool {
		return slices.ContainsFunc(client.ContextStatePushes(), hasPluginData)
	}, testutil.WaitMedium, testutil.IntervalFast, "expected a push with plugin data")

	client.SetAgentPluginsEnabled(false)
	client.LastWorkspaceAgent()

	require.Eventually(t, func() bool {
		pushes := client.ContextStatePushes()
		return !hasPluginData(pushes[len(pushes)-1])
	}, testutil.WaitMedium, testutil.IntervalFast, "expected a push without plugin data")
}

// The notice is logged only when the manifest has no directory.
func TestAgent_MissingDirectoryNotice(t *testing.T) {
	t.Parallel()

	const notice = "agent directory is not set; skipping working-directory context"
	hasNotice := func(e slog.SinkEntry) bool { return e.Message == notice }

	fieldsOf := func(t *testing.T, e slog.SinkEntry) (notScanned []string, hint string) {
		t.Helper()
		for _, f := range e.Fields {
			switch f.Name {
			case "not_scanned":
				v, ok := f.Value.([]string)
				require.True(t, ok, "not_scanned must be a []string, got %T", f.Value)
				notScanned = v
			case "hint":
				v, ok := f.Value.(string)
				require.True(t, ok, "hint must be a string, got %T", f.Value)
				hint = v
			}
		}
		return notScanned, hint
	}

	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("Unset/Enabled=%t", enabled), func(t *testing.T) {
			t.Parallel()
			sink := testutil.NewFakeSink(t)
			//nolint:dogsled // Only the side effect on the logger matters here.
			_, _, _, _, _ = setupAgent(t,
				agentsdk.Manifest{Directory: "", AgentPluginsEnabled: enabled},
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

			notScanned, hint := fieldsOf(t, entries[0])
			require.Contains(t, notScanned, "AGENTS.md")
			require.Equal(t, enabled, slices.Contains(notScanned, "plugin.json"), notScanned)
			require.Equal(t, enabled, slices.Contains(notScanned, filepath.Join(".agents", "plugins")), notScanned)
			require.Contains(t, hint, "coder_agent.dir")
			require.Equal(t, enabled, strings.Contains(hint, "~/.coder/.agents/plugins"), hint)
		})
	}

	t.Run("OncePerPluginSetting", func(t *testing.T) {
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
		waitHandled := func(n int) {
			require.Eventually(t, func() bool {
				return len(sink.Entries(manifestHandled)) == n
			}, testutil.WaitLong, testutil.IntervalFast, "expected %d handled manifests", n)
		}
		waitHandled(1)
		require.Len(t, sink.Entries(hasNotice), 1)

		// A reconnect with the same plugin setting does not log again.
		client.LastWorkspaceAgent()
		waitHandled(2)
		require.Len(t, sink.Entries(hasNotice), 1)

		// A changed plugin setting logs again.
		client.SetAgentPluginsEnabled(true)
		client.LastWorkspaceAgent()
		waitHandled(3)
		entries := sink.Entries(hasNotice)
		require.Len(t, entries, 2)
		_, hint := fieldsOf(t, entries[1])
		require.Contains(t, hint, "~/.coder/.agents/plugins")
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

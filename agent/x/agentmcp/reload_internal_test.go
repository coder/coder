package agentmcp

import (
	"context"
	"encoding/json"
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

// writeMCPConfig writes a .mcp.json file with the given server
// entries. Each entry maps a server name to its config.
func writeMCPConfig(t *testing.T, dir string, servers map[string]mcpServerEntry) string {
	t.Helper()
	path := filepath.Join(dir, ".mcp.json")
	cfg := mcpConfigFile{MCPServers: make(map[string]json.RawMessage)}
	for name, entry := range servers {
		raw, err := json.Marshal(entry)
		require.NoError(t, err)
		cfg.MCPServers[name] = raw
	}
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	err = os.WriteFile(path, data, 0o600)
	require.NoError(t, err)
	return path
}

// fakeMCPServerConfig returns a ServerConfig that launches a fake
// MCP server using the test binary re-exec pattern.
func fakeMCPServerConfig(t *testing.T, name string) (ServerConfig, mcpServerEntry) {
	t.Helper()
	testBin, err := os.Executable()
	require.NoError(t, err)
	cfg := ServerConfig{
		Name:      name,
		Transport: "stdio",
		Command:   testBin,
		Args:      []string{"-test.run=^TestConnectServer_StdioProcessSurvivesConnect$"},
		Env:       map[string]string{"TEST_MCP_FAKE_SERVER": "1"},
	}
	entry := mcpServerEntry{
		Command: testBin,
		Args:    []string{"-test.run=^TestConnectServer_StdioProcessSurvivesConnect$"},
		Env:     map[string]string{"TEST_MCP_FAKE_SERVER": "1"},
	}
	return cfg, entry
}

func TestSnapshotChanged(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name       string
		setup      func(t *testing.T, dir string) []string
		mutate     func(t *testing.T, dir string)
		checkPaths func(t *testing.T, dir string, initialPaths []string) []string
		want       bool
	}

	cases := []testCase{
		{
			name: "UnchangedFiles",
			setup: func(t *testing.T, dir string) []string {
				t.Helper()
				_, entry := fakeMCPServerConfig(t, "srv")
				configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})
				return []string{configPath}
			},
			want: false,
		},
		{
			name: "ContentChange",
			setup: func(t *testing.T, dir string) []string {
				t.Helper()
				_, entry := fakeMCPServerConfig(t, "srv")
				configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})
				return []string{configPath}
			},
			mutate: func(t *testing.T, dir string) {
				t.Helper()
				_, entry2 := fakeMCPServerConfig(t, "srv2")
				writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv2": entry2})
			},
			want: true,
		},
		{
			name: "FileBecomesMissing",
			setup: func(t *testing.T, dir string) []string {
				t.Helper()
				_, entry := fakeMCPServerConfig(t, "srv")
				configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})
				return []string{configPath}
			},
			mutate: func(t *testing.T, dir string) {
				t.Helper()
				require.NoError(t, os.Remove(filepath.Join(dir, ".mcp.json")))
			},
			want: true,
		},
		{
			name: "FileAppears",
			setup: func(t *testing.T, dir string) []string {
				t.Helper()
				return []string{filepath.Join(dir, ".mcp.json")}
			},
			mutate: func(t *testing.T, dir string) {
				t.Helper()
				_, entry := fakeMCPServerConfig(t, "srv")
				writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})
			},
			want: true,
		},
		{
			name: "BothAbsentUnchanged",
			setup: func(t *testing.T, dir string) []string {
				t.Helper()
				return []string{filepath.Join(dir, ".mcp.json")}
			},
			want: false,
		},
		{
			name: "PathSetDiffers",
			setup: func(t *testing.T, dir string) []string {
				t.Helper()
				_, entry := fakeMCPServerConfig(t, "srv")
				configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})
				return []string{configPath}
			},
			checkPaths: func(t *testing.T, dir string, initialPaths []string) []string {
				t.Helper()
				extraPath := filepath.Join(dir, "extra.mcp.json")
				return append(initialPaths, extraPath)
			},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
			dir := t.TempDir()

			paths := tc.setup(t, dir)

			m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
			t.Cleanup(func() { _ = m.Close() })

			err := m.Reload(ctx, paths)
			require.NoError(t, err)

			if tc.mutate != nil {
				tc.mutate(t, dir)
			}

			checkPaths := paths
			if tc.checkPaths != nil {
				checkPaths = tc.checkPaths(t, dir, paths)
			}

			changed := m.SnapshotChanged(checkPaths)
			assert.Equal(t, tc.want, changed)
		})
	}
}

func TestSnapshotChanged_MultipleConfigFiles(t *testing.T) {
	t.Parallel()

	if os.Getenv("TEST_MCP_FAKE_SERVER") == "1" {
		runFakeMCPServer()
		return
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	_, entry1 := fakeMCPServerConfig(t, "srv1")
	_, entry2 := fakeMCPServerConfig(t, "srv2")
	path1 := writeMCPConfig(t, dir1, map[string]mcpServerEntry{"srv1": entry1})
	path2 := writeMCPConfig(t, dir2, map[string]mcpServerEntry{"srv2": entry2})
	paths := []string{path1, path2}

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
	t.Cleanup(func() { _ = m.Close() })

	// Initial reload with both config files.
	err := m.Reload(ctx, paths)
	require.NoError(t, err)

	// Both files unchanged.
	assert.False(t, m.SnapshotChanged(paths),
		"snapshot should not change when both files are unchanged")

	// Mutate only the second file.
	_, entry2b := fakeMCPServerConfig(t, "srv2b")
	writeMCPConfig(t, dir2, map[string]mcpServerEntry{"srv2b": entry2b})

	assert.True(t, m.SnapshotChanged(paths),
		"snapshot should change when second file is mutated")

	// Reload picks up the mutation.
	err = m.Reload(ctx, paths)
	require.NoError(t, err)

	// Tools from both files should be present.
	tools := m.Tools()
	require.Len(t, tools, 2, "should have tools from both config files")
	assert.Contains(t, tools[0].Name, "srv1",
		"first tool should be from first config")
	assert.Contains(t, tools[1].Name, "srv2b",
		"second tool should be from second config")
}

func TestReload(t *testing.T) {
	t.Parallel()

	t.Run("SingleReloadUpdatesSnapshot", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry := fakeMCPServerConfig(t, "srv")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		t.Cleanup(func() { _ = m.Close() })

		err := m.Reload(ctx, []string{configPath})
		require.NoError(t, err)

		tools := m.Tools()
		require.Len(t, tools, 1, "should have one tool from the fake server")
		assert.Contains(t, tools[0].Name, "echo")

		// Snapshot should be fresh.
		assert.False(t, m.SnapshotChanged([]string{configPath}))
	})

	t.Run("ReloadAfterClose", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		require.NoError(t, m.Close())

		err := m.Reload(ctx, []string{"/nonexistent"})
		require.Error(t, err, "reload after close should fail")
	})

	t.Run("ConcurrentReloadsCoalesce", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry := fakeMCPServerConfig(t, "srv")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		t.Cleanup(func() { _ = m.Close() })

		// Launch multiple concurrent reloads.
		const numCallers = 5
		var wg sync.WaitGroup
		errs := make([]error, numCallers)
		for i := range numCallers {
			wg.Go(func() {
				errs[i] = m.Reload(ctx, []string{configPath})
			})
		}
		wg.Wait()

		for i, err := range errs {
			assert.NoError(t, err, "caller %d should not fail", i)
		}

		tools := m.Tools()
		require.Len(t, tools, 1)
	})

	t.Run("CallerContextCanceled", func(t *testing.T) {
		t.Parallel()
		mgrCtx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry := fakeMCPServerConfig(t, "srv")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		m := NewManager(mgrCtx, logger, agentexec.DefaultExecer, nil)
		t.Cleanup(func() { _ = m.Close() })

		// Use an already-canceled caller context.
		callerCtx, cancel := context.WithCancel(mgrCtx)
		cancel() // Cancel immediately.

		err := m.Reload(callerCtx, []string{configPath})
		// The caller context is already canceled, so Reload should
		// return the caller's context error.
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("SequentialReloadsDiffDetect", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry1 := fakeMCPServerConfig(t, "srv1")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv1": entry1})

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		t.Cleanup(func() { _ = m.Close() })

		// First reload.
		err := m.Reload(ctx, []string{configPath})
		require.NoError(t, err)
		tools1 := m.Tools()
		require.Len(t, tools1, 1)
		assert.Contains(t, tools1[0].Name, "srv1")

		// Rewrite config with a different server.
		_, entry2 := fakeMCPServerConfig(t, "srv2")
		writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv2": entry2})

		// Second reload detects the change.
		assert.True(t, m.SnapshotChanged([]string{configPath}))
		err = m.Reload(ctx, []string{configPath})
		require.NoError(t, err)
		tools2 := m.Tools()
		require.Len(t, tools2, 1)
		assert.Contains(t, tools2[0].Name, "srv2")
	})

	t.Run("PerServerConnectFailureUpdatesSnapshot", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		// Config with a nonexistent binary: connect will fail.
		path := filepath.Join(dir, ".mcp.json")
		data := `{"mcpServers":{"bad":{"command":"/nonexistent/binary","args":[]}}}`
		require.NoError(t, os.WriteFile(path, []byte(data), 0o600))

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		t.Cleanup(func() { _ = m.Close() })

		// Reload should succeed (per-server failures are logged and
		// swallowed) and snapshot should update.
		err := m.Reload(ctx, []string{path})
		require.NoError(t, err)
		assert.False(t, m.SnapshotChanged([]string{path}),
			"snapshot should be updated even on per-server connect failure")
	})

	t.Run("EmptyConfigClosesServers", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry := fakeMCPServerConfig(t, "srv")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		t.Cleanup(func() { _ = m.Close() })

		err := m.Reload(ctx, []string{configPath})
		require.NoError(t, err)
		require.Len(t, m.Tools(), 1)

		// Delete config file.
		require.NoError(t, os.Remove(configPath))

		err = m.Reload(ctx, []string{configPath})
		require.NoError(t, err)
		assert.Empty(t, m.Tools(), "tools should be empty after config deleted")

		// Subsequent reload finds snapshot unchanged.
		assert.False(t, m.SnapshotChanged([]string{configPath}))
	})
}

func TestDifferentialReload(t *testing.T) {
	t.Parallel()

	// These tests verify differential reload behavior: client
	// reuse for unchanged servers, reconnect for changed ones,
	// and close for removed ones.

	t.Run("UnchangedServerReusesClient", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry := fakeMCPServerConfig(t, "srv")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		t.Cleanup(func() { _ = m.Close() })

		err := m.Reload(ctx, []string{configPath})
		require.NoError(t, err)

		// Capture the client pointer.
		m.mu.RLock()
		origClient := m.servers["srv"].client
		m.mu.RUnlock()
		require.NotNil(t, origClient)

		// Add a new server without changing the existing one.
		_, entry2 := fakeMCPServerConfig(t, "srv2")
		cfgMap := map[string]mcpServerEntry{"srv": entry, "srv2": entry2}
		writeMCPConfig(t, dir, cfgMap)

		err = m.Reload(ctx, []string{configPath})
		require.NoError(t, err)

		// The unchanged server should reuse the same client.
		m.mu.RLock()
		newClient := m.servers["srv"].client
		m.mu.RUnlock()
		assert.Same(t, origClient, newClient,
			"unchanged server should reuse client pointer")

		// Both servers should have tools.
		tools := m.Tools()
		require.Len(t, tools, 2)
	})

	t.Run("ChangedServerGetsNewClient", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry := fakeMCPServerConfig(t, "srv")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		t.Cleanup(func() { _ = m.Close() })

		err := m.Reload(ctx, []string{configPath})
		require.NoError(t, err)

		m.mu.RLock()
		origClient := m.servers["srv"].client
		m.mu.RUnlock()

		// Change the server's args to trigger a diff.
		entry.Args = append(entry.Args, "-test.v")
		writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		err = m.Reload(ctx, []string{configPath})
		require.NoError(t, err)

		m.mu.RLock()
		newClient := m.servers["srv"].client
		m.mu.RUnlock()
		assert.NotSame(t, origClient, newClient,
			"changed server should get a new client")
	})

	t.Run("RemovedServerIsClosed", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entryA := fakeMCPServerConfig(t, "srvA")
		_, entryB := fakeMCPServerConfig(t, "srvB")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{
			"srvA": entryA, "srvB": entryB,
		})

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		t.Cleanup(func() { _ = m.Close() })

		err := m.Reload(ctx, []string{configPath})
		require.NoError(t, err)
		require.Len(t, m.Tools(), 2)

		// Capture srvB's client before removal.
		m.mu.RLock()
		oldClientB := m.servers["srvB"].client
		m.mu.RUnlock()
		require.NotNil(t, oldClientB)

		// Remove srvB from the config.
		writeMCPConfig(t, dir, map[string]mcpServerEntry{"srvA": entryA})

		err = m.Reload(ctx, []string{configPath})
		require.NoError(t, err)

		tools := m.Tools()
		require.Len(t, tools, 1)
		assert.Contains(t, tools[0].Name, "srvA")

		// The old client for srvB should be closed.
		// ListTools on a closed client returns an error.
		listCtx, cancel := context.WithTimeout(ctx, testutil.WaitShort)
		defer cancel()
		_, listErr := oldClientB.ListTools(listCtx, mcp.ListToolsRequest{})
		assert.Error(t, listErr, "ListTools on closed client should fail")
	})

	t.Run("ConnectFailureRetainsOldClient", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry := fakeMCPServerConfig(t, "srv")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		t.Cleanup(func() { _ = m.Close() })

		err := m.Reload(ctx, []string{configPath})
		require.NoError(t, err)
		require.Len(t, m.Tools(), 1)

		m.mu.RLock()
		origClient := m.servers["srv"].client
		m.mu.RUnlock()

		// Change config to use a bad command, so connect fails.
		path := filepath.Join(dir, ".mcp.json")
		data := `{"mcpServers":{"srv":{"command":"/nonexistent/binary","args":[]}}}`
		require.NoError(t, os.WriteFile(path, []byte(data), 0o600))

		err = m.Reload(ctx, []string{configPath})
		require.NoError(t, err)

		// The old client should be retained because the new connect
		// failed.
		m.mu.RLock()
		currentClient := m.servers["srv"].client
		m.mu.RUnlock()
		assert.Same(t, origClient, currentClient,
			"failed connect should retain old client")

		// Tools should still work.
		tools := m.Tools()
		require.Len(t, tools, 1)
	})

	t.Run("PostReloadToolCallReachesKeptServer", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry := fakeMCPServerConfig(t, "srv")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		t.Cleanup(func() { _ = m.Close() })

		err := m.Reload(ctx, []string{configPath})
		require.NoError(t, err)
		tools := m.Tools()
		require.Len(t, tools, 1)
		toolName := tools[0].Name

		// Add a second server (srv unchanged, so client is reused).
		_, entry2 := fakeMCPServerConfig(t, "srv2")
		writeMCPConfig(t, dir, map[string]mcpServerEntry{
			"srv": entry, "srv2": entry2,
		})

		err = m.Reload(ctx, []string{configPath})
		require.NoError(t, err)

		// A tool call to the kept server should reach it.
		// The client pointer for "srv" was reused, not replaced.
		_, err = m.CallTool(ctx, workspacesdk.CallMCPToolRequest{
			ToolName: toolName,
		})
		// The fake server does not implement tools/call, so we
		// expect an error from the server, but the call itself
		// should reach the server (not ErrUnknownServer).
		require.Error(t, err, "fake server does not implement tools/call")
		assert.NotErrorIs(t, err, ErrUnknownServer,
			"tool call should reach the server, not fail with unknown server")
	})
}

// TestReload_FirstBootPath verifies that the first-boot call site
// (agent.go) can be routed through Reload without behavioral change.
func TestReload_FirstBootPath(t *testing.T) {
	t.Parallel()

	if os.Getenv("TEST_MCP_FAKE_SERVER") == "1" {
		runFakeMCPServer()
		return
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	dir := t.TempDir()

	_, entry := fakeMCPServerConfig(t, "srv")
	configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
	t.Cleanup(func() { _ = m.Close() })

	// Simulate first-boot: Reload with the initial config.
	err := m.Reload(ctx, []string{configPath})
	require.NoError(t, err)

	tools := m.Tools()
	require.Len(t, tools, 1)
	assert.Contains(t, tools[0].Name, "echo")
}

// TestReload_NoopWhenUnchanged verifies that Reload returns
// immediately without reconnecting when the snapshot is fresh.
func TestReload_NoopWhenUnchanged(t *testing.T) {
	t.Parallel()

	if os.Getenv("TEST_MCP_FAKE_SERVER") == "1" {
		runFakeMCPServer()
		return
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	dir := t.TempDir()

	_, entry := fakeMCPServerConfig(t, "srv")
	configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
	t.Cleanup(func() { _ = m.Close() })

	err := m.Reload(ctx, []string{configPath})
	require.NoError(t, err)

	m.mu.RLock()
	origClient := m.servers["srv"].client
	m.mu.RUnlock()

	// Second reload with no changes should be a no-op.
	err = m.Reload(ctx, []string{configPath})
	require.NoError(t, err)

	m.mu.RLock()
	sameClient := m.servers["srv"].client
	m.mu.RUnlock()

	assert.Same(t, origClient, sameClient,
		"no-op reload should not replace the client")
}

// TestClose_SuppressesSubprocessExitError verifies that Close
// returns nil when servers have running subprocesses that exit
// with a kill signal during shutdown.
func TestClose_SuppressesSubprocessExitError(t *testing.T) {
	t.Parallel()

	if os.Getenv("TEST_MCP_FAKE_SERVER") == "1" {
		runFakeMCPServer()
		return
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	dir := t.TempDir()

	_, entry := fakeMCPServerConfig(t, "srv")
	configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
	t.Cleanup(func() { _ = m.Close() })

	err := m.Reload(ctx, []string{configPath})
	require.NoError(t, err)
	require.Len(t, m.Tools(), 1, "server should be connected")

	// Close kills the subprocess. The ExitError guard should
	// suppress the "signal: killed" error.
	err = m.Close()
	assert.NoError(t, err, "Close should not propagate subprocess kill errors")
}

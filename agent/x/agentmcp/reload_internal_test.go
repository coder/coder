package agentmcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
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

	t.Run("UnchangedFiles", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry := fakeMCPServerConfig(t, "srv")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		m := NewManager(ctx, logger)
		t.Cleanup(func() { _ = m.Close() })

		err := m.Reload(ctx, []string{configPath})
		require.NoError(t, err)

		// Same file, no changes: SnapshotChanged must return false.
		changed := m.SnapshotChanged([]string{configPath})
		assert.False(t, changed, "snapshot should not report changes for unchanged files")
	})

	t.Run("ContentChange", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry := fakeMCPServerConfig(t, "srv")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		m := NewManager(ctx, logger)
		t.Cleanup(func() { _ = m.Close() })

		err := m.Reload(ctx, []string{configPath})
		require.NoError(t, err)

		// Rewrite the file with different content.
		_, entry2 := fakeMCPServerConfig(t, "srv2")
		writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv2": entry2})

		changed := m.SnapshotChanged([]string{configPath})
		assert.True(t, changed, "snapshot should detect content changes")
	})

	t.Run("FileBecomesMissing", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry := fakeMCPServerConfig(t, "srv")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		m := NewManager(ctx, logger)
		t.Cleanup(func() { _ = m.Close() })

		err := m.Reload(ctx, []string{configPath})
		require.NoError(t, err)

		// Delete the file.
		require.NoError(t, os.Remove(configPath))

		changed := m.SnapshotChanged([]string{configPath})
		assert.True(t, changed, "snapshot should detect file deletion")
	})

	t.Run("FileAppears", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		missingPath := filepath.Join(dir, ".mcp.json")

		m := NewManager(ctx, logger)
		t.Cleanup(func() { _ = m.Close() })

		// Initial reload with a missing file.
		err := m.Reload(ctx, []string{missingPath})
		require.NoError(t, err)

		// Now create the file.
		_, entry := fakeMCPServerConfig(t, "srv")
		writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		changed := m.SnapshotChanged([]string{missingPath})
		assert.True(t, changed, "snapshot should detect file creation")
	})

	t.Run("BothAbsentUnchanged", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		missingPath := filepath.Join(dir, ".mcp.json")

		m := NewManager(ctx, logger)
		t.Cleanup(func() { _ = m.Close() })

		err := m.Reload(ctx, []string{missingPath})
		require.NoError(t, err)

		// File is still missing: unchanged.
		changed := m.SnapshotChanged([]string{missingPath})
		assert.False(t, changed, "absent-to-absent should be unchanged")
	})

	t.Run("PathSetDiffers", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry := fakeMCPServerConfig(t, "srv")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		m := NewManager(ctx, logger)
		t.Cleanup(func() { _ = m.Close() })

		err := m.Reload(ctx, []string{configPath})
		require.NoError(t, err)

		// Check with a different path set (extra path).
		extraPath := filepath.Join(dir, "extra.mcp.json")
		changed := m.SnapshotChanged([]string{configPath, extraPath})
		assert.True(t, changed, "different path set should be detected as changed")
	})
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

		m := NewManager(ctx, logger)
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

		m := NewManager(ctx, logger)
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

		m := NewManager(ctx, logger)
		t.Cleanup(func() { _ = m.Close() })

		// Launch multiple concurrent reloads.
		const numCallers = 5
		var wg sync.WaitGroup
		errs := make([]error, numCallers)
		for i := range numCallers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs[i] = m.Reload(ctx, []string{configPath})
			}()
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

		m := NewManager(mgrCtx, logger)
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

		m := NewManager(ctx, logger)
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

		m := NewManager(ctx, logger)
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

		m := NewManager(ctx, logger)
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

	// These tests verify D12: differential close behavior.
	// They use the real fake server pattern to confirm client
	// reuse vs. reconnect.

	t.Run("UnchangedServerReusesClient", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry := fakeMCPServerConfig(t, "srv")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		m := NewManager(ctx, logger)
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

		m := NewManager(ctx, logger)
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

		m := NewManager(ctx, logger)
		t.Cleanup(func() { _ = m.Close() })

		err := m.Reload(ctx, []string{configPath})
		require.NoError(t, err)
		require.Len(t, m.Tools(), 2)

		// Remove srvB from the config.
		writeMCPConfig(t, dir, map[string]mcpServerEntry{"srvA": entryA})

		err = m.Reload(ctx, []string{configPath})
		require.NoError(t, err)

		tools := m.Tools()
		require.Len(t, tools, 1)
		assert.Contains(t, tools[0].Name, "srvA")
	})

	t.Run("ConnectFailureRetainsOldClient", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry := fakeMCPServerConfig(t, "srv")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		m := NewManager(ctx, logger)
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

	t.Run("InFlightToolCallSurvivesReload", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry := fakeMCPServerConfig(t, "srv")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		m := NewManager(ctx, logger)
		t.Cleanup(func() { _ = m.Close() })

		err := m.Reload(ctx, []string{configPath})
		require.NoError(t, err)

		// Capture tool list before reload.
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

		// The original server's tool should still be callable.
		// This confirms in-flight compatibility: the client pointer
		// for "srv" was reused.
		resp, err := m.CallTool(ctx, workspacesdk.CallMCPToolRequest{
			ToolName: toolName,
		})
		// The fake server does not implement tools/call, so we
		// expect an error from the server, but the call itself
		// should reach the server (not ErrUnknownServer).
		if err != nil {
			assert.NotErrorIs(t, err, ErrUnknownServer,
				"tool call should reach the server, not fail with unknown server")
		}
		_ = resp
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

	m := NewManager(ctx, logger)
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

	m := NewManager(ctx, logger)
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

package agentmcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// These tests exercise the dual-agent late-file regression: the
// inner sandbox agent settles startup quickly and calls Reload
// while `~/.mcp.json` still does not exist on disk. The host
// agent then writes the file ~20s later. Before this fix, the
// manager cached an empty snapshot and stayed empty until a
// subsequent HTTP call lazily restated the file. With the
// fsnotify-backed watcher, the manager picks up the late file
// without external prompting.

// awaitTools polls cachedTools until the predicate succeeds or
// the context expires. It avoids time.Sleep loops in callers.
func awaitTools(ctx context.Context, t *testing.T, m *Manager, pred func([]workspacesdk.MCPToolInfo) bool) []workspacesdk.MCPToolInfo {
	t.Helper()
	var final []workspacesdk.MCPToolInfo
	testutil.Eventually(ctx, t, func(context.Context) bool {
		final = m.cachedTools()
		return pred(final)
	}, testutil.IntervalFast)
	return final
}

// useFastDebounce shortens the watcher's debounce window so
// real-clock tests do not stall on the 250 ms default. Must be
// called before any Reload arms the watcher.
func useFastDebounce(t *testing.T, m *Manager) {
	t.Helper()
	m.mu.Lock()
	m.watchDebounce = 10 * time.Millisecond
	m.mu.Unlock()
}

func TestWatcher_LateFileTriggersReload(t *testing.T) {
	t.Parallel()

	if os.Getenv("TEST_MCP_FAKE_SERVER") == "1" {
		runFakeMCPServer()
		return
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".mcp.json")

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
	useFastDebounce(t, m)
	m.MarkStartupSettled()
	t.Cleanup(func() { _ = m.Close() })

	// First Reload arms the watcher but finds nothing on disk.
	require.NoError(t, m.Reload(ctx, []string{configPath}))
	require.Empty(t, m.cachedTools(), "manager should start with no tools")

	// Write the file after the manager has already settled. The
	// watcher must observe the Create event, debounce it, and
	// trigger a fresh Reload without any external HTTP call.
	_, entry := fakeMCPServerConfig(t, "srv")
	writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

	tools := awaitTools(ctx, t, m, func(tools []workspacesdk.MCPToolInfo) bool {
		return len(tools) == 1
	})
	require.Len(t, tools, 1)
	assert.Contains(t, tools[0].Name, "echo")

	// The snapshot must now reflect the on-disk file so the
	// next Reload short-circuits.
	assert.False(t, m.SnapshotChanged([]string{configPath}))
}

func TestWatcher_RewriteTriggersReload(t *testing.T) {
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
	useFastDebounce(t, m)
	m.MarkStartupSettled()
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, m.Reload(ctx, []string{configPath}))
	tools := m.cachedTools()
	require.Len(t, tools, 1)
	assert.Contains(t, tools[0].Name, "srv")

	// Overwrite the config with a different server name. The
	// watcher should fire and the cache should reflect the new
	// server without any caller-driven Reload.
	_, entry2 := fakeMCPServerConfig(t, "srv2")
	writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv2": entry2})

	tools = awaitTools(ctx, t, m, func(tools []workspacesdk.MCPToolInfo) bool {
		return len(tools) == 1 && len(tools[0].Name) > 0 &&
			(tools[0].ServerName == "srv2")
	})
	require.Len(t, tools, 1)
	assert.Equal(t, "srv2", tools[0].ServerName)
}

func TestWatcher_RemovalTransitionsToEmpty(t *testing.T) {
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
	useFastDebounce(t, m)
	m.MarkStartupSettled()
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, m.Reload(ctx, []string{configPath}))
	require.Len(t, m.cachedTools(), 1)

	require.NoError(t, os.Remove(configPath))

	awaitTools(ctx, t, m, func(tools []workspacesdk.MCPToolInfo) bool {
		return len(tools) == 0
	})
	assert.Empty(t, m.cachedTools())
}

// TestWatcher_DebouncesBurst uses the quartz mock clock to
// confirm that two writes inside a single debounce window
// produce exactly one onChange invocation. This is the
// guarantee that lets the watcher coalesce editor-style
// multi-event writes (write + chmod + rename) into a single
// Reload.
func TestWatcher_DebouncesBurst(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	mClock := quartz.NewMock(t)

	var fires atomic.Int64
	fired := make(chan struct{}, 4)
	cw, err := newConfigWatcher(logger, mClock, 100*time.Millisecond, func() {
		fires.Add(1)
		fired <- struct{}{}
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = cw.Close() })

	dir := t.TempDir()
	target := filepath.Join(dir, ".mcp.json")
	cw.Sync([]string{target})

	// First write: simulate two fsnotify events landing within
	// the debounce window. We do this by directly calling
	// scheduleFire, which is exactly what handleEvent does for
	// each matching event.
	cw.scheduleFire()
	cw.scheduleFire()
	cw.scheduleFire()

	// Before the timer fires, no callback should have run.
	require.Equal(t, int64(0), fires.Load())

	// Advance past the debounce window. Only one fire is
	// expected because all three scheduleFire calls reused the
	// same timer.
	_, waiter := mClock.AdvanceNext()
	waiter.MustWait(testutil.Context(t, testutil.WaitShort))

	select {
	case <-fired:
	case <-time.After(testutil.WaitShort):
		t.Fatal("expected one fire after debounce window")
	}

	// Drain any spurious extra fire briefly.
	select {
	case <-fired:
		t.Fatal("unexpected additional fire within debounce window")
	default:
	}
	require.Equal(t, int64(1), fires.Load())

	// A second burst after the first window settles must fire
	// again (debounce per-window, not global).
	cw.scheduleFire()
	cw.scheduleFire()
	_, waiter = mClock.AdvanceNext()
	waiter.MustWait(testutil.Context(t, testutil.WaitShort))

	select {
	case <-fired:
	case <-time.After(testutil.WaitShort):
		t.Fatal("expected fire after second window")
	}
	require.Equal(t, int64(2), fires.Load())
}

// TestWatcher_CloseStopsGoroutine asserts that Close releases the
// fsnotify watcher fd and stops its goroutine. We rely on the
// race detector and on creating a fresh manager on the same path
// to surface fd or goroutine leaks.
func TestWatcher_CloseStopsGoroutine(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".mcp.json")

	for i := 0; i < 5; i++ {
		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		useFastDebounce(t, m)
		m.MarkStartupSettled()
		require.NoError(t, m.Reload(ctx, []string{configPath}))
		require.NoError(t, m.Close())

		// After Close the watcher field is cleared and the
		// fsnotify watcher is shut down.
		m.mu.RLock()
		w := m.watcher
		m.mu.RUnlock()
		require.Nil(t, w, "watcher must be nil after Close")
	}
}

// TestWatcher_DualAgentHTTPNoStall mimics the dual-agent
// workspace scenario from workspace-otto-aa16: the inner sandbox
// agent calls MarkStartupSettled and Reload while the host agent
// has not yet written ~/.mcp.json. Once the file appears, an
// HTTP request to /tools must return the MCP tools quickly
// instead of triggering a multi-second "reload canceled" stall.
func TestWatcher_DualAgentHTTPNoStall(t *testing.T) {
	t.Parallel()

	if os.Getenv("TEST_MCP_FAKE_SERVER") == "1" {
		runFakeMCPServer()
		return
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".mcp.json")

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
	useFastDebounce(t, m)
	m.MarkStartupSettled()
	t.Cleanup(func() { _ = m.Close() })

	// First Reload races ahead of the host agent: empty config.
	require.NoError(t, m.Reload(ctx, []string{configPath}))
	require.Empty(t, m.cachedTools())

	api := NewAPI(logger, m, func() []string { return []string{configPath} })

	// Host agent writes the file later.
	_, entry := fakeMCPServerConfig(t, "srv")
	writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

	// Wait for the watcher to pick up the file so we know the
	// cache is warm before issuing the HTTP request.
	awaitTools(ctx, t, m, func(tools []workspacesdk.MCPToolInfo) bool {
		return len(tools) == 1
	})

	req := httptest.NewRequest(http.MethodGet, "/tools", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	start := time.Now()
	api.Routes().ServeHTTP(rec, req)
	elapsed := time.Since(start)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Less(t, elapsed, testutil.WaitShort,
		"warm HTTP request should not stall on watcher reload; took %s", elapsed)

	var resp workspacesdk.ListMCPToolsResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Len(t, resp.Tools, 1)
	assert.Contains(t, resp.Tools[0].Name, "echo")
}

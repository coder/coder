package agentmcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// These tests exercise the dual-agent late-file regression: the
// inner sandbox agent settles startup quickly and calls Reload
// while `~/.mcp.json` still does not exist on disk. The host
// agent then writes the file ~20s later. Before this fix, the
// manager cached an empty snapshot and stayed empty until a
// subsequent HTTP call lazily re-statted the file. With the
// fsnotify-backed watcher, the manager picks up the late file
// without external prompting.

// awaitTools polls connectedTools until the predicate succeeds or
// the context expires. It avoids time.Sleep loops in callers.
func awaitTools(ctx context.Context, t *testing.T, m *Manager, pred func([]catalogTool) bool) []catalogTool {
	t.Helper()
	var final []catalogTool
	testutil.Eventually(ctx, t, func(context.Context) bool {
		final = m.connectedTools()
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
	t.Cleanup(func() { _ = m.Close() })

	// First Reload arms the watcher but finds nothing on disk.
	require.NoError(t, m.Reload(ctx, []string{configPath}))
	require.Empty(t, m.connectedTools(), "manager should start with no tools")

	// Write the file after the manager has already settled. The
	// watcher must observe the Create event, debounce it, and
	// trigger a fresh Reload without any external HTTP call.
	_, entry := fakeMCPServerConfig(t, "srv")
	writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

	tools := awaitTools(ctx, t, m, func(tools []catalogTool) bool {
		return len(tools) == 1
	})
	require.Len(t, tools, 1)
	assert.Equal(t, "echo", tools[0].tool)

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
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, m.Reload(ctx, []string{configPath}))
	tools := m.connectedTools()
	require.Len(t, tools, 1)
	assert.Equal(t, "srv", tools[0].server)

	// Overwrite the config with a different server name. The
	// watcher should fire and the cache should reflect the new
	// server without any caller-driven Reload.
	_, entry2 := fakeMCPServerConfig(t, "srv2")
	writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv2": entry2})

	tools = awaitTools(ctx, t, m, func(tools []catalogTool) bool {
		return len(tools) == 1 && tools[0].server == "srv2"
	})
	require.Len(t, tools, 1)
	assert.Equal(t, "srv2", tools[0].server)
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
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, m.Reload(ctx, []string{configPath}))
	require.Len(t, m.connectedTools(), 1)

	require.NoError(t, os.Remove(configPath))

	awaitTools(ctx, t, m, func(tools []catalogTool) bool {
		return len(tools) == 0
	})
	assert.Empty(t, m.connectedTools())
}

// TestWatcher_DebouncesBurst uses the quartz mock clock to
// confirm that three writes inside a single debounce window
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

	// First burst: simulate three fsnotify events landing within
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

	for range 5 {
		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		useFastDebounce(t, m)
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

// TestWatcher_DualAgentLateConfigWarmsCatalog mimics the dual-agent
// workspace scenario from workspace-otto-aa16: the inner sandbox
// agent Reloads while the host agent has not yet written
// ~/.mcp.json. Once the file appears, the config watcher must pick
// it up and warm the catalog so the tools surface without a
// multi-second "reload canceled" stall, and reading the catalog
// must never block on an in-flight reload.
func TestWatcher_DualAgentLateConfigWarmsCatalog(t *testing.T) {
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
	t.Cleanup(func() { _ = m.Close() })

	// First Reload races ahead of the host agent: empty config.
	require.NoError(t, m.Reload(ctx, []string{configPath}))
	require.Empty(t, m.connectedTools())

	// Host agent writes the file later. The watcher must pick it up
	// and warm the catalog.
	_, entry := fakeMCPServerConfig(t, "srv")
	writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

	tools := awaitTools(ctx, t, m, func(tools []catalogTool) bool {
		return len(tools) == 1
	})
	require.Len(t, tools, 1)
	assert.Equal(t, "echo", tools[0].tool)

	// Reading the catalog never blocks on a reload.
	start := time.Now()
	_ = m.Catalog()
	require.Less(t, time.Since(start), testutil.WaitShort,
		"reading the catalog must not block on watcher reload")
}

// TestWatcher_LateParentDirTriggersReload exercises the
// ancestor-walk-up branch (handleEvent re-arm path,
// armAncestorLocked walk-up). The watcher is started with the
// final parent directory missing; once that directory is
// created, the watcher must promote its watch deeper and then
// fire on the file write.
func TestWatcher_LateParentDirTriggersReload(t *testing.T) {
	t.Parallel()

	if os.Getenv("TEST_MCP_FAKE_SERVER") == "1" {
		runFakeMCPServer()
		return
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	root := t.TempDir()
	// Parent directory does not exist yet: armAncestorLocked
	// will watch root instead.
	missing := filepath.Join(root, "config")
	configPath := filepath.Join(missing, ".mcp.json")

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
	useFastDebounce(t, m)
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, m.Reload(ctx, []string{configPath}))
	require.Empty(t, m.connectedTools())

	// Create the missing parent directory. fsnotify will deliver
	// a Create event on root; handleEvent must release the root
	// watch, re-arm on the new parent, and schedule a reload.
	require.NoError(t, os.MkdirAll(missing, 0o755))

	_, entry := fakeMCPServerConfig(t, "srv")
	writeMCPConfig(t, missing, map[string]mcpServerEntry{"srv": entry})

	tools := awaitTools(ctx, t, m, func(tools []catalogTool) bool {
		return len(tools) == 1
	})
	require.Len(t, tools, 1)
	assert.Equal(t, "echo", tools[0].tool)
}

// TestWatcher_SharedParentRefcount covers the multi-path
// directory-watch refcount path: two configured paths in the
// same parent dir should produce a single fsnotify watch, and
// removing one path via a subsequent Sync must keep the
// remaining path armed.
func TestWatcher_SharedParentRefcount(t *testing.T) {
	t.Parallel()

	if os.Getenv("TEST_MCP_FAKE_SERVER") == "1" {
		runFakeMCPServer()
		return
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	// On macOS, t.TempDir() lives under /var which is a symlink
	// to /private/var. The watcher canonicalizes paths before
	// storing parent-dir keys in w.dirs, so the test must look up
	// the resolved form to match.
	dir := testutil.TempDirResolved(t)
	pathA := filepath.Join(dir, "a.mcp.json")
	pathB := filepath.Join(dir, "b.mcp.json")

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
	useFastDebounce(t, m)
	t.Cleanup(func() { _ = m.Close() })

	// First Reload arms both paths, sharing the dir watch.
	require.NoError(t, m.Reload(ctx, []string{pathA, pathB}))

	m.mu.RLock()
	w := m.watcher
	m.mu.RUnlock()
	require.NotNil(t, w, "watcher must be armed")

	w.mu.Lock()
	require.Equal(t, 2, len(w.files), "two files tracked")
	require.Equal(t, 1, len(w.dirs), "shared parent dir")
	require.Equal(t, 2, w.dirs[dir], "refcount equals number of files")
	w.mu.Unlock()

	// Second Reload removes pathB, so the dir refcount drops to
	// 1 but the watch must remain in place for pathA.
	require.NoError(t, m.Reload(ctx, []string{pathA}))

	w.mu.Lock()
	require.Equal(t, 1, len(w.files), "one file tracked after removal")
	require.Equal(t, 1, w.dirs[dir], "refcount decremented but not zero")
	w.mu.Unlock()

	// Writing pathA should still trigger a reload via the
	// surviving dir watch.
	_, entry := fakeMCPServerConfig(t, "srv")
	cfg := mcpConfigFile{MCPServers: make(map[string]json.RawMessage)}
	raw, err := json.Marshal(entry)
	require.NoError(t, err)
	cfg.MCPServers["srv"] = raw
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(pathA, data, 0o600))

	tools := awaitTools(ctx, t, m, func(tools []catalogTool) bool {
		return len(tools) == 1
	})
	require.Len(t, tools, 1)
}

// TestWatcher_CloseDoesNotStallOnInFlightReload guards the
// shutdown-ordering invariant: Close() must mark the manager
// closed before w.Close() so an in-flight watcher-driven Reload
// short-circuits instead of blocking firesWG.Wait() for the full
// connect timeout. Without the ordering, this test would block
// at Close() for ~30 s.
//
// The test installs a connectStartedHook that signals when a
// watcher-driven reload has reached connectAll and then blocks
// until released. While the hook is blocking the singleflight
// reload goroutine, the test calls Close() and asserts it
// returns quickly: the DEREM-5 ordering ensures m.closedCh is
// closed before w.Close()'s firesWG.Wait(), so waitReload
// observes the close, fire() returns, and firesWG drains. If
// the ordering is reverted, w.Close() blocks on firesWG.Wait()
// while fire() is stuck inside waitReload waiting for the
// connect that will never finish.
func TestWatcher_CloseDoesNotStallOnInFlightReload(t *testing.T) {
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

	// Arm the watcher with an initial empty Reload. We install the
	// hook after this so the first connectAll (with empty
	// toConnect) is not blocked.
	require.NoError(t, m.Reload(ctx, []string{configPath}))

	reached := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseHook := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseHook)

	m.mu.Lock()
	var hookOnce sync.Once
	m.connectStartedHook = func() {
		hookOnce.Do(func() { close(reached) })
		<-release
	}
	m.mu.Unlock()

	// Write the file. The watcher will fire a debounced reload
	// that hits the connectStartedHook and blocks there.
	_, entry := fakeMCPServerConfig(t, "srv")
	writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

	select {
	case <-reached:
	case <-time.After(testutil.WaitLong):
		t.Fatal("watcher-driven reload never reached connectAll")
	}

	// Reload is in-flight: connectAll is blocked inside the hook,
	// the singleflight body has not returned, and fire() is
	// blocked in waitReload. Now call Close. With the correct
	// ordering (m.closedCh closed before w.Close()), this returns
	// quickly even though the hook is still blocking.
	done := make(chan error, 1)
	go func() { done <- m.Close() }()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(testutil.WaitMedium):
		t.Fatal("Close stalled; ordering bug: w.Close before m.closed=true")
	}

	// Release the hook so the leaked singleflight goroutine can
	// drain. The manager is already closed, so its work has no
	// observable effect.
	releaseHook()
}

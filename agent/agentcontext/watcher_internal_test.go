package agentcontext

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

// A removed directory whose re-add fails must not stay marked as
// watched, or later Syncs skip it and its edits are missed.
func TestWatcherSync_RetriesFailedReAdd(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("only inotify checks read permission when re-adding an existing watch")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the read permission a watch requires")
	}

	dir := testutil.TempDirResolved(t)
	skillDir := filepath.Join(dir, ".agents", "skills", "s")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	t.Cleanup(func() { _ = os.Chmod(skillDir, 0o755) })

	w, err := NewWatcher(WatcherOptions{
		Logger:   testutil.Logger(t).Named("watcher"),
		OnChange: func() {},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })

	ctx := testutil.Context(t, testutil.WaitShort)
	roots := []ScanRoot{{Path: dir}}
	w.Sync(ctx, roots, ResolveOptions{})
	require.Contains(t, *w.watched.Load(), skillDir)

	// Simulate the directory being replaced by one the watcher cannot
	// read, so the re-add fails.
	require.NoError(t, os.Chmod(skillDir, 0o000))
	w.eventMu.Lock()
	w.removed = map[string]struct{}{skillDir: {}}
	w.eventMu.Unlock()
	w.Sync(ctx, roots, ResolveOptions{})
	require.NotContains(t, *w.watched.Load(), skillDir)

	require.NoError(t, os.Chmod(skillDir, 0o755))
	w.Sync(ctx, roots, ResolveOptions{})
	require.Contains(t, *w.watched.Load(), skillDir)
}

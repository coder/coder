package agentcontext_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestWatcher_FiresOnAgentsMdEdit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("v1"), 0o600))

	var fires atomic.Int32
	w, err := agentcontext.NewWatcher(agentcontext.WatcherOptions{
		Logger:   testutil.Logger(t).Named("watcher"),
		Clock:    quartz.NewReal(),
		Debounce: 10 * time.Millisecond,
		OnChange: func() { fires.Add(1) },
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })

	ctx := testutil.Context(t, testutil.WaitShort)
	w.Sync(ctx, []agentcontext.ScanRoot{{Path: dir}})

	// Rewrite the file inside Eventually so the test does not race
	// fsnotify's watch-setup window. As soon as the watch is live,
	// the next write fires the debounce timer.
	require.Eventually(t, func() bool {
		_ = os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("v2"), 0o600)
		return fires.Load() >= 1
	}, testutil.WaitShort, testutil.IntervalFast, "expected at least one fire after AGENTS.md edit")
}

func TestWatcher_FiresOnNewSkillFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	skillsRoot := filepath.Join(dir, ".agents", "skills")
	require.NoError(t, os.MkdirAll(skillsRoot, 0o755))

	var fires atomic.Int32
	w, err := agentcontext.NewWatcher(agentcontext.WatcherOptions{
		Logger:   testutil.Logger(t).Named("watcher"),
		Debounce: 10 * time.Millisecond,
		OnChange: func() { fires.Add(1) },
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })

	ctx := testutil.Context(t, testutil.WaitShort)
	w.Sync(ctx, []agentcontext.ScanRoot{{Path: dir}})

	// Create SKILL.md inside Eventually so the test does not race
	// fsnotify's watch-setup window. The Manager pre-creates the
	// skill dir, then rewrites SKILL.md each tick until the watcher
	// fires at least once.
	skillDir := filepath.Join(skillsRoot, "foo")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.Eventually(t, func() bool {
		_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: foo\ndescription: bar\n---\nbody"), 0o600)
		return fires.Load() >= 1
	}, testutil.WaitShort, testutil.IntervalFast, "expected fire after SKILL.md create")
}

func TestWatcher_CloseIsIdempotent(t *testing.T) {
	t.Parallel()
	w, err := agentcontext.NewWatcher(agentcontext.WatcherOptions{
		Logger:   testutil.Logger(t).Named("watcher"),
		OnChange: func() {},
	})
	require.NoError(t, err)
	require.NoError(t, w.Close())
	require.NoError(t, w.Close())
}

func TestWatcher_SyncAfterCloseNoop(t *testing.T) {
	t.Parallel()
	w, err := agentcontext.NewWatcher(agentcontext.WatcherOptions{
		Logger:   testutil.Logger(t).Named("watcher"),
		OnChange: func() {},
	})
	require.NoError(t, err)
	require.NoError(t, w.Close())

	// Must not panic.
	w.Sync(context.Background(), []agentcontext.ScanRoot{{Path: t.TempDir()}})
}

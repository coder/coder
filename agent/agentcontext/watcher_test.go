package agentcontext_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
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
	w.Sync(ctx, []agentcontext.ScanRoot{{Path: dir}}, agentcontext.ResolveOptions{})

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
	w.Sync(ctx, []agentcontext.ScanRoot{{Path: dir}}, agentcontext.ResolveOptions{})

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

func TestWatcher_FiresOnPluginSkillFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, ".agents", "plugins", "p")
	mustWritePlugin(t, pluginDir, "p")
	// The skill directory exists before Sync so the test exercises
	// the watch on the plugin's immediate skill directories, not
	// the directory-create path on its skills container.
	skillDir := filepath.Join(pluginDir, "skills", "foo")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	var fires atomic.Int32
	w, err := agentcontext.NewWatcher(agentcontext.WatcherOptions{
		Logger:   testutil.Logger(t).Named("watcher"),
		Debounce: 10 * time.Millisecond,
		OnChange: func() { fires.Add(1) },
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })

	ctx := testutil.Context(t, testutil.WaitShort)
	w.Sync(ctx, []agentcontext.ScanRoot{{Path: dir}}, agentcontext.ResolveOptions{PluginsEnabled: true})

	// Write SKILL.md inside Eventually so the test does not race
	// fsnotify's watch-setup window.
	require.Eventually(t, func() bool {
		_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: foo\ndescription: bar\n---\nbody"), 0o600)
		return fires.Load() >= 1
	}, testutil.WaitShort, testutil.IntervalFast, "expected fire after plugin SKILL.md write")
}

func TestWatcher_FiresOnPluginManifestEdit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins", "p")
	mustWritePlugin(t, pluginDir, "p")

	var fires atomic.Int32
	w, err := agentcontext.NewWatcher(agentcontext.WatcherOptions{
		Logger:   testutil.Logger(t).Named("watcher"),
		Debounce: 10 * time.Millisecond,
		OnChange: func() { fires.Add(1) },
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })

	ctx := testutil.Context(t, testutil.WaitShort)
	w.Sync(ctx, []agentcontext.ScanRoot{{Path: dir}}, agentcontext.ResolveOptions{PluginsEnabled: true})

	manifest := pluginJSON(t, map[string]any{"name": "p", "version": "2"})
	require.Eventually(t, func() bool {
		_ = os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o600)
		return fires.Load() >= 1
	}, testutil.WaitShort, testutil.IntervalFast, "expected fire after plugin.json edit")
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
	w.Sync(context.Background(), []agentcontext.ScanRoot{{Path: t.TempDir()}}, agentcontext.ResolveOptions{})
}

func TestWatcher_FiresOnNestedPluginContainerCreate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".agents", "skills", "s"), 0o755))

	var fires atomic.Int32
	w, err := agentcontext.NewWatcher(agentcontext.WatcherOptions{
		Logger:   testutil.Logger(t).Named("watcher"),
		Debounce: 10 * time.Millisecond,
		OnChange: func() { fires.Add(1) },
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })

	ctx := testutil.Context(t, testutil.WaitShort)
	w.Sync(ctx, []agentcontext.ScanRoot{{Path: dir}}, agentcontext.ResolveOptions{PluginsEnabled: true})

	// Toggle the container inside Eventually so the test does not
	// race fsnotify's watch-setup window.
	container := filepath.Join(dir, ".agents", "plugins")
	require.Eventually(t, func() bool {
		if err := os.Mkdir(container, 0o755); err != nil {
			_ = os.Remove(container)
		}
		return fires.Load() >= 1
	}, testutil.WaitShort, testutil.IntervalFast, "expected fire after .agents/plugins create")
}

func TestWatcher_PluginDirsWatchedOnlyWhenEnabled(t *testing.T) {
	t.Parallel()
	dir := testutil.TempDirResolved(t)
	pluginDir := filepath.Join(dir, "plugins", "auth")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".agents"), 0o755))
	roots := []agentcontext.ScanRoot{{Path: dir}}

	off := agentcontext.WatchDirs(roots, agentcontext.ResolveOptions{})
	require.NotContains(t, off, filepath.Join(dir, "plugins"))
	require.NotContains(t, off, pluginDir)
	require.NotContains(t, off, filepath.Join(dir, ".agents"))

	on := agentcontext.WatchDirs(roots, agentcontext.ResolveOptions{PluginsEnabled: true})
	require.Contains(t, on, filepath.Join(dir, "plugins"))
	require.Contains(t, on, pluginDir)
	require.Contains(t, on, filepath.Join(dir, ".agents"), "parent of the missing .agents/plugins")
}

// A plugin candidate directory is watched so a new plugin.json is
// seen, but other edits in it do not re-resolve, and its skills are
// watched only once it holds a plugin.json.
func TestWatcher_PluginCandidateEventsFiltered(t *testing.T) {
	t.Parallel()
	dir := testutil.TempDirResolved(t)
	pluginDir := filepath.Join(dir, "plugins", "auth")
	skillDir := filepath.Join(pluginDir, "skills", "s")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	roots := []agentcontext.ScanRoot{{Path: dir}}
	opts := agentcontext.ResolveOptions{PluginsEnabled: true}

	require.NotContains(t, agentcontext.WatchDirs(roots, opts), skillDir)
	relevant := func(name string, op fsnotify.Op) bool {
		return agentcontext.WatchEventRelevant(roots, opts, fsnotify.Event{Name: filepath.Join(pluginDir, name), Op: op})
	}
	require.False(t, relevant("main.go", fsnotify.Create))
	require.False(t, relevant("main.go", fsnotify.Write))
	require.False(t, relevant("AGENTS.md", fsnotify.Write))
	require.True(t, relevant("plugin.json", fsnotify.Create))
	require.True(t, relevant("skills", fsnotify.Remove))
	require.True(t, agentcontext.WatchEventRelevant(roots, opts,
		fsnotify.Event{Name: filepath.Join(dir, "plugins", "new"), Op: fsnotify.Create}))

	mustWritePlugin(t, pluginDir, "auth")
	require.Contains(t, agentcontext.WatchDirs(roots, opts), skillDir)
}

// A scan root that is a plugin has its skills/ read through the plugin
// rules, so an in-root symlinked skill directory there is watched.
func TestWatcher_RootPluginSymlinkedSkillDirWatched(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}
	t.Parallel()
	root := testutil.TempDirResolved(t)
	mustWritePlugin(t, root, "rp")
	shared := filepath.Join(root, "shared", "s")
	require.NoError(t, os.MkdirAll(shared, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "skills"), 0o755))
	link := filepath.Join(root, "skills", "s")
	require.NoError(t, os.Symlink(shared, link))
	roots := []agentcontext.ScanRoot{{Path: root}}

	on := agentcontext.WatchDirs(roots, agentcontext.ResolveOptions{PluginsEnabled: true})
	require.Contains(t, on, link)

	off := agentcontext.WatchDirs(roots, agentcontext.ResolveOptions{})
	require.NotContains(t, off, link)
}

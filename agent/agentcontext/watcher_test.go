package agentcontext_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

// TestWatcher_SyncDoesNotDeadlockOnActiveDirectory is a regression
// test for a Windows-only deadlock in Sync. Before the fix, Sync
// held the watcher mutex while calling fsnotify.Watcher.Add. On
// Windows fsnotify routes Add through a thread-pinned reader
// goroutine; the reader can only process the next Add after it
// delivers any pending event to the Events channel. The Events
// consumer is the watcher's run goroutine, which acquires the same
// mutex via schedule(). The combination produced a cycle:
//
//	Sync (holds mu)
//	  -> fsnotify.Add -> waits for reader reply
//	fsnotify reader
//	  -> sendEvent -> waits to send into Events
//	Watcher.run
//	  -> schedule() -> waits for mu
//
// The bug surfaced as TestAgent_Startup/HomeDirectory and
// /HomeEnvironmentVariable hanging until the 20m test timeout
// fired on the Windows nightly-gauntlet test-go-pg job. This
// test reliably reproduces the cycle on Windows by rewriting a
// single watched file while issuing repeated Sync calls. The
// rewrites keep the fsnotify Events channel non-empty, exposing
// the lock-during-Add window. On Linux and macOS the deadlock
// cannot manifest because their fsnotify backends do not route
// Add through the reader, but the test still exercises the
// diff-then-apply logic and confirms the watch set converges
// under churn.
func TestWatcher_SyncDoesNotDeadlockOnActiveDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("v1"), 0o600))

	w, err := agentcontext.NewWatcher(agentcontext.WatcherOptions{
		Logger:   testutil.Logger(t).Named("watcher"),
		Debounce: 10 * time.Millisecond,
		OnChange: func() {},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })

	ctx := testutil.Context(t, testutil.WaitMedium)
	w.Sync(ctx, []agentcontext.ScanRoot{{Path: root}})

	// Rewrite AGENTS.md in a tight loop so the fsnotify Events
	// channel always has something pending. Rewriting an existing
	// file (rather than creating new ones) keeps the directory
	// shape stable so each Sync walks the same small tree.
	var (
		stop atomic.Bool
		wg   sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; !stop.Load(); i++ {
			_ = os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(fmt.Sprintf("v%d", i)), 0o600)
		}
	}()
	t.Cleanup(func() {
		stop.Store(true)
		wg.Wait()
	})

	// Repeatedly re-sync while events flow. Under the previous
	// code, any one of these Sync calls could deadlock on Windows;
	// the surrounding ctx deadline catches the hang.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			w.Sync(ctx, []agentcontext.ScanRoot{{Path: root}})
			if ctx.Err() != nil {
				return
			}
		}
	}()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("Sync deadlocked under filesystem event churn: %v", ctx.Err())
	}
}

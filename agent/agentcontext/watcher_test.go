package agentcontext_test

import (
	"context"
	"fmt"
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

func TestWatcher_FiresOnChildRepoInstructionFile(t *testing.T) {
	t.Parallel()
	for _, marker := range []string{"git-directory", "git-file", "CLAUDE.md", ".cursorrules"} {
		t.Run(marker, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			child := filepath.Join(dir, "repo")
			switch marker {
			case "git-directory":
				require.NoError(t, os.MkdirAll(filepath.Join(child, ".git"), 0o755))
			case "git-file":
				mustWriteFile(t, filepath.Join(child, ".git"), "gitdir: elsewhere")
			default:
				mustWriteFile(t, filepath.Join(child, marker), "rules")
			}
			fired := make(chan struct{}, 1)
			w, err := agentcontext.NewWatcher(agentcontext.WatcherOptions{
				Logger:   testutil.Logger(t).Named("watcher"),
				Debounce: 10 * time.Millisecond,
				OnChange: func() {
					select {
					case fired <- struct{}{}:
					default:
					}
				},
			})
			require.NoError(t, err)
			t.Cleanup(func() { _ = w.Close() })

			ctx := testutil.Context(t, testutil.WaitShort)
			w.Sync(ctx, []agentcontext.ScanRoot{{Path: dir, ChildProjects: true}})
			mustWriteFile(t, filepath.Join(child, "AGENTS.md"), "new rules")
			select {
			case <-fired:
			case <-ctx.Done():
				require.Fail(t, "expected callback after child AGENTS.md create")
			}
		})
	}
}

// TestWatcher_ChildCapPrefersInstructionFiles fills the child cap with
// repositories that have no instruction file yet and checks that a later
// child the resolver would publish is still watched.
func TestWatcher_ChildCapPrefersInstructionFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for i := range 64 {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, fmt.Sprintf("clone-%02d", i), ".git"), 0o755))
	}
	published := filepath.Join(dir, "zz-published")
	mustWriteFile(t, filepath.Join(published, "AGENTS.md"), "rules")

	fired := make(chan struct{}, 1)
	w, err := agentcontext.NewWatcher(agentcontext.WatcherOptions{
		Logger:   testutil.Logger(t).Named("watcher"),
		Debounce: 10 * time.Millisecond,
		OnChange: func() {
			select {
			case fired <- struct{}{}:
			default:
			}
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })

	ctx := testutil.Context(t, testutil.WaitShort)
	w.Sync(ctx, []agentcontext.ScanRoot{{Path: dir, ChildProjects: true}})
	mustWriteFile(t, filepath.Join(published, "AGENTS.md"), "edited rules")
	select {
	case <-fired:
	case <-ctx.Done():
		require.Fail(t, "expected callback after editing a published child's AGENTS.md")
	}
}

// TestWatcher_ChildCapIgnoresWrongCaseNames fills the working directory with
// children that hold only a lower-case agents.md, which the resolver does not
// publish, and checks that a later child with an exact AGENTS.md is still
// watched: on a case-insensitive file system a fixed-name probe hits the
// lower-case files, so only the listing check keeps them out of the slots.
func TestWatcher_ChildCapIgnoresWrongCaseNames(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for i := range 64 {
		mustWriteFile(t, filepath.Join(dir, fmt.Sprintf("docs-%02d", i), "agents.md"), "api reference")
	}
	published := filepath.Join(dir, "zz-published")
	mustWriteFile(t, filepath.Join(published, "AGENTS.md"), "rules")

	fired := make(chan struct{}, 1)
	w, err := agentcontext.NewWatcher(agentcontext.WatcherOptions{
		Logger:   testutil.Logger(t).Named("watcher"),
		Debounce: 10 * time.Millisecond,
		OnChange: func() {
			select {
			case fired <- struct{}{}:
			default:
			}
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })

	ctx := testutil.Context(t, testutil.WaitShort)
	w.Sync(ctx, []agentcontext.ScanRoot{{Path: dir, ChildProjects: true}})
	mustWriteFile(t, filepath.Join(published, "AGENTS.md"), "edited rules")
	select {
	case <-fired:
	case <-ctx.Done():
		require.Fail(t, "expected callback after editing a published child's AGENTS.md")
	}
}

// TestWatcher_GitOnlyChildWatchedWhenPublishedSlotsFull fills the published
// slots and checks that a clone sorting before them, which has no instruction
// file yet, still fires when its AGENTS.md is checked out: that file would
// change which 64 children the resolver publishes.
func TestWatcher_GitOnlyChildWatchedWhenPublishedSlotsFull(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for i := range 64 {
		mustWriteFile(t, filepath.Join(dir, fmt.Sprintf("repo-%02d", i), "AGENTS.md"), "rules")
	}
	clone := filepath.Join(dir, "aa-clone")
	require.NoError(t, os.MkdirAll(filepath.Join(clone, ".git"), 0o755))

	fired := make(chan struct{}, 1)
	w, err := agentcontext.NewWatcher(agentcontext.WatcherOptions{
		Logger:   testutil.Logger(t).Named("watcher"),
		Debounce: 10 * time.Millisecond,
		OnChange: func() {
			select {
			case fired <- struct{}{}:
			default:
			}
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })

	ctx := testutil.Context(t, testutil.WaitShort)
	w.Sync(ctx, []agentcontext.ScanRoot{{Path: dir, ChildProjects: true}})
	mustWriteFile(t, filepath.Join(clone, "AGENTS.md"), "rules")
	select {
	case <-fired:
	case <-ctx.Done():
		require.Fail(t, "expected callback after a git-only clone gained an AGENTS.md")
	}
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

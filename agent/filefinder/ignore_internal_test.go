package filefinder

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/coder/quartz"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/testutil"
)

func gitIgnorerForTest(t *testing.T) *Ignorer {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	ig := NewIgnorer(afero.NewOsFs(), "")
	require.NotNil(t, ig.git)
	return ig
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	out, err := exec.Command("git", "init", "-q", dir).CombinedOutput()
	require.NoError(t, err, string(out))
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// TestFSWatcher_FlushAfterIgnoredBatch drives the batch timer with a
// mock clock: after a batch that filtering empties, the next event must
// still arm a timer and be delivered.
func TestFSWatcher_FlushAfterIgnoredBatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	writeTestFile(t, filepath.Join(dir, ".gitignore"), "*.log\n")
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	w, err := newFSWatcher(dir, logger, NewIgnorer(afero.NewOsFs(), "", WithGit(nil, "")))
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })
	mClock := quartz.NewMock(t)
	trap := mClock.Trap().NewTimer("fsWatcher", "batch")
	defer trap.Close()
	w.clock = mClock
	ctx := testutil.Context(t, testutil.WaitShort)
	w.Start(ctx)

	writeTestFile(t, filepath.Join(dir, "a.log"), "")
	trap.MustWait(ctx).MustRelease(ctx)
	mClock.Advance(50 * time.Millisecond).MustWait(ctx)
	// An ignored create becomes a removal, in case a walk indexed it.
	require.Equal(t, []FSEvent{{Op: OpRemove, Path: filepath.Join(dir, "a.log")}}, testutil.RequireReceive(ctx, t, w.Events()))

	writeTestFile(t, filepath.Join(dir, "b.go"), "")
	trap.MustWait(ctx).MustRelease(ctx)
	mClock.Advance(50 * time.Millisecond).MustWait(ctx)
	events := testutil.RequireReceive(ctx, t, w.Events())
	require.Equal(t, []FSEvent{{Op: OpCreate, Path: filepath.Join(dir, "b.go")}}, events)
}

// TestEngine_IgnoredCreateRetractsWalkedEntry covers an ignored file
// that a walk indexed because it appeared after the walk's ignore
// snapshot: its create event must remove it again.
func TestEngine_IgnoredCreateRetractsWalkedEntry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	writeTestFile(t, filepath.Join(dir, ".gitignore"), "*.log\n")
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	e := NewEngine(logger, WithIgnorer(NewIgnorer(afero.NewOsFs(), "", WithGit(nil, ""))))
	t.Cleanup(func() { _ = e.Close() })
	ctx := testutil.Context(t, testutil.WaitShort)
	require.NoError(t, e.AddRoot(ctx, dir))

	// Simulate the walk having indexed the file before it existed on
	// disk, then create it.
	e.mu.Lock()
	gen := e.roots[dir].gen
	e.mu.Unlock()
	late := filepath.Join(dir, "late.log")
	e.applyEvents(rootEvent{root: dir, gen: gen, events: []FSEvent{{Op: OpCreate, Path: late}}})
	writeTestFile(t, late, "")

	require.Eventually(t, func() bool {
		e.mu.Lock()
		defer e.mu.Unlock()
		return !e.roots[dir].index.Has("late.log")
	}, testutil.WaitShort, testutil.IntervalFast)
}

// TestWalk_RevalidatesChangedDirectories walks with a listing taken
// before an ignored file existed, as happens when the file is created
// during the walk.
func TestWalk_RevalidatesChangedDirectories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	writeTestFile(t, filepath.Join(dir, ".gitignore"), "*.log\n")
	writeTestFile(t, filepath.Join(dir, "late.log"), "")
	writeTestFile(t, filepath.Join(dir, "sub", "late.log"), "")
	writeTestFile(t, filepath.Join(dir, "kept.go"), "")
	ig := NewIgnorer(afero.NewOsFs(), "", WithGit(nil, ""))

	stale := walkScope{set: map[string]struct{}{}, listedAt: time.Now()}
	var seen []string
	err := ig.walkScoped(context.Background(), dir, stale, func(path string, _ os.DirEntry) error {
		rel, _ := filepath.Rel(dir, path)
		seen = append(seen, filepath.ToSlash(rel))
		return nil
	}, nil)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{".gitignore", "kept.go", "sub"}, seen)
}

func TestGitScope_DirectoryRecordsMustBeIgnoredThemselves(t *testing.T) {
	t.Parallel()
	ig := gitIgnorerForTest(t)
	repo := t.TempDir()
	gitInit(t, repo)
	writeTestFile(t, filepath.Join(repo, ".gitignore"), "*.log\nbig/\n")
	writeTestFile(t, filepath.Join(repo, "logs", "a.log"), "")
	writeTestFile(t, filepath.Join(repo, "logs", "big", "x.txt"), "")

	// ls-files reports logs/ because only its contents are ignored.
	s := ig.scopeFor(context.Background(), repo)
	require.False(t, s.ignored(filepath.Join(repo, "logs"), true))
	require.True(t, s.ignored(filepath.Join(repo, "logs", "a.log"), false))
	require.True(t, s.ignored(filepath.Join(repo, "logs", "big"), true))

	// From inside logs, ls-files reports "./", which is not ignored.
	s = ig.scopeFor(context.Background(), filepath.Join(repo, "logs"))
	require.False(t, s.all)
	require.True(t, s.ignored(filepath.Join(repo, "logs", "a.log"), false))
}

func TestGitScope_BudgetOverflowFallsBack(t *testing.T) {
	t.Parallel()
	ig := gitIgnorerForTest(t)
	ig.git.maxRecords = 2
	repo := t.TempDir()
	gitInit(t, repo)
	writeTestFile(t, filepath.Join(repo, ".gitignore"), "*.log\n")
	for _, name := range []string{"a.log", "b.log", "c.log", "kept.go"} {
		writeTestFile(t, filepath.Join(repo, name), "")
	}

	_, err := ig.gitScope(context.Background(), repo)
	require.ErrorIs(t, err, errGitOutputTooLarge)
	s := ig.scopeFor(context.Background(), repo)
	require.NotNil(t, s.rules, "expected the go-git fallback")
	require.True(t, s.ignored(filepath.Join(repo, "a.log"), false))
	require.False(t, s.ignored(filepath.Join(repo, "kept.go"), false))
}

// failingExecer runs a command that always fails in place of git.
type failingExecer struct{ agentexec.Execer }

func (failingExecer) CommandContext(ctx context.Context, _ string, _ ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", "exit 2")
}

func TestIgnorer_RuntimeGitFailureUsesGlobalExcludes(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	home := t.TempDir()
	writeTestFile(t, filepath.Join(home, ".gitconfig"), "[core]\n\texcludesFile = ~/myignore\n")
	writeTestFile(t, filepath.Join(home, "myignore"), "*.swp\n")
	repo := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o755))

	ig := NewIgnorer(afero.NewOsFs(), home, WithGit(failingExecer{}, "sh"))
	require.NotNil(t, ig.git)
	ignored := ig.Ignored(context.Background(), []IgnoreQuery{
		{Path: filepath.Join(repo, "a.swp")},
		{Path: filepath.Join(repo, "a.go")},
	})
	require.Equal(t, []bool{true, false}, ignored)
}

// orphanExecer starts a background child that holds stdout open after
// the shell is killed.
type orphanExecer struct{ agentexec.Execer }

func (orphanExecer) CommandContext(ctx context.Context, _ string, _ ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", "sleep 60 & wait")
}

func TestGitRunner_CancelKillsDescendants(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	g := newGitRunner(orphanExecer{}, "sh")
	require.NotNil(t, g)

	done := make(chan error, 1)
	go func() {
		done <- g.run(context.Background(), 100*time.Millisecond, t.TempDir(), nil, func(string) error { return nil }, "ls-files")
	}()
	ctx := testutil.Context(t, testutil.WaitShort)
	require.Error(t, testutil.RequireReceive(ctx, t, done))
}

func TestEngine_ReaddedRootRejectsOldEvents(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	e := NewEngine(logger, WithIgnorer(NewIgnorer(afero.NewOsFs(), "", WithGit(nil, ""))))
	t.Cleanup(func() { _ = e.Close() })
	ctx := testutil.Context(t, testutil.WaitShort)

	require.NoError(t, e.AddRoot(ctx, dir))
	e.mu.Lock()
	oldGen := e.roots[dir].gen
	e.mu.Unlock()
	require.NoError(t, e.RemoveRoot(dir))
	require.NoError(t, e.AddRoot(ctx, dir))

	e.applyEvents(rootEvent{root: dir, gen: oldGen, events: []FSEvent{{Op: OpCreate, Path: filepath.Join(dir, "old.go")}}})
	e.mu.Lock()
	defer e.mu.Unlock()
	require.NotEqual(t, oldGen, e.roots[dir].gen)
	require.False(t, e.roots[dir].index.Has("old.go"))
}

// detachedExecer starts a child in its own session, outside git's
// process group, that holds only stdout and records its PID.
type detachedExecer struct {
	agentexec.Execer
	pidFile string
}

func (e detachedExecer) CommandContext(ctx context.Context, _ string, _ ...string) *exec.Cmd {
	//nolint:gosec // Test-controlled arguments.
	return exec.CommandContext(ctx, "sh", "-c",
		`setsid sh -c 'echo $$ > "$0"; exec sleep 60' "$1" 2>/dev/null </dev/null & sleep 60`, "sh", e.pidFile)
}

func TestGitRunner_CancelUnblocksDetachedStdout(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("setsid is not installed")
	}
	pidFile := filepath.Join(t.TempDir(), "pid")
	g := newGitRunner(detachedExecer{pidFile: pidFile}, "sh")
	require.NotNil(t, g)
	t.Cleanup(func() {
		if data, err := os.ReadFile(pidFile); err == nil {
			_ = exec.Command("kill", "-9", strings.TrimSpace(string(data))).Run()
		}
	})

	ctx := testutil.Context(t, testutil.WaitShort)
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- g.run(runCtx, time.Minute, t.TempDir(), nil, func(string) error { return nil }, "ls-files")
	}()
	// Cancel once the detached child is running and holding stdout.
	require.Eventually(t, func() bool {
		_, err := os.Stat(pidFile)
		return err == nil
	}, testutil.WaitShort, testutil.IntervalFast)
	cancel()
	require.ErrorIs(t, testutil.RequireReceive(ctx, t, done), context.Canceled)
}

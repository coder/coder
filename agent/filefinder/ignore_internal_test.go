package filefinder

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
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

func createFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	writeTestFile(t, filepath.Join(dir, relPath), content)
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

// ignoreMode is a way of evaluating ignore rules: native git, or the
// go-git fallback used when git is not installed.
type ignoreMode struct {
	name string
	git  bool
}

var ignoreModes = []ignoreMode{{name: "Git", git: true}, {name: "Fallback"}}

func (m ignoreMode) ignorer(t *testing.T) *Ignorer {
	t.Helper()
	if !m.git {
		return NewIgnorer(afero.NewOsFs(), "", WithGit(nil, ""))
	}
	requireGit(t)
	ig := NewIgnorer(afero.NewOsFs(), "")
	require.NotNil(t, ig.git)
	return ig
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput() //nolint:gosec // Test-controlled arguments.
	require.NoError(t, err, string(out))
}

// makeRepo makes dir a repository: a real one in git mode, and a bare
// .git directory for the fallback, which needs no git.
func (m ignoreMode) makeRepo(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	if m.git {
		runGit(t, dir, "init", "-q")
		return
	}
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git", "info"), 0o755))
}

func requireIndexed(t *testing.T, idx *Index, want map[string]bool) {
	t.Helper()
	for path, indexed := range want {
		require.Equal(t, indexed, idx.Has(path), "indexed %q", path)
	}
}

func TestWalk_Gitignore(t *testing.T) {
	t.Parallel()
	for _, mode := range ignoreModes {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()
			ig := mode.ignorer(t)

			dir := t.TempDir()
			mode.makeRepo(t, dir)
			createFile(t, dir, ".gitignore", "build/\n*.log\n!keep.log\n/anchored.txt\n")
			createFile(t, dir, ".git/info/exclude", "excluded.txt\n")
			createFile(t, dir, "src/main.go", "")
			createFile(t, dir, "src/debug.log", "")
			createFile(t, dir, "src/keep.log", "")
			createFile(t, dir, "build/out.bin", "")
			createFile(t, dir, "anchored.txt", "")
			createFile(t, dir, "src/anchored.txt", "")
			createFile(t, dir, "excluded.txt", "")
			createFile(t, dir, "src/sub/.gitignore", "*.tmp\n/local\n")
			createFile(t, dir, "src/sub/a.tmp", "")
			createFile(t, dir, "src/sub/local/x.go", "")
			createFile(t, dir, "src/a.tmp", "")
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "src", "empty"), 0o755))
			// A nested repository uses only its own rules.
			mode.makeRepo(t, filepath.Join(dir, "inner"))
			createFile(t, dir, "inner/.gitignore", "only_inner.txt\n")
			createFile(t, dir, "inner/debug.log", "")
			createFile(t, dir, "inner/only_inner.txt", "")
			createFile(t, dir, "only_inner.txt", "")

			idx, _, err := walkRoot(context.Background(), ig, dir)
			require.NoError(t, err)
			requireIndexed(t, idx, map[string]bool{
				".gitignore":           true,
				"src/main.go":          true,
				"src/debug.log":        false,
				"src/keep.log":         true,
				"src/empty":            true,
				"build":                false,
				"build/out.bin":        false,
				"anchored.txt":         false,
				"src/anchored.txt":     true,
				"excluded.txt":         false,
				"src/sub/a.tmp":        false,
				"src/sub/local":        false,
				"src/sub/local/x.go":   false,
				"src/a.tmp":            true,
				"inner/debug.log":      true,
				"inner/only_inner.txt": false,
				"only_inner.txt":       true,
			})

			// A root inside a repository applies the enclosing rules.
			idx, _, err = walkRoot(context.Background(), ig, filepath.Join(dir, "src"))
			require.NoError(t, err)
			requireIndexed(t, idx, map[string]bool{
				"main.go":   true,
				"debug.log": false,
				"keep.log":  true,
				"sub/a.tmp": false,
			})

			// Classification agrees with the walk, including inside an
			// ignored directory.
			ignored := ig.Ignored(context.Background(), []IgnoreQuery{
				{Path: filepath.Join(dir, "build"), IsDir: true},
				{Path: filepath.Join(dir, "build", "out.bin")},
				{Path: filepath.Join(dir, "src", "debug.log")},
				{Path: filepath.Join(dir, "src", "keep.log")},
				{Path: filepath.Join(dir, "inner", "debug.log")},
				{Path: filepath.Join(dir, "inner", "only_inner.txt")},
			})
			require.Equal(t, []bool{true, true, true, false, false, true}, ignored)
		})
	}
}

func TestWalk_OutsideRepoIgnoresNothing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createFile(t, dir, ".gitignore", "*.log\n")
	createFile(t, dir, "a.log", "")

	idx, _, err := walkRoot(context.Background(), NewIgnorer(afero.NewOsFs(), ""), dir)
	require.NoError(t, err)
	require.True(t, idx.Has("a.log"))
}

// TestIgnorer_GitParity covers rules the go-git fallback gets wrong,
// so they need native git.
func TestIgnorer_GitParity(t *testing.T) {
	t.Parallel()
	ig := ignoreModes[0].ignorer(t)

	t.Run("Wildmatch", func(t *testing.T) {
		t.Parallel()
		repo := t.TempDir()
		runGit(t, repo, "init", "-q")
		createFile(t, repo, ".gitignore", "foo/**\n!foo/keep.txt\n**/foo/bar\nbaz/a**b\n")
		for _, rel := range []string{"foo/keep.txt", "foo/other.txt", "x/foo/x/foo/bar", "baz/axxb"} {
			createFile(t, repo, rel, "")
		}
		ignored := ig.Ignored(context.Background(), []IgnoreQuery{
			{Path: filepath.Join(repo, "foo", "keep.txt")},
			{Path: filepath.Join(repo, "foo", "other.txt")},
			{Path: filepath.Join(repo, "x", "foo", "x", "foo", "bar")},
			{Path: filepath.Join(repo, "baz", "axxb")},
		})
		require.Equal(t, []bool{false, true, true, true}, ignored)

		idx, _, err := walkRoot(context.Background(), ig, repo)
		require.NoError(t, err)
		requireIndexed(t, idx, map[string]bool{
			"foo/keep.txt":    true,
			"foo/other.txt":   false,
			"x/foo/x/foo/bar": false,
			"baz/axxb":        false,
		})
	})

	t.Run("IgnoreCase", func(t *testing.T) {
		t.Parallel()
		repo := t.TempDir()
		runGit(t, repo, "init", "-q")
		runGit(t, repo, "config", "core.ignoreCase", "true")
		createFile(t, repo, ".gitignore", "*.LOG\n")
		createFile(t, repo, "a.log", "")
		require.Equal(t, []bool{true}, ig.Ignored(context.Background(), []IgnoreQuery{{Path: filepath.Join(repo, "a.log")}}))
	})

	t.Run("SeparateGitDir", func(t *testing.T) {
		t.Parallel()
		repo := filepath.Join(t.TempDir(), "repo")
		gitDir := filepath.Join(t.TempDir(), "gitdir")
		runGit(t, filepath.Dir(repo), "init", "-q", "--separate-git-dir="+gitDir, repo)
		createFile(t, gitDir, "info/exclude", "excluded.txt\n")
		createFile(t, repo, "excluded.txt", "")
		createFile(t, repo, "kept.txt", "")

		idx, _, err := walkRoot(context.Background(), ig, repo)
		require.NoError(t, err)
		requireIndexed(t, idx, map[string]bool{"excluded.txt": false, "kept.txt": true})
	})

	t.Run("ConfigInclude", func(t *testing.T) {
		t.Parallel()
		repo := t.TempDir()
		runGit(t, repo, "init", "-q")
		extra := filepath.Join(t.TempDir(), "extra.gitconfig")
		excludes := filepath.Join(t.TempDir(), "excludes")
		require.NoError(t, os.WriteFile(excludes, []byte("*.swp\n"), 0o600))
		// Git config treats a backslash as an escape, so Windows paths
		// must use forward slashes, which git accepts there.
		require.NoError(t, os.WriteFile(extra, []byte("[core]\n\texcludesFile = "+filepath.ToSlash(excludes)+"\n"), 0o600))
		runGit(t, repo, "config", "include.path", extra)
		createFile(t, repo, "a.swp", "")
		require.Equal(t, []bool{true}, ig.Ignored(context.Background(), []IgnoreQuery{{Path: filepath.Join(repo, "a.swp")}}))
	})
}

func TestIgnorer_FallbackGlobalExcludes(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	createFile(t, home, ".gitconfig", "[core]\n\texcludesFile = ~/myignore\n")
	createFile(t, home, "myignore", "*.swp\n")
	repo := t.TempDir()
	ignoreModes[1].makeRepo(t, repo)

	ig := NewIgnorer(afero.NewOsFs(), home, WithGit(nil, ""))
	ignored := ig.Ignored(context.Background(), []IgnoreQuery{
		{Path: filepath.Join(repo, "a.swp")},
		{Path: filepath.Join(repo, "a.go")},
	})
	require.Equal(t, []bool{true, false}, ignored)
}

func TestIgnorer_FallbackSeparateGitDir(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	gitDir := t.TempDir()
	createFile(t, repo, ".git", "gitdir: "+gitDir+"\n")
	createFile(t, gitDir, "info/exclude", "excluded.txt\n")

	ig := NewIgnorer(afero.NewOsFs(), "", WithGit(nil, ""))
	require.Equal(t, []bool{true}, ig.Ignored(context.Background(), []IgnoreQuery{{Path: filepath.Join(repo, "excluded.txt")}}))
}

func newIgnoreTestEngine(t *testing.T, ig *Ignorer) (*Engine, context.Context) {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	eng := NewEngine(logger, WithIgnorer(ig))
	t.Cleanup(func() { _ = eng.Close() })
	return eng, testutil.Context(t, testutil.WaitLong)
}

func searchPaths(ctx context.Context, t *testing.T, eng *Engine, q string) []string {
	t.Helper()
	results, err := eng.Search(ctx, q, DefaultSearchOptions())
	require.NoError(t, err)
	paths := make([]string, len(results))
	for i, r := range results {
		paths[i] = r.Path
	}
	slices.Sort(paths)
	return paths
}

func requireEventuallyPaths(ctx context.Context, t *testing.T, eng *Engine, q string, want ...string) {
	t.Helper()
	slices.Sort(want)
	var got []string
	require.Eventually(t, func() bool {
		got = searchPaths(ctx, t, eng, q)
		return slices.Equal(want, got) || len(want) == 0 && len(got) == 0
	}, testutil.WaitShort, testutil.IntervalFast, "want %v", want)
}

func TestEngine_WatcherSkipsIgnored(t *testing.T) {
	t.Parallel()
	for _, mode := range ignoreModes {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()
			ig := mode.ignorer(t)

			dir := t.TempDir()
			mode.makeRepo(t, dir)
			createFile(t, dir, ".gitignore", "build/\n*.log\n")
			createFile(t, dir, "build/old.go", "")

			eng, ctx := newIgnoreTestEngine(t, ig)
			require.NoError(t, eng.AddRoot(ctx, dir))

			createFile(t, dir, "watchtest_ignored.log", "")
			createFile(t, dir, "build/watchtest_built.go", "")
			createFile(t, dir, "newdir/out/watchtest_kept.go", "")
			createFile(t, dir, "newdir/watchtest_nested.log", "")
			createFile(t, dir, "watchtest_last.go", "")

			requireEventuallyPaths(ctx, t, eng, "watchtest", "newdir/out/watchtest_kept.go", "watchtest_last.go")
		})
	}
}

func TestEngine_IgnoreChangeReloadsRoot(t *testing.T) {
	t.Parallel()
	for _, mode := range ignoreModes {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()
			ig := mode.ignorer(t)

			dir := t.TempDir()
			mode.makeRepo(t, dir)
			createFile(t, dir, ".gitignore", "build/\n")
			createFile(t, dir, "build/reload_built.go", "")
			createFile(t, dir, "reload_secret.txt", "")

			eng, ctx := newIgnoreTestEngine(t, ig)
			require.NoError(t, eng.AddRoot(ctx, dir))
			require.Equal(t, []string{"reload_secret.txt"}, searchPaths(ctx, t, eng, "reload_"))

			// Unignoring build indexes it, and the new watcher watches
			// it. Ignoring the secret drops it.
			createFile(t, dir, ".gitignore", "reload_secret.txt\n")
			requireEventuallyPaths(ctx, t, eng, "reload_", "build/reload_built.go")
			createFile(t, dir, "build/reload_later.go", "")
			requireEventuallyPaths(ctx, t, eng, "reload_", "build/reload_built.go", "build/reload_later.go")

			// Exclude file changes reload too.
			createFile(t, dir, ".git/info/exclude", "build/\n")
			requireEventuallyPaths(ctx, t, eng, "reload_")
		})
	}
}

func TestEngine_InheritedRulesReloadRoot(t *testing.T) {
	t.Parallel()
	for _, mode := range ignoreModes {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()
			ig := mode.ignorer(t)

			repo := t.TempDir()
			mode.makeRepo(t, repo)
			createFile(t, repo, "src/inherit_a.go", "")
			createFile(t, repo, "src/inherit_b.txt", "")
			root := filepath.Join(repo, "src")

			eng, ctx := newIgnoreTestEngine(t, ig)
			require.NoError(t, eng.AddRoot(ctx, root))
			require.Equal(t, []string{"inherit_a.go", "inherit_b.txt"}, searchPaths(ctx, t, eng, "inherit_"))

			// The repository's .gitignore and exclude file are outside
			// the root but still apply.
			createFile(t, repo, ".gitignore", "*.txt\n")
			requireEventuallyPaths(ctx, t, eng, "inherit_", "inherit_a.go")
			createFile(t, repo, ".git/info/exclude", "*.go\n")
			requireEventuallyPaths(ctx, t, eng, "inherit_")
		})
	}
}

func TestEngine_SeparateGitDirExcludeReloadsRoot(t *testing.T) {
	t.Parallel()
	ig := ignoreModes[0].ignorer(t)

	repo := filepath.Join(t.TempDir(), "repo")
	gitDir := filepath.Join(t.TempDir(), "gitdir")
	runGit(t, filepath.Dir(repo), "init", "-q", "--separate-git-dir="+gitDir, repo)
	createFile(t, repo, "sepdir_kept.go", "")
	createFile(t, repo, "sepdir_gone.go", "")

	eng, ctx := newIgnoreTestEngine(t, ig)
	require.NoError(t, eng.AddRoot(ctx, repo))
	require.Equal(t, []string{"sepdir_gone.go", "sepdir_kept.go"}, searchPaths(ctx, t, eng, "sepdir_"))

	createFile(t, gitDir, "info/exclude", "sepdir_gone.go\n")
	requireEventuallyPaths(ctx, t, eng, "sepdir_", "sepdir_kept.go")
}

// TestEngine_ReloadKeepsConcurrentChanges changes files right after an
// ignore change, while the reload may be walking, and requires the
// result to converge to the filesystem.
func TestEngine_ReloadKeepsConcurrentChanges(t *testing.T) {
	t.Parallel()
	for _, mode := range ignoreModes {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()
			ig := mode.ignorer(t)

			dir := t.TempDir()
			mode.makeRepo(t, dir)
			for i := range 200 {
				createFile(t, dir, filepath.Join("bulk", "f"+string(rune('a'+i%26))+string(rune('a'+i/26))+".txt"), "")
			}
			createFile(t, dir, "handoff_deleted.go", "")

			eng, ctx := newIgnoreTestEngine(t, ig)
			require.NoError(t, eng.AddRoot(ctx, dir))

			createFile(t, dir, ".gitignore", "*.log\n")
			require.NoError(t, os.Remove(filepath.Join(dir, "handoff_deleted.go")))
			createFile(t, dir, "handoff_created.go", "")
			createFile(t, dir, "handoff_marker.go", "")

			requireEventuallyPaths(ctx, t, eng, "handoff_", "handoff_created.go", "handoff_marker.go")
		})
	}
}

// BenchmarkIgnored_EventBatch classifies a 10k-event batch spread over
// 100 directories, the shape the watcher sends per flush.
func BenchmarkIgnored_EventBatch(b *testing.B) {
	if _, err := exec.LookPath("git"); err != nil {
		b.Skip("git is not installed")
	}
	repo := b.TempDir()
	out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput()
	require.NoError(b, err, string(out))
	require.NoError(b, os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("*.log\nbuild/\n"), 0o600))
	queries := make([]IgnoreQuery, 0, 10_000)
	for d := range 100 {
		dir := filepath.Join(repo, "src", "pkg"+string(rune('a'+d%26))+string(rune('a'+d/26)))
		require.NoError(b, os.MkdirAll(dir, 0o755))
		for f := range 100 {
			queries = append(queries, IgnoreQuery{Path: filepath.Join(dir, "f"+string(rune('a'+f%26))+string(rune('a'+f/26))+".go")})
		}
	}
	ig := NewIgnorer(afero.NewOsFs(), "")
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = ig.Ignored(ctx, queries)
	}
}

// TestEngine_DirectoryOfIgnoredFilesStaysWatched covers a directory
// that git lists only because all of its contents are ignored.
func TestEngine_DirectoryOfIgnoredFilesStaysWatched(t *testing.T) {
	t.Parallel()
	ig := ignoreModes[0].ignorer(t)

	for name, rootRel := range map[string]string{"RepoRoot": "", "IgnoredContentsRoot": "logs"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			repo := t.TempDir()
			runGit(t, repo, "init", "-q")
			createFile(t, repo, ".gitignore", "*.log\n")
			createFile(t, repo, "logs/a.log", "")
			root := filepath.Join(repo, rootRel)

			eng, ctx := newIgnoreTestEngine(t, ig)
			require.NoError(t, eng.AddRoot(ctx, root))
			createFile(t, repo, "logs/dirwatch_new.go", "")
			rel, err := filepath.Rel(root, filepath.Join(repo, "logs", "dirwatch_new.go"))
			require.NoError(t, err)
			requireEventuallyPaths(ctx, t, eng, "dirwatch_", filepath.ToSlash(rel))
		})
	}
}

// TestIgnorer_IgnoresRepoSelectingEnv checks that GIT_DIR and
// GIT_WORK_TREE inherited by the agent do not redirect classification.
// It cannot run in parallel because it sets the process environment.
//
//nolint:paralleltest // t.Setenv.
func TestIgnorer_IgnoresRepoSelectingEnv(t *testing.T) {
	requireGit(t)
	other := t.TempDir()
	runGit(t, other, "init", "-q")
	createFile(t, other, ".gitignore", "*.go\n")
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	createFile(t, repo, ".gitignore", "*.log\n")

	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(other, ".git", "index"))
	ig := NewIgnorer(afero.NewOsFs(), "")
	require.Equal(t, []bool{false, true}, ig.Ignored(context.Background(), []IgnoreQuery{
		{Path: filepath.Join(repo, "a.go")},
		{Path: filepath.Join(repo, "a.log")},
	}))
}

package filefinder_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/filefinder"
	"github.com/coder/coder/v2/testutil"
)

// ignoreMode is a way of evaluating ignore rules: native git, or the
// go-git fallback used when git is not installed.
type ignoreMode struct {
	name string
	git  bool
}

var ignoreModes = []ignoreMode{{name: "Git", git: true}, {name: "Fallback"}}

func (m ignoreMode) ignorer(t *testing.T) *filefinder.Ignorer {
	t.Helper()
	if !m.git {
		return filefinder.NewIgnorer(afero.NewOsFs(), "", filefinder.WithGit(nil, ""))
	}
	requireGit(t)
	ig := filefinder.NewIgnorer(afero.NewOsFs(), "")
	require.True(t, filefinder.IgnorerUsesGit(ig))
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

func requireIndexed(t *testing.T, idx *filefinder.Index, want map[string]bool) {
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

			idx, err := filefinder.BuildTestIndexWith(ig, dir)
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
			idx, err = filefinder.BuildTestIndexWith(ig, filepath.Join(dir, "src"))
			require.NoError(t, err)
			requireIndexed(t, idx, map[string]bool{
				"main.go":   true,
				"debug.log": false,
				"keep.log":  true,
				"sub/a.tmp": false,
			})

			// Classification agrees with the walk, including inside an
			// ignored directory.
			ignored := ig.Ignored(context.Background(), []filefinder.IgnoreQuery{
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

	idx, err := filefinder.BuildTestIndex(dir)
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
		ignored := ig.Ignored(context.Background(), []filefinder.IgnoreQuery{
			{Path: filepath.Join(repo, "foo", "keep.txt")},
			{Path: filepath.Join(repo, "foo", "other.txt")},
			{Path: filepath.Join(repo, "x", "foo", "x", "foo", "bar")},
			{Path: filepath.Join(repo, "baz", "axxb")},
		})
		require.Equal(t, []bool{false, true, true, true}, ignored)

		idx, err := filefinder.BuildTestIndexWith(ig, repo)
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
		require.Equal(t, []bool{true}, ig.Ignored(context.Background(), []filefinder.IgnoreQuery{{Path: filepath.Join(repo, "a.log")}}))
	})

	t.Run("SeparateGitDir", func(t *testing.T) {
		t.Parallel()
		repo := filepath.Join(t.TempDir(), "repo")
		gitDir := filepath.Join(t.TempDir(), "gitdir")
		runGit(t, filepath.Dir(repo), "init", "-q", "--separate-git-dir="+gitDir, repo)
		createFile(t, gitDir, "info/exclude", "excluded.txt\n")
		createFile(t, repo, "excluded.txt", "")
		createFile(t, repo, "kept.txt", "")

		idx, err := filefinder.BuildTestIndexWith(ig, repo)
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
		require.NoError(t, os.WriteFile(extra, []byte("[core]\n\texcludesFile = "+excludes+"\n"), 0o600))
		runGit(t, repo, "config", "include.path", extra)
		createFile(t, repo, "a.swp", "")
		require.Equal(t, []bool{true}, ig.Ignored(context.Background(), []filefinder.IgnoreQuery{{Path: filepath.Join(repo, "a.swp")}}))
	})
}

func TestIgnorer_FallbackGlobalExcludes(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	createFile(t, home, ".gitconfig", "[core]\n\texcludesFile = ~/myignore\n")
	createFile(t, home, "myignore", "*.swp\n")
	repo := t.TempDir()
	ignoreModes[1].makeRepo(t, repo)

	ig := filefinder.NewIgnorer(afero.NewOsFs(), home, filefinder.WithGit(nil, ""))
	ignored := ig.Ignored(context.Background(), []filefinder.IgnoreQuery{
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

	ig := filefinder.NewIgnorer(afero.NewOsFs(), "", filefinder.WithGit(nil, ""))
	require.Equal(t, []bool{true}, ig.Ignored(context.Background(), []filefinder.IgnoreQuery{{Path: filepath.Join(repo, "excluded.txt")}}))
}

func newIgnoreTestEngine(t *testing.T, ig *filefinder.Ignorer) (*filefinder.Engine, context.Context) {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	eng := filefinder.NewEngine(logger, filefinder.WithIgnorer(ig))
	t.Cleanup(func() { _ = eng.Close() })
	return eng, testutil.Context(t, testutil.WaitLong)
}

func searchPaths(ctx context.Context, t *testing.T, eng *filefinder.Engine, q string) []string {
	t.Helper()
	results, err := eng.Search(ctx, q, filefinder.DefaultSearchOptions())
	require.NoError(t, err)
	paths := resultPaths(results)
	slices.Sort(paths)
	return paths
}

func requireEventuallyPaths(ctx context.Context, t *testing.T, eng *filefinder.Engine, q string, want ...string) {
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
	queries := make([]filefinder.IgnoreQuery, 0, 10_000)
	for d := range 100 {
		dir := filepath.Join(repo, "src", "pkg"+string(rune('a'+d%26))+string(rune('a'+d/26)))
		require.NoError(b, os.MkdirAll(dir, 0o755))
		for f := range 100 {
			queries = append(queries, filefinder.IgnoreQuery{Path: filepath.Join(dir, "f"+string(rune('a'+f%26))+string(rune('a'+f/26))+".go")})
		}
	}
	ig := filefinder.NewIgnorer(afero.NewOsFs(), "")
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
	ig := filefinder.NewIgnorer(afero.NewOsFs(), "")
	require.Equal(t, []bool{false, true}, ig.Ignored(context.Background(), []filefinder.IgnoreQuery{
		{Path: filepath.Join(repo, "a.go")},
		{Path: filepath.Join(repo, "a.log")},
	}))
}

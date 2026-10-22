package filefinder_test

import (
	"context"
	"os"
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

func makeRepo(t *testing.T, dir string) {
	t.Helper()
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

	dir := t.TempDir()
	makeRepo(t, dir)
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
	// A nested repository uses only its own rules.
	makeRepo(t, filepath.Join(dir, "inner"))
	createFile(t, dir, "inner/.gitignore", "only_inner.txt\n")
	createFile(t, dir, "inner/debug.log", "")
	createFile(t, dir, "inner/only_inner.txt", "")
	createFile(t, dir, "only_inner.txt", "")

	idx, err := filefinder.BuildTestIndex(dir)
	require.NoError(t, err)
	requireIndexed(t, idx, map[string]bool{
		".gitignore":           true,
		"src/main.go":          true,
		"src/debug.log":        false,
		"src/keep.log":         true,
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
	idx, err = filefinder.BuildTestIndex(filepath.Join(dir, "src"))
	require.NoError(t, err)
	requireIndexed(t, idx, map[string]bool{
		"main.go":   true,
		"debug.log": false,
		"keep.log":  true,
		"sub/a.tmp": false,
	})
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

func TestIgnorer_GlobalExcludes(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	createFile(t, home, ".gitconfig", "[core]\n\texcludesFile = ~/myignore\n")
	createFile(t, home, "myignore", "*.swp\n")
	repo := t.TempDir()
	makeRepo(t, repo)

	ig := filefinder.NewIgnorer(afero.NewOsFs(), home)
	rules := ig.Rules(repo)
	require.True(t, rules.Ignored(filepath.Join(repo, "a.swp"), false))
	require.False(t, rules.Ignored(filepath.Join(repo, "a.go"), false))
}

func TestIgnorer_RulesInsideIgnoredDir(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	makeRepo(t, repo)
	createFile(t, repo, ".gitignore", "/build\n")
	createFile(t, repo, "build/x/y.go", "")

	ig := filefinder.NewIgnorer(afero.NewOsFs(), "")
	require.True(t, ig.Rules(repo).Ignored(filepath.Join(repo, "build"), true))
	require.True(t, ig.Rules(filepath.Join(repo, "build", "x")).Ignored(filepath.Join(repo, "build", "x", "y.go"), false))

	// The cached rules follow .gitignore changes.
	createFile(t, repo, ".gitignore", "/other\n")
	require.False(t, ig.Rules(filepath.Join(repo, "build", "x")).Ignored(filepath.Join(repo, "build", "x", "y.go"), false))
}

func newIgnoreTestEngine(t *testing.T) (*filefinder.Engine, context.Context) {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	eng := filefinder.NewEngine(logger, filefinder.WithIgnorer(filefinder.NewIgnorer(afero.NewOsFs(), "")))
	t.Cleanup(func() { _ = eng.Close() })
	return eng, testutil.Context(t, testutil.WaitLong)
}

func searchPaths(ctx context.Context, t *testing.T, eng *filefinder.Engine, q string) []string {
	t.Helper()
	results, err := eng.Search(ctx, q, filefinder.DefaultSearchOptions())
	require.NoError(t, err)
	return resultPaths(results)
}

func TestEngine_WatcherSkipsIgnored(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	makeRepo(t, dir)
	createFile(t, dir, ".gitignore", "build/\n*.log\n")
	createFile(t, dir, "build/old.go", "")

	eng, ctx := newIgnoreTestEngine(t)
	require.NoError(t, eng.AddRoot(ctx, dir))

	createFile(t, dir, "watchtest_ignored.log", "")
	createFile(t, dir, "build/watchtest_built.go", "")
	createFile(t, dir, "newdir/out/watchtest_kept.go", "")
	createFile(t, dir, "newdir/watchtest_nested.log", "")
	createFile(t, dir, "watchtest_last.go", "")

	// Events apply in order, so once the last file is indexed the
	// earlier ones have been handled.
	require.Eventually(t, func() bool {
		return slices.Contains(searchPaths(ctx, t, eng, "watchtest"), "watchtest_last.go")
	}, testutil.WaitShort, testutil.IntervalFast)
	require.Eventually(t, func() bool {
		return slices.Contains(searchPaths(ctx, t, eng, "watchtest"), "newdir/out/watchtest_kept.go")
	}, testutil.WaitShort, testutil.IntervalFast)
	paths := searchPaths(ctx, t, eng, "watchtest")
	require.NotContains(t, paths, "watchtest_ignored.log")
	require.NotContains(t, paths, "build/watchtest_built.go")
	require.NotContains(t, paths, "newdir/watchtest_nested.log")
}

func TestEngine_IgnoreChangeReloadsRoot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	makeRepo(t, dir)
	createFile(t, dir, ".gitignore", "build/\n")
	createFile(t, dir, "build/reload_built.go", "")
	createFile(t, dir, "reload_secret.txt", "")

	eng, ctx := newIgnoreTestEngine(t)
	require.NoError(t, eng.AddRoot(ctx, dir))
	require.Equal(t, []string{"reload_secret.txt"}, searchPaths(ctx, t, eng, "reload_"))

	// Unignoring build indexes it, and the new watcher watches it.
	// Ignoring the secret drops it.
	createFile(t, dir, ".gitignore", "reload_secret.txt\n")
	require.Eventually(t, func() bool {
		return slices.Equal([]string{"build/reload_built.go"}, searchPaths(ctx, t, eng, "reload_"))
	}, testutil.WaitShort, testutil.IntervalFast)
	createFile(t, dir, "build/reload_later.go", "")
	require.Eventually(t, func() bool {
		return slices.Contains(searchPaths(ctx, t, eng, "reload_"), "build/reload_later.go")
	}, testutil.WaitShort, testutil.IntervalFast)

	// .git/info/exclude changes reload too.
	createFile(t, dir, ".git/info/exclude", "build/\n")
	require.Eventually(t, func() bool {
		return len(searchPaths(ctx, t, eng, "reload_")) == 0
	}, testutil.WaitShort, testutil.IntervalFast)
}

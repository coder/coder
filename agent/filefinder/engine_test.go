package filefinder_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/filefinder"
	"github.com/coder/coder/v2/testutil"
)

func newTestEngine(t *testing.T) (*filefinder.Engine, context.Context) {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	eng := filefinder.NewEngine(logger)
	t.Cleanup(func() { _ = eng.Close() })
	return eng, context.Background()
}

func requireResultHasPath(t *testing.T, results []filefinder.Result, path string) {
	t.Helper()
	for _, r := range results {
		if r.Path == path {
			return
		}
	}
	t.Errorf("expected %q in results, got %v", path, resultPaths(results))
}

func TestEngine_SearchFindsKnownFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	createFile(t, dir, "src/main.go", "package main")
	createFile(t, dir, "src/handler.go", "package main")
	createFile(t, dir, "README.md", "# hello")

	eng, ctx := newTestEngine(t)
	require.NoError(t, eng.AddRoot(ctx, dir))

	results, err := eng.Search(ctx, "main.go", filefinder.DefaultSearchOptions())
	require.NoError(t, err)
	require.NotEmpty(t, results, "expected to find main.go")
	requireResultHasPath(t, results, "src/main.go")
}

func TestEngine_SearchFuzzyMatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	createFile(t, dir, "src/controllers/user_handler.go", "package controllers")
	createFile(t, dir, "src/models/user.go", "package models")
	createFile(t, dir, "docs/api.md", "# API")

	eng, ctx := newTestEngine(t)
	require.NoError(t, eng.AddRoot(ctx, dir))

	// "handler" should match "user_handler.go".
	results, err := eng.Search(ctx, "handler", filefinder.DefaultSearchOptions())
	require.NoError(t, err)
	// The query is a subsequence of "user_handler.go" so it
	// should appear somewhere in the results.
	requireResultHasPath(t, results, "src/controllers/user_handler.go")
}

func TestEngine_IndexPicksUpNewFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	createFile(t, dir, "existing.txt", "hello")

	eng, ctx := newTestEngine(t)
	require.NoError(t, eng.AddRoot(ctx, dir))
	createFile(t, dir, "newfile_unique.txt", "world")

	require.Eventually(t, func() bool {
		results, sErr := eng.Search(ctx, "newfile_unique", filefinder.DefaultSearchOptions())
		if sErr != nil {
			return false
		}
		for _, r := range results {
			if r.Path == "newfile_unique.txt" {
				return true
			}
		}
		return false
	}, testutil.WaitShort, testutil.IntervalFast, "expected newfile_unique.txt to appear via watcher")
}

func TestEngine_IndexRemovesDeletedFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	createFile(t, dir, "deleteme_unique.txt", "goodbye")
	createFile(t, dir, "keeper.txt", "stay")

	eng, ctx := newTestEngine(t)
	require.NoError(t, eng.AddRoot(ctx, dir))

	results, err := eng.Search(ctx, "deleteme_unique", filefinder.DefaultSearchOptions())
	require.NoError(t, err)
	require.NotEmpty(t, results, "expected to find deleteme_unique.txt initially")

	require.NoError(t, os.Remove(filepath.Join(dir, "deleteme_unique.txt")))

	require.Eventually(t, func() bool {
		results, sErr := eng.Search(ctx, "deleteme_unique", filefinder.DefaultSearchOptions())
		if sErr != nil {
			return false
		}
		for _, r := range results {
			if r.Path == "deleteme_unique.txt" {
				return false // still found
			}
		}
		return true
	}, testutil.WaitShort, testutil.IntervalFast, "expected deleteme_unique.txt to disappear after removal")
}

func TestEngine_MultipleRoots(t *testing.T) {
	t.Parallel()
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	createFile(t, dir1, "alpha_unique.go", "package alpha")
	createFile(t, dir2, "beta_unique.go", "package beta")

	eng, ctx := newTestEngine(t)
	require.NoError(t, eng.AddRoot(ctx, dir1))
	require.NoError(t, eng.AddRoot(ctx, dir2))

	results, err := eng.Search(ctx, "alpha_unique", filefinder.DefaultSearchOptions())
	require.NoError(t, err)
	requireResultHasPath(t, results, "alpha_unique.go")

	results, err = eng.Search(ctx, "beta_unique", filefinder.DefaultSearchOptions())
	require.NoError(t, err)
	requireResultHasPath(t, results, "beta_unique.go")
}

func TestEngine_EmptyQueryReturnsEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	createFile(t, dir, "something.txt", "data")

	eng, ctx := newTestEngine(t)
	require.NoError(t, eng.AddRoot(ctx, dir))

	results, err := eng.Search(ctx, "", filefinder.DefaultSearchOptions())
	require.NoError(t, err)
	require.Empty(t, results, "empty query should return no results")
}

func TestEngine_CloseIsClean(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	createFile(t, dir, "file.txt", "data")

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx := context.Background()
	eng := filefinder.NewEngine(logger)
	require.NoError(t, eng.AddRoot(ctx, dir))
	require.NoError(t, eng.Close())

	_, err := eng.Search(ctx, "file", filefinder.DefaultSearchOptions())
	require.Error(t, err)
}

func TestEngine_AddRootIdempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	createFile(t, dir, "file.txt", "data")

	eng, ctx := newTestEngine(t)
	require.NoError(t, eng.AddRoot(ctx, dir))
	require.NoError(t, eng.AddRoot(ctx, dir))

	snapLen := filefinder.EngineSnapLen(eng)
	require.Equal(t, 1, snapLen, "expected exactly one root after duplicate add")
}

func TestEngine_RemoveRoot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	createFile(t, dir, "file.txt", "data")

	eng, ctx := newTestEngine(t)
	require.NoError(t, eng.AddRoot(ctx, dir))

	results, err := eng.Search(ctx, "file", filefinder.DefaultSearchOptions())
	require.NoError(t, err)
	require.NotEmpty(t, results)

	require.NoError(t, eng.RemoveRoot(dir))

	results, err = eng.Search(ctx, "file", filefinder.DefaultSearchOptions())
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestEngine_Rebuild(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	createFile(t, dir, "original.txt", "data")

	eng, ctx := newTestEngine(t)
	require.NoError(t, eng.AddRoot(ctx, dir))

	createFile(t, dir, "sneaky_rebuild.txt", "hidden")
	require.NoError(t, eng.Rebuild(ctx, dir))

	results, err := eng.Search(ctx, "sneaky_rebuild", filefinder.DefaultSearchOptions())
	require.NoError(t, err)
	requireResultHasPath(t, results, "sneaky_rebuild.txt")
}

// createFile creates a file (and parent dirs) at relPath under dir.
func createFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o600))
}

func resultPaths(results []filefinder.Result) []string {
	paths := make([]string, len(results))
	for i, r := range results {
		paths[i] = r.Path
	}
	sort.Strings(paths)
	return paths
}

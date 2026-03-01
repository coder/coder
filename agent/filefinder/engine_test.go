package filefinder

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
)

func TestEngine_SearchFindsKnownFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createFile(t, dir, "src/main.go", "package main")
	createFile(t, dir, "src/handler.go", "package main")
	createFile(t, dir, "README.md", "# hello")

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx := context.Background()

	eng := NewEngine(logger)
	defer func() { _ = eng.Close() }()

	err := eng.AddRoot(ctx, dir)
	require.NoError(t, err)

	results, err := eng.Search(ctx, "main.go", DefaultSearchOptions())
	require.NoError(t, err)
	require.NotEmpty(t, results, "expected to find main.go")

	found := false
	for _, r := range results {
		if r.Path == "src/main.go" {
			found = true
			break
		}
	}
	require.True(t, found, "expected src/main.go in results, got %v", resultPaths(results))
}

func TestEngine_SearchFuzzyMatch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createFile(t, dir, "src/controllers/user_handler.go", "package controllers")
	createFile(t, dir, "src/models/user.go", "package models")
	createFile(t, dir, "docs/api.md", "# API")

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx := context.Background()

	eng := NewEngine(logger)
	defer func() { _ = eng.Close() }()

	err := eng.AddRoot(ctx, dir)
	require.NoError(t, err)

	// "handler" should match "user_handler.go".
	results, err := eng.Search(ctx, "handler", DefaultSearchOptions())
	require.NoError(t, err)

	// The query is a subsequence of "user_handler.go" so it
	// should appear somewhere in the results.
	found := false
	for _, r := range results {
		if r.Path == "src/controllers/user_handler.go" {
			found = true
			break
		}
	}
	require.True(t, found, "expected fuzzy match for user_handler.go, got %v", resultPaths(results))
}

func TestEngine_IndexPicksUpNewFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createFile(t, dir, "existing.txt", "hello")

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx := context.Background()

	eng := NewEngine(logger)
	defer func() { _ = eng.Close() }()

	err := eng.AddRoot(ctx, dir)
	require.NoError(t, err)

	// Create a new file after the root was added.
	createFile(t, dir, "newfile_unique.txt", "world")

	// Wait for the watcher to pick it up. The watcher batches
	// events every 50 ms; give it a reasonable window.
	require.Eventually(t, func() bool {
		results, sErr := eng.Search(ctx, "newfile_unique", DefaultSearchOptions())
		if sErr != nil {
			return false
		}
		for _, r := range results {
			if r.Path == "newfile_unique.txt" {
				return true
			}
		}
		return false
	}, 5*time.Second, 100*time.Millisecond, "expected newfile_unique.txt to appear via watcher")
}

func TestEngine_IndexRemovesDeletedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createFile(t, dir, "deleteme_unique.txt", "goodbye")
	createFile(t, dir, "keeper.txt", "stay")

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx := context.Background()

	eng := NewEngine(logger)
	defer func() { _ = eng.Close() }()

	err := eng.AddRoot(ctx, dir)
	require.NoError(t, err)

	// Verify the file is found initially.
	results, err := eng.Search(ctx, "deleteme_unique", DefaultSearchOptions())
	require.NoError(t, err)
	require.NotEmpty(t, results, "expected to find deleteme_unique.txt initially")

	// Delete the file.
	require.NoError(t, os.Remove(filepath.Join(dir, "deleteme_unique.txt")))

	// Wait for the watcher to process the removal.
	require.Eventually(t, func() bool {
		results, sErr := eng.Search(ctx, "deleteme_unique", DefaultSearchOptions())
		if sErr != nil {
			return false
		}
		for _, r := range results {
			if r.Path == "deleteme_unique.txt" {
				return false // still found
			}
		}
		return true
	}, 5*time.Second, 100*time.Millisecond, "expected deleteme_unique.txt to disappear after removal")
}

func TestEngine_MultipleRoots(t *testing.T) {
	t.Parallel()

	dir1 := t.TempDir()
	dir2 := t.TempDir()
	createFile(t, dir1, "alpha_unique.go", "package alpha")
	createFile(t, dir2, "beta_unique.go", "package beta")

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx := context.Background()

	eng := NewEngine(logger)
	defer func() { _ = eng.Close() }()

	require.NoError(t, eng.AddRoot(ctx, dir1))
	require.NoError(t, eng.AddRoot(ctx, dir2))

	// Search for alpha.
	results, err := eng.Search(ctx, "alpha_unique", DefaultSearchOptions())
	require.NoError(t, err)
	foundAlpha := false
	for _, r := range results {
		if r.Path == "alpha_unique.go" {
			foundAlpha = true
			break
		}
	}
	require.True(t, foundAlpha, "expected alpha_unique.go, got %v", resultPaths(results))

	// Search for beta.
	results, err = eng.Search(ctx, "beta_unique", DefaultSearchOptions())
	require.NoError(t, err)
	foundBeta := false
	for _, r := range results {
		if r.Path == "beta_unique.go" {
			foundBeta = true
			break
		}
	}
	require.True(t, foundBeta, "expected beta_unique.go, got %v", resultPaths(results))
}

func TestEngine_EmptyQueryReturnsEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createFile(t, dir, "something.txt", "data")

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx := context.Background()

	eng := NewEngine(logger)
	defer func() { _ = eng.Close() }()

	require.NoError(t, eng.AddRoot(ctx, dir))

	results, err := eng.Search(ctx, "", DefaultSearchOptions())
	require.NoError(t, err)
	require.Empty(t, results, "empty query should return no results")
}

func TestEngine_CloseIsClean(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createFile(t, dir, "file.txt", "data")

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx := context.Background()

	eng := NewEngine(logger)
	require.NoError(t, eng.AddRoot(ctx, dir))

	// Close should not panic or hang.
	require.NoError(t, eng.Close())

	// Search after close should return error.
	_, err := eng.Search(ctx, "file", DefaultSearchOptions())
	require.Error(t, err)
}

func TestEngine_AddRootIdempotent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createFile(t, dir, "file.txt", "data")

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx := context.Background()

	eng := NewEngine(logger)
	defer func() { _ = eng.Close() }()

	require.NoError(t, eng.AddRoot(ctx, dir))
	// Adding the same root again should be a no-op.
	require.NoError(t, eng.AddRoot(ctx, dir))

	snapPtr := eng.snap.Load()
	require.NotNil(t, snapPtr)
	require.Len(t, *snapPtr, 1, "expected exactly one root after duplicate add")
}

func TestEngine_RemoveRoot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createFile(t, dir, "file.txt", "data")

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx := context.Background()

	eng := NewEngine(logger)
	defer func() { _ = eng.Close() }()

	require.NoError(t, eng.AddRoot(ctx, dir))

	// Files should be findable.
	results, err := eng.Search(ctx, "file", DefaultSearchOptions())
	require.NoError(t, err)
	require.NotEmpty(t, results)

	require.NoError(t, eng.RemoveRoot(dir))

	// After removal the root's files should not be found.
	results, err = eng.Search(ctx, "file", DefaultSearchOptions())
	require.NoError(t, err)
	require.Empty(t, results)
}

func TestEngine_Rebuild(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	createFile(t, dir, "original.txt", "data")

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx := context.Background()

	eng := NewEngine(logger)
	defer func() { _ = eng.Close() }()

	require.NoError(t, eng.AddRoot(ctx, dir))

	// Add a file directly (bypass watcher) and rebuild.
	createFile(t, dir, "sneaky_rebuild.txt", "hidden")
	require.NoError(t, eng.Rebuild(ctx, dir))

	results, err := eng.Search(ctx, "sneaky_rebuild", DefaultSearchOptions())
	require.NoError(t, err)
	found := false
	for _, r := range results {
		if r.Path == "sneaky_rebuild.txt" {
			found = true
			break
		}
	}
	require.True(t, found, "expected sneaky_rebuild.txt after rebuild, got %v", resultPaths(results))
}

// createFile is a test helper that creates a file (and parent
// directories) at the given path relative to dir.
func createFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

// resultPaths extracts the path strings from a result slice for
// diagnostic messages.
func resultPaths(results []Result) []string {
	paths := make([]string, len(results))
	for i, r := range results {
		paths[i] = r.Path
	}
	sort.Strings(paths)
	return paths
}

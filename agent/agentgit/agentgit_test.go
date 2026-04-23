package agentgit_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentgit"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/wsjson"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

// gitCmd runs a git command in the given directory and fails the test
// on error.
func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

// initTestRepo creates a temporary git repo with an initial commit
// and returns the repo root path.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Resolve symlinks and short (8.3) names on Windows so test
	// expectations match the canonical paths returned by git.
	resolved, err := filepath.EvalSymlinks(dir)
	if err == nil {
		dir = resolved
	}

	gitCmd(t, dir, "init")
	gitCmd(t, dir, "config", "user.name", "Test")
	gitCmd(t, dir, "config", "user.email", "test@test.com")

	// Create a file and commit it so the repo has HEAD.
	testFile := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(testFile, []byte("# Test\n"), 0o600))

	gitCmd(t, dir, "add", "README.md")
	gitCmd(t, dir, "commit", "-m", "initial commit")

	return dir
}

func TestSubscribeBulkPathsAndDedupes(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	logger := slogtest.Make(t, nil)

	h := agentgit.NewHandler(logger)

	// Subscribe with multiple paths in the same repo — should dedupe
	// to one repo root.
	filePath1 := filepath.Join(repoDir, "a.go")
	filePath2 := filepath.Join(repoDir, "b.go")
	added := h.Subscribe([]string{filePath1, filePath2})
	require.True(t, added, "first subscribe should add a repo")

	// Subscribing again with the same paths should not add new repos.
	added = h.Subscribe([]string{filePath1})
	require.False(t, added, "duplicate subscribe should not add repos")
}

func TestSubscribeNonGitPathsIgnored(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	h := agentgit.NewHandler(logger)

	nonGitDir := t.TempDir()
	added := h.Subscribe([]string{filepath.Join(nonGitDir, "file.txt")})
	require.False(t, added, "non-git paths should be ignored")
}

func TestSubscribeRelativePathsIgnored(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	h := agentgit.NewHandler(logger)

	added := h.Subscribe([]string{"relative/path.go"})
	require.False(t, added, "relative paths should be ignored")
}

func TestSubscribeEmptyPaths(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	h := agentgit.NewHandler(logger)

	added := h.Subscribe([]string{})
	require.False(t, added, "empty slice should not add any repos")

	added = h.Subscribe(nil)
	require.False(t, added, "nil slice should not add any repos")

	ctx := context.Background()
	msg := h.Scan(ctx)
	require.Nil(t, msg, "scan should return nil with no repos")
}

func TestScanReturnsRepoChanges(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	logger := slogtest.Make(t, nil)

	h := agentgit.NewHandler(logger)

	// Create a dirty file.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "new.go"), []byte("package main\n"), 0o600))

	h.Subscribe([]string{filepath.Join(repoDir, "new.go")})

	ctx := context.Background()
	msg := h.Scan(ctx)
	require.NotNil(t, msg)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msg.Type)
	require.Len(t, msg.Repositories, 1)

	repo := msg.Repositories[0]
	require.Equal(t, repoDir, repo.RepoRoot)
	require.NotEmpty(t, repo.Branch)
	require.NotEmpty(t, repo.UnifiedDiff)

	// Verify the new file appears in the unified diff.
	require.Contains(t, repo.UnifiedDiff, "new.go")
}

func TestScanRespectsGitignore(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	logger := slogtest.Make(t, nil)

	// Add a .gitignore that ignores *.log files and the build/ directory.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".gitignore"), []byte("*.log\nbuild/\n"), 0o600))
	gitCmd(t, repoDir, "add", ".gitignore")
	gitCmd(t, repoDir, "commit", "-m", "add gitignore")

	// Create unstaged files: two normal, three matching gitignore patterns.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "util.go"), []byte("package util\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "debug.log"), []byte("some log output\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "error.log"), []byte("some error\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, "build"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "build", "output.bin"), []byte("binary\n"), 0o600))

	h := agentgit.NewHandler(logger)
	h.Subscribe([]string{filepath.Join(repoDir, "main.go")})

	ctx := context.Background()
	msg := h.Scan(ctx)
	require.NotNil(t, msg)
	require.Len(t, msg.Repositories, 1)

	diff := msg.Repositories[0].UnifiedDiff

	// The non-ignored files should appear in the diff.
	assert.Contains(t, diff, "main.go")
	assert.Contains(t, diff, "util.go")
	// The gitignored files must not appear in the diff.
	assert.NotContains(t, diff, "debug.log")
	assert.NotContains(t, diff, "error.log")
	assert.NotContains(t, diff, "output.bin")
}

func TestScanRespectsGitignoreNestedNegation(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	logger := slogtest.Make(t, nil)

	// Add a .gitignore that ignores node_modules/.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".gitignore"), []byte("node_modules/\n"), 0o600))
	gitCmd(t, repoDir, "add", ".gitignore")
	gitCmd(t, repoDir, "commit", "-m", "add gitignore")

	// Simulate the tailwindcss stubs directory which contains a nested
	// .gitignore with "!*" (negation that un-ignores everything).
	// Real git keeps the parent node_modules/ ignore rule, but go-git
	// incorrectly lets the child negation override it.
	stubsDir := filepath.Join(repoDir, "site", "node_modules", ".pnpm",
		"tailwindcss@3.4.18", "node_modules", "tailwindcss", "stubs")
	require.NoError(t, os.MkdirAll(stubsDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(stubsDir, ".gitignore"), []byte("!*\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(stubsDir, "config.full.js"), []byte("module.exports = {}\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(stubsDir, "tailwind.config.js"), []byte("// tw config\n"), 0o600))

	// Also create a normal file outside node_modules.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n"), 0o600))

	h := agentgit.NewHandler(logger)
	h.Subscribe([]string{filepath.Join(repoDir, "main.go")})

	ctx := context.Background()
	msg := h.Scan(ctx)
	require.NotNil(t, msg)
	require.Len(t, msg.Repositories, 1)

	diff := msg.Repositories[0].UnifiedDiff

	// The non-ignored file should appear in the diff.
	assert.Contains(t, diff, "main.go")
	// Files inside node_modules must not appear even though a nested
	// .gitignore contains "!*". The parent node_modules/ rule takes
	// precedence in real git.
	assert.NotContains(t, diff, "config.full.js")
	assert.NotContains(t, diff, "tailwind.config.js")
}

func TestScanDeltaEmission(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	logger := slogtest.Make(t, nil)

	h := agentgit.NewHandler(logger)

	// Create a dirty file.
	dirtyFile := filepath.Join(repoDir, "dirty.go")
	require.NoError(t, os.WriteFile(dirtyFile, []byte("package dirty\n"), 0o600))

	h.Subscribe([]string{dirtyFile})
	ctx := context.Background()

	// First scan — returns all files (no previous snapshot).
	msg1 := h.Scan(ctx)
	require.NotNil(t, msg1)
	require.Len(t, msg1.Repositories, 1)

	// Second scan with no changes. Should emit a heartbeat with a
	// fresh ScannedAt but no repositories. This lets the UI's
	// "checked Ns ago" label stay honest on an idle clean repo.
	msg2 := h.Scan(ctx)
	require.NotNil(t, msg2, "heartbeat should fire even with no delta")
	require.NotNil(t, msg2.ScannedAt)
	require.Empty(t, msg2.Repositories, "heartbeat must not report per-repo changes")

	// Revert the dirty file (make repo clean).
	require.NoError(t, os.Remove(dirtyFile))

	// Third scan — should emit a "clean" delta for dirty.go.
	msg3 := h.Scan(ctx)
	require.NotNil(t, msg3)
	require.Len(t, msg3.Repositories, 1)

	// The file was reverted, so it should no longer appear in the diff.
	require.NotContains(t, msg3.Repositories[0].UnifiedDiff, "dirty.go")
}

// TestScanHeartbeatOnCleanRepo pins the heartbeat contract: while any
// repo is subscribed, every scan emits a non-nil message with a fresh
// ScannedAt, even when no repo produced a delta. The UI's
// "checked Ns ago" label depends on this so an idle clean repo does
// not drift while the agent is still polling.
func TestScanHeartbeatOnCleanRepo(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	logger := slogtest.Make(t, nil)

	h := agentgit.NewHandler(logger)
	require.True(t, h.Subscribe([]string{repoDir}))
	ctx := context.Background()

	// First scan on a clean repo captures branch/remote/empty-diff.
	msg1 := h.Scan(ctx)
	require.NotNil(t, msg1)
	require.NotNil(t, msg1.ScannedAt)
	require.Len(t, msg1.Repositories, 1)
	require.Empty(t, msg1.Repositories[0].UnifiedDiff)
	firstScanAt := *msg1.ScannedAt

	// Second scan: no delta, but heartbeat must still advance
	// ScannedAt so clients can render an honest "checked Ns ago".
	msg2 := h.Scan(ctx)
	require.NotNil(t, msg2, "heartbeat should fire on a no-delta scan")
	require.NotNil(t, msg2.ScannedAt)
	require.Empty(t, msg2.Repositories, "heartbeat carries no per-repo changes")
	require.False(t, msg2.ScannedAt.Before(firstScanAt),
		"heartbeat ScannedAt must not go backwards")

	// Third scan: also a heartbeat. Still non-nil, still empty.
	msg3 := h.Scan(ctx)
	require.NotNil(t, msg3)
	require.Empty(t, msg3.Repositories)
}

// TestScanNoHeartbeatWithoutSubscribedRoots pins that the heartbeat
// only fires when there is at least one subscribed repo. Before any
// subscribe call, Scan() must still short-circuit to nil so the
// WebSocket handler does not spam empty messages to a client that
// has not registered any paths yet.
func TestScanNoHeartbeatWithoutSubscribedRoots(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	h := agentgit.NewHandler(logger)

	msg := h.Scan(context.Background())
	require.Nil(t, msg, "no subscribed roots should mean no heartbeat")
}

func TestScanDeltaDetectsContentChanges(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	logger := slogtest.Make(t, nil)

	h := agentgit.NewHandler(logger)

	// Modify a committed file.
	readmePath := filepath.Join(repoDir, "README.md")
	require.NoError(t, os.WriteFile(readmePath, []byte("# Edit 1\n"), 0o600))

	h.Subscribe([]string{readmePath})
	ctx := context.Background()

	// First scan — returns the initial dirty state.
	msg1 := h.Scan(ctx)
	require.NotNil(t, msg1)
	require.Len(t, msg1.Repositories, 1)

	require.Contains(t, msg1.Repositories[0].UnifiedDiff, "README.md")

	// Second scan with no changes: heartbeat, no repositories.
	msg2 := h.Scan(ctx)
	require.NotNil(t, msg2, "heartbeat should fire even with no delta")
	require.Empty(t, msg2.Repositories)

	// Now modify the SAME file further (still "Modified" status, but
	// different content).
	require.NoError(t, os.WriteFile(readmePath, []byte("# Edit 2\nMore lines\nEven more\n"), 0o600))

	// Third scan — should detect the content change even though the
	// status is still "Modified".
	msg3 := h.Scan(ctx)
	require.NotNil(t, msg3, "content change in already-dirty file should emit delta")
	require.Len(t, msg3.Repositories, 1)

	require.Contains(t, msg3.Repositories[0].UnifiedDiff, "README.md")

	// Also test an untracked (unstaged) file — its status is "Added"
	// throughout, but further edits should still emit deltas.
	untrackedPath := filepath.Join(repoDir, "untracked.go")
	require.NoError(t, os.WriteFile(untrackedPath, []byte("package main\n"), 0o600))

	h.Subscribe([]string{untrackedPath})
	msg4 := h.Scan(ctx)
	require.NotNil(t, msg4)

	require.Contains(t, msg4.Repositories[0].UnifiedDiff, "untracked.go")

	// No changes: heartbeat, no repositories.
	msg5 := h.Scan(ctx)
	require.NotNil(t, msg5, "heartbeat should fire even with no delta")
	require.Empty(t, msg5.Repositories)

	// Modify the untracked file further.
	require.NoError(t, os.WriteFile(untrackedPath, []byte("package main\n\nfunc init() {}\n"), 0o600))

	msg6 := h.Scan(ctx)
	require.NotNil(t, msg6, "content change in untracked file should emit delta")

	require.Contains(t, msg6.Repositories[0].UnifiedDiff, "untracked.go")
}

func TestScanRateLimiting(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	logger := slogtest.Make(t, nil)

	h := agentgit.NewHandler(logger)

	h.Subscribe([]string{filepath.Join(repoDir, "file.go")})

	// First scan should succeed.
	ctx := context.Background()
	msg1 := h.Scan(ctx)
	// Even if no dirty files, the first scan always runs.
	// The important thing is it doesn't panic.
	_ = msg1

	// Create a dirty file so the next scan has something to report.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "new.go"), []byte("package x\n"), 0o600))

	msg2 := h.Scan(ctx)
	require.NotNil(t, msg2, "scan with new dirty file should return changes")
}

func TestSubscribeDeeplyNestedFile(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	logger := slogtest.Make(t, nil)

	// Create a deeply nested directory structure inside the repo.
	nestedDir := filepath.Join(repoDir, "a", "b", "c")
	require.NoError(t, os.MkdirAll(nestedDir, 0o700))
	nestedFile := filepath.Join(nestedDir, "deep.go")
	require.NoError(t, os.WriteFile(nestedFile, []byte("package deep\n"), 0o600))

	h := agentgit.NewHandler(logger)

	added := h.Subscribe([]string{nestedFile})
	require.True(t, added, "deeply nested file should resolve to repo root")

	msg := h.Scan(context.Background())
	require.NotNil(t, msg)
	require.Len(t, msg.Repositories, 1)
	require.Equal(t, repoDir, msg.Repositories[0].RepoRoot)

	// The nested file should appear in the unified diff.
	require.Contains(t, msg.Repositories[0].UnifiedDiff, "a/b/c/deep.go")
}

func TestSubscribeNestedGitRepos(t *testing.T) {
	t.Parallel()

	// Create an outer repo.
	outerDir := initTestRepo(t)

	// Create an inner repo nested inside the outer one.
	innerDir := filepath.Join(outerDir, "subproject")
	require.NoError(t, os.MkdirAll(innerDir, 0o700))

	gitCmd(t, innerDir, "init")
	gitCmd(t, innerDir, "config", "user.name", "Test")
	gitCmd(t, innerDir, "config", "user.email", "test@test.com")

	// Commit a file in the inner repo so it has HEAD.
	innerFile := filepath.Join(innerDir, "inner.go")
	require.NoError(t, os.WriteFile(innerFile, []byte("package inner\n"), 0o600))
	gitCmd(t, innerDir, "add", "inner.go")
	gitCmd(t, innerDir, "commit", "-m", "inner commit")

	// Now create a dirty file in the inner repo.
	dirtyFile := filepath.Join(innerDir, "dirty.go")
	require.NoError(t, os.WriteFile(dirtyFile, []byte("package inner\n"), 0o600))

	logger := slogtest.Make(t, nil)
	h := agentgit.NewHandler(logger)

	// Subscribe with the path inside the inner repo.
	added := h.Subscribe([]string{dirtyFile})
	require.True(t, added)

	msg := h.Scan(context.Background())
	require.NotNil(t, msg)
	require.Len(t, msg.Repositories, 1, "should track only one repo")

	// The tracked repo should be the inner repo, not the outer one.
	require.Equal(t, innerDir, msg.Repositories[0].RepoRoot,
		"should track the inner (nearest) repo, not the outer one")
}

func TestScanDeletedRepoEmitsRemoved(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	logger := slogtest.Make(t, nil)

	h := agentgit.NewHandler(logger)

	// Create a dirty file so the initial scan has something to track.
	dirtyFile := filepath.Join(repoDir, "dirty.go")
	require.NoError(t, os.WriteFile(dirtyFile, []byte("package dirty\n"), 0o600))

	h.Subscribe([]string{dirtyFile})
	ctx := context.Background()

	// Initial scan — populates the snapshot with the dirty file.
	msg1 := h.Scan(ctx)
	require.NotNil(t, msg1)
	require.Len(t, msg1.Repositories, 1)
	require.False(t, msg1.Repositories[0].Removed)

	// Delete the entire repo directory.
	require.NoError(t, os.RemoveAll(repoDir))

	// Next scan should emit a removal entry.
	msg2 := h.Scan(ctx)
	require.NotNil(t, msg2)
	require.Len(t, msg2.Repositories, 1)

	removed := msg2.Repositories[0]
	require.True(t, removed.Removed, "repo should be marked as removed")
	require.Equal(t, repoDir, removed.RepoRoot)
	require.Empty(t, removed.Branch)

	// Removed repo should have an empty diff.
	require.Empty(t, removed.UnifiedDiff)

	// Subsequent scan should return nil — the repo was evicted from
	// the watch set.
	msg3 := h.Scan(ctx)
	require.Nil(t, msg3, "evicted repo should not appear in subsequent scans")
}

func TestScanDeletedGitDirEmitsRemoved(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	logger := slogtest.Make(t, nil)

	h := agentgit.NewHandler(logger)

	dirtyFile := filepath.Join(repoDir, "dirty.go")
	require.NoError(t, os.WriteFile(dirtyFile, []byte("package dirty\n"), 0o600))

	h.Subscribe([]string{dirtyFile})
	ctx := context.Background()

	msg1 := h.Scan(ctx)
	require.NotNil(t, msg1)

	// Remove only the .git directory (repo root still exists).
	require.NoError(t, os.RemoveAll(filepath.Join(repoDir, ".git")))

	msg2 := h.Scan(ctx)
	require.NotNil(t, msg2)
	require.Len(t, msg2.Repositories, 1)
	require.True(t, msg2.Repositories[0].Removed,
		"removing .git dir should trigger removal")
}

func TestScanDeletedWorktreeGitdirEmitsRemoved(t *testing.T) {
	t.Parallel()

	// Set up a main repo that we'll use as the source for a worktree.
	mainRepoDir := initTestRepo(t)

	// Create a linked worktree using git CLI.
	wtBase := t.TempDir()
	// Resolve symlinks and short (8.3) names on Windows so test
	// expectations match the canonical paths returned by git.
	if resolved, err := filepath.EvalSymlinks(wtBase); err == nil {
		wtBase = resolved
	}
	worktreeDir := filepath.Join(wtBase, "wt")
	gitCmd(t, mainRepoDir, "branch", "worktree-branch")
	gitCmd(t, mainRepoDir, "worktree", "add", worktreeDir, "worktree-branch")

	logger := slogtest.Make(t, nil)
	h := agentgit.NewHandler(logger)

	// Create a dirty file so the initial scan has something to report.
	dirtyFile := filepath.Join(worktreeDir, "dirty.go")
	require.NoError(t, os.WriteFile(dirtyFile, []byte("package dirty\n"), 0o600))

	h.Subscribe([]string{dirtyFile})
	ctx := context.Background()

	// Initial scan should succeed.
	msg1 := h.Scan(ctx)
	require.NotNil(t, msg1)
	require.Len(t, msg1.Repositories, 1)
	require.False(t, msg1.Repositories[0].Removed)

	// Now delete the target gitdir inside .git/worktrees/. The .git
	// file in the worktree still exists, but it points to a directory
	// that is gone.
	gitdirPath := filepath.Join(mainRepoDir, ".git", "worktrees", filepath.Base(worktreeDir))
	require.NoError(t, os.RemoveAll(gitdirPath))

	// Verify the .git file still exists (this is the bug scenario).
	_, err := os.Stat(filepath.Join(worktreeDir, ".git"))
	require.NoError(t, err, ".git file should still exist")

	// Next scan should detect the broken worktree and emit removal.
	msg2 := h.Scan(ctx)
	require.NotNil(t, msg2)
	require.Len(t, msg2.Repositories, 1)
	require.True(t, msg2.Repositories[0].Removed,
		"worktree with deleted gitdir should be marked as removed")
	require.Equal(t, worktreeDir, msg2.Repositories[0].RepoRoot)

	// Repo should be evicted — subsequent scan returns nil.
	msg3 := h.Scan(ctx)
	require.Nil(t, msg3, "evicted worktree should not appear in subsequent scans")
}

func TestScanTransientErrorDoesNotRemoveRepo(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	logger := slogtest.Make(t, nil)

	h := agentgit.NewHandler(logger)

	dirtyFile := filepath.Join(repoDir, "dirty.go")
	require.NoError(t, os.WriteFile(dirtyFile, []byte("package dirty\n"), 0o600))

	h.Subscribe([]string{dirtyFile})
	ctx := context.Background()

	// Initial scan succeeds.
	msg1 := h.Scan(ctx)
	require.NotNil(t, msg1)
	require.Len(t, msg1.Repositories, 1)
	require.False(t, msg1.Repositories[0].Removed)

	// Corrupt the repo by replacing HEAD with invalid content.
	// The directory and .git still exist, so this is a transient
	// error, not a deletion.
	headPath := filepath.Join(repoDir, ".git", "HEAD")
	require.NoError(t, os.WriteFile(headPath, []byte("corrupt"), 0o600))

	// The scan should log a warning but not emit a removal. The
	// repo stays in the watch set.
	msg2 := h.Scan(ctx)
	// msg2 may be nil (no results) since the scan error is
	// transient. Importantly, it must NOT contain a removed entry.
	if msg2 != nil {
		for _, repo := range msg2.Repositories {
			require.False(t, repo.Removed,
				"transient error should not trigger removal")
		}
	}

	// Repair the repo and verify it's still being watched.
	require.NoError(t, os.WriteFile(headPath, []byte("ref: refs/heads/master\n"), 0o600))

	// Modify a file so the next scan has something new to report.
	require.NoError(t, os.WriteFile(
		filepath.Join(repoDir, "new.go"),
		[]byte("package main\n"), 0o600,
	))

	msg3 := h.Scan(ctx)
	require.NotNil(t, msg3, "repo should still be watched after transient error")
	require.Len(t, msg3.Repositories, 1)
	require.False(t, msg3.Repositories[0].Removed)
	require.Equal(t, repoDir, msg3.Repositories[0].RepoRoot)
}

// --- WebSocket end-to-end tests ---

// dialGitWatch starts an httptest server with the agentgit API and
// returns a wsjson.Stream connected to it. The server and connection
// are cleaned up when the test ends.
func dialGitWatch(t *testing.T, opts ...agentgit.Option) *wsjson.Stream[
	codersdk.WorkspaceAgentGitServerMessage,
	codersdk.WorkspaceAgentGitClientMessage,
] {
	t.Helper()
	logger := slogtest.Make(t, nil)
	api := agentgit.NewAPI(logger, nil, opts...)
	srv := httptest.NewServer(api.Routes())
	t.Cleanup(srv.Close)

	wsURL := "ws" + srv.URL[len("http"):] + "/watch"
	conn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "") })

	return wsjson.NewStream[
		codersdk.WorkspaceAgentGitServerMessage,
		codersdk.WorkspaceAgentGitClientMessage,
	](conn, websocket.MessageText, websocket.MessageText, logger)
}

// dialGitWatchWithPathStore starts an httptest server backed by the
// given PathStore and returns a stream connected with the given
// chat ID. The PathStore is used to feed paths into the handler
// instead of client-side subscribe messages.
func dialGitWatchWithPathStore(
	t *testing.T,
	ps *agentgit.PathStore,
	chatID uuid.UUID,
	opts ...agentgit.Option,
) *wsjson.Stream[
	codersdk.WorkspaceAgentGitServerMessage,
	codersdk.WorkspaceAgentGitClientMessage,
] {
	t.Helper()
	logger := slogtest.Make(t, nil)
	api := agentgit.NewAPI(logger, ps, opts...)
	srv := httptest.NewServer(api.Routes())
	t.Cleanup(srv.Close)

	wsURL := "ws" + srv.URL[len("http"):] + "/watch?chat_id=" + chatID.String()
	conn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "") })

	return wsjson.NewStream[
		codersdk.WorkspaceAgentGitServerMessage,
		codersdk.WorkspaceAgentGitClientMessage,
	](conn, websocket.MessageText, websocket.MessageText, logger)
}

// recvMsg reads the next server message, using the provided
// context for the timeout instead of a raw time.After.
func recvMsg(ctx context.Context, t *testing.T, ch <-chan codersdk.WorkspaceAgentGitServerMessage) codersdk.WorkspaceAgentGitServerMessage {
	t.Helper()
	select {
	case msg, ok := <-ch:
		require.True(t, ok, "channel closed unexpectedly")
		return msg
	case <-ctx.Done():
		t.Fatal("timed out waiting for server message")
		return codersdk.WorkspaceAgentGitServerMessage{}
	}
}

func TestWebSocketSubscribeAndReceiveChanges(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	repoDir := initTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "ws.go"), []byte("package ws\n"), 0o600))

	ps := agentgit.NewPathStore()
	chatID := uuid.New()

	// Add paths before connecting so the handler picks them up on
	// startup.
	ps.AddPaths([]uuid.UUID{chatID}, []string{filepath.Join(repoDir, "ws.go")})

	stream := dialGitWatchWithPathStore(t, ps, chatID)
	ch := stream.Chan()

	msg := recvMsg(ctx, t, ch)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msg.Type)
	require.NotNil(t, msg.ScannedAt)
	require.NotEmpty(t, msg.Repositories)
	require.Equal(t, repoDir, msg.Repositories[0].RepoRoot)
}

func TestWebSocketMultipleRepos(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	repoA := initTestRepo(t)
	repoB := initTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoA, "a.go"), []byte("package a\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repoB, "b.go"), []byte("package b\n"), 0o600))

	ps := agentgit.NewPathStore()
	chatID := uuid.New()
	ps.AddPaths([]uuid.UUID{chatID}, []string{
		filepath.Join(repoA, "a.go"),
		filepath.Join(repoB, "b.go"),
	})

	stream := dialGitWatchWithPathStore(t, ps, chatID)
	ch := stream.Chan()

	msg := recvMsg(ctx, t, ch)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msg.Type)
	require.Len(t, msg.Repositories, 2, "should include both repos")

	roots := map[string]bool{}
	for _, r := range msg.Repositories {
		roots[r.RepoRoot] = true
	}
	require.True(t, roots[repoA], "repo A missing")
	require.True(t, roots[repoB], "repo B missing")
}

func TestWebSocketIncrementalSubscribe(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	repoA := initTestRepo(t)
	repoB := initTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoA, "a.go"), []byte("package a\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repoB, "b.go"), []byte("package b\n"), 0o600))

	ps := agentgit.NewPathStore()
	chatID := uuid.New()
	mClock := quartz.NewMock(t)

	// Seed repo A before connecting.
	ps.AddPaths([]uuid.UUID{chatID}, []string{filepath.Join(repoA, "a.go")})

	stream := dialGitWatchWithPathStore(t, ps, chatID, agentgit.WithClock(mClock))
	ch := stream.Chan()

	msg1 := recvMsg(ctx, t, ch)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msg1.Type)
	require.Len(t, msg1.Repositories, 1)
	require.Equal(t, repoA, msg1.Repositories[0].RepoRoot)

	// Advance past the scan cooldown so the next scan fires
	// immediately.
	mClock.Advance(2 * time.Second).MustWait(context.Background())

	// Now add repo B via the PathStore (incremental).
	ps.AddPaths([]uuid.UUID{chatID}, []string{filepath.Join(repoB, "b.go")})

	msg2 := recvMsg(ctx, t, ch)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msg2.Type)
	// The second message should include repo B. It may or may not
	// include repo A depending on delta logic (no change in A since
	// last emit), but repo B must be present.
	foundB := false
	for _, r := range msg2.Repositories {
		if r.RepoRoot == repoB {
			foundB = true
		}
	}
	require.True(t, foundB, "incremental subscribe should include repo B")
}

func TestWebSocketRefreshTriggersChanges(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	repoDir := initTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "r.go"), []byte("package r\n"), 0o600))

	ps := agentgit.NewPathStore()
	chatID := uuid.New()
	ps.AddPaths([]uuid.UUID{chatID}, []string{filepath.Join(repoDir, "r.go")})

	mClock := quartz.NewMock(t)
	stream := dialGitWatchWithPathStore(t, ps, chatID, agentgit.WithClock(mClock))
	ch := stream.Chan()

	// Consume initial changes.
	_ = recvMsg(ctx, t, ch)

	// Advance past cooldown so the refresh scan fires immediately.
	mClock.Advance(2 * time.Second).MustWait(context.Background())

	// Modify a file, then send refresh.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "r2.go"), []byte("package r\n"), 0o600))
	err := stream.Send(codersdk.WorkspaceAgentGitClientMessage{
		Type: codersdk.WorkspaceAgentGitClientMessageTypeRefresh,
	})
	require.NoError(t, err)

	msg := recvMsg(ctx, t, ch)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msg.Type)
	require.NotEmpty(t, msg.Repositories)
}

func TestWebSocketUnknownMessageType(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	stream := dialGitWatch(t)
	ch := stream.Chan()

	err := stream.Send(codersdk.WorkspaceAgentGitClientMessage{
		Type: "bogus",
	})
	require.NoError(t, err)

	msg := recvMsg(ctx, t, ch)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeError, msg.Type)
	require.Contains(t, msg.Message, "unknown")
}

func TestGetRepoChangesStagedModifiedDeleted(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	logger := slogtest.Make(t, nil)

	h := agentgit.NewHandler(logger)

	// Modify the committed file (worktree modified).
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Modified\n"), 0o600))

	// Stage a new file.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "staged.go"), []byte("package staged\n"), 0o600))
	gitCmd(t, repoDir, "add", "staged.go")

	// Create an untracked file.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "untracked.txt"), []byte("hello\n"), 0o600))

	h.Subscribe([]string{filepath.Join(repoDir, "README.md")})
	msg := h.Scan(context.Background())
	require.NotNil(t, msg)
	require.Len(t, msg.Repositories, 1)

	diff := msg.Repositories[0].UnifiedDiff

	// README.md was committed then modified in worktree.
	require.Contains(t, diff, "README.md")
	require.Contains(t, diff, "--- a/README.md")
	require.Contains(t, diff, "+++ b/README.md")
	require.Contains(t, diff, "-# Test")
	require.Contains(t, diff, "+# Modified")

	// staged.go was added to the staging area.
	require.Contains(t, diff, "staged.go")
	require.Contains(t, diff, "+package staged")

	// untracked.txt is untracked (shown via --no-index diff).
	require.Contains(t, diff, "untracked.txt")
	require.Contains(t, diff, "+hello")
}

func TestFallbackPollTriggersScan(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	repoDir := initTestRepo(t)
	mClock := quartz.NewMock(t)

	ps := agentgit.NewPathStore()
	chatID := uuid.New()

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "poll.go"), []byte("package poll\n"), 0o600))
	ps.AddPaths([]uuid.UUID{chatID}, []string{filepath.Join(repoDir, "poll.go")})

	// Only the fallback poll can trigger scans (no filesystem
	// watcher).
	stream := dialGitWatchWithPathStore(t, ps, chatID, agentgit.WithClock(mClock))
	ch := stream.Chan()

	// We should get an initial scan from subscribe.
	msg1 := recvMsg(ctx, t, ch)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msg1.Type)

	// Add a new dirty file so the next scan has a delta to report.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "poll2.go"), []byte("package poll\n"), 0o600))

	// Advance to the fallback poll interval. This should trigger a
	// scan without any explicit refresh.
	mClock.Advance(5 * time.Second).MustWait(context.Background())

	msg2 := recvMsg(ctx, t, ch)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msg2.Type)
	require.NotEmpty(t, msg2.Repositories)
}

func TestMultipleConcurrentConnections(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	repoDir := initTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "c.go"), []byte("package c\n"), 0o600))

	ps := agentgit.NewPathStore()
	chatID := uuid.New()
	ps.AddPaths([]uuid.UUID{chatID}, []string{filepath.Join(repoDir, "c.go")})

	logger := slogtest.Make(t, nil)
	api := agentgit.NewAPI(logger, ps)
	srv := httptest.NewServer(api.Routes())
	t.Cleanup(srv.Close)

	wsURL := "ws" + srv.URL[len("http"):] + "/watch?chat_id=" + chatID.String()

	// Create two independent connections.
	conn1, _, err := websocket.Dial(context.Background(), wsURL, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn1.Close(websocket.StatusNormalClosure, "") })

	conn2, _, err := websocket.Dial(context.Background(), wsURL, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn2.Close(websocket.StatusNormalClosure, "") })

	stream1 := wsjson.NewStream[
		codersdk.WorkspaceAgentGitServerMessage,
		codersdk.WorkspaceAgentGitClientMessage,
	](conn1, websocket.MessageText, websocket.MessageText, logger)
	ch1 := stream1.Chan()

	stream2 := wsjson.NewStream[
		codersdk.WorkspaceAgentGitServerMessage,
		codersdk.WorkspaceAgentGitClientMessage,
	](conn2, websocket.MessageText, websocket.MessageText, logger)
	ch2 := stream2.Chan()

	// Both should receive independent responses.
	msg1 := recvMsg(ctx, t, ch1)
	msg2 := recvMsg(ctx, t, ch2)

	assert.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msg1.Type)
	assert.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msg2.Type)
	assert.NotEmpty(t, msg1.Repositories)
	assert.NotEmpty(t, msg2.Repositories)
}

func TestScanLargeFileTooLargeToDiff(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	logger := slogtest.Make(t, nil)

	h := agentgit.NewHandler(logger)

	// Create a large text file (1 MiB). The diff produced by git
	// CLI will be under maxTotalDiffSize (3 MiB) so it appears in
	// the unified diff output.
	largeContent := make([]byte, 1*1024*1024)
	for i := range largeContent {
		largeContent[i] = byte('A' + (i % 26))
		if i%80 == 79 {
			largeContent[i] = '\n'
		}
	}
	largeFile := filepath.Join(repoDir, "large.txt")
	require.NoError(t, os.WriteFile(largeFile, largeContent, 0o600))

	h.Subscribe([]string{largeFile})

	ctx := context.Background()
	msg := h.Scan(ctx)
	require.NotNil(t, msg)
	require.Len(t, msg.Repositories, 1)

	repo := msg.Repositories[0]

	// The large file should appear in the unified diff.
	require.Contains(t, repo.UnifiedDiff, "large.txt")
}

func TestScanLargeFileDeltaTracking(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	logger := slogtest.Make(t, nil)

	h := agentgit.NewHandler(logger)

	// Create a large file (3 MiB).
	largeContent := make([]byte, 3*1024*1024)
	for i := range largeContent {
		largeContent[i] = byte('X')
	}
	largeFile := filepath.Join(repoDir, "big.dat")
	require.NoError(t, os.WriteFile(largeFile, largeContent, 0o600))

	h.Subscribe([]string{largeFile})
	ctx := context.Background()

	// First scan — should include the large file.
	msg1 := h.Scan(ctx)
	require.NotNil(t, msg1)

	// Second scan with no changes: heartbeat, no repositories.
	msg2 := h.Scan(ctx)
	require.NotNil(t, msg2, "heartbeat should fire even with no delta")
	require.Empty(t, msg2.Repositories, "no delta means no repo entries")

	// Remove the large file — should emit a clean delta.
	require.NoError(t, os.Remove(largeFile))
	msg3 := h.Scan(ctx)
	require.NotNil(t, msg3)

	// The file was removed, so it should no longer appear in the diff.
	require.NotContains(t, msg3.Repositories[0].UnifiedDiff, "big.dat")
}

func TestScanTotalDiffTooLargeForWire(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	logger := slogtest.Make(t, nil)

	h := agentgit.NewHandler(logger)

	// Create many files whose individual diffs are under 256 KiB
	// but whose total exceeds maxTotalDiffSize (3 MiB).
	// ~100 files x 50 KiB content each = ~5 MiB of diffs.
	var paths []string
	for i := range 100 {
		content := make([]byte, 50*1024)
		for j := range content {
			content[j] = byte('A' + (i+j)%26)
		}
		name := fmt.Sprintf("file_%03d.txt", i)
		fullPath := filepath.Join(repoDir, name)
		require.NoError(t, os.WriteFile(fullPath, content, 0o600))
		paths = append(paths, fullPath)
	}

	h.Subscribe(paths)

	ctx := context.Background()
	msg := h.Scan(ctx)
	require.NotNil(t, msg)
	require.Len(t, msg.Repositories, 1)

	repo := msg.Repositories[0]

	// The total diff exceeds 3 MiB, so we should get the
	// total-diff placeholder.
	require.Contains(t, repo.UnifiedDiff, "Total diff too large to show")

	// Branch and remote metadata should still be present.
	require.NotEmpty(t, repo.Branch, "branch should still be populated")

	// The placeholder message should be well under 3 MiB.
	require.Less(t, len(repo.UnifiedDiff), 4*1024*1024,
		"placeholder diff should be much smaller than maxTotalDiffSize")
}

func TestScanBinaryFileDiff(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	logger := slogtest.Make(t, nil)

	h := agentgit.NewHandler(logger)

	// Create a new binary file (contains null bytes).
	binaryContent := []byte("hello\x00world\x00binary")
	binaryFile := filepath.Join(repoDir, "image.png")
	require.NoError(t, os.WriteFile(binaryFile, binaryContent, 0o600))

	h.Subscribe([]string{binaryFile})

	ctx := context.Background()
	msg := h.Scan(ctx)
	require.NotNil(t, msg)
	require.Len(t, msg.Repositories, 1)

	repo := msg.Repositories[0]

	// The binary file should appear in the unified diff.
	require.Contains(t, repo.UnifiedDiff, "image.png")

	// The unified diff should contain the git binary marker,
	// not the raw binary content.
	require.Contains(t, repo.UnifiedDiff, "Binary")
	require.NotContains(t, repo.UnifiedDiff, "\x00",
		"raw binary content should not appear in diff")
}

func TestScanBinaryFileModifiedDiff(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	gitCmd(t, dir, "init")
	gitCmd(t, dir, "config", "user.name", "Test")
	gitCmd(t, dir, "config", "user.email", "test@test.com")

	// Commit a binary file.
	binPath := filepath.Join(dir, "data.bin")
	require.NoError(t, os.WriteFile(binPath, []byte("v1\x00\x01\x02"), 0o600))

	gitCmd(t, dir, "add", "data.bin")
	gitCmd(t, dir, "commit", "-m", "add binary")

	// Modify the binary file in the worktree.
	require.NoError(t, os.WriteFile(binPath, []byte("v2\x00\x03\x04\x05"), 0o600))

	logger := slogtest.Make(t, nil)
	h := agentgit.NewHandler(logger)
	h.Subscribe([]string{binPath})

	ctx := context.Background()
	msg := h.Scan(ctx)
	require.NotNil(t, msg)
	require.Len(t, msg.Repositories, 1)

	repoChanges := msg.Repositories[0]

	// The binary file should appear in the unified diff.
	require.Contains(t, repoChanges.UnifiedDiff, "data.bin")

	// Diff should show binary marker for modification too.
	require.Contains(t, repoChanges.UnifiedDiff, "Binary")
	require.NotContains(t, repoChanges.UnifiedDiff, "\x00",
		"raw binary content should not appear in diff")
}

func TestScanFileDiffTooLargeForWire(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	logger := slogtest.Make(t, nil)

	h := agentgit.NewHandler(logger)

	// Create a single file whose diff is large. With git CLI, the
	// diff is produced by git itself so per-file size limiting is
	// handled by the total diff size check.
	content := make([]byte, 512*1024)
	for i := range content {
		content[i] = byte('A' + (i % 26))
	}
	bigFile := filepath.Join(repoDir, "big_diff.txt")
	require.NoError(t, os.WriteFile(bigFile, content, 0o600))

	h.Subscribe([]string{bigFile})

	ctx := context.Background()
	msg := h.Scan(ctx)
	require.NotNil(t, msg)
	require.Len(t, msg.Repositories, 1)

	repo := msg.Repositories[0]

	// The file should appear in the diff output.
	require.Contains(t, repo.UnifiedDiff, "big_diff.txt")

	// Branch metadata should still be present.
	require.NotEmpty(t, repo.Branch)
}

func TestWebSocketLargePathStoreSubscription(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	repoDir := initTestRepo(t)

	// Create a dirty file so we get a response.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "large.go"), []byte("package large\n"), 0o600))

	ps := agentgit.NewPathStore()
	chatID := uuid.New()

	// Build a path list with 500 paths — one real repo path and 499
	// long non-git paths that will be silently ignored.
	paths := make([]string, 500)
	for i := range paths {
		if i == 0 {
			paths[i] = filepath.Join(repoDir, "large.go")
		} else {
			// ~100 chars of padding.
			padding := filepath.Join("/tmp", t.Name(), "deep", "nested",
				"directory", "structure", "to", "pad", "the", "path",
				"even", "more", "so", "it", "is", "long", "enough",
				string(rune('a'+i%26))+".go")
			paths[i] = padding
		}
	}
	ps.AddPaths([]uuid.UUID{chatID}, paths)

	stream := dialGitWatchWithPathStore(t, ps, chatID)
	ch := stream.Chan()

	// The handler must process the large path set and respond with
	// changes.
	msg := recvMsg(ctx, t, ch)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msg.Type)
	require.Len(t, msg.Repositories, 1)
	require.Equal(t, repoDir, msg.Repositories[0].RepoRoot)
}

// --- End-to-end integration tests (PathStore → git watch pipeline) ---

func TestE2E_WriteFileTriggersGitWatch(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	repoDir := initTestRepo(t)

	// Write a dirty file into the repo.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "newfile.go"), []byte("package newfile\n"), 0o600))

	ps := agentgit.NewPathStore()
	chatID := uuid.New()
	mClock := quartz.NewMock(t)

	// Connect the git watch WebSocket BEFORE adding any paths.
	stream := dialGitWatchWithPathStore(t, ps, chatID, agentgit.WithClock(mClock))
	ch := stream.Chan()

	// Simulate what HandleWriteFile does: add a path to the
	// PathStore. This triggers a notification → subscribe → scan.
	ps.AddPaths([]uuid.UUID{chatID}, []string{filepath.Join(repoDir, "newfile.go")})

	// The WebSocket should receive a changes message showing the
	// repo with the dirty file.
	msg := recvMsg(ctx, t, ch)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msg.Type)
	require.NotEmpty(t, msg.Repositories)

	foundRepo := false
	for _, r := range msg.Repositories {
		if r.RepoRoot == repoDir {
			foundRepo = true
			require.Contains(t, r.UnifiedDiff, "newfile.go")
		}
	}
	require.True(t, foundRepo, "expected repo %s in changes message", repoDir)
}

func TestE2E_SubagentAncestorWatch(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	repoDir := initTestRepo(t)

	// Write a dirty file that the child agent will "touch".
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "child.go"), []byte("package child\n"), 0o600))

	ps := agentgit.NewPathStore()
	parentChatID := uuid.New()
	childChatID := uuid.New()
	mClock := quartz.NewMock(t)

	// Connect a git watch WebSocket for the PARENT chat.
	stream := dialGitWatchWithPathStore(t, ps, parentChatID, agentgit.WithClock(mClock))
	ch := stream.Chan()

	// Simulate a tool call from the CHILD chat with the parent as
	// ancestor. The PathStore propagates the paths to all ancestor
	// chat IDs.
	ps.AddPaths([]uuid.UUID{childChatID, parentChatID}, []string{filepath.Join(repoDir, "child.go")})

	// The parent's git watch connection should receive a changes
	// message because AddPaths notified parentChatID's subscribers.
	msg := recvMsg(ctx, t, ch)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msg.Type)
	require.NotEmpty(t, msg.Repositories)

	foundRepo := false
	for _, r := range msg.Repositories {
		if r.RepoRoot == repoDir {
			foundRepo = true
			require.Contains(t, r.UnifiedDiff, "child.go")
		}
	}
	require.True(t, foundRepo, "parent watcher should see repo from child's tool call")
}

func TestE2E_MultipleConcurrentChatWatchers(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)

	// Create two separate git repos.
	repoA := initTestRepo(t)
	repoB := initTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoA, "a.go"), []byte("package a\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repoB, "b.go"), []byte("package b\n"), 0o600))

	ps := agentgit.NewPathStore()
	chatA := uuid.New()
	chatB := uuid.New()

	// Pre-populate each chat with its own repo's paths.
	ps.AddPaths([]uuid.UUID{chatA}, []string{filepath.Join(repoA, "a.go")})
	ps.AddPaths([]uuid.UUID{chatB}, []string{filepath.Join(repoB, "b.go")})

	// Connect two separate git watch WebSockets, one per chat.
	streamA := dialGitWatchWithPathStore(t, ps, chatA)
	chA := streamA.Chan()

	streamB := dialGitWatchWithPathStore(t, ps, chatB)
	chB := streamB.Chan()

	// Chat A should only see repoA.
	msgA := recvMsg(ctx, t, chA)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msgA.Type)
	require.NotEmpty(t, msgA.Repositories)
	for _, r := range msgA.Repositories {
		require.Equal(t, repoA, r.RepoRoot,
			"chatA should only see repoA, got %s", r.RepoRoot)
	}

	// Chat B should only see repoB.
	msgB := recvMsg(ctx, t, chB)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msgB.Type)
	require.NotEmpty(t, msgB.Repositories)
	for _, r := range msgB.Repositories {
		require.Equal(t, repoB, r.RepoRoot,
			"chatB should only see repoB, got %s", r.RepoRoot)
	}
}

func TestE2E_ReEditedFileTriggersRescan(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	repoDir := initTestRepo(t)

	// Write initial dirty file.
	filePath := filepath.Join(repoDir, "edited.go")
	require.NoError(t, os.WriteFile(filePath, []byte("package v1\n"), 0o600))

	ps := agentgit.NewPathStore()
	chatID := uuid.New()
	mClock := quartz.NewMock(t)

	// First AddPaths — registers the path and repo.
	ps.AddPaths([]uuid.UUID{chatID}, []string{filePath})

	stream := dialGitWatchWithPathStore(t, ps, chatID, agentgit.WithClock(mClock))
	ch := stream.Chan()

	// Receive the initial scan showing the dirty file.
	msg1 := recvMsg(ctx, t, ch)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msg1.Type)
	require.NotEmpty(t, msg1.Repositories)
	require.Contains(t, msg1.Repositories[0].UnifiedDiff, "v1")

	// Modify the same file again — the repo is already watched,
	// so Subscribe returns false. The handler must still scan.
	require.NoError(t, os.WriteFile(filePath, []byte("package v2\n"), 0o600))

	// Advance past the scan cooldown so the second scan fires
	// immediately.
	mClock.Advance(2 * time.Second).MustWait(context.Background())

	// AddPaths with the same path — triggers PathStore notification.
	ps.AddPaths([]uuid.UUID{chatID}, []string{filePath})

	// The handler should rescan and send an updated diff.
	msg2 := recvMsg(ctx, t, ch)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msg2.Type)
	require.NotEmpty(t, msg2.Repositories)
	require.Contains(t, msg2.Repositories[0].UnifiedDiff, "v2")
}

func TestE2E_RepoDeletionEmitsRemoved(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	repoDir := initTestRepo(t)

	// Write a dirty file so the initial scan has something to track.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "doomed.go"), []byte("package doomed\n"), 0o600))

	ps := agentgit.NewPathStore()
	chatID := uuid.New()
	mClock := quartz.NewMock(t)

	// Pre-populate paths and connect.
	ps.AddPaths([]uuid.UUID{chatID}, []string{filepath.Join(repoDir, "doomed.go")})

	stream := dialGitWatchWithPathStore(t, ps, chatID, agentgit.WithClock(mClock))
	ch := stream.Chan()

	// Receive the initial changes message.
	msg1 := recvMsg(ctx, t, ch)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msg1.Type)
	require.NotEmpty(t, msg1.Repositories)
	require.False(t, msg1.Repositories[0].Removed)

	// Delete the entire repo directory.
	require.NoError(t, os.RemoveAll(repoDir))

	// Advance past the scan cooldown so the refresh fires
	// immediately.
	mClock.Advance(2 * time.Second).MustWait(context.Background())

	// Send a refresh message to trigger a new scan.
	err := stream.Send(codersdk.WorkspaceAgentGitClientMessage{
		Type: codersdk.WorkspaceAgentGitClientMessageTypeRefresh,
	})
	require.NoError(t, err)

	// The next message should indicate the repo was removed.
	msg2 := recvMsg(ctx, t, ch)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msg2.Type)
	require.NotEmpty(t, msg2.Repositories)

	foundRemoved := false
	for _, r := range msg2.Repositories {
		if r.RepoRoot == repoDir && r.Removed {
			foundRemoved = true
		}
	}
	require.True(t, foundRemoved, "expected repo %s to be marked as removed", repoDir)
}

// TestRunLoopExitsPromptlyOnCancel_DuringPoll pins that RunLoop
// returns quickly when its context is cancelled while it is blocked
// on the fallback poll ticker. Regression guard for the fallback
// interval: if a future change introduces a non-cancellable wait
// here, this test will hang and fail.
func TestRunLoopExitsPromptlyOnCancel_DuringPoll(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	mClock := quartz.NewMock(t)
	h := agentgit.NewHandler(logger, agentgit.WithClock(mClock))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Trap NewTicker so the test can synchronize on RunLoop's
	// ticker creation rather than racing against it with a
	// best-effort Advance.
	tickerTrap := mClock.Trap().NewTicker()
	defer tickerTrap.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.RunLoop(ctx, func() {})
	}()

	// Wait until RunLoop has actually called clock.NewTicker, then
	// release the trap so the ticker is installed. At this point
	// RunLoop is deterministically inside its select, blocked on
	// <-ticker.C / <-scanTrigger / <-ctx.Done().
	tickerTrap.MustWait(ctx).MustRelease(ctx)

	cancel()

	select {
	case <-done:
	case <-time.After(testutil.WaitShort):
		t.Fatal("RunLoop did not return within WaitShort after ctx cancel")
	}
}

// TestRunLoopExitsPromptlyOnCancel_DuringCooldown pins that RunLoop
// returns quickly when its context is cancelled while a
// rateLimitedScan is sleeping out the cooldown between scans.
// Regression guard: all waits inside the cooldown path must select
// on ctx.Done().
func TestRunLoopExitsPromptlyOnCancel_DuringCooldown(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	logger := slogtest.Make(t, nil)
	mClock := quartz.NewMock(t)
	h := agentgit.NewHandler(logger, agentgit.WithClock(mClock))

	// Subscribe a real repo so Scan() actually does work and, on
	// completion, updates lastScanAt. Without this, Scan() early-
	// returns on empty roots and the cooldown branch never arms.
	require.True(t, h.Subscribe([]string{repoDir}))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Trap NewTicker (for RunLoop) and NewTimer (for the cooldown
	// wait inside rateLimitedScan) so the test synchronizes on each
	// wait point instead of racing against goroutine scheduling.
	tickerTrap := mClock.Trap().NewTicker()
	defer tickerTrap.Close()
	timerTrap := mClock.Trap().NewTimer()
	defer timerTrap.Close()

	scanStarted := make(chan struct{}, 1)
	blocked := make(chan struct{})
	scanFn := func() {
		// Run a real Scan so lastScanAt is set by the handler;
		// that is the precondition for the cooldown branch.
		_ = h.Scan(ctx)
		select {
		case scanStarted <- struct{}{}:
		default:
		}
		// Block until the test releases us, mimicking a slow
		// follow-up scan that parks RunLoop inside rateLimitedScan.
		<-blocked
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.RunLoop(ctx, scanFn)
	}()

	// Release the fallback ticker so RunLoop enters its select.
	tickerTrap.MustWait(ctx).MustRelease(ctx)

	// First trigger: consumed immediately (lastScanAt is zero).
	// scanFn runs Scan() (which sets lastScanAt), signals
	// scanStarted, then blocks on <-blocked.
	h.RequestScan()
	<-scanStarted

	// Release the first scan; RunLoop loops back to select.
	close(blocked)

	// Fire a second trigger. Because lastScanAt is fresh (set by
	// the real Scan above), rateLimitedScan enters its cooldown
	// wait and calls clock.NewTimer. The trap blocks the goroutine
	// inside that call until we release it, so we know exactly
	// when it is sitting on the cooldown select.
	h.RequestScan()
	timerCall := timerTrap.MustWait(ctx)

	// Cancel while the goroutine is still paused inside NewTimer.
	// Release the trap; rateLimitedScan then enters the select on
	// the cooldown timer vs. ctx.Done(), and ctx.Done() is already
	// ready so it wins. MustRelease uses Background because the
	// test ctx is the one we just cancelled.
	releaseCtx, releaseCancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer releaseCancel()
	cancel()
	timerCall.MustRelease(releaseCtx)

	select {
	case <-done:
	case <-time.After(testutil.WaitShort):
		t.Fatal("RunLoop did not return within WaitShort after ctx cancel during cooldown")
	}
}

// TestFallbackPollSkipsWhenRecentlyScanned pins the RunLoop optimization
// that swallows a fallback tick when a trigger-driven scan already
// covered the last fallback interval. Without the skip, a busy chat
// (agent editing + PathStore notifications) would pay the full fallback
// scan cost on top of trigger-driven scans.
func TestFallbackPollSkipsWhenRecentlyScanned(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	repoDir := initTestRepo(t)
	mClock := quartz.NewMock(t)

	ps := agentgit.NewPathStore()
	chatID := uuid.New()

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "a.go"), []byte("package a\n"), 0o600))
	ps.AddPaths([]uuid.UUID{chatID}, []string{filepath.Join(repoDir, "a.go")})

	stream := dialGitWatchWithPathStore(t, ps, chatID, agentgit.WithClock(mClock))
	ch := stream.Chan()

	// Consume the initial scan from subscribe.
	msg1 := recvMsg(ctx, t, ch)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msg1.Type)

	// A trigger-driven scan within the fallback interval should
	// cause the next fallback tick to be skipped. Advance part-way
	// to the 5s tick, fire a notification to trigger a scan, then
	// advance the rest of the way to the tick. The tick should be
	// swallowed because lastScanAt is recent.
	mClock.Advance(4 * time.Second).MustWait(context.Background())
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "a.go"), []byte("package a\n// edit\n"), 0o600))
	ps.Notify([]uuid.UUID{chatID})

	// Consume the trigger-driven scan. lastScanAt is now ~t=4s.
	msg2 := recvMsg(ctx, t, ch)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msg2.Type)

	// Dirty the tree further so the fallback tick would have
	// something to emit if it were not skipped.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "b.go"), []byte("package b\n"), 0o600))

	// Advance to the 5s ticker boundary. The tick fires but is
	// skipped because Since(lastScanAt) = 1s < fallbackPollInterval.
	mClock.Advance(1 * time.Second).MustWait(context.Background())

	// Confirm no scan arrived for the skipped tick.
	select {
	case msg := <-ch:
		t.Fatalf("unexpected scan after skipped fallback tick: %+v", msg)
	case <-time.After(testutil.IntervalFast):
	}

	// Advance to the next ticker boundary (t=10s). lastScanAt is
	// ~4s, so Since = 6s >= fallbackPollInterval and the tick
	// should no longer be skipped.
	mClock.Advance(5 * time.Second).MustWait(context.Background())

	msg3 := recvMsg(ctx, t, ch)
	require.Equal(t, codersdk.WorkspaceAgentGitServerMessageTypeChanges, msg3.Type)
}

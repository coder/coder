// Package agentgit provides a WebSocket-based service for watching git
// repository changes on the agent. It is mounted at /api/v0/git/watch
// and allows clients to subscribe to file paths, triggering scans of
// the corresponding git repositories.
package agentgit

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

// Option configures the git watch service.
type Option func(*Handler)

// WithClock sets a controllable clock for testing. Defaults to
// quartz.NewReal().
func WithClock(c quartz.Clock) Option {
	return func(h *Handler) {
		h.clock = c
	}
}

// WithGitBinary overrides the git binary path (for testing).
func WithGitBinary(path string) Option {
	return func(h *Handler) {
		h.gitBin = path
	}
}

const (
	// scanCooldown is the minimum interval between successive scans.
	scanCooldown = 1 * time.Second
	// fallbackPollInterval is the safety-net poll period used when no
	// filesystem events arrive.
	fallbackPollInterval = 30 * time.Second
	// maxTotalDiffSize is the maximum size of the combined
	// unified diff for an entire repository sent over the wire.
	// This must stay under the WebSocket message size limit.
	maxTotalDiffSize = 3 * 1024 * 1024 // 3 MiB
)

// Handler manages per-connection git watch state.
type Handler struct {
	logger slog.Logger
	clock  quartz.Clock
	gitBin string // path to git binary; empty means "git" (from PATH)

	mu            sync.Mutex
	repoRoots     map[string]struct{}     // watched repo roots
	lastSnapshots map[string]repoSnapshot // last emitted snapshot per repo
	lastScanAt    time.Time               // when the last scan completed
	scanTrigger   chan struct{}           // buffered(1), poked by triggers
}

// repoSnapshot captures the last emitted state for delta comparison.
type repoSnapshot struct {
	branch       string
	remoteOrigin string
	unifiedDiff  string
}

// NewHandler creates a new git watch handler.
func NewHandler(logger slog.Logger, opts ...Option) *Handler {
	h := &Handler{
		logger:        logger,
		clock:         quartz.NewReal(),
		gitBin:        "git",
		repoRoots:     make(map[string]struct{}),
		lastSnapshots: make(map[string]repoSnapshot),
		scanTrigger:   make(chan struct{}, 1),
	}
	for _, opt := range opts {
		opt(h)
	}

	// Check if git is available.
	if _, err := exec.LookPath(h.gitBin); err != nil {
		h.logger.Warn(context.Background(), "git binary not found, git scanning disabled")
	}

	return h
}

// gitAvailable returns true if the configured git binary can be found
// in PATH.
func (h *Handler) gitAvailable() bool {
	_, err := exec.LookPath(h.gitBin)
	return err == nil
}

// Subscribe processes a subscribe message, resolving paths to git repo
// roots and adding new repos to the watch set. Returns true if any new
// repo roots were added.
func (h *Handler) Subscribe(paths []string) bool {
	if !h.gitAvailable() {
		return false
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	added := false
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			continue
		}
		p = filepath.Clean(p)

		root, err := findRepoRoot(h.gitBin, p)
		if err != nil {
			// Not a git path — silently ignore.
			continue
		}
		if _, ok := h.repoRoots[root]; ok {
			continue
		}
		h.repoRoots[root] = struct{}{}
		added = true
	}
	return added
}

// RequestScan pokes the scan trigger so the run loop performs a scan.
func (h *Handler) RequestScan() {
	select {
	case h.scanTrigger <- struct{}{}:
	default:
		// Already pending.
	}
}

// Scan performs a scan of all subscribed repos and computes deltas
// against the previously emitted snapshots.
func (h *Handler) Scan(ctx context.Context) *codersdk.WorkspaceAgentGitServerMessage {
	if !h.gitAvailable() {
		return nil
	}

	h.mu.Lock()
	roots := make([]string, 0, len(h.repoRoots))
	for r := range h.repoRoots {
		roots = append(roots, r)
	}
	h.mu.Unlock()

	if len(roots) == 0 {
		return nil
	}

	now := h.clock.Now().UTC()
	var repos []codersdk.WorkspaceAgentRepoChanges

	// Perform all I/O outside the lock to avoid blocking
	// AddPaths/GetPaths/Subscribe callers during disk-heavy scans.
	type scanResult struct {
		root    string
		changes codersdk.WorkspaceAgentRepoChanges
		err     error
	}
	results := make([]scanResult, 0, len(roots))
	for _, root := range roots {
		changes, err := getRepoChanges(ctx, h.logger, h.gitBin, root)
		results = append(results, scanResult{root: root, changes: changes, err: err})
	}

	// Re-acquire the lock only to commit snapshot updates.
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, res := range results {
		if res.err != nil {
			if isRepoDeleted(h.gitBin, res.root) {
				// Repo root or .git directory was removed.
				// Emit a removal entry, then evict from watch set.
				removal := codersdk.WorkspaceAgentRepoChanges{
					RepoRoot: res.root,
					Removed:  true,
				}
				delete(h.repoRoots, res.root)
				delete(h.lastSnapshots, res.root)
				repos = append(repos, removal)
			} else {
				// Transient error — log and skip without
				// removing the repo from the watch set.
				h.logger.Warn(ctx, "scan repo failed",
					slog.F("root", res.root),
					slog.Error(res.err),
				)
			}
			continue
		}

		prev, hasPrev := h.lastSnapshots[res.root]
		if hasPrev &&
			prev.branch == res.changes.Branch &&
			prev.remoteOrigin == res.changes.RemoteOrigin &&
			prev.unifiedDiff == res.changes.UnifiedDiff {
			// No change in this repo since last emit.
			continue
		}

		// Update snapshot.
		h.lastSnapshots[res.root] = repoSnapshot{
			branch:       res.changes.Branch,
			remoteOrigin: res.changes.RemoteOrigin,
			unifiedDiff:  res.changes.UnifiedDiff,
		}

		repos = append(repos, res.changes)
	}

	h.lastScanAt = now

	if len(repos) == 0 {
		return nil
	}

	return &codersdk.WorkspaceAgentGitServerMessage{
		Type:         codersdk.WorkspaceAgentGitServerMessageTypeChanges,
		ScannedAt:    &now,
		Repositories: repos,
	}
}

// RunLoop runs the main event loop that listens for refresh requests
// and fallback poll ticks. It calls scanFn whenever a scan should
// happen (rate-limited to scanCooldown). It blocks until ctx is
// canceled.
func (h *Handler) RunLoop(ctx context.Context, scanFn func()) {
	fallbackTicker := h.clock.NewTicker(fallbackPollInterval)
	defer fallbackTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-h.scanTrigger:
			h.rateLimitedScan(ctx, scanFn)

		case <-fallbackTicker.C:
			h.rateLimitedScan(ctx, scanFn)
		}
	}
}

func (h *Handler) rateLimitedScan(ctx context.Context, scanFn func()) {
	h.mu.Lock()
	elapsed := h.clock.Since(h.lastScanAt)
	if elapsed < scanCooldown {
		h.mu.Unlock()

		// Wait for cooldown then scan.
		remaining := scanCooldown - elapsed
		timer := h.clock.NewTimer(remaining)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		scanFn()
		return
	}
	h.mu.Unlock()
	scanFn()
}

// isRepoDeleted returns true when the repo root directory or its .git
// entry no longer represents a valid git repository. This
// distinguishes a genuine repo deletion from a transient scan error
// (e.g. lock contention).
//
// It handles three deletion cases:
//  1. The repo root directory itself was removed.
//  2. The .git entry (directory or file) was removed.
//  3. The .git entry is a file (worktree/submodule) whose target
//     gitdir was removed. In this case .git exists on disk but
//     `git rev-parse --git-dir` fails because the referenced
//     directory is gone.
func isRepoDeleted(gitBin string, repoRoot string) bool {
	if _, err := os.Stat(repoRoot); os.IsNotExist(err) {
		return true
	}
	gitPath := filepath.Join(repoRoot, ".git")
	fi, err := os.Stat(gitPath)
	if os.IsNotExist(err) {
		return true
	}
	// If .git is a regular file (worktree or submodule), the actual
	// git object store lives elsewhere. Validate that the target is
	// still reachable by running git rev-parse.
	if err == nil && !fi.IsDir() {
		cmd := exec.CommandContext(context.Background(), gitBin, "-C", repoRoot, "rev-parse", "--git-dir")
		if err := cmd.Run(); err != nil {
			return true
		}
	}
	return false
}

// findRepoRoot uses `git rev-parse --show-toplevel` to find the
// repository root for the given path.
func findRepoRoot(gitBin string, p string) (string, error) {
	// If p is a file, start from its parent directory.
	dir := p
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	cmd := exec.CommandContext(context.Background(), gitBin, "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", xerrors.Errorf("no git repo found for %s", p)
	}
	root := filepath.FromSlash(strings.TrimSpace(string(out)))
	// Resolve symlinks and short (8.3) names on Windows so the
	// returned root matches paths produced by Go's filepath APIs.
	if resolved, evalErr := filepath.EvalSymlinks(root); evalErr == nil {
		root = resolved
	}
	return root, nil
}

// getRepoChanges reads the current state of a git repository using
// the git CLI. It returns branch, remote origin, and a unified diff.
func getRepoChanges(ctx context.Context, logger slog.Logger, gitBin string, repoRoot string) (codersdk.WorkspaceAgentRepoChanges, error) {
	result := codersdk.WorkspaceAgentRepoChanges{
		RepoRoot: repoRoot,
	}

	// Verify this is still a valid git repository before doing
	// anything else. This catches deleted repos early.
	verifyCmd := exec.CommandContext(ctx, gitBin, "-C", repoRoot, "rev-parse", "--git-dir")
	if err := verifyCmd.Run(); err != nil {
		return result, xerrors.Errorf("not a git repository: %w", err)
	}

	// Read branch name.
	branchCmd := exec.CommandContext(ctx, gitBin, "-C", repoRoot, "symbolic-ref", "--short", "HEAD")
	if out, err := branchCmd.Output(); err == nil {
		result.Branch = strings.TrimSpace(string(out))
	} else {
		logger.Debug(ctx, "failed to read HEAD", slog.F("root", repoRoot), slog.Error(err))
	}

	// Read remote origin URL.
	remoteCmd := exec.CommandContext(ctx, gitBin, "-C", repoRoot, "config", "--get", "remote.origin.url")
	if out, err := remoteCmd.Output(); err == nil {
		result.RemoteOrigin = strings.TrimSpace(string(out))
	}

	// Compute unified diff.
	// `git diff HEAD` shows both staged and unstaged changes vs HEAD.
	// For repos with no commits yet, fall back to showing untracked
	// files only.
	diff, err := computeGitDiff(ctx, logger, gitBin, repoRoot)
	if err != nil {
		return result, xerrors.Errorf("compute diff: %w", err)
	}

	result.UnifiedDiff = diff
	if len(result.UnifiedDiff) > maxTotalDiffSize {
		result.UnifiedDiff = "Total diff too large to show. Size: " + humanize.IBytes(uint64(len(result.UnifiedDiff))) + ". Showing branch and remote only."
	}

	return result, nil
}

// computeGitDiff produces a unified diff string for the repository by
// combining `git diff HEAD` (staged + unstaged changes) with diffs
// for untracked files.
func computeGitDiff(ctx context.Context, logger slog.Logger, gitBin string, repoRoot string) (string, error) {
	var diffParts []string

	// Check if the repo has any commits.
	hasCommits := true
	checkCmd := exec.CommandContext(ctx, gitBin, "-C", repoRoot, "rev-parse", "HEAD")
	if err := checkCmd.Run(); err != nil {
		hasCommits = false
	}

	if hasCommits {
		// `git diff HEAD` captures both staged and unstaged changes
		// relative to HEAD in a single unified diff.
		cmd := exec.CommandContext(ctx, gitBin, "-C", repoRoot, "diff", "HEAD")
		out, err := cmd.Output()
		if err != nil {
			return "", xerrors.Errorf("git diff HEAD: %w", err)
		}
		if len(out) > 0 {
			diffParts = append(diffParts, string(out))
		}
	}

	// Show untracked files as diffs too.
	// `git ls-files --others --exclude-standard` lists untracked,
	// non-ignored files.
	lsCmd := exec.CommandContext(ctx, gitBin, "-C", repoRoot, "ls-files", "--others", "--exclude-standard")
	lsOut, err := lsCmd.Output()
	if err != nil {
		logger.Debug(ctx, "failed to list untracked files", slog.F("root", repoRoot), slog.Error(err))
		return strings.Join(diffParts, ""), nil
	}

	untrackedFiles := strings.Split(strings.TrimSpace(string(lsOut)), "\n")
	for _, f := range untrackedFiles {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		// Use `git diff --no-index /dev/null <file>` to generate
		// a unified diff for untracked files.
		var stdout bytes.Buffer
		untrackedCmd := exec.CommandContext(ctx, gitBin, "-C", repoRoot, "diff", "--no-index", "--", "/dev/null", f)
		untrackedCmd.Stdout = &stdout
		// git diff --no-index exits with 1 when files differ,
		// which is expected. We ignore the error and check for
		// output instead.
		_ = untrackedCmd.Run()
		if stdout.Len() > 0 {
			diffParts = append(diffParts, stdout.String())
		}
	}

	return strings.Join(diffParts, ""), nil
}

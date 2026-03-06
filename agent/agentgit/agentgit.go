// Package agentgit provides a WebSocket-based service for watching git
// repository changes on the agent. It is mounted at /api/v0/git/watch
// and allows clients to subscribe to file paths, triggering scans of
// the corresponding git repositories.
package agentgit

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	fdiff "github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/utils/diff"
	dmp "github.com/sergi/go-diff/diffmatchpatch"
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

const (
	// scanCooldown is the minimum interval between successive scans.
	scanCooldown = 1 * time.Second
	// fallbackPollInterval is the safety-net poll period used when no
	// filesystem events arrive.
	fallbackPollInterval = 30 * time.Second
	// maxFileReadSize is the maximum file size that will be read
	// into memory. Files larger than this are tracked by status
	// only, and their diffs show a placeholder message.
	maxFileReadSize = 2 * 1024 * 1024 // 2 MiB
	// maxFileDiffSize is the maximum encoded size of a single
	// file's diff. If an individual file's diff exceeds this
	// limit, it is replaced with a placeholder stub.
	maxFileDiffSize = 256 * 1024 // 256 KiB
	// maxTotalDiffSize is the maximum size of the combined
	// unified diff for an entire repository sent over the wire.
	// This must stay under the WebSocket message size limit.
	maxTotalDiffSize = 3 * 1024 * 1024 // 3 MiB
)

// Handler manages per-connection git watch state.
type Handler struct {
	logger slog.Logger
	clock  quartz.Clock

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
		repoRoots:     make(map[string]struct{}),
		lastSnapshots: make(map[string]repoSnapshot),
		scanTrigger:   make(chan struct{}, 1),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Subscribe processes a subscribe message, resolving paths to git repo
// roots and adding new repos to the watch set. Returns true if any new
// repo roots were added.
func (h *Handler) Subscribe(paths []string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	added := false
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			continue
		}
		p = filepath.Clean(p)

		root, err := findRepoRoot(p)
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
		changes, err := getRepoChanges(ctx, h.logger, root)
		results = append(results, scanResult{root: root, changes: changes, err: err})
	}

	// Re-acquire the lock only to commit snapshot updates.
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, res := range results {
		if res.err != nil {
			if isRepoDeleted(res.root) {
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
//     git.PlainOpen fails because the referenced directory is gone.
func isRepoDeleted(repoRoot string) bool {
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
	// still reachable by attempting to open the repo.
	if err == nil && !fi.IsDir() {
		if _, openErr := git.PlainOpen(repoRoot); openErr != nil {
			return true
		}
	}
	return false
}

// findRepoRoot walks up from the given path to find a .git directory.
func findRepoRoot(p string) (string, error) {
	// If p is a file, start from its directory.
	dir := p
	for {
		_, err := git.PlainOpen(dir)
		if err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", xerrors.Errorf("no git repo found for %s", p)
		}
		dir = parent
	}
}

// getRepoChanges reads the current state of a git repository using
// go-git. It returns branch, remote origin, and per-file status.
func getRepoChanges(ctx context.Context, logger slog.Logger, repoRoot string) (codersdk.WorkspaceAgentRepoChanges, error) {
	repo, err := git.PlainOpen(repoRoot)
	if err != nil {
		return codersdk.WorkspaceAgentRepoChanges{}, xerrors.Errorf("open repo: %w", err)
	}

	result := codersdk.WorkspaceAgentRepoChanges{
		RepoRoot: repoRoot,
	}

	// Read branch.
	headRef, err := repo.Head()
	if err != nil {
		// Repo may have no commits yet.
		logger.Debug(ctx, "failed to read HEAD", slog.F("root", repoRoot), slog.Error(err))
	} else if headRef.Name().IsBranch() {
		result.Branch = headRef.Name().Short()
	}

	// Read remote origin URL.
	cfg, err := repo.Config()
	if err == nil {
		if origin, ok := cfg.Remotes["origin"]; ok && len(origin.URLs) > 0 {
			result.RemoteOrigin = origin.URLs[0]
		}
	}

	// Get worktree status.
	wt, err := repo.Worktree()
	if err != nil {
		return result, xerrors.Errorf("get worktree: %w", err)
	}

	status, err := wt.Status()
	if err != nil {
		return result, xerrors.Errorf("worktree status: %w", err)
	}

	worktreeDiff, err := computeWorktreeDiff(repo, repoRoot, status)
	if err != nil {
		return result, xerrors.Errorf("compute worktree diff: %w", err)
	}

	result.UnifiedDiff = worktreeDiff.unifiedDiff
	if len(result.UnifiedDiff) > maxTotalDiffSize {
		result.UnifiedDiff = "Total diff too large to show. Size: " + humanize.IBytes(uint64(len(result.UnifiedDiff))) + ". Showing branch and remote only."
	}

	return result, nil
}

type worktreeDiffResult struct {
	unifiedDiff string
	additions   int
	deletions   int
}

type fileSnapshot struct {
	exists   bool
	content  []byte
	mode     filemode.FileMode
	binary   bool
	tooLarge bool
	size     int64 // actual file size on disk, set even when tooLarge
}

func computeWorktreeDiff(
	repo *git.Repository,
	repoRoot string,
	status git.Status,
) (worktreeDiffResult, error) {
	headTree, err := getHeadTree(repo)
	if err != nil {
		return worktreeDiffResult{}, xerrors.Errorf("get head tree: %w", err)
	}

	paths := sortedStatusPaths(status)
	filePatches := make([]fdiff.FilePatch, 0, len(paths))
	totalAdditions := 0
	totalDeletions := 0

	for _, path := range paths {
		fileStatus := status[path]

		fromPath := path
		if isRenamed(fileStatus) && fileStatus.Extra != "" {
			fromPath = fileStatus.Extra
		}
		toPath := path

		before, err := readHeadFileSnapshot(headTree, fromPath)
		if err != nil {
			return worktreeDiffResult{}, xerrors.Errorf("read head file %q: %w", fromPath, err)
		}

		after, err := readWorktreeFileSnapshot(repoRoot, toPath)
		if err != nil {
			return worktreeDiffResult{}, xerrors.Errorf("read worktree file %q: %w", toPath, err)
		}

		filePatch, additions, deletions := buildFilePatch(fromPath, toPath, before, after)
		if filePatch == nil {
			continue
		}

		// Check whether this single file's diff exceeds the
		// per-file limit. If so, replace it with a stub.
		encoded, err := encodeUnifiedDiff([]fdiff.FilePatch{filePatch})
		if err != nil {
			return worktreeDiffResult{}, xerrors.Errorf("encode file diff %q: %w", toPath, err)
		}
		if len(encoded) > maxFileDiffSize {
			msg := "File diff too large to show. Diff size: " + humanize.IBytes(uint64(len(encoded)))
			filePatch = buildStubFilePatch(fromPath, toPath, before, after, msg)
			additions = 0
			deletions = 0
		}

		filePatches = append(filePatches, filePatch)
		totalAdditions += additions
		totalDeletions += deletions
	}

	diffText, err := encodeUnifiedDiff(filePatches)
	if err != nil {
		return worktreeDiffResult{}, xerrors.Errorf("encode unified diff: %w", err)
	}

	return worktreeDiffResult{
		unifiedDiff: diffText,
		additions:   totalAdditions,
		deletions:   totalDeletions,
	}, nil
}

func getHeadTree(repo *git.Repository) (*object.Tree, error) {
	headRef, err := repo.Head()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return nil, nil
		}
		return nil, err
	}

	commit, err := repo.CommitObject(headRef.Hash())
	if err != nil {
		return nil, err
	}

	return commit.Tree()
}

func readHeadFileSnapshot(headTree *object.Tree, path string) (fileSnapshot, error) {
	if headTree == nil {
		return fileSnapshot{}, nil
	}

	file, err := headTree.File(path)
	if err != nil {
		if errors.Is(err, object.ErrFileNotFound) {
			return fileSnapshot{}, nil
		}
		return fileSnapshot{}, err
	}

	if file.Size > maxFileReadSize {
		return fileSnapshot{
			exists:   true,
			tooLarge: true,
			size:     file.Size,
			mode:     file.Mode,
		}, nil
	}

	content, err := file.Contents()
	if err != nil {
		return fileSnapshot{}, err
	}

	isBinary, err := file.IsBinary()
	if err != nil {
		return fileSnapshot{}, err
	}

	return fileSnapshot{
		exists:  true,
		content: []byte(content),
		mode:    file.Mode,
		binary:  isBinary,
	}, nil
}

func readWorktreeFileSnapshot(repoRoot string, path string) (fileSnapshot, error) {
	absPath := filepath.Join(repoRoot, filepath.FromSlash(path))
	fileInfo, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fileSnapshot{}, nil
		}
		return fileSnapshot{}, err
	}
	if fileInfo.IsDir() {
		return fileSnapshot{}, nil
	}

	if fileInfo.Size() > maxFileReadSize {
		mode, err := filemode.NewFromOSFileMode(fileInfo.Mode())
		if err != nil {
			mode = filemode.Regular
		}
		return fileSnapshot{
			exists:   true,
			tooLarge: true,
			size:     fileInfo.Size(),
			mode:     mode,
		}, nil
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fileSnapshot{}, nil
		}
		return fileSnapshot{}, err
	}

	mode, err := filemode.NewFromOSFileMode(fileInfo.Mode())
	if err != nil {
		mode = filemode.Regular
	}

	return fileSnapshot{
		exists:  true,
		content: content,
		mode:    mode,
		binary:  isBinaryContent(content),
		size:    fileInfo.Size(),
	}, nil
}

func buildFilePatch(
	fromPath string,
	toPath string,
	before fileSnapshot,
	after fileSnapshot,
) (fdiff.FilePatch, int, int) {
	if !before.exists && !after.exists {
		return nil, 0, 0
	}

	unchangedContent := bytes.Equal(before.content, after.content)
	if before.exists &&
		after.exists &&
		fromPath == toPath &&
		before.mode == after.mode &&
		unchangedContent {
		return nil, 0, 0
	}

	// Files that exceed the read size limit get a stub patch
	// instead of a full diff to avoid OOM.
	if before.tooLarge || after.tooLarge {
		sz := max(after.size, 0)
		//nolint:gosec // sz is guaranteed to fit in uint64
		msg := "File too large to diff. Current size: " + humanize.IBytes(uint64(sz))
		return buildStubFilePatch(fromPath, toPath, before, after, msg), 0, 0
	}

	patch := &workspaceFilePatch{
		from: snapshotToDiffFile(fromPath, before),
		to:   snapshotToDiffFile(toPath, after),
	}

	if before.binary || after.binary {
		patch.binary = true
		return patch, 0, 0
	}

	diffs := diff.Do(string(before.content), string(after.content))
	chunks := make([]fdiff.Chunk, 0, len(diffs))
	additions := 0
	deletions := 0

	for _, d := range diffs {
		var operation fdiff.Operation
		switch d.Type {
		case dmp.DiffEqual:
			operation = fdiff.Equal
		case dmp.DiffDelete:
			operation = fdiff.Delete
			deletions += countChunkLines(d.Text)
		case dmp.DiffInsert:
			operation = fdiff.Add
			additions += countChunkLines(d.Text)
		default:
			continue
		}

		chunks = append(chunks, workspaceDiffChunk{
			content: d.Text,
			op:      operation,
		})
	}

	patch.chunks = chunks
	return patch, additions, deletions
}

func buildStubFilePatch(fromPath, toPath string, before, after fileSnapshot, message string) fdiff.FilePatch {
	return &workspaceFilePatch{
		from: snapshotToDiffFile(fromPath, before),
		to:   snapshotToDiffFile(toPath, after),
		chunks: []fdiff.Chunk{
			workspaceDiffChunk{
				content: message + "\n",
				op:      fdiff.Add,
			},
		},
	}
}

func snapshotToDiffFile(path string, snapshot fileSnapshot) fdiff.File {
	if !snapshot.exists {
		return nil
	}

	return workspaceDiffFile{
		path: path,
		mode: snapshot.mode,
		hash: plumbing.ComputeHash(plumbing.BlobObject, snapshot.content),
	}
}

func encodeUnifiedDiff(filePatches []fdiff.FilePatch) (string, error) {
	if len(filePatches) == 0 {
		return "", nil
	}

	patch := workspaceDiffPatch{filePatches: filePatches}
	var builder strings.Builder
	encoder := fdiff.NewUnifiedEncoder(&builder, fdiff.DefaultContextLines)
	if err := encoder.Encode(patch); err != nil {
		return "", err
	}

	return builder.String(), nil
}

func sortedStatusPaths(status git.Status) []string {
	paths := make([]string, 0, len(status))
	for path := range status {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func isRenamed(fileStatus *git.FileStatus) bool {
	return fileStatus.Staging == git.Renamed || fileStatus.Worktree == git.Renamed
}

func countChunkLines(content string) int {
	if content == "" {
		return 0
	}

	lines := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") {
		lines++
	}
	return lines
}

func isBinaryContent(content []byte) bool {
	return bytes.IndexByte(content, 0) >= 0
}

type workspaceDiffPatch struct {
	filePatches []fdiff.FilePatch
}

func (p workspaceDiffPatch) FilePatches() []fdiff.FilePatch {
	return p.filePatches
}

func (workspaceDiffPatch) Message() string {
	return ""
}

type workspaceFilePatch struct {
	from   fdiff.File
	to     fdiff.File
	chunks []fdiff.Chunk
	binary bool
}

func (p *workspaceFilePatch) IsBinary() bool {
	return p.binary
}

func (p *workspaceFilePatch) Files() (fdiff.File, fdiff.File) {
	return p.from, p.to
}

func (p *workspaceFilePatch) Chunks() []fdiff.Chunk {
	return p.chunks
}

type workspaceDiffFile struct {
	path string
	mode filemode.FileMode
	hash plumbing.Hash
}

func (f workspaceDiffFile) Hash() plumbing.Hash {
	return f.hash
}

func (f workspaceDiffFile) Mode() filemode.FileMode {
	return f.mode
}

func (f workspaceDiffFile) Path() string {
	return f.path
}

type workspaceDiffChunk struct {
	content string
	op      fdiff.Operation
}

func (c workspaceDiffChunk) Content() string {
	return c.content
}

func (c workspaceDiffChunk) Type() fdiff.Operation {
	return c.op
}

package agentgitchanges

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	fdiff "github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
	godiff "github.com/go-git/go-git/v5/utils/diff"
	dmp "github.com/sergi/go-diff/diffmatchpatch"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
	"github.com/coder/websocket"
)

const (
	defaultUpdateInterval = 5 * time.Second
	// maxDiffSize caps unified diff output per repo.
	maxDiffSize = 1 << 20 // 1 MiB
	// maxDiscoveryDepth limits how deep we traverse to find git repos.
	maxDiscoveryDepth = 5
)

// Option is a functional option for configuring the API.
type Option func(*API)

// WithUpdateInterval sets the interval between git scans.
func WithUpdateInterval(d time.Duration) Option {
	return func(a *API) {
		a.updateInterval = d
	}
}

// WithClock sets the clock for testing.
func WithClock(c quartz.Clock) Option {
	return func(a *API) {
		a.clock = c
	}
}

// WithDirectory sets the root directory to scan for git repos.
func WithDirectory(dir string) Option {
	return func(a *API) {
		a.directory = dir
	}
}

// API provides git-changes streaming over HTTP/WebSocket.
// It periodically scans for git repos and computes working
// directory diffs, broadcasting changes to connected clients.
type API struct {
	ctx    context.Context
	cancel context.CancelFunc

	updaterDone chan struct{}
	logger      slog.Logger
	clock       quartz.Clock

	updateInterval time.Duration
	directory      string

	mu                sync.RWMutex
	closed            bool
	initialUpdateDone chan struct{}
	updateChans       []chan struct{}
	lastResponse      codersdk.WorkspaceAgentGitChangesResponse
	lastDiffHash      string // Simple hash to detect changes.
}

// NewAPI creates a new git changes API.
func NewAPI(logger slog.Logger, options ...Option) *API {
	ctx, cancel := context.WithCancel(context.Background())
	api := &API{
		ctx:               ctx,
		cancel:            cancel,
		initialUpdateDone: make(chan struct{}),
		updateInterval:    defaultUpdateInterval,
		logger:            logger,
		clock:             quartz.NewReal(),
	}
	for _, opt := range options {
		opt(api)
	}
	return api
}

// Init applies additional options to the API. This must be
// called before Start.
func (api *API) Init(opts ...Option) {
	api.mu.Lock()
	defer api.mu.Unlock()
	for _, opt := range opts {
		opt(api)
	}
}

// Start begins the background scan loop.
func (api *API) Start() {
	api.mu.Lock()
	defer api.mu.Unlock()
	if api.closed {
		return
	}
	api.updaterDone = make(chan struct{})
	go api.updaterLoop()
}

// Routes returns an http.Handler for the git changes endpoints.
func (api *API) Routes() http.Handler {
	r := chi.NewRouter()

	ensureInitialUpdateDoneMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			select {
			case <-api.ctx.Done():
				httpapi.Write(r.Context(), rw, http.StatusServiceUnavailable, codersdk.Response{
					Message: "API closed",
					Detail:  "The API is closed and cannot process requests.",
				})
				return
			case <-r.Context().Done():
				return
			case <-api.initialUpdateDone:
			}
			next.ServeHTTP(rw, r)
		})
	}

	r.Use(ensureInitialUpdateDoneMW)
	r.Get("/", api.handleList)
	r.Get("/watch", api.handleWatch)

	return r
}

func (api *API) handleList(rw http.ResponseWriter, r *http.Request) {
	resp := api.getResponse()
	httpapi.Write(r.Context(), rw, http.StatusOK, resp)
}

func (api *API) handleWatch(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionNoContextTakeover,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to upgrade connection to websocket.",
			Detail:  err.Error(),
		})
		return
	}

	_ = conn.CloseRead(context.Background())

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ctx, wsNetConn := codersdk.WebsocketNetConn(ctx, conn, websocket.MessageText)
	defer wsNetConn.Close()

	go httpapi.HeartbeatClose(ctx, api.logger, cancel, conn)

	updateCh := make(chan struct{}, 1)

	api.mu.Lock()
	api.updateChans = append(api.updateChans, updateCh)
	api.mu.Unlock()

	defer func() {
		api.mu.Lock()
		api.updateChans = slices.DeleteFunc(api.updateChans, func(ch chan struct{}) bool {
			return ch == updateCh
		})
		close(updateCh)
		api.mu.Unlock()
	}()

	encoder := json.NewEncoder(wsNetConn)

	// Send initial state.
	resp := api.getResponse()
	if err := encoder.Encode(resp); err != nil {
		api.logger.Error(ctx, "encode git changes", slog.Error(err))
		return
	}

	for {
		select {
		case <-api.ctx.Done():
			return
		case <-ctx.Done():
			return
		case <-updateCh:
			resp := api.getResponse()
			if err := encoder.Encode(resp); err != nil {
				api.logger.Error(ctx, "encode git changes", slog.Error(err))
				return
			}
		}
	}
}

func (api *API) getResponse() codersdk.WorkspaceAgentGitChangesResponse {
	api.mu.RLock()
	defer api.mu.RUnlock()
	return api.lastResponse
}

// broadcastUpdatesLocked sends a signal to all listening clients.
// Caller must hold api.mu.
func (api *API) broadcastUpdatesLocked() {
	for _, ch := range api.updateChans {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (api *API) updaterLoop() {
	defer close(api.updaterDone)
	defer api.logger.Debug(api.ctx, "git changes updater loop stopped")
	api.logger.Debug(api.ctx, "git changes updater loop started")

	// Perform initial scan.
	api.updateGitChanges()
	close(api.initialUpdateDone)

	ticker := api.clock.TickerFunc(api.ctx, api.updateInterval, func() error {
		api.updateGitChanges()
		return nil
	}, "agentgitchanges", "updaterLoop")

	// Wait blocks until the ticker is done (context cancelled).
	_ = ticker.Wait("agentgitchanges", "updaterLoop")
}

func (api *API) updateGitChanges() {
	if api.directory == "" {
		return
	}

	repos := discoverRepos(api.ctx, api.logger, api.directory)
	var response codersdk.WorkspaceAgentGitChangesResponse

	for _, repoRoot := range repos {
		rc, err := getRepoChanges(api.ctx, api.logger, repoRoot)
		if err != nil {
			api.logger.Warn(api.ctx, "failed to get repo changes",
				slog.F("repo_root", repoRoot),
				slog.Error(err),
			)
			continue
		}
		response.Repos = append(response.Repos, rc)
	}

	// Sort repos by path for deterministic output.
	sort.Slice(response.Repos, func(i, j int) bool {
		return response.Repos[i].RepoRoot < response.Repos[j].RepoRoot
	})

	// Build a simple content hash to detect changes.
	hash := buildDiffHash(response)

	api.mu.Lock()
	defer api.mu.Unlock()

	if hash != api.lastDiffHash {
		api.lastResponse = response
		api.lastDiffHash = hash
		if len(api.updateChans) > 0 {
			api.broadcastUpdatesLocked()
		}
	}
}

// discoverRepos walks the directory tree to find git repositories.
// It deduplicates by repo root and limits traversal depth.
func discoverRepos(ctx context.Context, logger slog.Logger, root string) []string {
	seen := make(map[string]bool)
	var repos []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return nil // Skip inaccessible paths.
		}

		// Limit depth.
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		depth := strings.Count(rel, string(filepath.Separator))
		if depth > maxDiscoveryDepth {
			return filepath.SkipDir
		}

		if !d.IsDir() {
			return nil
		}

		// Skip hidden directories other than ".git".
		if d.Name() != "." && strings.HasPrefix(d.Name(), ".") && d.Name() != ".git" {
			return filepath.SkipDir
		}

		// Check if this directory contains a .git directory.
		gitDir := filepath.Join(path, ".git")
		info, statErr := os.Stat(gitDir)
		if statErr != nil || !info.IsDir() {
			return nil
		}

		repoRoot := path
		if !seen[repoRoot] {
			seen[repoRoot] = true
			repos = append(repos, repoRoot)
		}

		// Don't descend into this repo's subdirectories
		// for finding more repos.
		if path != root {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil && ctx.Err() == nil {
		logger.Warn(ctx, "error walking directory for git repos", slog.Error(err))
	}

	sort.Strings(repos)
	return repos
}

// getRepoChanges computes the working directory changes for a
// single git repository using go-git (in-process, no CLI).
func getRepoChanges(_ context.Context, _ slog.Logger, repoRoot string) (codersdk.WorkspaceAgentRepoChanges, error) {
	rc := codersdk.WorkspaceAgentRepoChanges{
		RepoRoot: repoRoot,
	}

	repo, err := git.PlainOpen(repoRoot)
	if err != nil {
		return rc, err
	}

	// Get branch name.
	headRef, err := repo.Head()
	if err == nil && headRef.Name().IsBranch() {
		rc.Branch = headRef.Name().Short()
	}

	// Get remote origin URL.
	cfg, err := repo.Config()
	if err == nil {
		if origin, ok := cfg.Remotes["origin"]; ok && len(origin.URLs) > 0 {
			rc.RemoteOrigin = origin.URLs[0]
		}
	}

	// Compute unified diff and stats for working directory changes.
	diffStr, changedFiles, additions, deletions, untrackedFiles, err := computeWorktreeDiff(repo, repoRoot)
	if err != nil {
		return rc, err
	}

	rc.UnifiedDiff = diffStr
	rc.ChangedFiles = changedFiles
	rc.Additions = additions
	rc.Deletions = deletions
	rc.UntrackedFiles = untrackedFiles

	return rc, nil
}

// worktreeFile implements fdiff.File for encoding unified diffs.
type worktreeFile struct {
	hash plumbing.Hash
	mode filemode.FileMode
	path string
}

func (f *worktreeFile) Hash() plumbing.Hash     { return f.hash }
func (f *worktreeFile) Mode() filemode.FileMode { return f.mode }
func (f *worktreeFile) Path() string            { return f.path }

// worktreeChunk implements fdiff.Chunk.
type worktreeChunk struct {
	content string
	op      fdiff.Operation
}

func (c *worktreeChunk) Content() string       { return c.content }
func (c *worktreeChunk) Type() fdiff.Operation { return c.op }

// worktreeFilePatch implements fdiff.FilePatch.
type worktreeFilePatch struct {
	isBinary bool
	from, to fdiff.File
	chunks   []fdiff.Chunk
}

func (fp *worktreeFilePatch) IsBinary() bool                  { return fp.isBinary }
func (fp *worktreeFilePatch) Files() (fdiff.File, fdiff.File) { return fp.from, fp.to }
func (fp *worktreeFilePatch) Chunks() []fdiff.Chunk           { return fp.chunks }

// worktreePatch implements fdiff.Patch.
type worktreePatch struct {
	filePatches []fdiff.FilePatch
	message     string
}

func (p *worktreePatch) FilePatches() []fdiff.FilePatch { return p.filePatches }
func (p *worktreePatch) Message() string                { return p.message }

// computeWorktreeDiff diffs HEAD against the working directory and
// returns a unified diff string, stats, and the list of untracked
// files. It mirrors `git diff HEAD` plus synthetic diffs for
// untracked files.
func computeWorktreeDiff(repo *git.Repository, repoRoot string) (
	unifiedDiff string,
	changedFiles, additions, deletions int,
	untrackedFiles []string,
	err error,
) {
	// Get HEAD tree (nil for repos with no commits yet).
	var headTree *object.Tree
	headRef, err := repo.Head()
	if err == nil {
		commit, cerr := repo.CommitObject(headRef.Hash())
		if cerr == nil {
			headTree, _ = commit.Tree()
		}
	}

	wt, err := repo.Worktree()
	if err != nil {
		return "", 0, 0, 0, nil, err
	}

	status, err := wt.Status()
	if err != nil {
		return "", 0, 0, 0, nil, err
	}

	// Sort paths for deterministic output.
	paths := make([]string, 0, len(status))
	for p := range status {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var (
		filePatches []fdiff.FilePatch
		diffSize    int
	)

	for _, path := range paths {
		fs := status[path]

		// Collect untracked files separately; we append their
		// diffs after tracked changes.
		if fs.Staging == git.Untracked && fs.Worktree == git.Untracked {
			untrackedFiles = append(untrackedFiles, path)
			continue
		}

		// Skip files with no changes relative to HEAD.
		if fs.Staging == git.Unmodified && fs.Worktree == git.Unmodified {
			continue
		}

		fp, adds, dels, fErr := buildFilePatch(headTree, repoRoot, path)
		if fErr != nil {
			// Skip files we can't diff (e.g. permission errors).
			continue
		}

		changedFiles++
		additions += adds
		deletions += dels
		filePatches = append(filePatches, fp)

		// Estimate encoded size and stop at file boundaries if
		// we exceed the budget.
		diffSize += estimateFilePatchSize(fp)
		if diffSize > maxDiffSize {
			break
		}
	}

	// Append synthetic diffs for untracked files.
	for _, path := range untrackedFiles {
		fp, adds, _, fErr := buildNewFilePatch(repoRoot, path)
		if fErr != nil {
			continue
		}

		changedFiles++
		additions += adds
		filePatches = append(filePatches, fp)

		diffSize += estimateFilePatchSize(fp)
		if diffSize > maxDiffSize {
			break
		}
	}

	// Encode all file patches into a unified diff string.
	patch := &worktreePatch{filePatches: filePatches}
	var buf bytes.Buffer
	enc := fdiff.NewUnifiedEncoder(&buf, fdiff.DefaultContextLines)
	if encErr := enc.Encode(patch); encErr != nil {
		return "", 0, 0, 0, nil, encErr
	}

	return buf.String(), changedFiles, additions, deletions, untrackedFiles, nil
}

// buildFilePatch creates a FilePatch diffing a tracked file from
// HEAD to the working directory. Returns the patch and line counts.
func buildFilePatch(headTree *object.Tree, repoRoot, path string) (fdiff.FilePatch, int, int, error) {
	fromContent, fromHash, fromMode := readFileFromTree(headTree, path)

	absPath := filepath.Join(repoRoot, path)
	toBytes, err := os.ReadFile(absPath)
	if err != nil {
		// File deleted from worktree.
		if os.IsNotExist(err) {
			return buildDeletedFilePatch(fromContent, fromHash, fromMode, path)
		}
		return nil, 0, 0, err
	}

	toContent := string(toBytes)
	toHash := plumbing.ComputeHash(plumbing.BlobObject, toBytes)
	toMode := fileModeFromOS(absPath)

	// Binary check.
	if isBinaryContent(toBytes) || isBinaryContent([]byte(fromContent)) {
		fp := &worktreeFilePatch{
			isBinary: true,
			from:     &worktreeFile{hash: fromHash, mode: fromMode, path: path},
			to:       &worktreeFile{hash: toHash, mode: toMode, path: path},
		}
		return fp, 0, 0, nil
	}

	chunks, adds, dels := diffToChunks(fromContent, toContent)
	fp := &worktreeFilePatch{
		from:   &worktreeFile{hash: fromHash, mode: fromMode, path: path},
		to:     &worktreeFile{hash: toHash, mode: toMode, path: path},
		chunks: chunks,
	}
	return fp, adds, dels, nil
}

// buildDeletedFilePatch creates a FilePatch for a file that exists
// in HEAD but was deleted from the working directory.
func buildDeletedFilePatch(fromContent string, fromHash plumbing.Hash, fromMode filemode.FileMode, path string) (fdiff.FilePatch, int, int, error) {
	if isBinaryContent([]byte(fromContent)) {
		fp := &worktreeFilePatch{
			isBinary: true,
			from:     &worktreeFile{hash: fromHash, mode: fromMode, path: path},
			to:       nil,
		}
		return fp, 0, 0, nil
	}

	chunks, _, dels := diffToChunks(fromContent, "")
	fp := &worktreeFilePatch{
		from:   &worktreeFile{hash: fromHash, mode: fromMode, path: path},
		to:     nil,
		chunks: chunks,
	}
	return fp, 0, dels, nil
}

// buildNewFilePatch creates a FilePatch for an untracked (new) file.
func buildNewFilePatch(repoRoot, path string) (fdiff.FilePatch, int, int, error) {
	absPath := filepath.Join(repoRoot, path)
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, 0, 0, err
	}

	toHash := plumbing.ComputeHash(plumbing.BlobObject, data)
	toMode := fileModeFromOS(absPath)

	if isBinaryContent(data) {
		fp := &worktreeFilePatch{
			isBinary: true,
			from:     nil,
			to:       &worktreeFile{hash: toHash, mode: toMode, path: path},
		}
		return fp, 0, 0, nil
	}

	content := string(data)
	chunks, adds, _ := diffToChunks("", content)
	fp := &worktreeFilePatch{
		from:   nil,
		to:     &worktreeFile{hash: toHash, mode: toMode, path: path},
		chunks: chunks,
	}
	return fp, adds, 0, nil
}

// readFileFromTree reads a file's content, hash, and mode from a
// git tree. Returns zero values if the tree is nil or the file
// doesn't exist in the tree.
func readFileFromTree(tree *object.Tree, path string) (content string, hash plumbing.Hash, mode filemode.FileMode) {
	if tree == nil {
		return "", plumbing.ZeroHash, filemode.Empty
	}
	f, err := tree.File(path)
	if err != nil {
		return "", plumbing.ZeroHash, filemode.Empty
	}
	c, err := f.Contents()
	if err != nil {
		return "", f.Hash, f.Mode
	}
	return c, f.Hash, f.Mode
}

// isBinaryContent reports whether data looks like a binary file by
// checking for a NUL byte in the first 8000 bytes.
func isBinaryContent(data []byte) bool {
	return bytes.ContainsRune(data[:min(len(data), 8000)], 0)
}

// diffToChunks computes a line-oriented diff and returns fdiff
// chunks plus addition/deletion counts.
func diffToChunks(from, to string) (chunks []fdiff.Chunk, additions, deletions int) {
	diffs := godiff.Do(from, to)
	for _, d := range diffs {
		var op fdiff.Operation
		switch d.Type {
		case dmp.DiffEqual:
			op = fdiff.Equal
		case dmp.DiffDelete:
			op = fdiff.Delete
			deletions += countLines(d.Text)
		case dmp.DiffInsert:
			op = fdiff.Add
			additions += countLines(d.Text)
		}
		chunks = append(chunks, &worktreeChunk{content: d.Text, op: op})
	}
	return chunks, additions, deletions
}

// countLines counts the number of lines in text. A trailing
// newline does not count as an extra line (matching git behavior).
func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	// If the text doesn't end with a newline, there's one more
	// line than the count of newlines.
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

// fileModeFromOS returns the git filemode for a filesystem path.
func fileModeFromOS(path string) filemode.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		return filemode.Regular
	}
	m, err := filemode.NewFromOSFileMode(info.Mode())
	if err != nil {
		return filemode.Regular
	}
	return m
}

// estimateFilePatchSize gives a rough byte count for an encoded
// file patch. Used to enforce maxDiffSize at file boundaries.
func estimateFilePatchSize(fp fdiff.FilePatch) int {
	size := 128 // Header overhead estimate.
	for _, c := range fp.Chunks() {
		size += len(c.Content())
	}
	return size
}

func buildDiffHash(resp codersdk.WorkspaceAgentGitChangesResponse) string {
	var buf bytes.Buffer
	for _, r := range resp.Repos {
		buf.WriteString(r.RepoRoot)
		buf.WriteString(r.UnifiedDiff)
		for _, f := range r.UntrackedFiles {
			buf.WriteString(f)
		}
	}
	return buf.String()
}

// Close shuts down the API and waits for goroutines to finish.
func (api *API) Close() error {
	api.mu.Lock()
	if api.closed {
		api.mu.Unlock()
		return nil
	}
	api.closed = true
	api.cancel()
	api.mu.Unlock()

	if api.updaterDone != nil {
		<-api.updaterDone
	}
	return nil
}

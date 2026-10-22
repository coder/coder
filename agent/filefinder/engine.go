// Package filefinder provides an in-memory file index with trigram
// matching, fuzzy search, and filesystem watching. It is designed
// to power file-finding features on workspace agents.
package filefinder

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

// SearchOptions controls search behavior.
type SearchOptions struct {
	Limit         int
	MaxCandidates int
	// Roots restricts the search to these absolute roots. Empty
	// searches all roots.
	Roots []string
}

// DefaultSearchOptions returns sensible default search options.
func DefaultSearchOptions() SearchOptions {
	return SearchOptions{Limit: 100, MaxCandidates: 10000}
}

type rootSnapshot struct {
	root string
	snap *Snapshot
}

// Engine is the main file finder. Safe for concurrent use.
type Engine struct {
	snap    atomic.Pointer[[]*rootSnapshot]
	logger  slog.Logger
	mu      sync.Mutex
	roots   map[string]*rootState
	eventCh chan rootEvent
	closeCh chan struct{}
	closed  atomic.Bool
	wg      sync.WaitGroup
	ignorer *Ignorer
	// ctx is canceled by Close; each root's context derives from it.
	ctx    context.Context
	cancel context.CancelFunc
	// walks admits at most maxConcurrentWalks root walks at a time,
	// covering both new roots and reloads.
	walks chan struct{}
}
type rootState struct {
	root    string
	index   *Index
	watcher *fsWatcher
	// ctx lives until the root is removed or the engine closes; stop
	// cancels it, which also cancels any reload of the root.
	ctx  context.Context
	stop context.CancelFunc
	// cancelWatcher stops the current watcher.
	cancelWatcher context.CancelFunc
	// gen identifies the current watcher. Events from an earlier
	// watcher are dropped so they cannot modify a newer index.
	gen uint64
	// reloading is set while reloadRoot runs; reloadAgain asks it to
	// run once more because ignore rules changed again meanwhile.
	reloading   bool
	reloadAgain bool
}
type rootEvent struct {
	root   string
	gen    uint64
	events []FSEvent
}

const (
	// maxIndexEntries bounds the entries indexed and watched per root
	// so a single large root cannot exhaust memory or inotify
	// watches. Entries past the budget are not indexed.
	maxIndexEntries = 300_000
	// maxConcurrentWalks bounds concurrent root walks.
	maxConcurrentWalks = 2
)

// walkRoot walks absRoot without watching it and returns an Index of
// the entries that are not ignored. It stops early when ctx is
// canceled or the index reaches maxIndexEntries, reporting the latter
// as truncated.
func walkRoot(ctx context.Context, ig *Ignorer, absRoot string) (idx *Index, truncated bool, err error) {
	idx = NewIndex()
	err = ig.walk(ctx, absRoot, func(path string, d fs.DirEntry) error {
		if idx.Len() >= maxIndexEntries {
			truncated = true
			return filepath.SkipAll
		}
		var flags uint16
		if d.IsDir() {
			flags = uint16(FlagDir)
		} else if d.Type()&fs.ModeSymlink != 0 {
			flags = uint16(FlagSymlink)
		}
		addToIndex(idx, absRoot, path, flags)
		return nil
	}, nil)
	return idx, truncated, err
}

func addToIndex(idx *Index, root, path string, flags uint16) {
	if rel, err := filepath.Rel(root, path); err == nil {
		idx.Add(filepath.ToSlash(rel), flags)
	}
}

// EngineOption configures an Engine.
type EngineOption func(*Engine)

// WithIgnorer sets the gitignore rules the engine applies. The default
// uses git from PATH, falling back to go-git with the current user's
// global excludes.
func WithIgnorer(ig *Ignorer) EngineOption {
	return func(e *Engine) {
		e.ignorer = ig
	}
}

// NewEngine creates a new Engine.
func NewEngine(logger slog.Logger, opts ...EngineOption) *Engine {
	e := &Engine{
		logger:  logger,
		roots:   make(map[string]*rootState),
		eventCh: make(chan rootEvent, 256),
		closeCh: make(chan struct{}),
		walks:   make(chan struct{}, maxConcurrentWalks),
	}
	for _, opt := range opts {
		opt(e)
	}
	if e.ignorer == nil {
		home, _ := os.UserHomeDir()
		e.ignorer = NewIgnorer(afero.NewOsFs(), home)
	}
	e.ctx, e.cancel = context.WithCancel(context.Background())
	empty := make([]*rootSnapshot, 0)
	e.snap.Store(&empty)
	e.wg.Add(1)
	go e.start()
	return e
}

// ErrClosed is returned when operations are attempted on a
// closed engine.
var ErrClosed = xerrors.New("engine is closed")

func (e *Engine) acquireWalk(ctx context.Context) error {
	select {
	case e.walks <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-e.closeCh:
		return ErrClosed
	}
}

func (e *Engine) releaseWalk() { <-e.walks }

// startWatcher starts a watcher for root and indexes what its initial
// walk found. Watches are added before each directory is read, so
// changes during the walk are delivered as events after it; the caller
// applies them to the returned index by forwarding the watcher's
// events.
func (e *Engine) startWatcher(ctx context.Context, root string) (*fsWatcher, *Index, error) {
	w, err := newFSWatcher(root, e.logger, e.ignorer)
	if err != nil {
		return nil, nil, xerrors.Errorf("create watcher: %w", err)
	}
	initial := w.Start(ctx)
	if err := ctx.Err(); err != nil {
		_ = w.Close()
		return nil, nil, err
	}
	if len(initial) >= maxIndexEntries {
		e.logger.Warn(ctx, "root exceeds the index budget, indexing partially",
			slog.F("root", root),
			slog.F("max_entries", maxIndexEntries),
		)
	}
	idx := NewIndex()
	for _, ev := range initial {
		var flags uint16
		if ev.IsDir {
			flags = uint16(FlagDir)
		}
		addToIndex(idx, root, ev.Path, flags)
	}
	return w, idx, nil
}

// AddRoot adds a directory root to the engine. It waits for a walk
// slot, then indexes and watches the root.
func (e *Engine) AddRoot(ctx context.Context, root string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return xerrors.Errorf("resolve root: %w", err)
	}
	e.mu.Lock()
	if e.closed.Load() {
		e.mu.Unlock()
		return ErrClosed
	}
	if _, exists := e.roots[absRoot]; exists {
		e.mu.Unlock()
		return nil
	}
	e.mu.Unlock()

	if err := e.acquireWalk(ctx); err != nil {
		return err
	}
	rootCtx, rootStop := context.WithCancel(e.ctx)
	wCtx, wCancel := context.WithCancel(rootCtx)
	// Canceling ctx stops the walk; once added, the root lives until
	// RemoveRoot or Close.
	stop := context.AfterFunc(ctx, rootStop)
	w, idx, err := e.startWatcher(wCtx, absRoot)
	canceled := !stop()
	e.releaseWalk()
	cleanup := func() {
		wCancel()
		rootStop()
		if w != nil {
			_ = w.Close()
		}
	}
	if canceled {
		cleanup()
		return ctx.Err()
	}
	if err != nil {
		cleanup()
		return xerrors.Errorf("walk root: %w", err)
	}

	e.mu.Lock()
	// Re-check after re-acquiring the lock: another goroutine may
	// have added this root or closed the engine while we walked.
	if e.closed.Load() {
		e.mu.Unlock()
		cleanup()
		return ErrClosed
	}
	if _, exists := e.roots[absRoot]; exists {
		e.mu.Unlock()
		cleanup()
		return nil
	}
	rs := &rootState{
		root: absRoot, index: idx, watcher: w,
		ctx: rootCtx, stop: rootStop, cancelWatcher: wCancel, gen: 1,
	}
	e.roots[absRoot] = rs
	e.wg.Add(1)
	go e.forwardEvents(wCtx, absRoot, rs.gen, w)
	e.publishSnapshot()
	fileCount := idx.Len()
	e.mu.Unlock()
	e.logger.Info(ctx, "added root to engine",
		slog.F("root", absRoot),
		slog.F("files", fileCount),
	)
	return nil
}

// RemoveRoot stops watching a root and removes it, canceling any
// reload of it.
func (e *Engine) RemoveRoot(root string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return xerrors.Errorf("resolve root: %w", err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	rs, exists := e.roots[absRoot]
	if !exists {
		return xerrors.Errorf("root %q not found", absRoot)
	}
	rs.stop()
	_ = rs.watcher.Close()
	delete(e.roots, absRoot)
	e.publishSnapshot()
	return nil
}

// Search performs a fuzzy file search across all roots.
func (e *Engine) Search(_ context.Context, query string, opts SearchOptions) ([]Result, error) {
	if e.closed.Load() {
		return nil, ErrClosed
	}
	snapPtr := e.snap.Load()
	if snapPtr == nil || len(*snapPtr) == 0 {
		return nil, nil
	}
	roots := *snapPtr
	plan := newQueryPlan(query)
	if len(plan.Normalized) == 0 {
		return nil, nil
	}
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	if opts.MaxCandidates <= 0 {
		opts.MaxCandidates = 10000
	}
	params := defaultScoreParams()
	var allCands []candidate
	for _, rs := range roots {
		if len(opts.Roots) > 0 && !slices.Contains(opts.Roots, rs.root) {
			continue
		}
		cands := searchSnapshot(plan, rs.snap, opts.MaxCandidates)
		for i := range cands {
			cands[i].Root = rs.root
		}
		allCands = append(allCands, cands...)
	}
	results := mergeAndScore(allCands, plan, params, opts.Limit)
	return results, nil
}

// Close shuts down the engine.
func (e *Engine) Close() error {
	if e.closed.Swap(true) {
		return nil
	}
	close(e.closeCh)
	e.cancel()
	e.mu.Lock()
	for _, rs := range e.roots {
		rs.stop()
		_ = rs.watcher.Close()
	}
	e.roots = make(map[string]*rootState)
	e.mu.Unlock()
	e.wg.Wait()
	return nil
}

// Rebuild forces a complete re-walk and re-index of a root. Unlike a
// reload after an ignore change, it keeps the current watcher.
func (e *Engine) Rebuild(ctx context.Context, root string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return xerrors.Errorf("resolve root: %w", err)
	}
	if err := e.acquireWalk(ctx); err != nil {
		return err
	}
	idx, _, walkErr := walkRoot(ctx, e.ignorer, absRoot)
	e.releaseWalk()
	if walkErr != nil {
		return xerrors.Errorf("rebuild walk: %w", walkErr)
	}

	e.mu.Lock()
	rs, exists := e.roots[absRoot]
	if !exists {
		e.mu.Unlock()
		return xerrors.Errorf("root %q not found", absRoot)
	}
	rs.index = idx
	e.publishSnapshot()
	fileCount := idx.Len()
	e.mu.Unlock()
	e.logger.Info(ctx, "rebuilt root in engine",
		slog.F("root", absRoot),
		slog.F("files", fileCount),
	)
	return nil
}

func (e *Engine) start() {
	defer e.wg.Done()
	for {
		select {
		case <-e.closeCh:
			return
		case re, ok := <-e.eventCh:
			if !ok {
				return
			}
			e.applyEvents(re)
		}
	}
}

func (e *Engine) forwardEvents(ctx context.Context, root string, gen uint64, w *fsWatcher) {
	defer e.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.closeCh:
			return
		case evts, ok := <-w.Events():
			if !ok {
				return
			}
			select {
			case e.eventCh <- rootEvent{root: root, gen: gen, events: evts}:
			case <-ctx.Done():
				return
			case <-e.closeCh:
				return
			}
		}
	}
}

func (e *Engine) applyEvents(re rootEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	rs, exists := e.roots[re.root]
	if !exists || rs.gen != re.gen {
		return
	}
	changed := false
	for _, ev := range re.events {
		if ev.Op == OpIgnoreChanged {
			e.scheduleReloadLocked(rs)
			continue
		}
		relPath, err := filepath.Rel(rs.root, ev.Path)
		if err != nil {
			continue
		}
		relPath = filepath.ToSlash(relPath)
		switch ev.Op {
		case OpCreate:
			if rs.index.Has(relPath) {
				continue
			}
			var flags uint16
			if ev.IsDir {
				flags = uint16(FlagDir)
			}
			rs.index.Add(relPath, flags)
			changed = true
		case OpRemove, OpRename:
			if rs.index.Remove(relPath) {
				changed = true
			}
			if ev.IsDir || ev.Op == OpRename {
				prefix := strings.ToLower(filepath.ToSlash(relPath)) + "/"
				for path := range rs.index.byPath {
					if strings.HasPrefix(path, prefix) {
						rs.index.Remove(path)
						changed = true
					}
				}
			}
		case OpModify:
		}
	}
	if changed {
		e.publishSnapshot()
	}
}

// publishSnapshot builds and atomically publishes a new snapshot.
// Must be called with e.mu held.
func (e *Engine) publishSnapshot() {
	roots := make([]*rootSnapshot, 0, len(e.roots))
	for _, rs := range e.roots {
		roots = append(roots, &rootSnapshot{
			root: rs.root,
			snap: rs.index.Snapshot(),
		})
	}
	slices.SortFunc(roots, func(a, b *rootSnapshot) int {
		return strings.Compare(a.root, b.root)
	})
	e.snap.Store(&roots)
}

// scheduleReloadLocked starts reloadRoot for rs unless one is running,
// in which case that one runs again. Must be called with e.mu held.
func (e *Engine) scheduleReloadLocked(rs *rootState) {
	if rs.reloading {
		rs.reloadAgain = true
		return
	}
	rs.reloading = true
	e.wg.Add(1)
	go e.reloadRoot(rs)
}

// reloadRoot replaces rs's index and watcher after its ignore rules
// changed, so newly ignored paths are dropped and newly unignored
// directories are indexed and watched. The new watcher starts before
// its walk, so changes during the walk reach the new index as events
// once it is swapped in; events from the old watcher are dropped by
// generation. The old index serves searches until then. The walk
// takes a walk slot and stops when the root is removed.
func (e *Engine) reloadRoot(rs *rootState) {
	defer e.wg.Done()
	for {
		var (
			w   *fsWatcher
			idx *Index
		)
		wCtx, wCancel := context.WithCancel(rs.ctx)
		err := e.acquireWalk(rs.ctx)
		if err == nil {
			w, idx, err = e.startWatcher(wCtx, rs.root)
			e.releaseWalk()
		}

		e.mu.Lock()
		if err != nil || rs.ctx.Err() != nil || e.roots[rs.root] != rs {
			rs.reloading = false
			e.mu.Unlock()
			wCancel()
			if w != nil {
				_ = w.Close()
			}
			if err != nil && rs.ctx.Err() == nil {
				e.logger.Warn(rs.ctx, "reload root after ignore rules changed", slog.F("root", rs.root), slog.Error(err))
			}
			return
		}
		oldCancel, oldWatcher := rs.cancelWatcher, rs.watcher
		rs.gen++
		rs.index, rs.watcher, rs.cancelWatcher = idx, w, wCancel
		e.wg.Add(1)
		go e.forwardEvents(wCtx, rs.root, rs.gen, w)
		e.publishSnapshot()
		again := rs.reloadAgain
		rs.reloadAgain = false
		if !again {
			rs.reloading = false
		}
		e.mu.Unlock()

		oldCancel()
		_ = oldWatcher.Close()
		if !again {
			return
		}
	}
}

package filefinder

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

// SearchOptions controls search behavior.
type SearchOptions struct {
	Limit         int
	MaxCandidates int
}

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
}
type rootState struct {
	root    string
	index   *Index
	watcher *fsWatcher
	cancel  context.CancelFunc
}
type rootEvent struct {
	root   string
	events []FSEvent
}

func walkRoot(absRoot string) (*Index, error) {
	idx := NewIndex()
	err := filepath.Walk(absRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil //nolint:nilerr
		}
		base := filepath.Base(path)
		if _, skip := skipDirs[base]; skip && info.IsDir() {
			return filepath.SkipDir
		}
		if path == absRoot {
			return nil
		}
		relPath, relErr := filepath.Rel(absRoot, path)
		if relErr != nil {
			return nil //nolint:nilerr
		}
		relPath = filepath.ToSlash(relPath)
		var flags uint16
		if info.IsDir() {
			flags = uint16(FlagDir)
		} else if info.Mode()&os.ModeSymlink != 0 {
			flags = uint16(FlagSymlink)
		}
		idx.Add(relPath, flags)
		return nil
	})
	return idx, err
}

// NewEngine creates a new Engine.
func NewEngine(logger slog.Logger) *Engine {
	e := &Engine{
		logger:  logger,
		roots:   make(map[string]*rootState),
		eventCh: make(chan rootEvent, 256),
		closeCh: make(chan struct{}),
	}
	empty := make([]*rootSnapshot, 0)
	e.snap.Store(&empty)
	e.wg.Add(1)
	go e.start()
	return e
}

// AddRoot adds a directory root to the engine.
func (e *Engine) AddRoot(ctx context.Context, root string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return xerrors.Errorf("resolve root: %w", err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed.Load() {
		return xerrors.New("engine is closed")
	}
	if _, exists := e.roots[absRoot]; exists {
		return nil
	}
	idx, walkErr := walkRoot(absRoot)
	if walkErr != nil {
		return xerrors.Errorf("walk root: %w", walkErr)
	}
	wCtx, wCancel := context.WithCancel(ctx)
	w, wErr := newFSWatcher(absRoot, e.logger)
	if wErr != nil {
		wCancel()
		return xerrors.Errorf("create watcher: %w", wErr)
	}
	rs := &rootState{root: absRoot, index: idx, watcher: w, cancel: wCancel}
	e.roots[absRoot] = rs
	w.Start(wCtx)
	e.wg.Add(1)
	go e.forwardEvents(wCtx, absRoot, w)
	e.publishSnapshot()
	e.logger.Info(ctx, "added root to engine",
		slog.F("root", absRoot),
		slog.F("files", idx.Len()),
	)
	return nil
}

// RemoveRoot stops watching a root and removes it.
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
	rs.cancel()
	_ = rs.watcher.Close()
	delete(e.roots, absRoot)
	e.publishSnapshot()
	return nil
}

// Search performs a fuzzy file search across all roots.
func (e *Engine) Search(_ context.Context, query string, opts SearchOptions) ([]Result, error) {
	if e.closed.Load() {
		return nil, xerrors.New("engine is closed")
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
	params := DefaultScoreParams()
	var allCands []candidate
	for _, rs := range roots {
		allCands = append(allCands, searchSnapshot(plan, rs.snap, opts.MaxCandidates)...)
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
	e.mu.Lock()
	for _, rs := range e.roots {
		rs.cancel()
		_ = rs.watcher.Close()
	}
	e.roots = make(map[string]*rootState)
	e.mu.Unlock()
	e.wg.Wait()
	return nil
}

// Rebuild forces a complete re-walk and re-index of a root.
func (e *Engine) Rebuild(ctx context.Context, root string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return xerrors.Errorf("resolve root: %w", err)
	}
	e.mu.Lock()
	rs, exists := e.roots[absRoot]
	if !exists {
		e.mu.Unlock()
		return xerrors.Errorf("root %q not found", absRoot)
	}
	idx, walkErr := walkRoot(absRoot)
	if walkErr != nil {
		e.mu.Unlock()
		return xerrors.Errorf("rebuild walk: %w", walkErr)
	}
	rs.index = idx
	e.publishSnapshot()
	e.mu.Unlock()
	e.logger.Info(ctx, "rebuilt root in engine",
		slog.F("root", absRoot),
		slog.F("files", idx.Len()),
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

func (e *Engine) forwardEvents(ctx context.Context, root string, w *fsWatcher) {
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
			case e.eventCh <- rootEvent{root: root, events: evts}:
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
	if !exists {
		return
	}
	changed := false
	for _, ev := range re.events {
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

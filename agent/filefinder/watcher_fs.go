package filefinder

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"cdr.dev/slog/v3"
)

// FSEvent represents a filesystem change event.
type FSEvent struct {
	Op    FSEventOp
	Path  string
	IsDir bool
}

// FSEventOp represents the type of filesystem operation.
type FSEventOp uint8

// Filesystem operations reported by the watcher.
const (
	OpCreate FSEventOp = iota
	OpRemove
	OpRename
	OpModify
	// OpIgnoreChanged reports that a .gitignore or exclude file under
	// the root changed. It has no path; the engine re-indexes the root.
	OpIgnoreChanged
)

var skipDirs = map[string]struct{}{
	".git": {}, "node_modules": {}, ".hg": {}, ".svn": {},
	"__pycache__": {}, ".cache": {}, ".venv": {}, "vendor": {}, ".terraform": {},
}

type fsWatcher struct {
	w      *fsnotify.Watcher
	root   string
	ig     *Ignorer
	events chan []FSEvent
	logger slog.Logger
	mu     sync.Mutex
	closed bool
	// started is set once Start runs; done is only closed after that.
	started bool
	done    chan struct{}

	// ruleFiles are ignore rule sources outside the watched tree: the
	// .gitignore files inherited from ancestors of root and each
	// repository's exclude file. ruleDirs are the directories watched
	// only to see them; their other events are dropped. Both are only
	// used by Start and the loop goroutine, one after the other.
	ruleFiles map[string]struct{}
	ruleDirs  map[string]struct{}
}

func newFSWatcher(root string, logger slog.Logger, ig *Ignorer) (*fsWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &fsWatcher{
		w:         w,
		root:      root,
		ig:        ig,
		events:    make(chan []FSEvent, 64),
		logger:    logger,
		done:      make(chan struct{}),
		ruleFiles: make(map[string]struct{}),
		ruleDirs:  make(map[string]struct{}),
	}, nil
}

// Start watches the root and its ignore rule sources, starts the
// event loop, and returns create events for the entries found. Each
// directory is watched before it is read, so later changes arrive as
// events. It returns nil when ctx is canceled or after Close.
func (fw *fsWatcher) Start(ctx context.Context) []FSEvent {
	fw.mu.Lock()
	if fw.closed {
		fw.mu.Unlock()
		return nil
	}
	fw.started = true
	fw.mu.Unlock()

	if top := fw.ig.repoTop(fw.root); top != "" && top != fw.root {
		for d := filepath.Dir(fw.root); ; d = filepath.Dir(d) {
			fw.watchRuleFile(filepath.Join(d, gitignoreFile))
			if d == top || filepath.Dir(d) == d {
				break
			}
		}
	}
	initEvents := fw.addRecursive(ctx, fw.root)
	if ctx.Err() != nil {
		close(fw.done)
		return nil
	}
	fw.logger.Debug(ctx, "fs watcher started", slog.F("root", fw.root))
	go fw.loop(ctx)
	return initEvents
}

// watchRuleFile watches the directory of an ignore rule source
// outside the tree so changes to it trigger a reload.
func (fw *fsWatcher) watchRuleFile(path string) {
	fw.ruleFiles[path] = struct{}{}
	dir := filepath.Dir(path)
	if _, ok := fw.ruleDirs[dir]; ok {
		return
	}
	fw.ruleDirs[dir] = struct{}{}
	if err := fw.w.Add(dir); err != nil {
		fw.logger.Debug(context.Background(), "failed to watch ignore rule directory",
			slog.F("path", dir), slog.Error(err))
	}
}
func (fw *fsWatcher) Events() <-chan []FSEvent { return fw.events }
func (fw *fsWatcher) Close() error {
	fw.mu.Lock()
	if fw.closed {
		fw.mu.Unlock()
		return nil
	}
	fw.closed = true
	started := fw.started
	fw.mu.Unlock()
	err := fw.w.Close()
	if started {
		<-fw.done
	}
	return err
}

func (fw *fsWatcher) loop(ctx context.Context) {
	defer close(fw.done)
	const batchWindow = 50 * time.Millisecond
	var (
		batch  eventBatch
		timer  *time.Timer
		timerC <-chan time.Time
	)
	flush := func() {
		events := fw.filterIgnored(ctx, batch.take())
		if len(events) == 0 {
			return
		}
		select {
		case fw.events <- events:
		default:
			fw.logger.Warn(ctx, "fs watcher dropping batch", slog.F("count", len(events)))
		}
		if timer != nil {
			timer.Stop()
		}
		timer = nil
		timerC = nil
	}
	addToBatch := func(ev FSEvent) {
		batch.add(ev)
		if timer == nil {
			timer = time.NewTimer(batchWindow)
			timerC = timer.C
		}
	}
	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case ev, ok := <-fw.w.Events:
			if !ok {
				flush()
				return
			}
			fsev := translateEvent(ev)
			if fsev == nil {
				continue
			}
			if _, ok := fw.ruleFiles[fsev.Path]; ok ||
				filepath.Base(fsev.Path) == gitignoreFile && isBelow(fw.root, fsev.Path) {
				addToBatch(FSEvent{Op: OpIgnoreChanged})
			}
			if _, ok := fw.ruleDirs[filepath.Dir(fsev.Path)]; ok {
				continue
			}
			addToBatch(*fsev)
		case err, ok := <-fw.w.Errors:
			if !ok {
				flush()
				return
			}
			fw.logger.Warn(ctx, "fsnotify watcher error", slog.Error(err))
		case <-timerC:
			flush()
		}
	}
}

// eventBatch coalesces events to at most one per path while keeping
// the path's final state. A create, remove, or rename replaces any
// earlier event for the path and moves it to the end, so the batch
// applies in the order the last changes happened; for example, a temp
// file created and then renamed away ends as a rename and is removed
// from the index. A modify never replaces an earlier event because it
// does not change whether the path exists.
type eventBatch struct {
	events []FSEvent
	// index maps a path to its live entry in events.
	index map[string]int
	// stale marks entries superseded by a later event for the same
	// path.
	stale map[int]struct{}
}

func (b *eventBatch) add(ev FSEvent) {
	if b.index == nil {
		b.index = make(map[string]int)
		b.stale = make(map[int]struct{})
	}
	if i, ok := b.index[ev.Path]; ok {
		if ev.Op == OpModify {
			return
		}
		b.stale[i] = struct{}{}
	}
	b.index[ev.Path] = len(b.events)
	b.events = append(b.events, ev)
}

// take returns the coalesced events in order and resets the batch.
func (b *eventBatch) take() []FSEvent {
	events := make([]FSEvent, 0, len(b.events)-len(b.stale))
	for i, ev := range b.events {
		if _, ok := b.stale[i]; !ok {
			events = append(events, ev)
		}
	}
	*b = eventBatch{}
	return events
}

// filterIgnored drops created and modified paths that git ignores,
// with one classification per batch, and expands each remaining
// created directory into its contents.
func (fw *fsWatcher) filterIgnored(ctx context.Context, events []FSEvent) []FSEvent {
	var queries []IgnoreQuery
	for _, ev := range events {
		if ev.Op == OpCreate || ev.Op == OpModify {
			queries = append(queries, IgnoreQuery{Path: ev.Path, IsDir: ev.IsDir})
		}
	}
	if len(queries) == 0 {
		return events
	}
	ignored := fw.ig.Ignored(ctx, queries)
	out := make([]FSEvent, 0, len(events))
	q := 0
	for _, ev := range events {
		if ev.Op == OpCreate || ev.Op == OpModify {
			skip := ignored[q]
			q++
			if skip {
				continue
			}
		}
		out = append(out, ev)
		if ev.IsDir && ev.Op == OpCreate {
			out = append(out, fw.addRecursive(ctx, ev.Path)...)
		}
	}
	return out
}

// addRecursive watches dir and its subdirectories, skipping skipDirs
// and gitignored directories, and returns create events for the
// entries below dir. It also watches the exclude file of each
// repository whose rules apply.
func (fw *fsWatcher) addRecursive(ctx context.Context, dir string) []FSEvent {
	if _, skip := skipDirs[filepath.Base(dir)]; skip {
		return nil
	}
	var events []FSEvent
	fw.watchDir(dir)
	walkErr := fw.ig.walk(ctx, dir, func(path string, d fs.DirEntry) error {
		if len(events) >= maxIndexEntries {
			return filepath.SkipAll
		}
		if d.IsDir() {
			fw.watchDir(path)
		}
		events = append(events, FSEvent{Op: OpCreate, Path: path, IsDir: d.IsDir()})
		return nil
	}, func(top string) {
		fw.watchRuleFile(fw.ig.excludeFile(ctx, top))
	})
	if walkErr != nil && ctx.Err() == nil {
		fw.logger.Warn(context.Background(), "failed to walk directory",
			slog.F("dir", dir), slog.Error(walkErr))
	}
	return events
}

func (fw *fsWatcher) watchDir(dir string) {
	if err := fw.w.Add(dir); err != nil {
		fw.logger.Debug(context.Background(), "failed to add watch",
			slog.F("path", dir), slog.Error(err))
	}
}

func translateEvent(ev fsnotify.Event) *FSEvent {
	var op FSEventOp
	switch {
	case ev.Op&fsnotify.Create != 0:
		op = OpCreate
	case ev.Op&fsnotify.Remove != 0:
		op = OpRemove
	case ev.Op&fsnotify.Rename != 0:
		op = OpRename
	case ev.Op&fsnotify.Write != 0:
		op = OpModify
	default:
		return nil
	}
	isDir := false
	if op == OpCreate || op == OpModify {
		fi, err := os.Lstat(ev.Name)
		if err == nil {
			isDir = fi.IsDir()
		}
	}
	if isDir {
		if _, skip := skipDirs[filepath.Base(ev.Name)]; skip {
			return nil
		}
	}
	return &FSEvent{Op: op, Path: ev.Name, IsDir: isDir}
}

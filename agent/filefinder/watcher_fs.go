package filefinder

import (
	"context"
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
}

func newFSWatcher(root string, logger slog.Logger, ig *Ignorer) (*fsWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &fsWatcher{
		w:      w,
		root:   root,
		ig:     ig,
		events: make(chan []FSEvent, 64),
		logger: logger,
		done:   make(chan struct{}),
	}, nil
}

// Start adds watches under the root and starts the event loop. It
// stops early when ctx is canceled. It does nothing after Close.
func (fw *fsWatcher) Start(ctx context.Context) {
	fw.mu.Lock()
	if fw.closed {
		fw.mu.Unlock()
		return
	}
	fw.started = true
	fw.mu.Unlock()

	initEvents := fw.addRecursive(ctx, fw.root)
	if len(initEvents) > 0 {
		select {
		case fw.events <- initEvents:
		case <-ctx.Done():
			close(fw.done)
			return
		}
	}
	fw.logger.Debug(ctx, "fs watcher started", slog.F("root", fw.root))
	go fw.loop(ctx)
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
		events := batch.take()
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
			if isIgnoreFile(fsev.Path) {
				addToBatch(FSEvent{Op: OpIgnoreChanged})
			}
			// .git/info is watched only to see exclude changes.
			if isGitInfoPath(fsev.Path) {
				continue
			}
			if (fsev.Op == OpCreate || fsev.Op == OpModify) &&
				fw.ig.rulesBelow(filepath.Dir(fsev.Path), fw.root).Ignored(fsev.Path, fsev.IsDir) {
				continue
			}
			if fsev.IsDir && fsev.Op == OpCreate {
				for _, s := range fw.addRecursive(ctx, fsev.Path) {
					addToBatch(s)
				}
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

// addRecursive watches dir and its subdirectories, skipping skipDirs
// and gitignored directories, and returns create events for the
// entries below dir. It also watches each repository's .git/info so
// exclude changes are seen.
func (fw *fsWatcher) addRecursive(ctx context.Context, dir string) []FSEvent {
	if _, skip := skipDirs[filepath.Base(dir)]; skip {
		return nil
	}
	var events []FSEvent
	fw.watchDir(dir)
	walkErr := walkIndexable(ctx, fw.ig, dir, func(path string, info os.FileInfo) error {
		if len(events) >= maxIndexEntries {
			return filepath.SkipAll
		}
		if info.IsDir() {
			fw.watchDir(path)
		}
		events = append(events, FSEvent{Op: OpCreate, Path: path, IsDir: info.IsDir()})
		return nil
	})
	if walkErr != nil && ctx.Err() == nil {
		fw.logger.Warn(context.Background(), "failed to walk directory",
			slog.F("dir", dir), slog.Error(walkErr))
	}
	return events
}

func (fw *fsWatcher) watchDir(dir string) {
	if addErr := fw.w.Add(dir); addErr != nil {
		fw.logger.Debug(context.Background(), "failed to add watch",
			slog.F("path", dir), slog.Error(addErr))
	}
	info := filepath.Join(dir, ".git", "info")
	if fi, err := os.Stat(info); err == nil && fi.IsDir() {
		_ = fw.w.Add(info)
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

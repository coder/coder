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
)

var skipDirs = map[string]struct{}{
	".git": {}, "node_modules": {}, ".hg": {}, ".svn": {},
	"__pycache__": {}, ".cache": {}, ".venv": {}, "vendor": {}, ".terraform": {},
}

type fsWatcher struct {
	w      *fsnotify.Watcher
	root   string
	events chan []FSEvent
	logger slog.Logger
	mu     sync.Mutex
	closed bool
	done   chan struct{}
}

func newFSWatcher(root string, logger slog.Logger) (*fsWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &fsWatcher{
		w:      w,
		root:   root,
		events: make(chan []FSEvent, 64),
		logger: logger,
		done:   make(chan struct{}),
	}, nil
}

func (fw *fsWatcher) Start(ctx context.Context) {
	initEvents := fw.addRecursive(fw.root)
	if len(initEvents) > 0 {
		select {
		case fw.events <- initEvents:
		case <-ctx.Done():
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
	fw.mu.Unlock()
	err := fw.w.Close()
	<-fw.done
	return err
}

func (fw *fsWatcher) loop(ctx context.Context) {
	defer close(fw.done)
	const batchWindow = 50 * time.Millisecond
	var (
		batch  []FSEvent
		seen   = make(map[string]struct{})
		timer  *time.Timer
		timerC <-chan time.Time
	)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		select {
		case fw.events <- batch:
		default:
			fw.logger.Warn(ctx, "fs watcher dropping batch", slog.F("count", len(batch)))
		}
		batch = nil
		seen = make(map[string]struct{})
		if timer != nil {
			timer.Stop()
		}
		timer = nil
		timerC = nil
	}
	addToBatch := func(ev FSEvent) {
		if _, dup := seen[ev.Path]; dup {
			return
		}
		seen[ev.Path] = struct{}{}
		batch = append(batch, ev)
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
			if fsev.IsDir && fsev.Op == OpCreate {
				for _, s := range fw.addRecursive(fsev.Path) {
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

func (fw *fsWatcher) addRecursive(dir string) []FSEvent {
	var events []FSEvent
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // best-effort
		}
		base := filepath.Base(path)
		if _, skip := skipDirs[base]; skip && info.IsDir() {
			return filepath.SkipDir
		}
		if info.IsDir() {
			if addErr := fw.w.Add(path); addErr != nil {
				fw.logger.Debug(context.Background(), "failed to add watch",
					slog.F("path", path), slog.Error(addErr))
			}
			if path != dir {
				events = append(events, FSEvent{Op: OpCreate, Path: path, IsDir: true})
			}
			return nil
		}
		events = append(events, FSEvent{Op: OpCreate, Path: path, IsDir: false})
		return nil
	})
	return events
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

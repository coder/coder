package filefinder

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"cdr.dev/slog/v3"
)

// FSEvent represents a filesystem change.
type FSEvent struct {
	Op    FSEventOp
	Path  string
	IsDir bool
}

// FSEventOp classifies the type of filesystem change.
type FSEventOp uint8

const (
	OpCreate FSEventOp = iota
	OpRemove
	OpRename
	OpModify
)

// skipDirs is the set of directory basenames that should never be
// watched or indexed. Keeping this in sync with the builder's skip
// list avoids wasting inotify watches on directories that will
// never produce useful results.
var skipDirs = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	".hg":          {},
	".svn":         {},
	"__pycache__":  {},
	".cache":       {},
	".venv":        {},
	"vendor":       {},
	".terraform":   {},
}

// fsWatcher wraps fsnotify with recursive directory watching and
// event coalescing.
type fsWatcher struct {
	w      *fsnotify.Watcher
	root   string
	events chan []FSEvent
	logger slog.Logger
	mu     sync.Mutex
	closed bool
	done   chan struct{}
}

// newFSWatcher creates a watcher rooted at dir. It recursively
// adds watches for all subdirectories (skipping skipDirs) and
// returns synthetic Create events for every file found during the
// initial walk.
func newFSWatcher(root string, logger slog.Logger) (*fsWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	fw := &fsWatcher{
		w:      w,
		root:   root,
		events: make(chan []FSEvent, 64),
		logger: logger,
		done:   make(chan struct{}),
	}

	return fw, nil
}

// Start launches the background goroutine that reads fsnotify
// events, coalesces them in 50 ms batches, and forwards them to
// the Events channel. It also performs the initial recursive walk
// and emits synthetic Create events for all discovered files.
func (fw *fsWatcher) Start(ctx context.Context) {
	// Perform the initial recursive walk before entering the
	// event loop so callers can rely on the first batch of
	// events containing the full directory tree.
	initEvents := fw.addRecursive(fw.root)
	if len(initEvents) > 0 {
		select {
		case fw.events <- initEvents:
		case <-ctx.Done():
			return
		}
	}

	fw.logger.Debug(ctx, "fs watcher started",
		slog.F("root", fw.root),
	)

	go fw.loop(ctx)
}

// Events returns the channel that receives batched FS events.
func (fw *fsWatcher) Events() <-chan []FSEvent {
	return fw.events
}

// Close shuts down the watcher and waits for the loop goroutine
// to exit.
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

// loop is the main event loop. It batches fsnotify events for
// 50 ms before flushing them to the events channel.
func (fw *fsWatcher) loop(ctx context.Context) {
	defer close(fw.done)

	const batchWindow = 50 * time.Millisecond

	var (
		batch []FSEvent
		seen  = make(map[string]struct{})
		timer *time.Timer
		timerC <-chan time.Time
	)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		// Non-blocking send; if the consumer is slow we drop
		// the oldest batch. In practice the engine drains
		// quickly enough.
		select {
		case fw.events <- batch:
		default:
			fw.logger.Warn(ctx, "fs watcher dropping batch",
				slog.F("count", len(batch)),
			)
		}
		batch = nil
		seen = make(map[string]struct{})
		if timer != nil {
			timer.Stop()
		}
		timerC = nil
	}

	addToBatch := func(ev FSEvent) {
		key := ev.Path
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		batch = append(batch, ev)

		// Start or reset the batch timer.
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

			// On directory creation, recursively add watches
			// and emit synthetic creates for races.
			if fsev.IsDir && fsev.Op == OpCreate {
				synth := fw.addRecursive(fsev.Path)
				for _, s := range synth {
					addToBatch(s)
				}
			}
			addToBatch(*fsev)

		case err, ok := <-fw.w.Errors:
			if !ok {
				flush()
				return
			}
			fw.logger.Warn(ctx, "fsnotify error",
				slog.Error(err),
			)

		case <-timerC:
			flush()
		}
	}
}

// addRecursive walks dir and adds watches for all
// subdirectories. It returns synthetic Create events for all
// files found during the walk (to handle the inotify race where
// files appear between watch registration and the first event).
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
				fw.logger.Debug(context.Background(),
					"failed to add watch",
					slog.F("path", path),
					slog.Error(addErr),
				)
			}
			// Emit a Create event for the directory itself
			// (unless it's the root).
			if path != dir {
				events = append(events, FSEvent{
					Op:    OpCreate,
					Path:  path,
					IsDir: true,
				})
			}
			return nil
		}

		// Regular file or symlink — emit Create.
		events = append(events, FSEvent{
			Op:    OpCreate,
			Path:  path,
			IsDir: false,
		})
		return nil
	})

	return events
}

// translateEvent converts an fsnotify.Event to an FSEvent. It
// returns nil for events we don't care about (e.g. Chmod).
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

	// Check if the path is a directory. For Remove/Rename the
	// path may already be gone so we can't stat; assume file.
	isDir := false
	if op == OpCreate || op == OpModify {
		fi, err := os.Lstat(ev.Name)
		if err == nil {
			isDir = fi.IsDir()
		}
	}

	// Skip hidden files only when they start with '.' AND are
	// in our skipDirs list (for directories). Individual hidden
	// files like .gitignore are fine.
	base := filepath.Base(ev.Name)
	if isDir {
		if _, skip := skipDirs[base]; skip {
			return nil
		}
	}
	// Skip entries with path components matching skipDirs.
	for _, part := range strings.Split(ev.Name, string(filepath.Separator)) {
		if _, skip := skipDirs[part]; skip {
			return nil
		}
	}

	return &FSEvent{
		Op:    op,
		Path:  ev.Name,
		IsDir: isDir,
	}
}

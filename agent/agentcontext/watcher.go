package agentcontext

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"
)

// DefaultWatchDebounce coalesces editor-style multi-event writes
// (truncate plus rename plus chmod) into a single re-resolve.
// Mirrors the debounce window the existing MCP config watcher
// uses so behavior is consistent across the agent.
const DefaultWatchDebounce = 250 * time.Millisecond

// WatcherOptions parameterizes the watcher.
type WatcherOptions struct {
	Logger   slog.Logger
	Clock    quartz.Clock
	Debounce time.Duration
	// OnChange runs at most once per debounce window. The
	// caller must not block; the recommended pattern is a
	// non-blocking send on a re-resolve trigger channel.
	OnChange func()
}

// Watcher is a fixed-location fsnotify wrapper. It watches only
// the directories that can hold recognized resources (each scan
// root plus its skill containers and immediate skill dirs) rather
// than walking the tree, mirroring the resolver's fixed-location
// discovery. Inotify ENOSPC degrades the watcher into a poll-only
// mode that still re-resolves on Sync calls.
type Watcher struct {
	logger   slog.Logger
	clock    quartz.Clock
	debounce time.Duration
	onChange func()

	mu        sync.Mutex
	watcher   *fsnotify.Watcher
	watched   map[string]struct{}
	timer     *quartz.Timer
	degraded  string // non-empty when the watcher dropped events
	closed    bool
	closedCh  chan struct{}
	runDoneCh chan struct{}
}

// NewWatcher constructs a recursive watcher. The watcher does
// nothing until Sync is called.
func NewWatcher(opts WatcherOptions) (*Watcher, error) {
	if opts.OnChange == nil {
		return nil, xerrors.New("OnChange callback is required")
	}
	debounce := opts.Debounce
	if debounce <= 0 {
		debounce = DefaultWatchDebounce
	}
	clock := opts.Clock
	if clock == nil {
		clock = quartz.NewReal()
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		// On Linux, fsnotify.NewWatcher only fails when the
		// inotify subsystem is at the system-wide watch
		// limit. Surface a Watcher in "degraded" mode so the
		// caller can still rely on explicit Sync triggers.
		degraded := &Watcher{
			logger:    opts.Logger,
			clock:     clock,
			debounce:  debounce,
			onChange:  opts.OnChange,
			watched:   make(map[string]struct{}),
			degraded:  "fsnotify init failed: " + err.Error(),
			closedCh:  make(chan struct{}),
			runDoneCh: closedChan(),
		}
		return degraded, nil
	}

	cw := &Watcher{
		logger:    opts.Logger,
		clock:     clock,
		debounce:  debounce,
		onChange:  opts.OnChange,
		watcher:   w,
		watched:   make(map[string]struct{}),
		closedCh:  make(chan struct{}),
		runDoneCh: make(chan struct{}),
	}
	go cw.run()
	return cw, nil
}

// closedChan returns an already-closed channel for the
// degraded-watcher case where there is no run goroutine.
func closedChan() chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}

// Degraded returns a non-empty string when the watcher is
// running with reduced functionality (typically inotify
// ENOSPC). The string is suitable for use as a snapshot-level
// error message.
func (w *Watcher) Degraded() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.degraded
}

// Sync replaces the set of watched directories with the fixed
// locations that can hold recognized resources: each scan root,
// its skill containers, and the immediate skill subdirectories.
// Files are not watched directly; watching the parent directory
// catches creates, renames, removes, and writes that touch any
// recognized basename. Files that are themselves scan roots are
// handled by watching their parent.
//
// Sync is idempotent and safe to call repeatedly. The lock is
// released around the directory scan so concurrent Close,
// schedule, and the run goroutine are not blocked by a slow
// filesystem.
func (w *Watcher) Sync(ctx context.Context, roots []ScanRoot) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	if w.watcher == nil {
		// Degraded mode: no fsnotify, so there is nothing
		// to wire up. Do NOT fire the OnChange callback
		// from here; the Manager's signal handler is the
		// usual OnChange, and the Run loop calls back into
		// Sync when it observes that signal. Firing here
		// would re-arm an endless 250ms scan-and-push loop
		// on hosts where inotify cannot initialize. Manual
		// Resync, AddSource, and RemoveSource still drive
		// re-resolves; auto-updates on file edits simply
		// do not happen until fsnotify recovers.
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()

	// collectDirs touches the filesystem (stat/ReadDir on every
	// scan root and skill container). Compute the desired set
	// outside the mutex so it does not block the run goroutine,
	// Close, or schedule.
	desired := w.collectDirs(roots)

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}

	// Remove directories no longer wanted.
	for path := range w.watched {
		if _, ok := desired[path]; ok {
			continue
		}
		_ = w.watcher.Remove(path)
		delete(w.watched, path)
	}
	// Track whether every Add in this pass succeeded so a
	// recovered ENOSPC clears the degraded marker.
	addedAll := true
	// Add directories that are new.
	for path := range desired {
		if _, ok := w.watched[path]; ok {
			continue
		}
		if err := w.watcher.Add(path); err != nil {
			// ENOSPC means the kernel's per-user inotify
			// watch budget is exhausted. Mark the watcher
			// degraded; subsequent Sync calls still fire
			// the change callback so resync still works.
			if errors.Is(err, syscall.ENOSPC) {
				w.degraded = "inotify watch limit exceeded (ENOSPC)"
				addedAll = false
				w.logger.Warn(ctx, "context watcher degraded: inotify watch limit exceeded",
					slog.F("dir", path))
				break
			}
			w.logger.Debug(ctx, "context watcher could not add dir",
				slog.F("dir", path), slog.Error(err))
			continue
		}
		w.watched[path] = struct{}{}
	}
	// Clear a previously-set ENOSPC mark when every Add in this
	// pass succeeded. A user who bumps the kernel's inotify
	// limit and re-syncs now sees a clean snapshot instead of a
	// permanent SnapshotError.
	if addedAll && w.degraded != "" {
		w.degraded = ""
	}
}

// Close stops the watcher and releases all kernel watch slots.
// Close is idempotent.
func (w *Watcher) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	close(w.closedCh)
	timer := w.timer
	watcher := w.watcher
	w.timer = nil
	w.watcher = nil
	w.mu.Unlock()

	if timer != nil {
		timer.Stop()
	}
	if watcher != nil {
		_ = watcher.Close()
	}
	<-w.runDoneCh
	return nil
}

// run forwards fsnotify events into the debounce timer. It exits
// when Close is called or the underlying watcher is closed.
func (w *Watcher) run() {
	defer close(w.runDoneCh)
	// Capture the watcher reference once. Close may set the
	// field to nil concurrently; reading the captured local
	// keeps the event loop safe through the race window.
	w.mu.Lock()
	fsw := w.watcher
	w.mu.Unlock()
	if fsw == nil {
		return
	}
	for {
		select {
		case <-w.closedCh:
			return
		case ev, ok := <-fsw.Events:
			if !ok {
				return
			}
			if !w.eventRelevant(ev) {
				continue
			}
			w.schedule()
		case err, ok := <-fsw.Errors:
			if !ok {
				return
			}
			if err != nil {
				w.logger.Debug(context.Background(), "context watcher error", slog.Error(err))
			}
		}
	}
}

// eventRelevant filters out events that cannot affect any
// recognized resource. The check is conservative: any event on
// a directory triggers a re-resolve so newly created subtrees
// are picked up.
func (*Watcher) eventRelevant(ev fsnotify.Event) bool {
	name := filepath.Base(ev.Name)
	if recognizedInstructionFile(name) || name == mcpConfigFileName || name == skillMetaFileName {
		return true
	}
	// Directory create/remove flips re-resolve so new subtrees
	// arm watches and removed subtrees stop arming them.
	if ev.Has(fsnotify.Create) || ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename) {
		return true
	}
	return false
}

// schedule arms or resets the debounce timer.
func (w *Watcher) schedule() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	cb := w.onChange
	if w.timer != nil {
		w.timer.Reset(w.debounce)
		w.mu.Unlock()
		return
	}
	w.timer = w.clock.AfterFunc(w.debounce, func() {
		w.mu.Lock()
		w.timer = nil
		w.mu.Unlock()
		cb()
	})
	w.mu.Unlock()
}

// collectDirs returns the set of directories to watch. Discovery
// is fixed-location, mirroring the resolver: for each scan root we
// watch the root directory itself (catching top-level instruction
// and .mcp.json changes), plus every existing skill container and
// its immediate skill subdirectories (catching skill add/remove
// and SKILL.md writes). The watcher never recurses the tree.
func (*Watcher) collectDirs(roots []ScanRoot) map[string]struct{} {
	out := make(map[string]struct{})
	for _, root := range roots {
		if root.Path == "" {
			continue
		}
		info, err := os.Stat(root.Path)
		if err != nil {
			// Watch the deepest existing ancestor so the
			// root being created later still fires.
			if ancestor := existingAncestor(root.Path); ancestor != "" {
				out[ancestor] = struct{}{}
			}
			continue
		}
		if !info.IsDir() {
			out[filepath.Dir(root.Path)] = struct{}{}
			continue
		}
		out[root.Path] = struct{}{}
		for _, container := range skillContainersFor(root.Path) {
			out[container] = struct{}{}
			entries, err := os.ReadDir(container)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() {
					out[filepath.Join(container, e.Name())] = struct{}{}
				}
			}
		}
	}
	return out
}

// existingAncestor returns the deepest existing ancestor of
// path, or "" if no ancestor exists (e.g. an entirely missing
// drive on Windows).
func existingAncestor(path string) string {
	cur := filepath.Dir(path)
	for {
		if cur == "" || cur == "." {
			return ""
		}
		info, err := os.Stat(cur)
		if err == nil && info.IsDir() {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
}

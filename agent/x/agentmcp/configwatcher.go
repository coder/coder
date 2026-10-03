package agentmcp

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"
)

// defaultWatchDebounce coalesces editor-style multi-event writes
// (truncate plus rename plus chmod) into a single reload. The
// value is small enough to keep the late-file recovery latency
// well under a second.
const defaultWatchDebounce = 250 * time.Millisecond

// configWatcher watches the parent directories of one or more
// .mcp.json paths and fires a single debounced callback when any
// of those paths is created, modified, removed, or renamed.
//
// The watcher is deliberately tolerant of late-arriving config:
// if the parent directory does not exist yet, it walks up to the
// first existing ancestor and re-arms deeper as ancestors appear.
// Symlinks are resolved once at arming time; the watcher does not
// chase arbitrary symlink targets on every event.
type configWatcher struct {
	logger   slog.Logger
	clock    quartz.Clock
	debounce time.Duration

	// onChange is invoked once per debounce window when a watched
	// path is touched. It runs on a clock-managed timer goroutine
	// and must return promptly; callers should hand off to a
	// singleflight or background goroutine.
	onChange func()

	mu        sync.Mutex
	watcher   *fsnotify.Watcher
	files     map[string]string // resolved path -> watched ancestor dir.
	dirs      map[string]int    // ancestor dir -> refcount.
	timer     *quartz.Timer
	closed    bool
	closedCh  chan struct{}
	closeOnce sync.Once
	runDoneCh chan struct{}  // closed when run() exits.
	firesWG   sync.WaitGroup // tracks in-flight fire callbacks.
}

// newConfigWatcher creates a configWatcher and starts its event
// loop. Sync registers the actual paths to watch. The watcher does
// nothing until Sync is called.
func newConfigWatcher(
	logger slog.Logger,
	clock quartz.Clock,
	debounce time.Duration,
	onChange func(),
) (*configWatcher, error) {
	if onChange == nil {
		return nil, xerrors.New("onChange callback is required")
	}
	if debounce <= 0 {
		debounce = defaultWatchDebounce
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, xerrors.Errorf("create fsnotify watcher: %w", err)
	}

	cw := &configWatcher{
		logger:    logger,
		clock:     clock,
		debounce:  debounce,
		onChange:  onChange,
		watcher:   w,
		files:     make(map[string]string),
		dirs:      make(map[string]int),
		closedCh:  make(chan struct{}),
		runDoneCh: make(chan struct{}),
	}
	go cw.run()
	return cw, nil
}

// Sync replaces the watched set with paths. Files no longer in the
// list are removed; new files are added. Symlinks are resolved
// once. Individual arm failures are logged and skipped; partial
// arming is acceptable because parseAndDedup is the source of
// truth and the watcher exists purely to trigger a fresh stat.
//
// Sync is idempotent and safe to call repeatedly.
func (cw *configWatcher) Sync(paths []string) {
	if cw == nil {
		return
	}

	resolved := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		rp := resolvePath(p)
		if rp == "" {
			continue
		}
		resolved[rp] = struct{}{}
	}

	cw.mu.Lock()
	if cw.closed {
		cw.mu.Unlock()
		return
	}

	// Remove paths that are no longer wanted.
	for rp, dir := range cw.files {
		if _, keep := resolved[rp]; keep {
			continue
		}
		delete(cw.files, rp)
		cw.releaseDirLocked(dir)
	}

	// Add new paths.
	for rp := range resolved {
		if _, already := cw.files[rp]; already {
			continue
		}
		dir, err := cw.armAncestorLocked(rp)
		if err != nil {
			cw.logger.Warn(context.Background(),
				"failed to arm config file watch",
				slog.F("path", rp), slog.Error(err))
			continue
		}
		cw.files[rp] = dir
	}
	cw.mu.Unlock()
}

// armAncestorLocked walks up the parent chain from rp until it
// finds an existing directory, then watches that directory.
// Returns the actual directory it ended up watching. The last
// fsnotify Add error is preserved so callers can distinguish a
// missing-ancestor failure from an inotify-limit (ENOSPC) failure.
// Callers must hold cw.mu.
func (cw *configWatcher) armAncestorLocked(rp string) (string, error) {
	dir := filepath.Dir(rp)
	var lastAddDir string
	var lastAddErr error
	for {
		// Bail out if we somehow reached the root without finding
		// an existing directory. filepath.Dir("/") == "/" on POSIX
		// and "C:\" == "C:\" on Windows, so guard against an
		// infinite loop.
		if dir == "" || dir == "." {
			return "", noAncestorErr(rp, lastAddDir, lastAddErr)
		}

		if cw.dirs[dir] > 0 {
			cw.dirs[dir]++
			return dir, nil
		}

		err := cw.watcher.Add(dir)
		if err == nil {
			cw.dirs[dir] = 1
			return dir, nil
		}
		lastAddDir = dir
		lastAddErr = err

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", noAncestorErr(rp, lastAddDir, lastAddErr)
		}
		dir = parent
	}
}

// noAncestorErr formats the failure to register a watch on any
// ancestor of path. If the loop tried at least one Add, the
// underlying error (usually inotify ENOSPC) is wrapped so the
// operator sees the actual kernel-level cause instead of a generic
// "no existing ancestor" message.
func noAncestorErr(path, lastDir string, lastErr error) error {
	if lastErr != nil {
		return xerrors.Errorf("cannot watch any ancestor of %q (last attempt on %q): %w", path, lastDir, lastErr)
	}
	return xerrors.Errorf("no existing ancestor for %q", path)
}

// releaseDirLocked decrements the refcount for dir and removes the
// watch when no remaining file points at it. Callers must hold
// cw.mu.
func (cw *configWatcher) releaseDirLocked(dir string) {
	cw.dirs[dir]--
	if cw.dirs[dir] > 0 {
		return
	}
	delete(cw.dirs, dir)
	if err := cw.watcher.Remove(dir); err != nil {
		// Removal can fail when the directory no longer exists;
		// fsnotify already dropped the watch, so this is benign.
		cw.logger.Debug(context.Background(),
			"failed to remove config dir watch",
			slog.F("dir", dir), slog.Error(err))
	}
}

// run is the watcher loop. It exits when the underlying
// fsnotify.Watcher closes its channels or Close is called.
func (cw *configWatcher) run() {
	defer close(cw.runDoneCh)
	ctx := context.Background()
	for {
		select {
		case <-cw.closedCh:
			return
		case evt, ok := <-cw.watcher.Events:
			if !ok {
				return
			}
			cw.handleEvent(ctx, evt)
		case err, ok := <-cw.watcher.Errors:
			if !ok {
				return
			}
			cw.logger.Warn(ctx,
				"fsnotify watch error; config file changes may not be detected until the next HTTP request",
				slog.Error(err))
		}
	}
}

// handleEvent decides whether the event concerns one of the
// watched files (or could promote an ancestor watch) and, if so,
// schedules a debounced fire.
func (cw *configWatcher) handleEvent(ctx context.Context, evt fsnotify.Event) {
	cw.mu.Lock()
	if cw.closed {
		cw.mu.Unlock()
		return
	}

	// Match against any watched file. fsnotify event names are
	// already absolute when the watched directory is absolute,
	// which it is because armAncestorLocked called filepath.Dir
	// on a path resolved to absolute. The filepath.Abs call below
	// is a defensive normalization.
	evtAbs, err := filepath.Abs(evt.Name)
	if err != nil {
		cw.mu.Unlock()
		return
	}

	matchedFile := ""
	for rp := range cw.files {
		if rp == evtAbs {
			matchedFile = rp
			break
		}
	}

	// If a directory we are watching for an ancestor of an
	// unrealized path just gained a new child, try to re-arm
	// deeper. This handles `mkdir ~/.config; touch
	// ~/.config/.mcp.json` cases.
	if matchedFile == "" && evt.Has(fsnotify.Create) {
		for rp, dir := range cw.files {
			// Only re-arm files whose final parent is not yet
			// being watched directly.
			expected := filepath.Dir(rp)
			if dir == expected {
				continue
			}
			// If this event is a directory inside our currently
			// watched ancestor that lies on the way to rp,
			// re-arm.
			if isAncestorPathSegment(evtAbs, rp) {
				cw.releaseDirLocked(dir)
				newDir, armErr := cw.armAncestorLocked(rp)
				if armErr != nil {
					cw.logger.Debug(ctx,
						"failed to re-arm config file watch on ancestor create",
						slog.F("path", rp), slog.Error(armErr))
					// Leave the file unarmed for now;
					// next Sync will retry.
					delete(cw.files, rp)
					continue
				}
				cw.files[rp] = newDir
				// The new dir may already contain the
				// target file. Treat that as a match.
				matchedFile = rp
			}
		}
	}

	cw.mu.Unlock()

	if matchedFile == "" {
		return
	}
	cw.scheduleFire()
}

// isAncestorPathSegment reports whether candidate is on the path
// from the currently watched ancestor toward target.
func isAncestorPathSegment(candidate, target string) bool {
	// candidate must be a prefix of target's directory chain.
	tdir := filepath.Dir(target)
	for {
		if tdir == candidate {
			return true
		}
		parent := filepath.Dir(tdir)
		if parent == tdir {
			return false
		}
		tdir = parent
	}
}

// scheduleFire arms or extends a single debounce timer.
func (cw *configWatcher) scheduleFire() {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	if cw.closed {
		return
	}
	if cw.timer != nil {
		// Reset existing timer to extend the debounce window.
		// Stop reports whether the call stopped the timer before
		// it fired; if so we owe a Done because Add was called
		// when the timer was created.
		if cw.timer.Stop() {
			cw.firesWG.Done()
		}
	}
	cw.firesWG.Add(1)
	cw.timer = cw.clock.AfterFunc(cw.debounce, cw.fire, "agentmcp", "watch_debounce")
}

// fire is called once per debounce window. It invokes onChange
// outside the lock so reload code can re-enter Sync safely.
func (cw *configWatcher) fire() {
	defer cw.firesWG.Done()

	cw.mu.Lock()
	if cw.closed {
		cw.mu.Unlock()
		return
	}
	cw.timer = nil
	cw.mu.Unlock()

	cw.onChange()
}

// Close stops the watcher and waits for the run goroutine and
// any in-flight debounced fire callbacks to exit. Close is
// idempotent.
func (cw *configWatcher) Close() error {
	if cw == nil {
		return nil
	}
	var closeErr error
	cw.closeOnce.Do(func() {
		cw.mu.Lock()
		cw.closed = true
		if cw.timer != nil {
			// Stop returns true if the call prevented the timer
			// callback from running. Account for the Add() that
			// scheduleFire performed when arming this timer.
			if cw.timer.Stop() {
				cw.firesWG.Done()
			}
			cw.timer = nil
		}
		cw.mu.Unlock()

		close(cw.closedCh)
		if err := cw.watcher.Close(); err != nil {
			closeErr = xerrors.Errorf("close fsnotify watcher: %w", err)
		}
		// Wait for run() to exit, then wait for any in-flight
		// fire callback to return. Callers should not observe a
		// stale onChange after Close returns; this is critical
		// for tests that use slogtest, which panics on log
		// calls made after the test has finished.
		<-cw.runDoneCh
		cw.firesWG.Wait()
	})
	return closeErr
}

// resolvePath converts a path to an absolute, symlink-resolved
// form. If the file does not exist, falls back to filepath.Abs so
// the caller can still arm an ancestor directory.
func resolvePath(p string) string {
	if p == "" {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		// EvalSymlinks fails on non-existent paths. Resolve as
		// far as possible without erroring out: walk up until
		// we find an existing ancestor, eval its symlinks, and
		// re-join the trailing segments.
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			return resolved
		}
		return resolvePathBestEffort(abs)
	}
	return ""
}

func resolvePathBestEffort(abs string) string {
	dir := filepath.Dir(abs)
	base := filepath.Base(abs)
	for dir != "" && dir != "." {
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(resolved, base)
		}
		parent := filepath.Dir(dir)
		base = filepath.Join(filepath.Base(dir), base)
		if parent == dir {
			break
		}
		dir = parent
	}
	return abs
}

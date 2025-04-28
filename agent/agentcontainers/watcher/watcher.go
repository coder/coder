// Package watcher provides file system watching capabilities for the
// agent. It defines an interface for monitoring file changes and
// implementations that can be used to detect when configuration files
// are modified. This is primarily used to track changes to devcontainer
// configuration files and notify users when containers need to be
// recreated to apply the new configuration.
package watcher

import (
	"context"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/xerrors"
)

var ErrWatcherClosed = xerrors.New("watcher closed")

// Watcher defines an interface for monitoring file system changes.
// Implementations track file modifications and provide an event stream
// that clients can consume to react to changes.
type Watcher interface {
	// Add starts watching a file for changes.
	Add(file string) error

	// Remove stops watching a file for changes.
	Remove(file string) error

	// Next blocks until a file system event occurs or the context is canceled.
	// It returns the next event or an error if the watcher encountered a problem.
	Next(context.Context) (*fsnotify.Event, error)

	// Close shuts down the watcher and releases any resources.
	Close() error
}

type fsnotifyWatcher struct {
	*fsnotify.Watcher

	mu           sync.Mutex      // Protects following.
	watchedFiles map[string]bool // Files being watched (absolute path -> bool).
	watchedDirs  map[string]int  // Refcount of directories being watched (absolute path -> count).
	closed       bool            // Protects closing of done.
	done         chan struct{}
}

// NewFSNotify creates a new file system watcher that watches parent directories
// instead of individual files for more reliable event detection.
func NewFSNotify() (Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, xerrors.Errorf("create fsnotify watcher: %w", err)
	}
	return &fsnotifyWatcher{
		Watcher:      w,
		done:         make(chan struct{}),
		watchedFiles: make(map[string]bool),
		watchedDirs:  make(map[string]int),
	}, nil
}

func (f *fsnotifyWatcher) Add(file string) error {
	absPath, err := filepath.Abs(file)
	if err != nil {
		return xerrors.Errorf("absolute path: %w", err)
	}

	dir := filepath.Dir(absPath)

	f.mu.Lock()
	defer f.mu.Unlock()

	// Already watching this file.
	if f.watchedFiles[absPath] {
		return nil
	}

	// Start watching the parent directory if not already watching.
	if f.watchedDirs[dir] == 0 {
		if err := f.Watcher.Add(dir); err != nil {
			return xerrors.Errorf("add directory to watcher: %w", err)
		}
	}

	// Increment the reference count for this directory.
	f.watchedDirs[dir]++
	// Mark this file as watched.
	f.watchedFiles[absPath] = true

	return nil
}

func (f *fsnotifyWatcher) Remove(file string) error {
	absPath, err := filepath.Abs(file)
	if err != nil {
		return xerrors.Errorf("absolute path: %w", err)
	}

	dir := filepath.Dir(absPath)

	f.mu.Lock()
	defer f.mu.Unlock()

	// Not watching this file.
	if !f.watchedFiles[absPath] {
		return nil
	}

	// Remove the file from our watch list.
	delete(f.watchedFiles, absPath)

	// Decrement the reference count for this directory.
	f.watchedDirs[dir]--

	// If no more files in this directory are being watched, stop
	// watching the directory.
	if f.watchedDirs[dir] <= 0 {
		f.watchedDirs[dir] = 0 // Ensure non-negative count.
		if err := f.Watcher.Remove(dir); err != nil {
			return xerrors.Errorf("remove directory from watcher: %w", err)
		}
		delete(f.watchedDirs, dir)
	}

	return nil
}

func (f *fsnotifyWatcher) Next(ctx context.Context) (event *fsnotify.Event, err error) {
	defer func() {
		if ctx.Err() != nil {
			event = nil
			err = ctx.Err()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case evt, ok := <-f.Events:
			if !ok {
				return nil, ErrWatcherClosed
			}

			// Get the absolute path to match against our watched files.
			absPath, err := filepath.Abs(evt.Name)
			if err != nil {
				continue
			}

			f.mu.Lock()
			isWatched := f.watchedFiles[absPath]
			f.mu.Unlock()
			if isWatched {
				return &evt, nil
			}

			continue // Ignore events for files not being watched.

		case err, ok := <-f.Errors:
			if !ok {
				return nil, ErrWatcherClosed
			}
			return nil, xerrors.Errorf("watcher error: %w", err)
		case <-f.done:
			return nil, ErrWatcherClosed
		}
	}
}

func (f *fsnotifyWatcher) Close() (err error) {
	f.mu.Lock()
	f.watchedFiles = nil
	f.watchedDirs = nil
	closed := f.closed
	f.closed = true
	f.mu.Unlock()

	if closed {
		return ErrWatcherClosed
	}

	close(f.done)

	if err := f.Watcher.Close(); err != nil {
		return xerrors.Errorf("close watcher: %w", err)
	}

	return nil
}

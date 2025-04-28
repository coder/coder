// Package watcher provides file system watching capabilities for the
// agent. It defines an interface for monitoring file changes and
// implementations that can be used to detect when configuration files
// are modified. This is primarily used to track changes to devcontainer
// configuration files and notify users when containers need to be
// recreated to apply the new configuration.
package watcher

import (
	"context"
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
	closeOnce sync.Once
	closed    chan struct{}
}

func NewFSNotify() (Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, xerrors.Errorf("create fsnotify watcher: %w", err)
	}
	return &fsnotifyWatcher{
		Watcher: w,
		closed:  make(chan struct{}),
	}, nil
}

func (f *fsnotifyWatcher) Add(path string) error {
	if err := f.Watcher.Add(path); err != nil {
		return xerrors.Errorf("add path to watcher: %w", err)
	}
	return nil
}

func (f *fsnotifyWatcher) Remove(path string) error {
	if err := f.Watcher.Remove(path); err != nil {
		return xerrors.Errorf("remove path from watcher: %w", err)
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

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case event, ok := <-f.Events:
		if !ok {
			return nil, ErrWatcherClosed
		}
		return &event, nil
	case err, ok := <-f.Errors:
		if !ok {
			return nil, ErrWatcherClosed
		}
		return nil, xerrors.Errorf("watcher error: %w", err)
	case <-f.closed:
		return nil, ErrWatcherClosed
	}
}

func (f *fsnotifyWatcher) Close() (err error) {
	err = ErrWatcherClosed
	f.closeOnce.Do(func() {
		if err = f.Watcher.Close(); err != nil {
			err = xerrors.Errorf("close watcher: %w", err)
		}
		close(f.closed)
	})
	return err
}

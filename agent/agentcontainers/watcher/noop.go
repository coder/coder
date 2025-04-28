package watcher

import (
	"context"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// NewNoop creates a new watcher that does nothing.
func NewNoop() Watcher {
	return &noopWatcher{done: make(chan struct{})}
}

type noopWatcher struct {
	mu     sync.Mutex
	closed bool
	done   chan struct{}
}

func (*noopWatcher) Add(string) error {
	return nil
}

func (*noopWatcher) Remove(string) error {
	return nil
}

// Next blocks until the context is canceled or the watcher is closed.
func (n *noopWatcher) Next(ctx context.Context) (*fsnotify.Event, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-n.done:
		return nil, ErrWatcherClosed
	}
}

func (n *noopWatcher) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return ErrWatcherClosed
	}
	n.closed = true
	close(n.done)
	return nil
}

package watcher

import (
	"context"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/xerrors"
)

func NewNoop() Watcher {
	return &noopWatcher{closed: make(chan struct{})}
}

type noopWatcher struct {
	mu     synx.Mutex
	closed bool
	done   chan struct{}
}

func (*noopWatcher) Add(string) error {
	return nil
}

func (*noopWatcher) Remove(string) error {
	return nil
}

func (n *noopWatcher) Next(ctx context.Context) (*fsnotify.Event, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-n.done:
		return nil, xerrors.New("watcher closed")
	}
}

func (n *noopWatcher) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return nil
	}
	n.closed = true
	close(n.done)
	return nil
}

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
	closed chan struct{}
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
	case <-n.closed:
		return nil, xerrors.New("watcher closed")
	}
}

func (n *noopWatcher) Close() error {
	close(n.closed)
	return nil
}

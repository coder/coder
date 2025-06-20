package files

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// CacheCloser is a cache wrapper used to close all acquired files.
// This is a more simple interface to use if opening multiple files at once.
type CacheCloser struct {
	cache FileAcquirer

	close  []*CloseFS
	mu     sync.Mutex
	closed bool
}

func NewCacheCloser(cache FileAcquirer) *CacheCloser {
	return &CacheCloser{
		cache: cache,
		close: make([]*CloseFS, 0),
	}
}

func (c *CacheCloser) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, fs := range c.close {
		fs.Close()
	}
	c.closed = true
}

func (c *CacheCloser) Acquire(ctx context.Context, fileID uuid.UUID) (*CloseFS, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, xerrors.New("cache is closed, and cannot acquire new files")
	}

	f, err := c.cache.Acquire(ctx, fileID)
	if err != nil {
		return nil, err
	}

	c.close = append(c.close, f)

	return f, nil
}

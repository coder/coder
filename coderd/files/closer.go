package files

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// CacheCloser is a cache wrapper used to close all acquired files.
// This is a more simple interface to use if opening multiple files at once.
type CacheCloser struct {
	cache FileAcquirer

	closers []func()
	mu      sync.Mutex
}

func NewCacheCloser(cache FileAcquirer) *CacheCloser {
	return &CacheCloser{
		cache:   cache,
		closers: make([]func(), 0),
	}
}

func (c *CacheCloser) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, doClose := range c.closers {
		doClose()
	}

	// Prevent further acquisitions
	c.cache = nil
	// Remove any references
	c.closers = nil
}

func (c *CacheCloser) Acquire(ctx context.Context, db database.Store, fileID uuid.UUID) (*CloseFS, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cache == nil {
		return nil, xerrors.New("cache is closed, and cannot acquire new files")
	}

	f, err := c.cache.Acquire(ctx, db, fileID)
	if err != nil {
		return nil, err
	}

	c.closers = append(c.closers, f.close)

	return f, nil
}

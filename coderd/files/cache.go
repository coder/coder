package files

import (
	"bytes"
	"context"
	"io/fs"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	archivefs "github.com/coder/coder/v2/archive/fs"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/lazy"
)

// NewFromStore returns a file cache that will fetch files from the provided
// database.
func NewFromStore(store database.Store) Cache {
	fetcher := func(ctx context.Context, fileID uuid.UUID) (fs.FS, error) {
		file, err := store.GetFileByID(ctx, fileID)
		if err != nil {
			return nil, xerrors.Errorf("failed to read file from database: %w", err)
		}

		content := bytes.NewBuffer(file.Data)
		return archivefs.FromTarReader(content), nil
	}

	return Cache{
		lock:    sync.Mutex{},
		data:    make(map[uuid.UUID]*cacheEntry),
		fetcher: fetcher,
	}
}

// Cache persists the files for template versions, and is used by dynamic
// parameters to deduplicate the files in memory. When any number of users opens
// the workspace creation form for a given template version, it's files are
// loaded into memory exactly once. We hold those files until there are no
// longer any open connections, and then we remove the value from the map.
type Cache struct {
	lock sync.Mutex
	data map[uuid.UUID]*cacheEntry
	fetcher
}

type cacheEntry struct {
	refCount *atomic.Int64
	value    *lazy.ValueWithError[fs.FS]
}

type fetcher func(context.Context, uuid.UUID) (fs.FS, error)

// Acquire will load the fs.FS for the given file. It guarantees that parallel
// calls for the same fileID will only result in one fetch, and that parallel
// calls for distinct fileIDs will fetch in parallel.
func (c *Cache) Acquire(ctx context.Context, fileID uuid.UUID) (fs.FS, error) {
	// It's important that this `Load` call occurs outside of `prepare`, after the
	// mutex has been released, or we would continue to hold the lock until the
	// entire file has been fetched, which may be slow, and would prevent other
	// files from being fetched in parallel.
	return c.prepare(ctx, fileID).Load()
}

func (c *Cache) prepare(ctx context.Context, fileID uuid.UUID) *lazy.ValueWithError[fs.FS] {
	c.lock.Lock()
	defer c.lock.Unlock()

	entry, ok := c.data[fileID]
	if !ok {
		var refCount atomic.Int64
		value := lazy.NewWithError(func() (fs.FS, error) {
			return c.fetcher(ctx, fileID)
		})

		entry = &cacheEntry{
			value:    value,
			refCount: &refCount,
		}
		c.data[fileID] = entry
	}

	entry.refCount.Add(1)
	return entry.value
}

// Release decrements the reference count for the given fileID, and frees the
// backing data if there are no further references being held.
func (c *Cache) Release(fileID uuid.UUID) {
	c.lock.Lock()
	defer c.lock.Unlock()

	entry, ok := c.data[fileID]
	if !ok {
		// If we land here, it's almost certainly because a bug already happened,
		// and we're freeing something that's already been freed, or we're calling
		// this function with an incorrect ID. Should this function return an error?
		return
	}
	refCount := entry.refCount.Add(-1)
	if refCount > 0 {
		return
	}
	delete(c.data, fileID)
}

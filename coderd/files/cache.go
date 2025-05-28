package files

import (
	"bytes"
	"context"
	"io/fs"
	"sync"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/xerrors"

	archivefs "github.com/coder/coder/v2/archive/fs"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/lazy"
)

// NewFromStore returns a file cache that will fetch files from the provided
// database.
func NewFromStore(store database.Store, registerer prometheus.Registerer) *Cache {
	fetch := func(ctx context.Context, fileID uuid.UUID) (fs.FS, int64, error) {
		file, err := store.GetFileByID(ctx, fileID)
		if err != nil {
			return nil, 0, xerrors.Errorf("failed to read file from database: %w", err)
		}

		content := bytes.NewBuffer(file.Data)
		return archivefs.FromTarReader(content), int64(content.Len()), nil
	}

	return New(fetch, registerer)
}

func New(fetch fetcher, registerer prometheus.Registerer) *Cache {
	return (&Cache{
		lock:    sync.Mutex{},
		data:    make(map[uuid.UUID]*cacheEntry),
		fetcher: fetch,
	}).registerMetrics(registerer)
}

func (c *Cache) registerMetrics(registerer prometheus.Registerer) *Cache {
	subsystem := "file_cache"
	f := promauto.With(registerer)

	c.currentCacheSize = f.NewGauge(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: subsystem,
		Name:      "open_files_size_current",
		Help:      "The current size of all files currently open in the file cache.",
	})

	c.totalCacheSize = f.NewCounter(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: subsystem,
		Name:      "open_files_size_total",
		Help:      "The total size of all files opened in the file cache.",
	})

	c.currentOpenFiles = f.NewGauge(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: subsystem,
		Name:      "open_files_current",
		Help:      "The number of unique files currently open in the file cache.",
	})

	c.totalOpenedFiles = f.NewCounter(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: subsystem,
		Name:      "open_files_total",
		Help:      "The number of unique files opened in the file cache.",
	})

	c.currentOpenFileReferences = f.NewGauge(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: subsystem,
		Name:      "open_file_refs_current",
		Help:      "The number of file references currently open in the file cache.",
	})

	c.totalOpenFileReferences = f.NewCounter(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: subsystem,
		Name:      "open_file_refs_total",
		Help:      "The number of file references currently open in the file cache.",
	})

	return c
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

	// metrics
	currentOpenFileReferences prometheus.Gauge
	totalOpenFileReferences   prometheus.Counter

	currentOpenFiles prometheus.Gauge
	totalOpenedFiles prometheus.Counter

	currentCacheSize prometheus.Gauge
	totalCacheSize   prometheus.Counter
}

type cacheEntryValue struct {
	dir  fs.FS
	size int64
}

type cacheEntry struct {
	// refCount must only be accessed while the Cache lock is held.
	refCount int
	value    *lazy.ValueWithError[cacheEntryValue]
}

type fetcher func(context.Context, uuid.UUID) (dir fs.FS, size int64, err error)

// Acquire will load the fs.FS for the given file. It guarantees that parallel
// calls for the same fileID will only result in one fetch, and that parallel
// calls for distinct fileIDs will fetch in parallel.
//
// Every call to Acquire must have a matching call to Release.
func (c *Cache) Acquire(ctx context.Context, fileID uuid.UUID) (fs.FS, error) {
	// It's important that this `Load` call occurs outside of `prepare`, after the
	// mutex has been released, or we would continue to hold the lock until the
	// entire file has been fetched, which may be slow, and would prevent other
	// files from being fetched in parallel.
	it, err := c.prepare(ctx, fileID).Load()
	if err != nil {
		c.Release(fileID)
	}
	return it.dir, err
}

func (c *Cache) prepare(ctx context.Context, fileID uuid.UUID) *lazy.ValueWithError[cacheEntryValue] {
	c.lock.Lock()
	defer c.lock.Unlock()

	entry, ok := c.data[fileID]
	if !ok {
		value := lazy.NewWithError(func() (cacheEntryValue, error) {
			dir, size, err := c.fetcher(ctx, fileID)

			// Always add to the cache size the bytes of the file loaded.
			if err == nil {
				c.currentCacheSize.Add(float64(size))
				c.totalCacheSize.Add(float64(size))
			}

			return cacheEntryValue{
				dir:  dir,
				size: size,
			}, err
		})

		entry = &cacheEntry{
			value:    value,
			refCount: 0,
		}
		c.data[fileID] = entry
		c.currentOpenFiles.Inc()
		c.totalOpenedFiles.Inc()
	}

	c.currentOpenFileReferences.Inc()
	c.totalOpenFileReferences.Inc()
	entry.refCount++
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

	c.currentOpenFileReferences.Dec()
	entry.refCount--
	if entry.refCount > 0 {
		return
	}

	c.currentOpenFiles.Dec()

	ev, err := entry.value.Load()
	if err == nil {
		c.currentCacheSize.Add(-1 * float64(ev.size))
	}

	delete(c.data, fileID)
}

// Count returns the number of files currently in the cache.
// Mainly used for unit testing assertions.
func (c *Cache) Count() int {
	c.lock.Lock()
	defer c.lock.Unlock()

	return len(c.data)
}

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
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/lazy"
)

// NewFromStore returns a file cache that will fetch files from the provided
// database.
func NewFromStore(store database.Store, registerer prometheus.Registerer, authz rbac.Authorizer) *Cache {
	fetch := func(ctx context.Context, fileID uuid.UUID) (CacheEntryValue, error) {
		// Make sure the read does not fail due to authorization issues.
		// Authz is checked on the Acquire call, so this is safe.
		//nolint:gocritic
		file, err := store.GetFileByID(dbauthz.AsFileReader(ctx), fileID)
		if err != nil {
			return CacheEntryValue{}, xerrors.Errorf("failed to read file from database: %w", err)
		}

		content := bytes.NewBuffer(file.Data)
		return CacheEntryValue{
			Object: file.RBACObject(),
			FS:     archivefs.FromTarReader(content),
			Size:   int64(content.Len()),
		}, nil
	}

	return New(fetch, registerer, authz)
}

func New(fetch fetcher, registerer prometheus.Registerer, authz rbac.Authorizer) *Cache {
	return (&Cache{
		lock:    sync.Mutex{},
		data:    make(map[uuid.UUID]*cacheEntry),
		fetcher: fetch,
		authz:   authz,
	}).registerMetrics(registerer)
}

func (c *Cache) registerMetrics(registerer prometheus.Registerer) *Cache {
	subsystem := "file_cache"
	f := promauto.With(registerer)

	c.currentCacheSize = f.NewGauge(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: subsystem,
		Name:      "open_files_size_bytes_current",
		Help:      "The current amount of memory of all files currently open in the file cache.",
	})

	c.totalCacheSize = f.NewCounter(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: subsystem,
		Name:      "open_files_size_bytes_total",
		Help:      "The total amount of memory ever opened in the file cache. This number never decrements.",
	})

	c.currentOpenFiles = f.NewGauge(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: subsystem,
		Name:      "open_files_current",
		Help:      "The count of unique files currently open in the file cache.",
	})

	c.totalOpenedFiles = f.NewCounter(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: subsystem,
		Name:      "open_files_total",
		Help:      "The total count of unique files ever opened in the file cache.",
	})

	c.currentOpenFileReferences = f.NewGauge(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: subsystem,
		Name:      "open_file_refs_current",
		Help:      "The count of file references currently open in the file cache. Multiple references can be held for the same file.",
	})

	c.totalOpenFileReferences = f.NewCounter(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: subsystem,
		Name:      "open_file_refs_total",
		Help:      "The total number of file references ever opened in the file cache.",
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
	authz rbac.Authorizer

	// metrics
	cacheMetrics
}

type cacheMetrics struct {
	currentOpenFileReferences prometheus.Gauge
	totalOpenFileReferences   prometheus.Counter

	currentOpenFiles prometheus.Gauge
	totalOpenedFiles prometheus.Counter

	currentCacheSize prometheus.Gauge
	totalCacheSize   prometheus.Counter
}

type CacheEntryValue struct {
	fs.FS
	Object rbac.Object
	Size   int64
}

type cacheEntry struct {
	// refCount must only be accessed while the Cache lock is held.
	refCount int
	value    *lazy.ValueWithError[CacheEntryValue]
}

type fetcher func(context.Context, uuid.UUID) (CacheEntryValue, error)

var _ fs.FS = (*CloseFS)(nil)

// CloseFS is a wrapper around fs.FS that implements io.Closer. The Close()
// method tells the cache to release the fileID. Once all open references are
// closed, the file is removed from the cache.
type CloseFS struct {
	fs.FS

	close func()
}

func (f *CloseFS) Close() { f.close() }

// Acquire will load the fs.FS for the given file. It guarantees that parallel
// calls for the same fileID will only result in one fetch, and that parallel
// calls for distinct fileIDs will fetch in parallel.
//
// Safety: Every call to Acquire that does not return an error must have a
// matching call to Release.
func (c *Cache) Acquire(ctx context.Context, fileID uuid.UUID) (*CloseFS, error) {
	// It's important that this `Load` call occurs outside `prepare`, after the
	// mutex has been released, or we would continue to hold the lock until the
	// entire file has been fetched, which may be slow, and would prevent other
	// files from being fetched in parallel.
	it, err := c.prepare(ctx, fileID).Load()
	if err != nil {
		c.release(fileID)
		return nil, err
	}

	subject, ok := dbauthz.ActorFromContext(ctx)
	if !ok {
		return nil, dbauthz.ErrNoActor
	}
	// Always check the caller can actually read the file.
	if err := c.authz.Authorize(ctx, subject, policy.ActionRead, it.Object); err != nil {
		c.release(fileID)
		return nil, err
	}

	var once sync.Once
	return &CloseFS{
		FS: it.FS,
		close: func() {
			// sync.Once makes the Close() idempotent, so we can call it
			// multiple times without worrying about double-releasing.
			once.Do(func() { c.release(fileID) })
		},
	}, nil
}

func (c *Cache) prepare(ctx context.Context, fileID uuid.UUID) *lazy.ValueWithError[CacheEntryValue] {
	c.lock.Lock()
	defer c.lock.Unlock()

	entry, ok := c.data[fileID]
	if !ok {
		value := lazy.NewWithError(func() (CacheEntryValue, error) {
			val, err := c.fetcher(ctx, fileID)

			// Always add to the cache size the bytes of the file loaded.
			if err == nil {
				c.currentCacheSize.Add(float64(val.Size))
				c.totalCacheSize.Add(float64(val.Size))
			}

			return val, err
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

// release decrements the reference count for the given fileID, and frees the
// backing data if there are no further references being held.
//
// release should only be called after a successful call to Acquire using the Release()
// method on the returned *CloseFS.
func (c *Cache) release(fileID uuid.UUID) {
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
		c.currentCacheSize.Add(-1 * float64(ev.Size))
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

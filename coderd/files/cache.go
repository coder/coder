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

type FileAcquirer interface {
	Acquire(ctx context.Context, db database.Store, fileID uuid.UUID) (*CloseFS, error)
}

// New returns a file cache that will fetch files from a database
func New(registerer prometheus.Registerer, authz rbac.Authorizer) *Cache {
	return &Cache{
		lock:         sync.Mutex{},
		data:         make(map[uuid.UUID]*cacheEntry),
		authz:        authz,
		cacheMetrics: newCacheMetrics(registerer),
	}
}

func newCacheMetrics(registerer prometheus.Registerer) cacheMetrics {
	subsystem := "file_cache"
	f := promauto.With(registerer)

	return cacheMetrics{
		currentCacheSize: f.NewGauge(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: subsystem,
			Name:      "open_files_size_bytes_current",
			Help:      "The current amount of memory of all files currently open in the file cache.",
		}),

		totalCacheSize: f.NewCounter(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: subsystem,
			Name:      "open_files_size_bytes_total",
			Help:      "The total amount of memory ever opened in the file cache. This number never decrements.",
		}),

		currentOpenFiles: f.NewGauge(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: subsystem,
			Name:      "open_files_current",
			Help:      "The count of unique files currently open in the file cache.",
		}),

		totalOpenedFiles: f.NewCounter(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: subsystem,
			Name:      "open_files_total",
			Help:      "The total count of unique files ever opened in the file cache.",
		}),

		currentOpenFileReferences: f.NewGauge(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: subsystem,
			Name:      "open_file_refs_current",
			Help:      "The count of file references currently open in the file cache. Multiple references can be held for the same file.",
		}),

		totalOpenFileReferences: f.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: subsystem,
			Name:      "open_file_refs_total",
			Help:      "The total number of file references ever opened in the file cache. The 'hit' label indicates if the file was loaded from the cache.",
		}, []string{"hit"}),
	}
}

// Cache persists the files for template versions, and is used by dynamic
// parameters to deduplicate the files in memory. When any number of users opens
// the workspace creation form for a given template version, it's files are
// loaded into memory exactly once. We hold those files until there are no
// longer any open connections, and then we remove the value from the map.
type Cache struct {
	lock  sync.Mutex
	data  map[uuid.UUID]*cacheEntry
	authz rbac.Authorizer

	// metrics
	cacheMetrics
}

type cacheMetrics struct {
	currentOpenFileReferences prometheus.Gauge
	totalOpenFileReferences   *prometheus.CounterVec

	currentOpenFiles prometheus.Gauge
	totalOpenedFiles prometheus.Counter

	currentCacheSize prometheus.Gauge
	totalCacheSize   prometheus.Counter
}

type cacheEntry struct {
	// Safety: refCount must only be accessed while the Cache lock is held.
	refCount int
	value    *lazy.ValueWithError[CacheEntryValue]

	// Safety: close must only be called while the Cache lock is held
	close func()
	// Safety: purge must only be called while the Cache lock is held
	purge func()
}

type CacheEntryValue struct {
	fs.FS
	Object rbac.Object
	Size   int64
}

var _ fs.FS = (*CloseFS)(nil)

// CloseFS is a wrapper around fs.FS that implements io.Closer. The Close()
// method tells the cache to release the fileID. Once all open references are
// closed, the file is removed from the cache.
type CloseFS struct {
	fs.FS

	close func()
}

func (f *CloseFS) Close() {
	f.close()
}

// Acquire will load the fs.FS for the given file. It guarantees that parallel
// calls for the same fileID will only result in one fetch, and that parallel
// calls for distinct fileIDs will fetch in parallel.
//
// Safety: Every call to Acquire that does not return an error must call close
// on the returned value when it is done being used.
func (c *Cache) Acquire(ctx context.Context, db database.Store, fileID uuid.UUID) (*CloseFS, error) {
	// It's important that this `Load` call occurs outside `prepare`, after the
	// mutex has been released, or we would continue to hold the lock until the
	// entire file has been fetched, which may be slow, and would prevent other
	// files from being fetched in parallel.
	e := c.prepare(db, fileID)
	ev, err := e.value.Load()
	if err != nil {
		c.lock.Lock()
		defer c.lock.Unlock()
		e.close()
		e.purge()
		return nil, err
	}

	cleanup := func() {
		c.lock.Lock()
		defer c.lock.Unlock()
		e.close()
	}

	// We always run the fetch under a system context and actor, so we need to
	// check the caller's context (including the actor) manually before returning.

	// Check if the caller's context was canceled. Even though `Authorize` takes
	// a context, we still check it manually first because none of our mock
	// database implementations check for context cancellation.
	if err := ctx.Err(); err != nil {
		cleanup()
		return nil, err
	}

	// Check that the caller is authorized to access the file
	subject, ok := dbauthz.ActorFromContext(ctx)
	if !ok {
		cleanup()
		return nil, dbauthz.ErrNoActor
	}
	if err := c.authz.Authorize(ctx, subject, policy.ActionRead, ev.Object); err != nil {
		cleanup()
		return nil, err
	}

	var closeOnce sync.Once
	return &CloseFS{
		FS: ev.FS,
		close: func() {
			// sync.Once makes the Close() idempotent, so we can call it
			// multiple times without worrying about double-releasing.
			closeOnce.Do(func() {
				c.lock.Lock()
				defer c.lock.Unlock()
				e.close()
			})
		},
	}, nil
}

func (c *Cache) prepare(db database.Store, fileID uuid.UUID) *cacheEntry {
	c.lock.Lock()
	defer c.lock.Unlock()

	hitLabel := "true"
	entry, ok := c.data[fileID]
	if !ok {
		hitLabel = "false"

		var purgeOnce sync.Once
		entry = &cacheEntry{
			value: lazy.NewWithError(func() (CacheEntryValue, error) {
				val, err := fetch(db, fileID)
				if err != nil {
					return val, err
				}

				// Add the size of the file to the cache size metrics.
				c.currentCacheSize.Add(float64(val.Size))
				c.totalCacheSize.Add(float64(val.Size))

				return val, err
			}),

			close: func() {
				entry.refCount--
				c.currentOpenFileReferences.Dec()
				if entry.refCount > 0 {
					return
				}

				entry.purge()
			},

			purge: func() {
				purgeOnce.Do(func() {
					c.purge(fileID)
				})
			},
		}
		c.data[fileID] = entry

		c.currentOpenFiles.Inc()
		c.totalOpenedFiles.Inc()
	}

	c.currentOpenFileReferences.Inc()
	c.totalOpenFileReferences.WithLabelValues(hitLabel).Inc()
	entry.refCount++
	return entry
}

// purge immediately removes an entry from the cache, even if it has open
// references.
// Safety: Must only be called while the Cache lock is held
func (c *Cache) purge(fileID uuid.UUID) {
	entry, ok := c.data[fileID]
	if !ok {
		// If we land here, it's probably because of a fetch attempt that
		// resulted in an error, and got purged already. It may also be an
		// erroneous extra close, but we can't really distinguish between those
		// two cases currently.
		return
	}

	// Purge the file from the cache.
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

func fetch(store database.Store, fileID uuid.UUID) (CacheEntryValue, error) {
	// Because many callers can be waiting on the same file fetch concurrently, we
	// want to prevent any failures that would cause them all to receive errors
	// because the caller who initiated the fetch would fail.
	// - We always run the fetch with an uncancelable context, and then check
	//   context cancellation for each acquirer afterwards.
	// - We always run the fetch as a system user, and then check authorization
	//   for each acquirer afterwards.
	// This prevents a canceled context or an unauthorized user from "holding up
	// the queue".
	//nolint:gocritic
	file, err := store.GetFileByID(dbauthz.AsFileReader(context.Background()), fileID)
	if err != nil {
		return CacheEntryValue{}, xerrors.Errorf("failed to read file from database: %w", err)
	}

	var files fs.FS
	switch file.Mimetype {
	case "application/zip", "application/x-zip-compressed":
		files, err = archivefs.FromZipReader(bytes.NewReader(file.Data), int64(len(file.Data)))
		if err != nil {
			return CacheEntryValue{}, xerrors.Errorf("failed to read zip file: %w", err)
		}
	default:
		// Assume '"application/x-tar"' as the default mimetype.
		files = archivefs.FromTarReader(bytes.NewBuffer(file.Data))
	}

	return CacheEntryValue{
		Object: file.RBACObject(),
		FS:     files,
		Size:   int64(len(file.Data)),
	}, nil
}

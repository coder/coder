package files

import (
	"context"
	"io/fs"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/v2/testutil"
)

var emptyFS fs.FS = afero.NewIOFS(afero.NewMemMapFs())

func TestConcurrency(t *testing.T) {
	t.Parallel()

	var fetches atomic.Int64
	c := newTestCache(func(_ context.Context, _ uuid.UUID) (fs.FS, error) {
		fetches.Add(1)
		// Wait long enough before returning to make sure that all of the goroutines
		// will be waiting in line, ensuring that no one duplicated a fetch.
		time.Sleep(testutil.IntervalMedium)
		return emptyFS, nil
	})

	batches := 1000
	groups := make([]*errgroup.Group, 0, batches)
	for range batches {
		groups = append(groups, new(errgroup.Group))
	}

	// Call Acquire with a unique ID per batch, many times per batch, with many
	// batches all in parallel. This is pretty much the worst-case scenario:
	// thousands of concurrent reads, with both warm and cold loads happening.
	batchSize := 10
	for _, g := range groups {
		id := uuid.New()
		for range batchSize {
			g.Go(func() error {
				_, err := c.Acquire(t.Context(), id)
				return err
			})
		}
	}

	for _, g := range groups {
		require.NoError(t, g.Wait())
	}
	require.Equal(t, int64(batches), fetches.Load())
}

func TestRelease(t *testing.T) {
	t.Parallel()

	c := newTestCache(func(_ context.Context, _ uuid.UUID) (fs.FS, error) {
		return emptyFS, nil
	})

	batches := 100
	ids := make([]uuid.UUID, 0, batches)
	for range batches {
		ids = append(ids, uuid.New())
	}

	// Acquire a bunch of references
	batchSize := 10
	for _, id := range ids {
		for range batchSize {
			fs, err := c.Acquire(t.Context(), id)
			require.NoError(t, err)
			require.Equal(t, emptyFS, fs)
		}
	}

	// Make sure cache is fully loaded
	require.Equal(t, len(c.data), batches)

	// Now release all of the references
	for _, id := range ids {
		for range batchSize {
			c.Release(id)
		}
	}

	// ...and make sure that the cache has emptied itself.
	require.Equal(t, len(c.data), 0)
}

func newTestCache(fetcher func(context.Context, uuid.UUID) (fs.FS, error)) Cache {
	return Cache{
		lock:    sync.Mutex{},
		data:    make(map[uuid.UUID]*cacheEntry),
		fetcher: fetcher,
	}
}

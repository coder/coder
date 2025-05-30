package files

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/v2/coderd/coderdtest/promhelp"
	"github.com/coder/coder/v2/testutil"
)

func cachePromMetricName(metric string) string {
	return "coderd_file_cache_" + metric
}

func TestConcurrency(t *testing.T) {
	t.Parallel()

	const fileSize = 10
	emptyFS := afero.NewIOFS(afero.NewReadOnlyFs(afero.NewMemMapFs()))
	var fetches atomic.Int64
	reg := prometheus.NewRegistry()
	c := New(func(_ context.Context, _ uuid.UUID) (cacheEntryValue, error) {
		fetches.Add(1)
		// Wait long enough before returning to make sure that all of the goroutines
		// will be waiting in line, ensuring that no one duplicated a fetch.
		time.Sleep(testutil.IntervalMedium)
		return cacheEntryValue{FS: emptyFS, size: fileSize}, nil
	}, reg)

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
				// We don't bother to Release these references because the Cache will be
				// released at the end of the test anyway.
				_, err := c.Acquire(t.Context(), id)
				return err
			})
		}
	}

	for _, g := range groups {
		require.NoError(t, g.Wait())
	}
	require.Equal(t, int64(batches), fetches.Load())

	// Verify all the counts & metrics are correct.
	require.Equal(t, batches, c.Count())
	require.Equal(t, batches*fileSize, promhelp.GaugeValue(t, reg, cachePromMetricName("open_files_size_bytes_current"), nil))
	require.Equal(t, batches*fileSize, promhelp.CounterValue(t, reg, cachePromMetricName("open_files_size_bytes_total"), nil))
	require.Equal(t, batches, promhelp.GaugeValue(t, reg, cachePromMetricName("open_files_current"), nil))
	require.Equal(t, batches, promhelp.CounterValue(t, reg, cachePromMetricName("open_files_total"), nil))
	require.Equal(t, batches*batchSize, promhelp.GaugeValue(t, reg, cachePromMetricName("open_file_refs_current"), nil))
	require.Equal(t, batches*batchSize, promhelp.CounterValue(t, reg, cachePromMetricName("open_file_refs_total"), nil))
}

func TestRelease(t *testing.T) {
	t.Parallel()

	const fileSize = 10
	emptyFS := afero.NewIOFS(afero.NewReadOnlyFs(afero.NewMemMapFs()))
	reg := prometheus.NewRegistry()
	c := New(func(_ context.Context, _ uuid.UUID) (cacheEntryValue, error) {
		return cacheEntryValue{
			FS:   emptyFS,
			size: fileSize,
		}, nil
	}, reg)

	batches := 100
	ids := make([]uuid.UUID, 0, batches)
	for range batches {
		ids = append(ids, uuid.New())
	}

	// Acquire a bunch of references
	batchSize := 10
	for openedIdx, id := range ids {
		for batchIdx := range batchSize {
			it, err := c.Acquire(t.Context(), id)
			require.NoError(t, err)
			require.Equal(t, emptyFS, it)

			// Each time a new file is opened, the metrics should be updated as so:
			opened := openedIdx + 1
			// Number of unique files opened is equal to the idx of the ids.
			require.Equal(t, opened, c.Count())
			require.Equal(t, opened, promhelp.GaugeValue(t, reg, cachePromMetricName("open_files_current"), nil))
			// Current file size is unique files * file size.
			require.Equal(t, opened*fileSize, promhelp.GaugeValue(t, reg, cachePromMetricName("open_files_size_bytes_current"), nil))
			// The number of refs is the current iteration of both loops.
			require.Equal(t, ((opened-1)*batchSize)+(batchIdx+1), promhelp.GaugeValue(t, reg, cachePromMetricName("open_file_refs_current"), nil))
		}
	}

	// Make sure cache is fully loaded
	require.Equal(t, len(c.data), batches)

	// Now release all of the references
	for closedIdx, id := range ids {
		stillOpen := len(ids) - closedIdx
		for closingIdx := range batchSize {
			c.Release(id)

			// Each time a file is released, the metrics should decrement the file refs
			require.Equal(t, (stillOpen*batchSize)-(closingIdx+1), promhelp.GaugeValue(t, reg, cachePromMetricName("open_file_refs_current"), nil))

			closed := closingIdx+1 == batchSize
			if closed {
				continue
			}

			// File ref still exists, so the counts should not change yet.
			require.Equal(t, stillOpen, c.Count())
			require.Equal(t, stillOpen, promhelp.GaugeValue(t, reg, cachePromMetricName("open_files_current"), nil))
			require.Equal(t, stillOpen*fileSize, promhelp.GaugeValue(t, reg, cachePromMetricName("open_files_size_bytes_current"), nil))
		}
	}

	// ...and make sure that the cache has emptied itself.
	require.Equal(t, len(c.data), 0)

	// Verify all the counts & metrics are correct.
	// All existing files are closed
	require.Equal(t, 0, c.Count())
	require.Equal(t, 0, promhelp.GaugeValue(t, reg, cachePromMetricName("open_files_size_bytes_current"), nil))
	require.Equal(t, 0, promhelp.GaugeValue(t, reg, cachePromMetricName("open_files_current"), nil))
	require.Equal(t, 0, promhelp.GaugeValue(t, reg, cachePromMetricName("open_file_refs_current"), nil))

	// Total counts remain
	require.Equal(t, batches*fileSize, promhelp.CounterValue(t, reg, cachePromMetricName("open_files_size_bytes_total"), nil))
	require.Equal(t, batches, promhelp.CounterValue(t, reg, cachePromMetricName("open_files_total"), nil))
	require.Equal(t, batches*batchSize, promhelp.CounterValue(t, reg, cachePromMetricName("open_file_refs_total"), nil))
}

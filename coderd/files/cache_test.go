package files_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/sync/errgroup"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/coderdtest/promhelp"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/testutil"
)

func TestCancelledFetch(t *testing.T) {
	t.Parallel()

	fileID := uuid.New()
	dbM := dbmock.NewMockStore(gomock.NewController(t))

	// The file fetch should succeed.
	dbM.EXPECT().GetFileByID(gomock.Any(), gomock.Any()).DoAndReturn(func(mTx context.Context, fileID uuid.UUID) (database.File, error) {
		return database.File{
			ID:   fileID,
			Data: make([]byte, 100),
		}, nil
	})

	cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})

	// Cancel the context for the first call; should fail.
	ctx, cancel := context.WithCancel(dbauthz.AsFileReader(testutil.Context(t, testutil.WaitShort)))
	cancel()
	_, err := cache.Acquire(ctx, dbM, fileID)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestCancelledConcurrentFetch runs 2 Acquire calls. The first has a canceled
// context and will get a ctx.Canceled error. The second call should get a warmfirst error and try to fetch the file
// again, which should succeed.
func TestCancelledConcurrentFetch(t *testing.T) {
	t.Parallel()

	fileID := uuid.New()
	dbM := dbmock.NewMockStore(gomock.NewController(t))

	// The file fetch should succeed.
	dbM.EXPECT().GetFileByID(gomock.Any(), gomock.Any()).DoAndReturn(func(mTx context.Context, fileID uuid.UUID) (database.File, error) {
		return database.File{
			ID:   fileID,
			Data: make([]byte, 100),
		}, nil
	})

	cache := files.LeakCache{Cache: files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})}

	ctx := dbauthz.AsFileReader(testutil.Context(t, testutil.WaitShort))

	// Cancel the context for the first call; should fail.
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, err := cache.Acquire(canceledCtx, dbM, fileID)
	require.ErrorIs(t, err, context.Canceled)

	// Second call, that should succeed without fetching from the database again
	// since the cache should be populated by the fetch the first request started
	// even if it doesn't wait for completion.
	_, err = cache.Acquire(ctx, dbM, fileID)
	require.NoError(t, err)
}

func TestConcurrentFetch(t *testing.T) {
	t.Parallel()

	fileID := uuid.New()

	// Only allow one call, which should succeed
	dbM := dbmock.NewMockStore(gomock.NewController(t))
	dbM.EXPECT().GetFileByID(gomock.Any(), gomock.Any()).DoAndReturn(func(mTx context.Context, fileID uuid.UUID) (database.File, error) {
		return database.File{ID: fileID}, nil
	})

	cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
	ctx := dbauthz.AsFileReader(testutil.Context(t, testutil.WaitShort))

	// Expect 2 calls to Acquire before we continue the test
	var wg sync.WaitGroup

	wg.Add(2)
	for range 2 {
		// TODO: wg.Go in Go 1.25
		go func() {
			defer wg.Done()
			_, err := cache.Acquire(ctx, dbM, fileID)
			assert.NoError(t, err)
		}()
	}

	// Wait for both go routines to assert their errors and finish.
	wg.Wait()
	require.Equal(t, 1, cache.Count())
}

// nolint:paralleltest,tparallel // Serially testing is easier
func TestCacheRBAC(t *testing.T) {
	t.Parallel()

	db, cache, rec := cacheAuthzSetup(t)
	ctx := testutil.Context(t, testutil.WaitMedium)

	file := dbgen.File(t, db, database.File{})

	nobodyID := uuid.New()
	nobody := dbauthz.As(ctx, rbac.Subject{
		ID:    nobodyID.String(),
		Roles: rbac.Roles{},
		Scope: rbac.ScopeAll,
	})

	userID := uuid.New()
	userReader := dbauthz.As(ctx, rbac.Subject{
		ID: userID.String(),
		Roles: rbac.Roles{
			must(rbac.RoleByName(rbac.RoleTemplateAdmin())),
		},
		Scope: rbac.ScopeAll,
	})

	cacheReader := dbauthz.AsFileReader(ctx)

	t.Run("NoRolesOpen", func(t *testing.T) {
		// Ensure start is clean
		require.Equal(t, 0, cache.Count())
		rec.Reset()

		_, err := cache.Acquire(nobody, db, file.ID)
		require.Error(t, err)
		require.True(t, rbac.IsUnauthorizedError(err))

		// Ensure that the cache is empty
		require.Equal(t, 0, cache.Count())

		// Check the assertions
		rec.AssertActorID(t, nobodyID.String(), rec.Pair(policy.ActionRead, file))
		rec.AssertActorID(t, rbac.SubjectTypeFileReaderID, rec.Pair(policy.ActionRead, file))
	})

	t.Run("CacheHasFile", func(t *testing.T) {
		rec.Reset()
		require.Equal(t, 0, cache.Count())

		// Read the file with a file reader to put it into the cache.
		a, err := cache.Acquire(cacheReader, db, file.ID)
		require.NoError(t, err)
		require.Equal(t, 1, cache.Count())

		// "nobody" should not be able to read the file.
		_, err = cache.Acquire(nobody, db, file.ID)
		require.Error(t, err)
		require.True(t, rbac.IsUnauthorizedError(err))
		require.Equal(t, 1, cache.Count())

		// UserReader can
		b, err := cache.Acquire(userReader, db, file.ID)
		require.NoError(t, err)
		require.Equal(t, 1, cache.Count())

		a.Close()
		b.Close()
		require.Equal(t, 0, cache.Count())

		rec.AssertActorID(t, nobodyID.String(), rec.Pair(policy.ActionRead, file))
		rec.AssertActorID(t, rbac.SubjectTypeFileReaderID, rec.Pair(policy.ActionRead, file))
		rec.AssertActorID(t, userID.String(), rec.Pair(policy.ActionRead, file))
	})
}

func cachePromMetricName(metric string) string {
	return "coderd_file_cache_" + metric
}

func TestConcurrency(t *testing.T) {
	t.Parallel()
	ctx := dbauthz.AsFileReader(t.Context())

	const fileSize = 10
	var fetches atomic.Int64
	reg := prometheus.NewRegistry()

	dbM := dbmock.NewMockStore(gomock.NewController(t))
	dbM.EXPECT().GetFileByID(gomock.Any(), gomock.Any()).DoAndReturn(func(mTx context.Context, fileID uuid.UUID) (database.File, error) {
		fetches.Add(1)
		// Wait long enough before returning to make sure that all the goroutines
		// will be waiting in line, ensuring that no one duplicated a fetch.
		time.Sleep(testutil.IntervalMedium)
		return database.File{
			Data: make([]byte, fileSize),
		}, nil
	}).AnyTimes()

	c := files.New(reg, &coderdtest.FakeAuthorizer{})

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
				_, err := c.Acquire(ctx, dbM, id)
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
	hit, miss := promhelp.CounterValue(t, reg, cachePromMetricName("open_file_refs_total"), prometheus.Labels{"hit": "false"}),
		promhelp.CounterValue(t, reg, cachePromMetricName("open_file_refs_total"), prometheus.Labels{"hit": "true"})
	require.Equal(t, batches*batchSize, hit+miss)
}

func TestRelease(t *testing.T) {
	t.Parallel()
	ctx := dbauthz.AsFileReader(t.Context())

	const fileSize = 10
	reg := prometheus.NewRegistry()
	dbM := dbmock.NewMockStore(gomock.NewController(t))
	dbM.EXPECT().GetFileByID(gomock.Any(), gomock.Any()).DoAndReturn(func(mTx context.Context, fileID uuid.UUID) (database.File, error) {
		return database.File{
			Data: make([]byte, fileSize),
		}, nil
	}).AnyTimes()

	c := files.New(reg, &coderdtest.FakeAuthorizer{})

	batches := 100
	ids := make([]uuid.UUID, 0, batches)
	for range batches {
		ids = append(ids, uuid.New())
	}

	releases := make(map[uuid.UUID][]func(), 0)
	// Acquire a bunch of references
	batchSize := 10
	for openedIdx, id := range ids {
		for batchIdx := range batchSize {
			it, err := c.Acquire(ctx, dbM, id)
			require.NoError(t, err)
			releases[id] = append(releases[id], it.Close)

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
	require.Equal(t, c.Count(), batches)

	// Now release all of the references
	for closedIdx, id := range ids {
		stillOpen := len(ids) - closedIdx
		for closingIdx := range batchSize {
			releases[id][0]()
			releases[id] = releases[id][1:]

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
	require.Equal(t, c.Count(), 0)

	// Verify all the counts & metrics are correct.
	// All existing files are closed
	require.Equal(t, 0, c.Count())
	require.Equal(t, 0, promhelp.GaugeValue(t, reg, cachePromMetricName("open_files_size_bytes_current"), nil))
	require.Equal(t, 0, promhelp.GaugeValue(t, reg, cachePromMetricName("open_files_current"), nil))
	require.Equal(t, 0, promhelp.GaugeValue(t, reg, cachePromMetricName("open_file_refs_current"), nil))

	// Total counts remain
	require.Equal(t, batches*fileSize, promhelp.CounterValue(t, reg, cachePromMetricName("open_files_size_bytes_total"), nil))
	require.Equal(t, batches, promhelp.CounterValue(t, reg, cachePromMetricName("open_files_total"), nil))
}

func cacheAuthzSetup(t *testing.T) (database.Store, *files.Cache, *coderdtest.RecordingAuthorizer) {
	t.Helper()

	logger := slogtest.Make(t, &slogtest.Options{})
	reg := prometheus.NewRegistry()

	db, _ := dbtestutil.NewDB(t)
	authz := rbac.NewAuthorizer(reg)
	rec := &coderdtest.RecordingAuthorizer{
		Called:  nil,
		Wrapped: authz,
	}

	// Dbauthz wrap the db
	db = dbauthz.New(db, rec, logger, coderdtest.AccessControlStorePointer())
	c := files.New(reg, rec)
	return db, c, rec
}

func must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}
	return t
}

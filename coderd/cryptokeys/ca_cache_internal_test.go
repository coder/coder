package cryptokeys

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestNATSCACache(t *testing.T) {
	t.Parallel()

	t.Run("RefreshesOnRotation", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		clock := quartz.NewMock(t)
		now := dbtime.Now()
		clock.Set(now)

		first := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureNATSCA,
			Sequence: 1,
			StartsAt: now.Add(-time.Hour),
		})

		cache, err := NewNATSCACache(ctx, testutil.Logger(t), db, WithNATSCACacheClock(clock))
		require.NoError(t, err)
		defer cache.Close()

		ca, err := cache.CA(ctx)
		require.NoError(t, err)
		require.Equal(t, first.Sequence, ca.Sequence)

		// Simulate a rotation by inserting a higher-sequence active CA.
		second := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureNATSCA,
			Sequence: 2,
			StartsAt: now.Add(-time.Minute),
		})

		// The cache still serves the old CA until the next refresh.
		ca, err = cache.CA(ctx)
		require.NoError(t, err)
		require.Equal(t, first.Sequence, ca.Sequence)

		// Fire the background refresher.
		clock.Advance(refreshInterval).MustWait(ctx)

		ca, err = cache.CA(ctx)
		require.NoError(t, err)
		require.Equal(t, second.Sequence, ca.Sequence)
	})

	t.Run("FetchesOnDemandAfterEmptyStart", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		now := dbtime.Now()

		// The cache is constructed against an empty DB, so the initial fetch
		// returns ErrNATSCANotFound and leaves the cache unpopulated.
		cache, err := NewNATSCACache(ctx, testutil.Logger(t), db)
		require.NoError(t, err)
		defer cache.Close()

		// Until a CA exists, CA surfaces the transient not-found condition.
		_, err = cache.CA(ctx)
		require.ErrorIs(t, err, ErrNATSCANotFound)

		// Once the rotator mints the CA, the on-demand fetch path populates the
		// cache without waiting for the periodic refresh.
		key := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureNATSCA,
			Sequence: 1,
			StartsAt: now.Add(-time.Hour),
		})

		ca, err := cache.CA(ctx)
		require.NoError(t, err)
		require.Equal(t, key.Sequence, ca.Sequence)
	})

	t.Run("CloseStops", func(t *testing.T) {
		t.Parallel()

		db, _ := dbtestutil.NewDB(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		cache, err := NewNATSCACache(ctx, testutil.Logger(t), db)
		require.NoError(t, err)
		require.NoError(t, cache.Close())

		_, err = cache.CA(ctx)
		require.ErrorIs(t, err, ErrClosed)
	})
}

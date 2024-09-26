package cryptokeys_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestDBKeyCache(t *testing.T) {
	t.Parallel()

	t.Run("NoKeys", func(t *testing.T) {
		t.Parallel()

		var (
			db, _  = dbtestutil.NewDB(t)
			clock  = quartz.NewMock(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = slogtest.Make(t, nil)
		)

		_, err := cryptokeys.NewDBCache(ctx, logger, db, database.CryptoKeyFeatureWorkspaceApps, cryptokeys.WithDBCacheClock(clock))
		require.NoError(t, err)
	})

	t.Run("Version", func(t *testing.T) {
		t.Parallel()

		t.Run("HitsCache", func(t *testing.T) {
			t.Parallel()

			var (
				db, _  = dbtestutil.NewDB(t)
				clock  = quartz.NewMock(t)
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = slogtest.Make(t, nil)
			)

			key := dbgen.CryptoKey(t, db, database.CryptoKey{
				Feature:  database.CryptoKeyFeatureWorkspaceApps,
				Sequence: 1,
				Secret: sql.NullString{
					String: "secret",
					Valid:  true,
				},
				StartsAt: clock.Now().UTC(),
			})

			k, err := cryptokeys.NewDBCache(ctx, logger, db, database.CryptoKeyFeatureWorkspaceApps, cryptokeys.WithDBCacheClock(clock))
			require.NoError(t, err)

			got, err := k.Version(ctx, key.Sequence)
			require.NoError(t, err)
			require.Equal(t, db2sdk.CryptoKey(key), got)
		})

		t.Run("MissesCache", func(t *testing.T) {
			t.Parallel()

			var (
				db, _  = dbtestutil.NewDB(t)
				clock  = quartz.NewMock(t)
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = slogtest.Make(t, nil)
			)

			_ = dbgen.CryptoKey(t, db, database.CryptoKey{
				Feature:  database.CryptoKeyFeatureWorkspaceApps,
				Sequence: 1,
				Secret: sql.NullString{
					String: "secret",
					Valid:  true,
				},
				StartsAt: clock.Now().UTC(),
			})

			k, err := cryptokeys.NewDBCache(ctx, logger, db, database.CryptoKeyFeatureWorkspaceApps, cryptokeys.WithDBCacheClock(clock))
			require.NoError(t, err)

			key := dbgen.CryptoKey(t, db, database.CryptoKey{
				Feature:  database.CryptoKeyFeatureWorkspaceApps,
				Sequence: 3,
				Secret: sql.NullString{
					String: "secret",
					Valid:  true,
				},
				StartsAt: clock.Now().UTC(),
			})

			got, err := k.Version(ctx, key.Sequence)
			require.NoError(t, err)
			require.Equal(t, db2sdk.CryptoKey(key), got)
		})
	})

	t.Run("Latest", func(t *testing.T) {
		t.Parallel()

		var (
			db, _  = dbtestutil.NewDB(t)
			clock  = quartz.NewMock(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = slogtest.Make(t, nil)
		)

		_ = dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 10,
			StartsAt: clock.Now().UTC(),
		})

		expectedKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 12,
			StartsAt: clock.Now().UTC(),
		})

		_ = dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 2,
			StartsAt: clock.Now().UTC(),
		})

		k, err := cryptokeys.NewDBCache(ctx, logger, db, database.CryptoKeyFeatureWorkspaceApps, cryptokeys.WithDBCacheClock(clock))
		require.NoError(t, err)

		got, err := k.Latest(ctx)
		require.NoError(t, err)
		require.Equal(t, db2sdk.CryptoKey(expectedKey), got)
	})

	t.Run("CacheRefreshes", func(t *testing.T) {
		t.Parallel()

		var (
			db, _  = dbtestutil.NewDB(t)
			clock  = quartz.NewMock(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = slogtest.Make(t, nil)
		)

		expiringKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 12,
			StartsAt: clock.Now().UTC(),
			DeletesAt: sql.NullTime{
				Time:  clock.Now().UTC().Add(time.Minute * 10),
				Valid: true,
			},
		})
		latest := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 24,
			StartsAt: clock.Now().UTC(),
			DeletesAt: sql.NullTime{
				Time:  clock.Now().UTC().Add(2 * 2 * time.Hour),
				Valid: true,
			},
		})

		wrongFeature := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureOidcConvert,
			Sequence: 30,
			StartsAt: clock.Now().UTC(),
		})

		trap := clock.Trap().TickerFunc()
		k, err := cryptokeys.NewDBCache(ctx, logger, db, database.CryptoKeyFeatureWorkspaceApps, cryptokeys.WithDBCacheClock(clock))
		require.NoError(t, err)

		// Should be able to fetch the expiring key since it's still valid.
		got, err := k.Version(ctx, expiringKey.Sequence)
		require.NoError(t, err)
		require.Equal(t, db2sdk.CryptoKey(expiringKey), got)

		_, err = k.Version(ctx, wrongFeature.Sequence)
		require.ErrorIs(t, err, cryptokeys.ErrKeyNotFound)

		newLatest := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 25,
			StartsAt: clock.Now().UTC(),
			DeletesAt: sql.NullTime{
				Time:  clock.Now().UTC().Add(2 * 2 * time.Hour),
				Valid: true,
			},
		})

		// The latest key should not be the one we just generated.
		got, err = k.Latest(ctx)
		require.NoError(t, err)
		require.Equal(t, db2sdk.CryptoKey(latest), got)

		// Wait for the ticker to fire and the cache to refresh.
		trap.MustWait(ctx).Release()
		_, wait := clock.AdvanceNext()
		wait.MustWait(ctx)

		// The latest key should be the one we just generated.
		got, err = k.Latest(ctx)
		require.NoError(t, err)
		require.Equal(t, db2sdk.CryptoKey(newLatest), got)

		// The expiring key should be invalid.
		_, err = k.Version(ctx, expiringKey.Sequence)
		require.ErrorIs(t, err, cryptokeys.ErrKeyInvalid)

		// Sanity check that the wrong feature is still not found.
		_, err = k.Version(ctx, wrongFeature.Sequence)
		require.ErrorIs(t, err, cryptokeys.ErrKeyNotFound)
	})
}

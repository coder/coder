package cryptokeys_test

import (
	"testing"

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

	t.Run("Verifying", func(t *testing.T) {
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
				StartsAt: clock.Now().UTC(),
			})

			k := cryptokeys.NewDBCache(ctx, logger, db, database.CryptoKeyFeatureWorkspaceApps, cryptokeys.WithDBCacheClock(clock))

			got, err := k.Verifying(ctx, key.Sequence)
			require.NoError(t, err)
			require.Equal(t, db2sdk.CryptoKey(key), got)
		})

		t.Run("NotFound", func(t *testing.T) {
			t.Parallel()

			var (
				db, _  = dbtestutil.NewDB(t)
				clock  = quartz.NewMock(t)
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = slogtest.Make(t, nil)
			)

			k := cryptokeys.NewDBCache(ctx, logger, db, database.CryptoKeyFeatureWorkspaceApps, cryptokeys.WithDBCacheClock(clock))

			_, err := k.Verifying(ctx, 123)
			require.ErrorIs(t, err, cryptokeys.ErrKeyNotFound)
		})
	})

	t.Run("Signing", func(t *testing.T) {
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

		k := cryptokeys.NewDBCache(ctx, logger, db, database.CryptoKeyFeatureWorkspaceApps, cryptokeys.WithDBCacheClock(clock))

		got, err := k.Signing(ctx)
		require.NoError(t, err)
		require.Equal(t, db2sdk.CryptoKey(expectedKey), got)
	})
}

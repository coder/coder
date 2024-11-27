package cryptokeys_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestRotator(t *testing.T) {
	t.Parallel()

	t.Run("NoKeysOnInit", func(t *testing.T) {
		t.Parallel()

		var (
			db, _  = dbtestutil.NewDB(t)
			clock  = quartz.NewMock(t)
			logger = testutil.Logger(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
		)

		dbkeys, err := db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, dbkeys, 0)

		cryptokeys.StartRotator(ctx, logger, db, cryptokeys.WithClock(clock))

		// Fetch the keys from the database and ensure they
		// are as expected.
		dbkeys, err = db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, dbkeys, len(database.AllCryptoKeyFeatureValues()))
		requireContainsAllFeatures(t, dbkeys)
	})

	t.Run("RotateKeys", func(t *testing.T) {
		t.Parallel()

		var (
			db, _  = dbtestutil.NewDB(t)
			clock  = quartz.NewMock(t)
			logger = testutil.Logger(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
		)

		now := clock.Now().UTC()

		rotatingKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			StartsAt: now.Add(-cryptokeys.DefaultKeyDuration + time.Hour + time.Minute),
			Sequence: 12345,
		})

		trap := clock.Trap().TickerFunc()
		t.Cleanup(trap.Close)

		cryptokeys.StartRotator(ctx, logger, db, cryptokeys.WithClock(clock))

		initialKeyLen := len(database.AllCryptoKeyFeatureValues())
		// Fetch the keys from the database and ensure they
		// are as expected.
		dbkeys, err := db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, dbkeys, initialKeyLen)
		requireContainsAllFeatures(t, dbkeys)

		trap.MustWait(ctx).Release()
		_, wait := clock.AdvanceNext()
		wait.MustWait(ctx)

		keys, err := db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, initialKeyLen+1)

		newKey, err := db.GetLatestCryptoKeyByFeature(ctx, database.CryptoKeyFeatureWorkspaceAppsAPIKey)
		require.NoError(t, err)
		require.Equal(t, rotatingKey.Sequence+1, newKey.Sequence)
		require.Equal(t, rotatingKey.ExpiresAt(cryptokeys.DefaultKeyDuration), newKey.StartsAt.UTC())
		require.False(t, newKey.DeletesAt.Valid)

		oldKey, err := db.GetCryptoKeyByFeatureAndSequence(ctx, database.GetCryptoKeyByFeatureAndSequenceParams{
			Feature:  rotatingKey.Feature,
			Sequence: rotatingKey.Sequence,
		})
		expectedDeletesAt := rotatingKey.StartsAt.Add(cryptokeys.DefaultKeyDuration + time.Hour + cryptokeys.WorkspaceAppsTokenDuration)
		require.NoError(t, err)
		require.Equal(t, rotatingKey.StartsAt, oldKey.StartsAt)
		require.True(t, oldKey.DeletesAt.Valid)
		require.Equal(t, expectedDeletesAt, oldKey.DeletesAt.Time)

		// Try rotating again and ensure no keys are rotated.
		_, wait = clock.AdvanceNext()
		wait.MustWait(ctx)

		keys, err = db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, initialKeyLen+1)
	})
}

func requireContainsAllFeatures(t *testing.T, keys []database.CryptoKey) {
	t.Helper()

	features := make(map[database.CryptoKeyFeature]bool)
	for _, key := range keys {
		features[key.Feature] = true
	}
	for _, feature := range database.AllCryptoKeyFeatureValues() {
		require.True(t, features[feature])
	}
}

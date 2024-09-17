package keyrotate_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/keyrotate"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestKeyRotator(t *testing.T) {
	t.Parallel()

	t.Run("NoKeysOnInit", func(t *testing.T) {
		t.Parallel()

		var (
			db, _  = dbtestutil.NewDB(t)
			clock  = quartz.NewMock(t)
			logger = slogtest.Make(t, nil).Leveled(slog.LevelDebug)
			ctx    = testutil.Context(t, testutil.WaitShort)
		)

		dbkeys, err := db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, dbkeys, 0)

		kr, err := keyrotate.Open(ctx, db, logger, clock, keyrotate.DefaultKeyDuration, keyrotate.DefaultRotationInterval, nil)
		require.NoError(t, err)
		t.Cleanup(kr.Close)

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
			logger = slogtest.Make(t, nil).Leveled(slog.LevelDebug)
			ctx    = testutil.Context(t, testutil.WaitShort)
		)

		now := clock.Now().UTC()

		rotatingKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			StartsAt: now.Add(-keyrotate.DefaultKeyDuration + time.Hour + time.Minute),
			Sequence: 12345,
		})
		resultsCh := make(chan []database.CryptoKey)

		kr, err := keyrotate.Open(ctx, db, logger, clock, keyrotate.DefaultKeyDuration, keyrotate.DefaultRotationInterval, resultsCh)
		require.NoError(t, err)
		t.Cleanup(kr.Close)

		// Fetch the keys from the database and ensure they
		// are as expected.
		dbkeys, err := db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, dbkeys, len(database.AllCryptoKeyFeatureValues()))
		requireContainsAllFeatures(t, dbkeys)

		go kr.Start(ctx)

		_, wait := clock.AdvanceNext()
		wait.MustWait(ctx)
		results := <-resultsCh
		require.Len(t, results, 2)

		keys, err := db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 4)

		newKey, err := db.GetLatestCryptoKeyByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps)
		require.NoError(t, err)
		require.Equal(t, rotatingKey.Sequence+1, newKey.Sequence)
		require.Equal(t, rotatingKey.ExpiresAt(keyrotate.DefaultKeyDuration), newKey.StartsAt.UTC())
		require.False(t, newKey.DeletesAt.Valid)

		oldKey, err := db.GetCryptoKeyByFeatureAndSequence(ctx, database.GetCryptoKeyByFeatureAndSequenceParams{
			Feature:  rotatingKey.Feature,
			Sequence: rotatingKey.Sequence,
		})
		expectedDeletesAt := rotatingKey.StartsAt.Add(keyrotate.DefaultKeyDuration + time.Hour + keyrotate.WorkspaceAppsTokenDuration)
		require.NoError(t, err)
		require.Equal(t, rotatingKey.StartsAt, oldKey.StartsAt)
		require.True(t, oldKey.DeletesAt.Valid)
		require.Equal(t, expectedDeletesAt, oldKey.DeletesAt.Time)

		// Try rotating again and ensure no keys are rotated.
		_, wait = clock.AdvanceNext()
		wait.MustWait(ctx)
		results = <-resultsCh
		require.Len(t, results, 0)
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

package keyrotate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

func TestKeyRotator(t *testing.T) {
	t.Run("NoExistingKeys", func(t *testing.T) {
		// t.Parallel()

		// var (
		// 	db, _     = dbtestutil.NewDB(t)
		// 	clock     = quartz.NewMock(t)
		// 	logger    = slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		// 	ctx       = testutil.Context(t, testutil.WaitShort)
		// 	resultsCh = make(chan []database.CryptoKey, 1)
		// )

		// kr := &KeyRotator{
		// 	DB:           db,
		// 	KeyDuration:  0,
		// 	Clock:        clock,
		// 	Logger:       logger,
		// 	ScanInterval: 0,
		// 	ResultsCh:    resultsCh,
		// }

		// now := dbnow(clock)
		// keys, err := kr.rotateKeys(ctx)
		// require.NoError(t, err)
		// require.Len(t, keys, len(database.AllCryptoKeyFeatureValues()))

		// // Fetch the keys from the database and ensure they
		// // are as expected.
		// dbkeys, err := db.GetCryptoKeys(ctx)
		// require.NoError(t, err)
		// require.Equal(t, keys, dbkeys)
		// requireContainsAllFeatures(t, keys)
		// for _, key := range keys {
		// 	requireKey(t, key, key.Feature, now, time.Time{}, 1)
		// }
	})
}

func requireContainsAllFeatures(t *testing.T, keys []database.CryptoKey) {
	t.Helper()

	features := make(map[database.CryptoKeyFeature]bool)
	for _, key := range keys {
		features[key.Feature] = true
	}
	require.True(t, features[database.CryptoKeyFeatureOidcConvert])
	require.True(t, features[database.CryptoKeyFeatureWorkspaceApps])
	require.True(t, features[database.CryptoKeyFeatureTailnetResume])
}

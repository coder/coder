package keyrotate

import (
	"database/sql"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func Test_rotateKeys(t *testing.T) {
	t.Parallel()

	t.Run("RotatesKeysNearExpiration", func(t *testing.T) {
		t.Parallel()

		var (
			db, _       = dbtestutil.NewDB(t)
			clock       = quartz.NewMock(t)
			keyDuration = time.Hour * 24 * 7
			logger      = slogtest.Make(t, nil).Leveled(slog.LevelDebug)
			ctx         = testutil.Context(t, testutil.WaitShort)
			resultsCh   = make(chan []database.CryptoKey, 1)
		)

		kr := &KeyRotator{
			DB:           db,
			KeyDuration:  keyDuration,
			Clock:        clock,
			Logger:       logger,
			ScanInterval: 0,
			ResultsCh:    resultsCh,
			features: []database.CryptoKeyFeature{
				database.CryptoKeyFeatureWorkspaceApps,
			},
		}

		now := dbnow(clock)

		// Seed the database with an existing key.
		oldKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			StartsAt: now,
			Sequence: 15,
		})

		// Advance the window to just inside rotation time.
		_ = clock.Advance(keyDuration - time.Minute*59)
		keys, err := kr.rotateKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 2)

		now = dbnow(clock)
		expectedDeletesAt := oldKey.ExpiresAt(keyDuration).Add(WorkspaceAppsTokenDuration + time.Hour)

		// Fetch the old key, it should have an expires_at now.
		oldKey, err = db.GetCryptoKeyByFeatureAndSequence(ctx, database.GetCryptoKeyByFeatureAndSequenceParams{
			Feature:  oldKey.Feature,
			Sequence: oldKey.Sequence,
		})
		require.NoError(t, err)
		require.Equal(t, oldKey.DeletesAt.Time.UTC(), expectedDeletesAt)

		// The new key should be created and have a starts_at of the old key's expires_at.
		newKey, err := db.GetCryptoKeyByFeatureAndSequence(ctx, database.GetCryptoKeyByFeatureAndSequenceParams{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: oldKey.Sequence + 1,
		})
		require.NoError(t, err)
		requireKey(t, newKey, database.CryptoKeyFeatureWorkspaceApps, oldKey.ExpiresAt(keyDuration), time.Time{}, oldKey.Sequence+1)

		// Advance the clock just before the keys delete time.
		clock.Advance(oldKey.DeletesAt.Time.UTC().Sub(now) - time.Second)

		// No action should be taken.
		keys, err = kr.rotateKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 0)

		// Advance the clock just past the keys delete time.
		clock.Advance(oldKey.DeletesAt.Time.UTC().Sub(now) + time.Second)

		// We should have deleted the old key.
		keys, err = kr.rotateKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 1)

		// The old key should be "deleted".
		_, err = db.GetCryptoKeyByFeatureAndSequence(ctx, database.GetCryptoKeyByFeatureAndSequenceParams{
			Feature:  oldKey.Feature,
			Sequence: oldKey.Sequence,
		})
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("DoesNotRotateValidKeys", func(t *testing.T) {
		t.Parallel()

		var (
			db, _       = dbtestutil.NewDB(t)
			clock       = quartz.NewMock(t)
			keyDuration = time.Hour * 24 * 7
			logger      = slogtest.Make(t, nil).Leveled(slog.LevelDebug)
			ctx         = testutil.Context(t, testutil.WaitShort)
			resultsCh   = make(chan []database.CryptoKey, 1)
		)

		kr := &KeyRotator{
			DB:           db,
			KeyDuration:  keyDuration,
			Clock:        clock,
			Logger:       logger,
			ScanInterval: 0,
			ResultsCh:    resultsCh,
			features: []database.CryptoKeyFeature{
				database.CryptoKeyFeatureWorkspaceApps,
			},
		}

		now := dbnow(clock)

		// Seed the database with an existing key
		existingKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			StartsAt: now,
			Sequence: 1,
		})

		// Advance the clock by 6 days, 23 hours. Once we
		// breach the last hour we will insert a new key.
		clock.Advance(keyDuration - time.Hour)

		keys, err := kr.rotateKeys(ctx)
		require.NoError(t, err)
		require.Empty(t, keys)

		// Verify that the existing key is still the only key in the database
		dbKeys, err := db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, dbKeys, 1)
		requireKey(t, dbKeys[0], existingKey.Feature, existingKey.StartsAt.UTC(), existingKey.DeletesAt.Time.UTC(), existingKey.Sequence)
	})

	// Simulate a situation where the database was manually altered such that we only have a key that is scheduled to be deleted and assert we insert a new key.
	t.Run("DeletesExpiredKeys", func(t *testing.T) {
		t.Parallel()

		var (
			db, _       = dbtestutil.NewDB(t)
			clock       = quartz.NewMock(t)
			keyDuration = time.Hour * 24 * 7
			logger      = slogtest.Make(t, nil).Leveled(slog.LevelDebug)
			ctx         = testutil.Context(t, testutil.WaitShort)
			resultsCh   = make(chan []database.CryptoKey, 1)
		)

		kr := &KeyRotator{
			DB:           db,
			KeyDuration:  keyDuration,
			Clock:        clock,
			Logger:       logger,
			ScanInterval: 0,
			ResultsCh:    resultsCh,
			features: []database.CryptoKeyFeature{
				database.CryptoKeyFeatureWorkspaceApps,
			},
		}

		now := dbnow(clock)

		// Seed the database with an existing key
		deletingKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			StartsAt: now.Add(-keyDuration),
			Sequence: 1,
			DeletesAt: sql.NullTime{
				Time:  now,
				Valid: true,
			},
		})

		keys, err := kr.rotateKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 2)

		// We should only get one key since the old key
		// should be deleted.
		dbKeys, err := db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, dbKeys, 1)
		requireKey(t, dbKeys[0], deletingKey.Feature, deletingKey.DeletesAt.Time.UTC(), time.Time{}, deletingKey.Sequence+1)
		// The old key should be "deleted".
		_, err = db.GetCryptoKeyByFeatureAndSequence(ctx, database.GetCryptoKeyByFeatureAndSequenceParams{
			Feature:  deletingKey.Feature,
			Sequence: deletingKey.Sequence,
		})
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	// This tests a situation where we have a key scheduled for deletion but it's still valid for use.
	// If no other key is detected we should insert a new key.
	t.Run("AddsKeyForDeletingKey", func(t *testing.T) {
		t.Parallel()

		var (
			db, _       = dbtestutil.NewDB(t)
			clock       = quartz.NewMock(t)
			keyDuration = time.Hour * 24 * 7
			logger      = slogtest.Make(t, nil).Leveled(slog.LevelDebug)
			ctx         = testutil.Context(t, testutil.WaitShort)
			resultsCh   = make(chan []database.CryptoKey, 1)
		)

		kr := &KeyRotator{
			DB:           db,
			KeyDuration:  keyDuration,
			Clock:        clock,
			Logger:       logger,
			ScanInterval: 0,
			ResultsCh:    resultsCh,
			features: []database.CryptoKeyFeature{
				database.CryptoKeyFeatureWorkspaceApps,
			},
		}

		now := dbnow(clock)

		// Seed the database with an existing key
		deletingKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			StartsAt: now,
			Sequence: 1,
			DeletesAt: sql.NullTime{
				Time:  now.Add(time.Hour),
				Valid: true,
			},
		})

		clock.Advance(time.Minute * 59)

		// We should only have inserted a key.
		keys, err := kr.rotateKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		expectedKeys := append(keys, deletingKey)

		dbKeys, err := db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, dbKeys, 2)
		for _, expectedKey := range expectedKeys {
			var found bool
			for _, dbKey := range dbKeys {
				if dbKey.Sequence == expectedKey.Sequence {
					requireKey(t, dbKey, expectedKey.Feature, expectedKey.StartsAt.UTC(), expectedKey.DeletesAt.Time.UTC(), expectedKey.Sequence)
					found = true
				}
			}
			require.True(t, found, "expected key %+v not found", expectedKey)
		}
	})

	t.Run("NoKeys", func(t *testing.T) {
		t.Parallel()

		var (
			db, _       = dbtestutil.NewDB(t)
			clock       = quartz.NewMock(t)
			keyDuration = time.Hour * 24 * 7
			logger      = slogtest.Make(t, nil).Leveled(slog.LevelDebug)
			ctx         = testutil.Context(t, testutil.WaitShort)
			resultsCh   = make(chan []database.CryptoKey, 1)
		)

		kr := &KeyRotator{
			DB:           db,
			KeyDuration:  keyDuration,
			Clock:        clock,
			Logger:       logger,
			ScanInterval: 0,
			ResultsCh:    resultsCh,
			features: []database.CryptoKeyFeature{
				database.CryptoKeyFeatureWorkspaceApps,
			},
		}

		now := dbnow(clock)

		keys, err := kr.rotateKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		requireKey(t, keys[0], database.CryptoKeyFeatureWorkspaceApps, now, time.Time{}, 1)
	})

	// Assert we insert a new key when the only key is deleted.
	t.Run("OnlyDeletedKeys", func(t *testing.T) {
		t.Parallel()

		var (
			db, _       = dbtestutil.NewDB(t)
			clock       = quartz.NewMock(t)
			keyDuration = time.Hour * 24 * 7
			logger      = slogtest.Make(t, nil).Leveled(slog.LevelDebug)
			ctx         = testutil.Context(t, testutil.WaitShort)
			resultsCh   = make(chan []database.CryptoKey, 1)
		)

		kr := &KeyRotator{
			DB:           db,
			KeyDuration:  keyDuration,
			Clock:        clock,
			Logger:       logger,
			ScanInterval: 0,
			ResultsCh:    resultsCh,
			features: []database.CryptoKeyFeature{
				database.CryptoKeyFeatureWorkspaceApps,
			},
		}

		now := dbnow(clock)

		deletedkey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			StartsAt: now,
			Sequence: 19,
			DeletesAt: sql.NullTime{
				Time:  now.Add(time.Hour),
				Valid: true,
			},
			Secret: sql.NullString{
				String: "deleted",
				Valid:  false,
			},
		})

		keys, err := kr.rotateKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		requireKey(t, keys[0], database.CryptoKeyFeatureWorkspaceApps, now, time.Time{}, deletedkey.Sequence+1)
	})
}

func dbnow(c quartz.Clock) time.Time {
	return dbtime.Time(c.Now().UTC())
}

func requireKey(t *testing.T, key database.CryptoKey, feature database.CryptoKeyFeature, startsAt time.Time, deletesAt time.Time, sequence int32) {
	t.Helper()
	require.Equal(t, feature, key.Feature)
	require.Equal(t, startsAt, key.StartsAt.UTC())
	require.Equal(t, deletesAt, key.DeletesAt.Time.UTC())
	require.Equal(t, sequence, key.Sequence)

	secret, err := hex.DecodeString(key.Secret.String)
	require.NoError(t, err)

	switch key.Feature {
	case database.CryptoKeyFeatureOidcConvert:
		require.Len(t, secret, 32)
	case database.CryptoKeyFeatureWorkspaceApps:
		require.Len(t, secret, 96)
	case database.CryptoKeyFeatureTailnetResume:
		require.Len(t, secret, 64)
	default:
		t.Fatalf("unknown key feature: %s", key.Feature)
	}
}

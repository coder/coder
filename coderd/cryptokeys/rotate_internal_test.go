package cryptokeys

import (
	"database/sql"
	"encoding/hex"
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

func Test_rotateKeys(t *testing.T) {
	t.Parallel()

	t.Run("RotatesKeysNearExpiration", func(t *testing.T) {
		t.Parallel()

		var (
			db, _       = dbtestutil.NewDB(t)
			clock       = quartz.NewMock(t)
			keyDuration = time.Hour * 24 * 7
			logger      = testutil.Logger(t)
			ctx         = testutil.Context(t, testutil.WaitShort)
		)

		kr := &rotator{
			db:          db,
			keyDuration: keyDuration,
			clock:       clock,
			logger:      logger,
			features: []database.CryptoKeyFeature{
				database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			},
		}

		now := dbnow(clock)

		// Seed the database with an existing key.
		oldKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			StartsAt: now,
			Sequence: 15,
		})

		// Advance the window to just inside rotation time.
		_ = clock.Advance(keyDuration - time.Minute*59)
		err := kr.rotateKeys(ctx)
		require.NoError(t, err)

		now = dbnow(clock)
		expectedDeletesAt := oldKey.ExpiresAt(keyDuration).Add(WorkspaceAppsTokenDuration + time.Hour)

		// Fetch the old key, it should have an deletes_at now.
		oldKey, err = db.GetCryptoKeyByFeatureAndSequence(ctx, database.GetCryptoKeyByFeatureAndSequenceParams{
			Feature:  oldKey.Feature,
			Sequence: oldKey.Sequence,
		})
		require.NoError(t, err)
		require.Equal(t, oldKey.DeletesAt.Time.UTC(), expectedDeletesAt)

		// The new key should be created and have a starts_at of the old key's expires_at.
		newKey, err := db.GetCryptoKeyByFeatureAndSequence(ctx, database.GetCryptoKeyByFeatureAndSequenceParams{
			Feature:  database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			Sequence: oldKey.Sequence + 1,
		})
		require.NoError(t, err)
		requireKey(t, newKey, database.CryptoKeyFeatureWorkspaceAppsAPIKey, oldKey.ExpiresAt(keyDuration), nullTime, oldKey.Sequence+1)

		// Advance the clock just before the keys delete time.
		clock.Advance(oldKey.DeletesAt.Time.UTC().Sub(now) - time.Second)

		// No action should be taken.
		err = kr.rotateKeys(ctx)
		require.NoError(t, err)

		keys, err := db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 2)

		// Advance the clock just past the keys delete time.
		clock.Advance(oldKey.DeletesAt.Time.UTC().Sub(now) + time.Second)

		// We should have deleted the old key.
		err = kr.rotateKeys(ctx)
		require.NoError(t, err)

		// The old key should be "deleted".
		_, err = db.GetCryptoKeyByFeatureAndSequence(ctx, database.GetCryptoKeyByFeatureAndSequenceParams{
			Feature:  oldKey.Feature,
			Sequence: oldKey.Sequence,
		})
		require.ErrorIs(t, err, sql.ErrNoRows)

		keys, err = db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		require.Equal(t, newKey, keys[0])
	})

	t.Run("DoesNotRotateValidKeys", func(t *testing.T) {
		t.Parallel()

		var (
			db, _       = dbtestutil.NewDB(t)
			clock       = quartz.NewMock(t)
			keyDuration = time.Hour * 24 * 7
			logger      = testutil.Logger(t)
			ctx         = testutil.Context(t, testutil.WaitShort)
		)

		kr := &rotator{
			db:          db,
			keyDuration: keyDuration,
			clock:       clock,
			logger:      logger,
			features: []database.CryptoKeyFeature{
				database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			},
		}

		now := dbnow(clock)

		// Seed the database with an existing key
		existingKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			StartsAt: now,
			Sequence: 123,
		})

		// Advance the clock by 6 days, 22 hours. Once we
		// breach the last hour we will insert a new key.
		clock.Advance(keyDuration - 2*time.Hour)

		err := kr.rotateKeys(ctx)
		require.NoError(t, err)

		keys, err := db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		require.Equal(t, existingKey, keys[0])

		// Advance it again to just before the key is scheduled to be rotated for sanity purposes.
		clock.Advance(time.Hour - time.Second)

		err = kr.rotateKeys(ctx)
		require.NoError(t, err)

		// Verify that the existing key is still the only key in the database
		keys, err = db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		requireKey(t, keys[0], existingKey.Feature, existingKey.StartsAt.UTC(), nullTime, existingKey.Sequence)
	})

	// Simulate a situation where the database was manually altered such that we only have a key that is scheduled to be deleted and assert we insert a new key.
	t.Run("DeletesExpiredKeys", func(t *testing.T) {
		t.Parallel()

		var (
			db, _       = dbtestutil.NewDB(t)
			clock       = quartz.NewMock(t)
			keyDuration = time.Hour * 24 * 7
			logger      = testutil.Logger(t)
			ctx         = testutil.Context(t, testutil.WaitShort)
		)

		kr := &rotator{
			db:          db,
			keyDuration: keyDuration,
			clock:       clock,
			logger:      logger,
			features: []database.CryptoKeyFeature{
				database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			},
		}

		now := dbnow(clock)

		// Seed the database with an existing key
		deletingKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			StartsAt: now.Add(-keyDuration),
			Sequence: 789,
			DeletesAt: sql.NullTime{
				Time:  now,
				Valid: true,
			},
		})

		err := kr.rotateKeys(ctx)
		require.NoError(t, err)

		// We should only get one key since the old key
		// should be deleted.
		keys, err := db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		requireKey(t, keys[0], deletingKey.Feature, deletingKey.DeletesAt.Time.UTC(), nullTime, deletingKey.Sequence+1)
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
			logger      = testutil.Logger(t)
			ctx         = testutil.Context(t, testutil.WaitShort)
		)

		kr := &rotator{
			db:          db,
			keyDuration: keyDuration,
			clock:       clock,
			logger:      logger,
			features: []database.CryptoKeyFeature{
				database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			},
		}

		now := dbnow(clock)

		// Seed the database with an existing key
		deletingKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			StartsAt: now,
			Sequence: 456,
			DeletesAt: sql.NullTime{
				Time:  now.Add(time.Hour),
				Valid: true,
			},
		})

		// We should only have inserted a key.
		err := kr.rotateKeys(ctx)
		require.NoError(t, err)

		keys, err := db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 2)
		oldKey, newKey := keys[0], keys[1]
		if oldKey.Sequence != deletingKey.Sequence {
			oldKey, newKey = newKey, oldKey
		}
		requireKey(t, oldKey, deletingKey.Feature, deletingKey.StartsAt.UTC(), deletingKey.DeletesAt, deletingKey.Sequence)
		requireKey(t, newKey, deletingKey.Feature, now, nullTime, deletingKey.Sequence+1)
	})

	t.Run("NoKeys", func(t *testing.T) {
		t.Parallel()

		var (
			db, _       = dbtestutil.NewDB(t)
			clock       = quartz.NewMock(t)
			keyDuration = time.Hour * 24 * 7
			logger      = testutil.Logger(t)
			ctx         = testutil.Context(t, testutil.WaitShort)
		)

		kr := &rotator{
			db:          db,
			keyDuration: keyDuration,
			clock:       clock,
			logger:      logger,
			features: []database.CryptoKeyFeature{
				database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			},
		}

		err := kr.rotateKeys(ctx)
		require.NoError(t, err)

		keys, err := db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		requireKey(t, keys[0], database.CryptoKeyFeatureWorkspaceAppsAPIKey, clock.Now().UTC(), nullTime, 1)
	})

	// Assert we insert a new key when the only key was manually deleted.
	t.Run("OnlyDeletedKeys", func(t *testing.T) {
		t.Parallel()

		var (
			db, _       = dbtestutil.NewDB(t)
			clock       = quartz.NewMock(t)
			keyDuration = time.Hour * 24 * 7
			logger      = testutil.Logger(t)
			ctx         = testutil.Context(t, testutil.WaitShort)
		)

		kr := &rotator{
			db:          db,
			keyDuration: keyDuration,
			clock:       clock,
			logger:      logger,
			features: []database.CryptoKeyFeature{
				database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			},
		}

		now := dbnow(clock)

		deletedkey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceAppsAPIKey,
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

		err := kr.rotateKeys(ctx)
		require.NoError(t, err)

		keys, err := db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		requireKey(t, keys[0], database.CryptoKeyFeatureWorkspaceAppsAPIKey, now, nullTime, deletedkey.Sequence+1)
	})

	// This tests ensures that rotation works with multiple
	// features. It's mainly a sanity test since some bugs
	// are not unveiled in the simple n=1 case.
	t.Run("AllFeatures", func(t *testing.T) {
		t.Parallel()

		var (
			db, _       = dbtestutil.NewDB(t)
			clock       = quartz.NewMock(t)
			keyDuration = time.Hour * 24 * 30
			logger      = testutil.Logger(t)
			ctx         = testutil.Context(t, testutil.WaitShort)
		)

		kr := &rotator{
			db:          db,
			keyDuration: keyDuration,
			clock:       clock,
			logger:      logger,
			features:    database.AllCryptoKeyFeatureValues(),
		}

		now := dbnow(clock)

		// We'll test a scenario where:
		// - One feature has no valid keys.
		// - One has a key that should be rotated.
		// - One has a valid key that shouldn't trigger an action.
		// - One has no keys at all.
		_ = dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureTailnetResume,
			StartsAt: now.Add(-keyDuration),
			Sequence: 5,
			Secret: sql.NullString{
				String: "older key",
				Valid:  false,
			},
		})
		// Generate another deleted key to ensure we insert after the latest sequence.
		deletedKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureTailnetResume,
			StartsAt: now.Add(-keyDuration),
			Sequence: 19,
			Secret: sql.NullString{
				String: "old key",
				Valid:  false,
			},
		})

		// Insert a key that should be rotated.
		rotatedKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			StartsAt: now.Add(-keyDuration + time.Hour),
			Sequence: 42,
		})

		// Insert a key that should not trigger an action.
		validKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureOIDCConvert,
			StartsAt: now,
			Sequence: 17,
		})

		err := kr.rotateKeys(ctx)
		require.NoError(t, err)

		keys, err := db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 5)

		kbf, err := keysByFeature(keys, database.AllCryptoKeyFeatureValues())
		require.NoError(t, err)

		// No actions on OIDC convert.
		require.Len(t, kbf[database.CryptoKeyFeatureOIDCConvert], 1)
		// Workspace apps should have been rotated.
		require.Len(t, kbf[database.CryptoKeyFeatureWorkspaceAppsAPIKey], 2)
		// No existing key for tailnet resume should've
		// caused a key to be inserted.
		require.Len(t, kbf[database.CryptoKeyFeatureTailnetResume], 1)
		require.Len(t, kbf[database.CryptoKeyFeatureWorkspaceAppsToken], 1)

		oidcKey := kbf[database.CryptoKeyFeatureOIDCConvert][0]
		tailnetKey := kbf[database.CryptoKeyFeatureTailnetResume][0]
		appTokenKey := kbf[database.CryptoKeyFeatureWorkspaceAppsToken][0]
		requireKey(t, oidcKey, database.CryptoKeyFeatureOIDCConvert, now, nullTime, validKey.Sequence)
		requireKey(t, tailnetKey, database.CryptoKeyFeatureTailnetResume, now, nullTime, deletedKey.Sequence+1)
		requireKey(t, appTokenKey, database.CryptoKeyFeatureWorkspaceAppsToken, now, nullTime, 1)
		newKey := kbf[database.CryptoKeyFeatureWorkspaceAppsAPIKey][0]
		oldKey := kbf[database.CryptoKeyFeatureWorkspaceAppsAPIKey][1]
		if newKey.Sequence == rotatedKey.Sequence {
			oldKey, newKey = newKey, oldKey
		}
		deletesAt := sql.NullTime{
			Time:  rotatedKey.ExpiresAt(keyDuration).Add(WorkspaceAppsTokenDuration + time.Hour),
			Valid: true,
		}
		requireKey(t, oldKey, database.CryptoKeyFeatureWorkspaceAppsAPIKey, rotatedKey.StartsAt.UTC(), deletesAt, rotatedKey.Sequence)
		requireKey(t, newKey, database.CryptoKeyFeatureWorkspaceAppsAPIKey, rotatedKey.ExpiresAt(keyDuration), nullTime, rotatedKey.Sequence+1)
	})

	t.Run("UnknownFeature", func(t *testing.T) {
		t.Parallel()

		var (
			db, _       = dbtestutil.NewDB(t)
			clock       = quartz.NewMock(t)
			keyDuration = time.Hour * 24 * 7
			logger      = testutil.Logger(t)
			ctx         = testutil.Context(t, testutil.WaitShort)
		)

		kr := &rotator{
			db:          db,
			keyDuration: keyDuration,
			clock:       clock,
			logger:      logger,
			features:    []database.CryptoKeyFeature{database.CryptoKeyFeature("unknown")},
		}

		err := kr.rotateKeys(ctx)
		require.Error(t, err)
	})

	t.Run("MinStartsAt", func(t *testing.T) {
		t.Parallel()

		var (
			db, _       = dbtestutil.NewDB(t)
			clock       = quartz.NewMock(t)
			keyDuration = time.Hour * 24 * 5
			logger      = testutil.Logger(t)
			ctx         = testutil.Context(t, testutil.WaitShort)
		)

		now := dbnow(clock)

		kr := &rotator{
			db:          db,
			keyDuration: keyDuration,
			clock:       clock,
			logger:      logger,
			features:    []database.CryptoKeyFeature{database.CryptoKeyFeatureWorkspaceAppsAPIKey},
		}

		expiringKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			StartsAt: now.Add(-keyDuration),
			Sequence: 345,
		})

		err := kr.rotateKeys(ctx)
		require.NoError(t, err)

		keys, err := db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 2)

		rotatedKey, err := db.GetCryptoKeyByFeatureAndSequence(ctx, database.GetCryptoKeyByFeatureAndSequenceParams{
			Feature:  expiringKey.Feature,
			Sequence: expiringKey.Sequence + 1,
		})
		require.NoError(t, err)
		require.Equal(t, now.Add(defaultRotationInterval*3), rotatedKey.StartsAt.UTC())
	})

	// Test that the the deletes_at of a key that is well past its expiration
	// Has its deletes_at field set to value that is relative
	// to the current time to afford propagation time for the
	// new key.
	t.Run("ExtensivelyExpiredKey", func(t *testing.T) {
		t.Parallel()

		var (
			db, _       = dbtestutil.NewDB(t)
			clock       = quartz.NewMock(t)
			keyDuration = time.Hour * 24 * 3
			logger      = testutil.Logger(t)
			ctx         = testutil.Context(t, testutil.WaitShort)
		)

		kr := &rotator{
			db:          db,
			keyDuration: keyDuration,
			clock:       clock,
			logger:      logger,
			features:    []database.CryptoKeyFeature{database.CryptoKeyFeatureWorkspaceAppsAPIKey},
		}

		now := dbnow(clock)

		expiredKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			StartsAt: now.Add(-keyDuration - 2*time.Hour),
			Sequence: 19,
		})

		deletedKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceAppsAPIKey,
			StartsAt: now,
			Sequence: 20,
			Secret: sql.NullString{
				String: "deleted",
				Valid:  false,
			},
		})

		err := kr.rotateKeys(ctx)
		require.NoError(t, err)

		keys, err := db.GetCryptoKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 2)

		deletesAtKey, err := db.GetCryptoKeyByFeatureAndSequence(ctx, database.GetCryptoKeyByFeatureAndSequenceParams{
			Feature:  expiredKey.Feature,
			Sequence: expiredKey.Sequence,
		})

		deletesAt := sql.NullTime{
			Time:  now.Add(defaultRotationInterval * 3).Add(WorkspaceAppsTokenDuration + time.Hour),
			Valid: true,
		}
		require.NoError(t, err)
		requireKey(t, deletesAtKey, expiredKey.Feature, expiredKey.StartsAt.UTC(), deletesAt, expiredKey.Sequence)

		newKey, err := db.GetCryptoKeyByFeatureAndSequence(ctx, database.GetCryptoKeyByFeatureAndSequenceParams{
			Feature:  expiredKey.Feature,
			Sequence: deletedKey.Sequence + 1,
		})
		require.NoError(t, err)
		requireKey(t, newKey, expiredKey.Feature, now.Add(defaultRotationInterval*3), nullTime, deletedKey.Sequence+1)
	})
}

func dbnow(c quartz.Clock) time.Time {
	return dbtime.Time(c.Now().UTC())
}

func requireKey(t *testing.T, key database.CryptoKey, feature database.CryptoKeyFeature, startsAt time.Time, deletesAt sql.NullTime, sequence int32) {
	t.Helper()
	require.Equal(t, feature, key.Feature)
	require.Equal(t, startsAt, key.StartsAt.UTC())
	require.Equal(t, deletesAt.Valid, key.DeletesAt.Valid)
	require.Equal(t, deletesAt.Time.UTC(), key.DeletesAt.Time.UTC())
	require.Equal(t, sequence, key.Sequence)

	secret, err := hex.DecodeString(key.Secret.String)
	require.NoError(t, err)

	switch key.Feature {
	case database.CryptoKeyFeatureOIDCConvert:
		require.Len(t, secret, 64)
	case database.CryptoKeyFeatureWorkspaceAppsToken:
		require.Len(t, secret, 64)
	case database.CryptoKeyFeatureWorkspaceAppsAPIKey:
		require.Len(t, secret, 32)
	case database.CryptoKeyFeatureTailnetResume:
		require.Len(t, secret, 64)
	default:
		t.Fatalf("unknown key feature: %s", key.Feature)
	}
}

var nullTime = sql.NullTime{Time: time.Time{}, Valid: false}

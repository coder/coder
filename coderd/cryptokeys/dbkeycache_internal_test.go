package cryptokeys

import (
	"database/sql"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func Test_version(t *testing.T) {
	t.Parallel()

	t.Run("HitsCache", func(t *testing.T) {
		t.Parallel()

		var (
			ctrl   = gomock.NewController(t)
			mockDB = dbmock.NewMockStore(ctrl)
			clock  = quartz.NewMock(t)
			logger = slogtest.Make(t, nil)
			ctx    = testutil.Context(t, testutil.WaitShort)
		)

		expectedKey := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 32,
			Secret: sql.NullString{
				String: mustGenerateKey(t),
				Valid:  true,
			},
		}

		cache := map[int32]database.CryptoKey{
			32: expectedKey,
		}

		k := newDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()
		k.keys = cache

		secret, err := k.sequence(ctx, keyID(expectedKey))
		require.NoError(t, err)
		require.Equal(t, decodedSecret(t, expectedKey), secret)
	})

	t.Run("MissesCache", func(t *testing.T) {
		t.Parallel()

		var (
			ctrl   = gomock.NewController(t)
			mockDB = dbmock.NewMockStore(ctrl)
			clock  = quartz.NewMock(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = slogtest.Make(t, nil)
		)

		expectedKey := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 33,
			StartsAt: clock.Now(),
			Secret: sql.NullString{
				String: mustGenerateKey(t),
				Valid:  true,
			},
		}

		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{expectedKey}, nil)

		k := newDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()

		got, err := k.sequence(ctx, keyID(expectedKey))
		require.NoError(t, err)
		require.Equal(t, decodedSecret(t, expectedKey), got)
	})

	t.Run("InvalidCachedKey", func(t *testing.T) {
		t.Parallel()

		var (
			ctrl   = gomock.NewController(t)
			mockDB = dbmock.NewMockStore(ctrl)
			clock  = quartz.NewMock(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = slogtest.Make(t, nil)
		)

		cache := map[int32]database.CryptoKey{
			32: {
				Feature:  database.CryptoKeyFeatureWorkspaceApps,
				Sequence: 32,
				Secret: sql.NullString{
					String: mustGenerateKey(t),
					Valid:  true,
				},
				DeletesAt: sql.NullTime{
					Time:  clock.Now(),
					Valid: true,
				},
			},
		}

		k := newDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()
		k.keys = cache

		_, err := k.sequence(ctx, "32")
		require.ErrorIs(t, err, ErrKeyInvalid)
	})

	t.Run("InvalidDBKey", func(t *testing.T) {
		t.Parallel()

		var (
			ctrl   = gomock.NewController(t)
			mockDB = dbmock.NewMockStore(ctrl)
			clock  = quartz.NewMock(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = slogtest.Make(t, nil)
		)

		invalidKey := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 32,
			Secret: sql.NullString{
				String: mustGenerateKey(t),
				Valid:  true,
			},
			DeletesAt: sql.NullTime{
				Time:  clock.Now(),
				Valid: true,
			},
		}
		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{invalidKey}, nil)

		k := newDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()

		_, err := k.sequence(ctx, keyID(invalidKey))
		require.ErrorIs(t, err, ErrKeyInvalid)
	})
}

func Test_latest(t *testing.T) {
	t.Parallel()

	t.Run("HitsCache", func(t *testing.T) {
		t.Parallel()

		var (
			ctrl   = gomock.NewController(t)
			mockDB = dbmock.NewMockStore(ctrl)
			clock  = quartz.NewMock(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = slogtest.Make(t, nil)
		)

		latestKey := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 32,
			Secret: sql.NullString{
				String: mustGenerateKey(t),
				Valid:  true,
			},
			StartsAt: clock.Now(),
		}
		k := newDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()

		k.latestKey = latestKey

		id, secret, err := k.latest(ctx)
		require.NoError(t, err)
		require.Equal(t, keyID(latestKey), id)
		require.Equal(t, decodedSecret(t, latestKey), secret)
	})

	t.Run("InvalidCachedKey", func(t *testing.T) {
		t.Parallel()

		var (
			ctrl   = gomock.NewController(t)
			mockDB = dbmock.NewMockStore(ctrl)
			clock  = quartz.NewMock(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = slogtest.Make(t, nil)
		)

		latestKey := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 33,
			Secret: sql.NullString{
				String: mustGenerateKey(t),
				Valid:  true,
			},
			StartsAt: clock.Now(),
		}

		invalidKey := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 32,
			Secret: sql.NullString{
				String: mustGenerateKey(t),
				Valid:  true,
			},
			StartsAt: clock.Now().Add(-time.Hour),
			DeletesAt: sql.NullTime{
				Time:  clock.Now(),
				Valid: true,
			},
		}

		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{latestKey}, nil)

		k := newDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()
		k.latestKey = invalidKey

		id, secret, err := k.latest(ctx)
		require.NoError(t, err)
		require.Equal(t, keyID(latestKey), id)
		require.Equal(t, decodedSecret(t, latestKey), secret)
	})

	t.Run("UsesActiveKey", func(t *testing.T) {
		t.Parallel()

		var (
			ctrl   = gomock.NewController(t)
			mockDB = dbmock.NewMockStore(ctrl)
			clock  = quartz.NewMock(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = slogtest.Make(t, nil)
		)

		inactiveKey := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 32,
			Secret: sql.NullString{
				String: mustGenerateKey(t),
				Valid:  true,
			},
			StartsAt: clock.Now().Add(time.Hour),
		}

		activeKey := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 33,
			Secret: sql.NullString{
				String: mustGenerateKey(t),
				Valid:  true,
			},
			StartsAt: clock.Now(),
		}

		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{inactiveKey, activeKey}, nil)

		k := newDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()

		id, secret, err := k.latest(ctx)
		require.NoError(t, err)
		require.Equal(t, keyID(activeKey), id)
		require.Equal(t, decodedSecret(t, activeKey), secret)
	})

	t.Run("NoValidKeys", func(t *testing.T) {
		t.Parallel()

		var (
			ctrl   = gomock.NewController(t)
			mockDB = dbmock.NewMockStore(ctrl)
			clock  = quartz.NewMock(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = slogtest.Make(t, nil)
		)

		inactiveKey := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 32,
			Secret: sql.NullString{
				String: mustGenerateKey(t),
				Valid:  true,
			},
			StartsAt: clock.Now().Add(time.Hour),
		}

		invalidKey := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 33,
			Secret: sql.NullString{
				String: mustGenerateKey(t),
				Valid:  true,
			},
			StartsAt: clock.Now().Add(-time.Hour),
			DeletesAt: sql.NullTime{
				Time:  clock.Now(),
				Valid: true,
			},
		}

		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{inactiveKey, invalidKey}, nil)

		k := newDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()

		_, _, err := k.latest(ctx)
		require.ErrorIs(t, err, ErrKeyInvalid)
	})
}

func Test_clear(t *testing.T) {
	t.Parallel()

	t.Run("InvalidatesCache", func(t *testing.T) {
		t.Parallel()

		var (
			ctrl   = gomock.NewController(t)
			mockDB = dbmock.NewMockStore(ctrl)
			clock  = quartz.NewMock(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = slogtest.Make(t, nil)
		)

		k := newDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()

		activeKey := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 33,
			Secret: sql.NullString{
				String: mustGenerateKey(t),
				Valid:  true,
			},
			StartsAt: clock.Now(),
		}

		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{activeKey}, nil)

		_, _, err := k.latest(ctx)
		require.NoError(t, err)

		dur, wait := clock.AdvanceNext()
		wait.MustWait(ctx)
		require.Equal(t, time.Minute*10, dur)
		require.Len(t, k.keys, 0)
		require.Equal(t, database.CryptoKey{}, k.latestKey)
	})

	t.Run("ResetsTimer", func(t *testing.T) {
		t.Parallel()

		var (
			ctrl   = gomock.NewController(t)
			mockDB = dbmock.NewMockStore(ctrl)
			clock  = quartz.NewMock(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = slogtest.Make(t, nil)
		)

		k := newDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()

		key := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 32,
			Secret: sql.NullString{
				String: mustGenerateKey(t),
				Valid:  true,
			},
			StartsAt: clock.Now(),
		}

		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{key}, nil)

		// Advance it five minutes so that we can test that the
		// timer is reset and doesn't fire after another five minute.
		clock.Advance(time.Minute * 5)

		id, secret, err := k.latest(ctx)
		require.NoError(t, err)
		require.Equal(t, keyID(key), id)
		require.Equal(t, decodedSecret(t, key), secret)

		// Advancing the clock now should require 10 minutes
		// before the timer fires again.
		dur, wait := clock.AdvanceNext()
		wait.MustWait(ctx)
		require.Equal(t, time.Minute*10, dur)
		require.Len(t, k.keys, 0)
		require.Equal(t, database.CryptoKey{}, k.latestKey)
	})

	// InvalidateAt tests that we have accounted for the race condition where a
	// timer fires to invalidate the cache at the same time we are fetching new
	// keys. In such cases we want to skip invalidation.
	t.Run("InvalidateAt", func(t *testing.T) {
		t.Parallel()

		var (
			ctrl   = gomock.NewController(t)
			mockDB = dbmock.NewMockStore(ctrl)
			clock  = quartz.NewMock(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = slogtest.Make(t, nil)
		)

		trap := clock.Trap().Now("clear")

		k := newDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()

		key := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 32,
			Secret: sql.NullString{
				String: mustGenerateKey(t),
				Valid:  true,
			},
			StartsAt: clock.Now(),
		}

		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{key}, nil).Times(2)

		// Move us past the initial timer.
		id, secret, err := k.latest(ctx)
		require.NoError(t, err)
		require.Equal(t, keyID(key), id)
		require.Equal(t, decodedSecret(t, key), secret)
		// Null these out so that we refetch.
		k.keys = nil
		k.latestKey = database.CryptoKey{}

		// Initiate firing the timer.
		dur, wait := clock.AdvanceNext()
		require.Equal(t, time.Minute*10, dur)
		// Trap the function just before acquiring the mutex.
		call := trap.MustWait(ctx)

		// Refetch keys.
		id, secret, err = k.latest(ctx)
		require.NoError(t, err)
		require.Equal(t, keyID(key), id)
		require.Equal(t, decodedSecret(t, key), secret)

		// Let the rest of the timer function run.
		// It should see that we have refetched keys and
		// not invalidate.
		call.Release()
		wait.MustWait(ctx)
		require.Len(t, k.keys, 1)
		require.Equal(t, key, k.latestKey)
		trap.Close()

		// Refetching the keys should've instantiated a new timer. This one should invalidate keys.
		_, wait = clock.AdvanceNext()
		wait.MustWait(ctx)
		require.Len(t, k.keys, 0)
		require.Equal(t, database.CryptoKey{}, k.latestKey)
	})
}

func mustGenerateKey(t *testing.T) string {
	t.Helper()
	key, err := generateKey(64)
	require.NoError(t, err)
	return key
}

func keyID(key database.CryptoKey) string {
	return strconv.FormatInt(int64(key.Sequence), 10)
}

func decodedSecret(t *testing.T, key database.CryptoKey) []byte {
	t.Helper()
	decoded, err := key.DecodeString()
	require.NoError(t, err)
	return decoded
}

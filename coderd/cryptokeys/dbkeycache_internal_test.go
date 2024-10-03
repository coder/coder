package cryptokeys

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func Test_Verifying(t *testing.T) {
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
				String: "secret",
				Valid:  true,
			},
		}

		cache := map[int32]database.CryptoKey{
			32: expectedKey,
		}

		k := NewDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()
		k.keys = cache

		id, secret, err := k.SigningKey(ctx)
		require.NoError(t, err)
		require.Equal(t, "32", id)
		require.Equal(t, db2sdk.CryptoKey(expectedKey), got)
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
				String: "secret",
				Valid:  true,
			},
		}

		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{expectedKey}, nil)

		k := NewDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()

		got, err := k.Verifying(ctx, 33)
		require.NoError(t, err)
		require.Equal(t, db2sdk.CryptoKey(expectedKey), got)
		require.Equal(t, db2sdk.CryptoKey(expectedKey), db2sdk.CryptoKey(k.latestKey))
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
					String: "secret",
					Valid:  true,
				},
				DeletesAt: sql.NullTime{
					Time:  clock.Now(),
					Valid: true,
				},
			},
		}

		k := NewDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()
		k.keys = cache

		_, err := k.Verifying(ctx, 32)
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
				String: "secret",
				Valid:  true,
			},
			DeletesAt: sql.NullTime{
				Time:  clock.Now(),
				Valid: true,
			},
		}
		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{invalidKey}, nil)

		k := NewDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()

		_, err := k.Verifying(ctx, 32)
		require.ErrorIs(t, err, ErrKeyInvalid)
	})
}

func Test_Signing(t *testing.T) {
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
				String: "secret",
				Valid:  true,
			},
			StartsAt: clock.Now(),
		}
		k := NewDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()

		k.latestKey = latestKey

		got, err := k.Signing(ctx)
		require.NoError(t, err)
		require.Equal(t, db2sdk.CryptoKey(latestKey), got)
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
				String: "secret",
				Valid:  true,
			},
			StartsAt: clock.Now(),
		}

		invalidKey := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 32,
			Secret: sql.NullString{
				String: "secret",
				Valid:  true,
			},
			StartsAt: clock.Now().Add(-time.Hour),
			DeletesAt: sql.NullTime{
				Time:  clock.Now(),
				Valid: true,
			},
		}

		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{latestKey}, nil)

		k := NewDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()
		k.latestKey = invalidKey

		got, err := k.Signing(ctx)
		require.NoError(t, err)
		require.Equal(t, db2sdk.CryptoKey(latestKey), got)
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
				String: "secret",
				Valid:  true,
			},
			StartsAt: clock.Now().Add(time.Hour),
		}

		activeKey := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 33,
			Secret: sql.NullString{
				String: "secret",
				Valid:  true,
			},
			StartsAt: clock.Now(),
		}

		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{inactiveKey, activeKey}, nil)

		k := NewDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()

		got, err := k.Signing(ctx)
		require.NoError(t, err)
		require.Equal(t, db2sdk.CryptoKey(activeKey), got)
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
				String: "secret",
				Valid:  true,
			},
			StartsAt: clock.Now().Add(time.Hour),
		}

		invalidKey := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 33,
			Secret: sql.NullString{
				String: "secret",
				Valid:  true,
			},
			StartsAt: clock.Now().Add(-time.Hour),
			DeletesAt: sql.NullTime{
				Time:  clock.Now(),
				Valid: true,
			},
		}

		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{inactiveKey, invalidKey}, nil)

		k := NewDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()

		_, err := k.Signing(ctx)
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

		k := NewDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()

		activeKey := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 33,
			Secret: sql.NullString{
				String: "secret",
				Valid:  true,
			},
			StartsAt: clock.Now(),
		}

		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{activeKey}, nil)

		_, err := k.Signing(ctx)
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

		k := NewDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()

		key := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 32,
			Secret: sql.NullString{
				String: "secret",
				Valid:  true,
			},
			StartsAt: clock.Now(),
		}

		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{key}, nil)

		// Advance it five minutes so that we can test that the
		// timer is reset and doesn't fire after another five minute.
		clock.Advance(time.Minute * 5)

		latest, err := k.Signing(ctx)
		require.NoError(t, err)
		require.Equal(t, db2sdk.CryptoKey(key), latest)

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

		k := NewDBCache(logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		defer k.Close()

		key := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 32,
			Secret: sql.NullString{
				String: "secret",
				Valid:  true,
			},
			StartsAt: clock.Now(),
		}

		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{key}, nil).Times(2)

		// Move us past the initial timer.
		latest, err := k.Signing(ctx)
		require.NoError(t, err)
		require.Equal(t, db2sdk.CryptoKey(key), latest)
		// Null these out so that we refetch.
		k.keys = nil
		k.latestKey = database.CryptoKey{}

		// Initiate firing the timer.
		dur, wait := clock.AdvanceNext()
		require.Equal(t, time.Minute*10, dur)
		// Trap the function just before acquiring the mutex.
		call := trap.MustWait(ctx)

		// Refetch keys.
		latest, err = k.Signing(ctx)
		require.NoError(t, err)
		require.Equal(t, db2sdk.CryptoKey(key), latest)

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

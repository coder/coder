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

		k := &DBCache{
			db:      mockDB,
			feature: database.CryptoKeyFeatureWorkspaceApps,
			keys:    cache,
			clock:   clock,
		}

		got, err := k.Verifying(ctx, 32)
		require.NoError(t, err)
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
			StartsAt: clock.Now().UTC(),
			Secret: sql.NullString{
				String: "secret",
				Valid:  true,
			},
		}

		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{expectedKey}, nil)

		k := NewDBCache(ctx, logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))

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
					Time:  clock.Now().UTC(),
					Valid: true,
				},
			},
		}

		k := &DBCache{
			db:      mockDB,
			feature: database.CryptoKeyFeatureWorkspaceApps,
			keys:    cache,
			clock:   clock,
		}

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
		)

		invalidKey := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 32,
			Secret: sql.NullString{
				String: "secret",
				Valid:  true,
			},
			DeletesAt: sql.NullTime{
				Time:  clock.Now().UTC(),
				Valid: true,
			},
		}
		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{invalidKey}, nil)

		k := &DBCache{
			db:      mockDB,
			feature: database.CryptoKeyFeatureWorkspaceApps,
			keys:    map[int32]database.CryptoKey{},
			clock:   clock,
		}

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
			StartsAt: clock.Now().UTC(),
		}
		k := NewDBCache(ctx, logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
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
			StartsAt: clock.Now().UTC(),
		}

		invalidKey := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 32,
			Secret: sql.NullString{
				String: "secret",
				Valid:  true,
			},
			StartsAt: clock.Now().UTC().Add(-time.Hour),
			DeletesAt: sql.NullTime{
				Time:  clock.Now().UTC(),
				Valid: true,
			},
		}

		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{latestKey}, nil)

		k := NewDBCache(ctx, logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
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
			StartsAt: clock.Now().UTC().Add(time.Hour),
		}

		activeKey := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 33,
			Secret: sql.NullString{
				String: "secret",
				Valid:  true,
			},
			StartsAt: clock.Now().UTC(),
		}

		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{inactiveKey, activeKey}, nil)

		k := NewDBCache(ctx, logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))

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
			StartsAt: clock.Now().UTC().Add(time.Hour),
		}

		invalidKey := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 33,
			Secret: sql.NullString{
				String: "secret",
				Valid:  true,
			},
			StartsAt: clock.Now().UTC().Add(-time.Hour),
			DeletesAt: sql.NullTime{
				Time:  clock.Now().UTC(),
				Valid: true,
			},
		}

		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{inactiveKey, invalidKey}, nil)

		k := NewDBCache(ctx, logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))

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

		trap := clock.Trap().AfterFunc()

		k := NewDBCache(ctx, logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))
		k.latestKey = database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 32,
			Secret: sql.NullString{
				String: "secret",
				Valid:  true,
			},
		}
		k.keys = map[int32]database.CryptoKey{
			32: {
				Feature:  database.CryptoKeyFeatureWorkspaceApps,
				Sequence: 32,
				Secret: sql.NullString{
					String: "secret",
					Valid:  true,
				},
			},
		}
		trap.MustWait(ctx).Release()
		_, wait := clock.AdvanceNext()
		wait.MustWait(ctx)
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

		trap := clock.Trap().AfterFunc()

		k := NewDBCache(ctx, logger, mockDB, database.CryptoKeyFeatureWorkspaceApps, WithDBCacheClock(clock))

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

		trap.MustWait(ctx).Release()

		// Advance it five minutes so that we can test that the
		// time is reset and doesn't fire after another five minute and doesn't fire after another five minutes.
		clock.Advance(time.Minute * 5)

		latest, err := k.Signing(ctx)
		require.NoError(t, err)
		require.Equal(t, db2sdk.CryptoKey(key), latest)

		trap.MustWait(ctx).Release()
		// Advancing the clock now should require 10 minutes
		// before the timer fires again.
		dur, wait := clock.AdvanceNext()
		wait.MustWait(ctx)
		require.Equal(t, time.Minute*10, dur)
		require.Len(t, k.keys, 0)
		require.Equal(t, database.CryptoKey{}, k.latestKey)
	})
}

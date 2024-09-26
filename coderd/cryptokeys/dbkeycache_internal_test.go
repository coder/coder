package cryptokeys

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func Test_Version(t *testing.T) {
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

		k := &DBKeyCache{
			db:      mockDB,
			feature: database.CryptoKeyFeatureWorkspaceApps,
			cache:   cache,
			Clock:   clock,
		}

		got, err := k.Version(ctx, 32)
		require.NoError(t, err)
		require.Equal(t, expectedKey, got)
	})

	t.Run("MissesCache", func(t *testing.T) {
		t.Parallel()

		var (
			ctrl   = gomock.NewController(t)
			mockDB = dbmock.NewMockStore(ctrl)
			clock  = quartz.NewMock(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
		)

		expectedKey := database.CryptoKey{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 33,
			Secret: sql.NullString{
				String: "secret",
				Valid:  true,
			},
			StartsAt: clock.Now().UTC(),
		}

		mockDB.EXPECT().GetCryptoKeyByFeatureAndSequence(ctx, database.GetCryptoKeyByFeatureAndSequenceParams{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 33,
		}).Return(expectedKey, nil)

		k := &DBKeyCache{
			db:      mockDB,
			feature: database.CryptoKeyFeatureWorkspaceApps,
			cache:   map[int32]database.CryptoKey{},
			Clock:   clock,
		}

		got, err := k.Version(ctx, 33)
		require.NoError(t, err)
		require.Equal(t, expectedKey, got)
		require.Equal(t, expectedKey, k.latestKey)
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

		k := &DBKeyCache{
			db:      mockDB,
			feature: database.CryptoKeyFeatureWorkspaceApps,
			cache:   cache,
			Clock:   clock,
		}

		_, err := k.Version(ctx, 32)
		require.ErrorIs(t, err, ErrKeyNotFound)
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
		mockDB.EXPECT().GetCryptoKeyByFeatureAndSequence(ctx, database.GetCryptoKeyByFeatureAndSequenceParams{
			Feature:  database.CryptoKeyFeatureWorkspaceApps,
			Sequence: 32,
		}).Return(invalidKey, nil)

		k := &DBKeyCache{
			db:      mockDB,
			feature: database.CryptoKeyFeatureWorkspaceApps,
			cache:   map[int32]database.CryptoKey{},
			Clock:   clock,
		}

		_, err := k.Version(ctx, 32)
		require.ErrorIs(t, err, ErrKeyInvalid)
	})
}

func Test_Latest(t *testing.T) {
	t.Parallel()

	t.Run("HitsCache", func(t *testing.T) {
		t.Parallel()

		var (
			ctrl   = gomock.NewController(t)
			mockDB = dbmock.NewMockStore(ctrl)
			clock  = quartz.NewMock(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
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
		k := &DBKeyCache{
			db:        mockDB,
			feature:   database.CryptoKeyFeatureWorkspaceApps,
			Clock:     clock,
			latestKey: latestKey,
		}

		got, err := k.Latest(ctx)
		require.NoError(t, err)
		require.Equal(t, latestKey, got)
	})

	t.Run("InvalidCachedKey", func(t *testing.T) {
		t.Parallel()

		var (
			ctrl   = gomock.NewController(t)
			mockDB = dbmock.NewMockStore(ctrl)
			clock  = quartz.NewMock(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
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

		mockDB.EXPECT().GetCryptoKeysByFeature(ctx, database.CryptoKeyFeatureWorkspaceApps).Return([]database.CryptoKey{latestKey}, nil)

		k := &DBKeyCache{
			db:      mockDB,
			feature: database.CryptoKeyFeatureWorkspaceApps,
			Clock:   clock,
			latestKey: database.CryptoKey{
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
			},
		}

		got, err := k.Latest(ctx)
		require.NoError(t, err)
		require.Equal(t, latestKey, got)
	})

	t.Run("UsesActiveKey", func(t *testing.T) {
		t.Parallel()

		var (
			ctrl   = gomock.NewController(t)
			mockDB = dbmock.NewMockStore(ctrl)
			clock  = quartz.NewMock(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
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

		k := &DBKeyCache{
			db:      mockDB,
			feature: database.CryptoKeyFeatureWorkspaceApps,
			Clock:   clock,
			cache:   map[int32]database.CryptoKey{},
		}

		got, err := k.Latest(ctx)
		require.NoError(t, err)
		require.Equal(t, activeKey, got)
	})

	t.Run("NoValidKeys", func(t *testing.T) {
		t.Parallel()

		var (
			ctrl   = gomock.NewController(t)
			mockDB = dbmock.NewMockStore(ctrl)
			clock  = quartz.NewMock(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
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

		k := &DBKeyCache{
			db:      mockDB,
			feature: database.CryptoKeyFeatureWorkspaceApps,
			Clock:   clock,
			cache:   map[int32]database.CryptoKey{},
		}

		_, err := k.Latest(ctx)
		require.ErrorIs(t, err, ErrKeyInvalid)
	})
}

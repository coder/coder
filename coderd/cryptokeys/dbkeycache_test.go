package cryptokeys_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestDBKeyCache(t *testing.T) {
	t.Parallel()

	t.Run("VerifyingKey", func(t *testing.T) {
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
				Feature:  database.CryptoKeyFeatureOidcConvert,
				Sequence: 1,
				StartsAt: clock.Now().UTC(),
			})

			k, err := cryptokeys.NewSigningCache(logger, db, database.CryptoKeyFeatureOidcConvert, cryptokeys.WithDBCacheClock(clock))
			require.NoError(t, err)
			defer k.Close()

			got, err := k.VerifyingKey(ctx, keyID(key))
			require.NoError(t, err)
			require.Equal(t, decodedSecret(t, key), got)
		})

		t.Run("NotFound", func(t *testing.T) {
			t.Parallel()

			var (
				db, _  = dbtestutil.NewDB(t)
				clock  = quartz.NewMock(t)
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = slogtest.Make(t, nil)
			)

			k, err := cryptokeys.NewSigningCache(logger, db, database.CryptoKeyFeatureOidcConvert, cryptokeys.WithDBCacheClock(clock))
			require.NoError(t, err)
			defer k.Close()

			_, err = k.VerifyingKey(ctx, "123")
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
			Feature:  database.CryptoKeyFeatureOidcConvert,
			Sequence: 10,
			StartsAt: clock.Now().UTC(),
		})

		expectedKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureOidcConvert,
			Sequence: 12,
			StartsAt: clock.Now().UTC(),
		})

		_ = dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureOidcConvert,
			Sequence: 2,
			StartsAt: clock.Now().UTC(),
		})

		k, err := cryptokeys.NewSigningCache(logger, db, database.CryptoKeyFeatureOidcConvert, cryptokeys.WithDBCacheClock(clock))
		require.NoError(t, err)
		defer k.Close()

		id, key, err := k.SigningKey(ctx)
		require.NoError(t, err)
		require.Equal(t, keyID(expectedKey), id)
		require.Equal(t, decodedSecret(t, expectedKey), key)
	})

	t.Run("Closed", func(t *testing.T) {
		t.Parallel()

		var (
			db, _  = dbtestutil.NewDB(t)
			clock  = quartz.NewMock(t)
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = slogtest.Make(t, nil)
		)

		expectedKey := dbgen.CryptoKey(t, db, database.CryptoKey{
			Feature:  database.CryptoKeyFeatureOidcConvert,
			Sequence: 10,
			StartsAt: clock.Now(),
		})

		k, err := cryptokeys.NewSigningCache(logger, db, database.CryptoKeyFeatureOidcConvert, cryptokeys.WithDBCacheClock(clock))
		require.NoError(t, err)
		defer k.Close()

		id, key, err := k.SigningKey(ctx)
		require.NoError(t, err)
		require.Equal(t, keyID(expectedKey), id)
		require.Equal(t, decodedSecret(t, expectedKey), key)

		key, err = k.VerifyingKey(ctx, keyID(expectedKey))
		require.NoError(t, err)
		require.Equal(t, decodedSecret(t, expectedKey), key)

		k.Close()

		_, _, err = k.SigningKey(ctx)
		require.ErrorIs(t, err, cryptokeys.ErrClosed)

		_, err = k.VerifyingKey(ctx, keyID(expectedKey))
		require.ErrorIs(t, err, cryptokeys.ErrClosed)
	})

	t.Run("InvalidSigningFeature", func(t *testing.T) {
		t.Parallel()

		var (
			db, _  = dbtestutil.NewDB(t)
			clock  = quartz.NewMock(t)
			logger = slogtest.Make(t, nil)
			ctx    = testutil.Context(t, testutil.WaitShort)
		)

		_, err := cryptokeys.NewSigningCache(logger, db, database.CryptoKeyFeatureWorkspaceApps, cryptokeys.WithDBCacheClock(clock))
		require.ErrorIs(t, err, cryptokeys.ErrInvalidFeature)

		// Instantiate a signing cache and try to use it as an encryption cache.
		sc, err := cryptokeys.NewSigningCache(logger, db, database.CryptoKeyFeatureOidcConvert, cryptokeys.WithDBCacheClock(clock))
		require.NoError(t, err)
		defer sc.Close()

		ec, ok := sc.(cryptokeys.EncryptionKeycache)
		require.True(t, ok)
		_, _, err = ec.EncryptingKey(ctx)
		require.ErrorIs(t, err, cryptokeys.ErrInvalidFeature)

		_, err = ec.DecryptingKey(ctx, "123")
		require.ErrorIs(t, err, cryptokeys.ErrInvalidFeature)
	})

	t.Run("InvalidEncryptionFeature", func(t *testing.T) {
		t.Parallel()

		var (
			db, _  = dbtestutil.NewDB(t)
			clock  = quartz.NewMock(t)
			logger = slogtest.Make(t, nil)
			ctx    = testutil.Context(t, testutil.WaitShort)
		)

		_, err := cryptokeys.NewEncryptionCache(logger, db, database.CryptoKeyFeatureOidcConvert, cryptokeys.WithDBCacheClock(clock))
		require.ErrorIs(t, err, cryptokeys.ErrInvalidFeature)

		// Instantiate an encryption cache and try to use it as a signing cache.
		ec, err := cryptokeys.NewEncryptionCache(logger, db, database.CryptoKeyFeatureWorkspaceApps, cryptokeys.WithDBCacheClock(clock))
		require.NoError(t, err)
		defer ec.Close()

		sc, ok := ec.(cryptokeys.SigningKeycache)
		require.True(t, ok)
		_, _, err = sc.SigningKey(ctx)
		require.ErrorIs(t, err, cryptokeys.ErrInvalidFeature)

		_, err = sc.VerifyingKey(ctx, "123")
		require.ErrorIs(t, err, cryptokeys.ErrInvalidFeature)
	})
}

func keyID(key database.CryptoKey) string {
	return strconv.FormatInt(int64(key.Sequence), 10)
}

func decodedSecret(t *testing.T, key database.CryptoKey) []byte {
	t.Helper()

	secret, err := key.DecodeString()
	require.NoError(t, err)

	return secret
}

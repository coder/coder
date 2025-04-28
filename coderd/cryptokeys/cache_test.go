package cryptokeys_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestCryptoKeyCache(t *testing.T) {
	t.Parallel()

	t.Run("Signing", func(t *testing.T) {
		t.Parallel()

		t.Run("HitsCache", func(t *testing.T) {
			t.Parallel()
			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = testutil.Logger(t)
				clock  = quartz.NewMock(t)
			)

			now := clock.Now().UTC()
			expected := codersdk.CryptoKey{
				Feature:  codersdk.CryptoKeyFeatureTailnetResume,
				Secret:   generateKey(t, 64),
				Sequence: 2,
				StartsAt: now,
			}

			ff := &fakeFetcher{
				keys: []codersdk.CryptoKey{expected},
			}

			cache, err := cryptokeys.NewSigningCache(ctx, logger, ff, codersdk.CryptoKeyFeatureTailnetResume, cryptokeys.WithCacheClock(clock))
			require.NoError(t, err)

			id, got, err := cache.SigningKey(ctx)
			require.NoError(t, err)
			require.Equal(t, keyID(expected), id)
			require.Equal(t, decodedSecret(t, expected), got)
			require.Equal(t, 1, ff.called)
		})

		t.Run("MissesCache", func(t *testing.T) {
			t.Parallel()
			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = testutil.Logger(t)
				clock  = quartz.NewMock(t)
			)

			ff := &fakeFetcher{
				keys: []codersdk.CryptoKey{},
			}

			cache, err := cryptokeys.NewSigningCache(ctx, logger, ff, codersdk.CryptoKeyFeatureTailnetResume, cryptokeys.WithCacheClock(clock))
			require.NoError(t, err)

			expected := codersdk.CryptoKey{
				Feature:  codersdk.CryptoKeyFeatureTailnetResume,
				Secret:   generateKey(t, 64),
				Sequence: 12,
				StartsAt: clock.Now().UTC(),
			}
			ff.keys = []codersdk.CryptoKey{expected}

			id, got, err := cache.SigningKey(ctx)
			require.NoError(t, err)
			require.Equal(t, decodedSecret(t, expected), got)
			require.Equal(t, keyID(expected), id)
			// 1 on startup + missing cache.
			require.Equal(t, 2, ff.called)

			// Ensure the cache gets hit this time.
			id, got, err = cache.SigningKey(ctx)
			require.NoError(t, err)
			require.Equal(t, decodedSecret(t, expected), got)
			require.Equal(t, keyID(expected), id)
			// 1 on startup + missing cache.
			require.Equal(t, 2, ff.called)
		})

		t.Run("IgnoresInvalid", func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = testutil.Logger(t)
				clock  = quartz.NewMock(t)
			)
			now := clock.Now().UTC()

			expected := codersdk.CryptoKey{
				Feature:  codersdk.CryptoKeyFeatureTailnetResume,
				Secret:   generateKey(t, 64),
				Sequence: 1,
				StartsAt: clock.Now().UTC(),
			}

			ff := &fakeFetcher{
				keys: []codersdk.CryptoKey{
					expected,
					{
						Feature:   codersdk.CryptoKeyFeatureTailnetResume,
						Secret:    generateKey(t, 64),
						Sequence:  2,
						StartsAt:  now.Add(-time.Second),
						DeletesAt: now,
					},
				},
			}

			cache, err := cryptokeys.NewSigningCache(ctx, logger, ff, codersdk.CryptoKeyFeatureTailnetResume, cryptokeys.WithCacheClock(clock))
			require.NoError(t, err)

			id, got, err := cache.SigningKey(ctx)
			require.NoError(t, err)
			require.Equal(t, decodedSecret(t, expected), got)
			require.Equal(t, keyID(expected), id)
			require.Equal(t, 1, ff.called)
		})

		t.Run("KeyNotFound", func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = testutil.Logger(t)
			)

			ff := &fakeFetcher{
				keys: []codersdk.CryptoKey{},
			}

			cache, err := cryptokeys.NewSigningCache(ctx, logger, ff, codersdk.CryptoKeyFeatureTailnetResume)
			require.NoError(t, err)

			_, _, err = cache.SigningKey(ctx)
			require.ErrorIs(t, err, cryptokeys.ErrKeyNotFound)
		})
	})

	t.Run("Verifying", func(t *testing.T) {
		t.Parallel()

		t.Run("HitsCache", func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = testutil.Logger(t)
				clock  = quartz.NewMock(t)
			)

			now := clock.Now().UTC()
			expected := codersdk.CryptoKey{
				Feature:  codersdk.CryptoKeyFeatureTailnetResume,
				Secret:   generateKey(t, 64),
				Sequence: 12,
				StartsAt: now,
			}
			ff := &fakeFetcher{
				keys: []codersdk.CryptoKey{
					expected,
					{
						Feature:  codersdk.CryptoKeyFeatureTailnetResume,
						Secret:   generateKey(t, 64),
						Sequence: 13,
						StartsAt: now,
					},
				},
			}

			cache, err := cryptokeys.NewSigningCache(ctx, logger, ff, codersdk.CryptoKeyFeatureTailnetResume, cryptokeys.WithCacheClock(clock))
			require.NoError(t, err)

			got, err := cache.VerifyingKey(ctx, keyID(expected))
			require.NoError(t, err)
			require.Equal(t, decodedSecret(t, expected), got)
			require.Equal(t, 1, ff.called)
		})

		t.Run("MissesCache", func(t *testing.T) {
			t.Parallel()
			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = testutil.Logger(t)
				clock  = quartz.NewMock(t)
			)

			ff := &fakeFetcher{
				keys: []codersdk.CryptoKey{},
			}

			cache, err := cryptokeys.NewSigningCache(ctx, logger, ff, codersdk.CryptoKeyFeatureTailnetResume, cryptokeys.WithCacheClock(clock))
			require.NoError(t, err)

			expected := codersdk.CryptoKey{
				Feature:  codersdk.CryptoKeyFeatureTailnetResume,
				Secret:   generateKey(t, 64),
				Sequence: 12,
				StartsAt: clock.Now().UTC(),
			}
			ff.keys = []codersdk.CryptoKey{expected}

			got, err := cache.VerifyingKey(ctx, keyID(expected))
			require.NoError(t, err)
			require.Equal(t, decodedSecret(t, expected), got)
			require.Equal(t, 2, ff.called)

			// Ensure the cache gets hit this time.
			got, err = cache.VerifyingKey(ctx, keyID(expected))
			require.NoError(t, err)
			require.Equal(t, decodedSecret(t, expected), got)
			require.Equal(t, 2, ff.called)
		})

		t.Run("AllowsBeforeStartsAt", func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = testutil.Logger(t)
				clock  = quartz.NewMock(t)
			)

			now := clock.Now().UTC()
			expected := codersdk.CryptoKey{
				Feature:  codersdk.CryptoKeyFeatureTailnetResume,
				Secret:   generateKey(t, 64),
				Sequence: 12,
				StartsAt: now.Add(-time.Second),
			}

			ff := &fakeFetcher{
				keys: []codersdk.CryptoKey{
					expected,
				},
			}

			cache, err := cryptokeys.NewSigningCache(ctx, logger, ff, codersdk.CryptoKeyFeatureTailnetResume, cryptokeys.WithCacheClock(clock))
			require.NoError(t, err)

			got, err := cache.VerifyingKey(ctx, keyID(expected))
			require.NoError(t, err)
			require.Equal(t, decodedSecret(t, expected), got)
			require.Equal(t, 1, ff.called)
		})

		t.Run("KeyPastDeletesAt", func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = testutil.Logger(t)
				clock  = quartz.NewMock(t)
			)

			now := clock.Now().UTC()
			expected := codersdk.CryptoKey{
				Feature:   codersdk.CryptoKeyFeatureTailnetResume,
				Secret:    generateKey(t, 64),
				Sequence:  12,
				StartsAt:  now.Add(-time.Second),
				DeletesAt: now,
			}

			ff := &fakeFetcher{
				keys: []codersdk.CryptoKey{
					expected,
				},
			}

			cache, err := cryptokeys.NewSigningCache(ctx, logger, ff, codersdk.CryptoKeyFeatureTailnetResume, cryptokeys.WithCacheClock(clock))
			require.NoError(t, err)

			_, err = cache.VerifyingKey(ctx, keyID(expected))
			require.ErrorIs(t, err, cryptokeys.ErrKeyInvalid)
			require.Equal(t, 1, ff.called)
		})

		t.Run("KeyNotFound", func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = testutil.Logger(t)
				clock  = quartz.NewMock(t)
			)

			ff := &fakeFetcher{
				keys: []codersdk.CryptoKey{},
			}

			cache, err := cryptokeys.NewSigningCache(ctx, logger, ff, codersdk.CryptoKeyFeatureTailnetResume, cryptokeys.WithCacheClock(clock))
			require.NoError(t, err)

			_, err = cache.VerifyingKey(ctx, "1")
			require.ErrorIs(t, err, cryptokeys.ErrKeyNotFound)
		})
	})

	t.Run("CacheRefreshes", func(t *testing.T) {
		t.Parallel()

		var (
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = testutil.Logger(t)
			clock  = quartz.NewMock(t)
		)

		now := clock.Now().UTC()
		expected := codersdk.CryptoKey{
			Feature:   codersdk.CryptoKeyFeatureTailnetResume,
			Secret:    generateKey(t, 64),
			Sequence:  12,
			StartsAt:  now,
			DeletesAt: now.Add(time.Minute * 10),
		}
		ff := &fakeFetcher{
			keys: []codersdk.CryptoKey{
				expected,
			},
		}

		cache, err := cryptokeys.NewSigningCache(ctx, logger, ff, codersdk.CryptoKeyFeatureTailnetResume, cryptokeys.WithCacheClock(clock))
		require.NoError(t, err)

		id, got, err := cache.SigningKey(ctx)
		require.NoError(t, err)
		require.Equal(t, decodedSecret(t, expected), got)
		require.Equal(t, keyID(expected), id)
		require.Equal(t, 1, ff.called)

		newKey := codersdk.CryptoKey{
			Feature:  codersdk.CryptoKeyFeatureTailnetResume,
			Secret:   generateKey(t, 64),
			Sequence: 13,
			StartsAt: now,
		}
		ff.keys = []codersdk.CryptoKey{newKey}

		// The ticker should fire and cause a request to coderd.
		dur, advance := clock.AdvanceNext()
		advance.MustWait(ctx)
		require.Equal(t, 2, ff.called)
		require.Equal(t, time.Minute*10, dur)

		// Assert hits cache.
		id, got, err = cache.SigningKey(ctx)
		require.NoError(t, err)
		require.Equal(t, keyID(newKey), id)
		require.Equal(t, decodedSecret(t, newKey), got)
		require.Equal(t, 2, ff.called)

		// We check again to ensure the timer has been reset.
		_, advance = clock.AdvanceNext()
		advance.MustWait(ctx)
		require.Equal(t, 3, ff.called)
		require.Equal(t, time.Minute*10, dur)
	})

	// This test ensures that if the refresh timer races with an inflight request
	// and loses that it doesn't cause a redundant fetch.

	t.Run("RefreshNoDoubleFetch", func(t *testing.T) {
		t.Parallel()

		var (
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = testutil.Logger(t)
			clock  = quartz.NewMock(t)
		)

		now := clock.Now().UTC()
		expected := codersdk.CryptoKey{
			Feature:   codersdk.CryptoKeyFeatureTailnetResume,
			Secret:    generateKey(t, 64),
			Sequence:  12,
			StartsAt:  now,
			DeletesAt: now.Add(time.Minute * 10),
		}
		ff := &fakeFetcher{
			keys: []codersdk.CryptoKey{
				expected,
			},
		}

		// Create a trap that blocks when the refresh timer fires.
		trap := clock.Trap().Now("refresh")
		cache, err := cryptokeys.NewSigningCache(ctx, logger, ff, codersdk.CryptoKeyFeatureTailnetResume, cryptokeys.WithCacheClock(clock))
		require.NoError(t, err)

		_, wait := clock.AdvanceNext()
		trapped := trap.MustWait(ctx)

		newKey := codersdk.CryptoKey{
			Feature:  codersdk.CryptoKeyFeatureTailnetResume,
			Secret:   generateKey(t, 64),
			Sequence: 13,
			StartsAt: now,
		}
		ff.keys = []codersdk.CryptoKey{newKey}

		key, err := cache.VerifyingKey(ctx, keyID(newKey))
		require.NoError(t, err)
		require.Equal(t, 2, ff.called)
		require.Equal(t, decodedSecret(t, newKey), key)

		trapped.Release()
		wait.MustWait(ctx)
		require.Equal(t, 2, ff.called)
		trap.Close()

		// The next timer should fire in 10 minutes.
		dur, wait := clock.AdvanceNext()
		wait.MustWait(ctx)
		require.Equal(t, time.Minute*10, dur)
		require.Equal(t, 3, ff.called)
	})

	t.Run("Closed", func(t *testing.T) {
		t.Parallel()

		var (
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = testutil.Logger(t)
			clock  = quartz.NewMock(t)
		)

		now := clock.Now()
		expected := codersdk.CryptoKey{
			Feature:  codersdk.CryptoKeyFeatureTailnetResume,
			Secret:   generateKey(t, 64),
			Sequence: 12,
			StartsAt: now,
		}
		ff := &fakeFetcher{
			keys: []codersdk.CryptoKey{
				expected,
			},
		}

		cache, err := cryptokeys.NewSigningCache(ctx, logger, ff, codersdk.CryptoKeyFeatureTailnetResume, cryptokeys.WithCacheClock(clock))
		require.NoError(t, err)

		id, got, err := cache.SigningKey(ctx)
		require.NoError(t, err)
		require.Equal(t, keyID(expected), id)
		require.Equal(t, decodedSecret(t, expected), got)
		require.Equal(t, 1, ff.called)

		key, err := cache.VerifyingKey(ctx, keyID(expected))
		require.NoError(t, err)
		require.Equal(t, decodedSecret(t, expected), key)
		require.Equal(t, 1, ff.called)

		cache.Close()

		_, _, err = cache.SigningKey(ctx)
		require.ErrorIs(t, err, cryptokeys.ErrClosed)

		_, err = cache.VerifyingKey(ctx, keyID(expected))
		require.ErrorIs(t, err, cryptokeys.ErrClosed)
	})
}

type fakeFetcher struct {
	keys   []codersdk.CryptoKey
	called int
}

func (f *fakeFetcher) Fetch(_ context.Context, _ codersdk.CryptoKeyFeature) ([]codersdk.CryptoKey, error) {
	f.called++
	return f.keys, nil
}

func keyID(key codersdk.CryptoKey) string {
	return strconv.FormatInt(int64(key.Sequence), 10)
}

func decodedSecret(t *testing.T, key codersdk.CryptoKey) []byte {
	t.Helper()

	secret, err := hex.DecodeString(key.Secret)
	require.NoError(t, err)

	return secret
}

func generateKey(t *testing.T, size int) string {
	t.Helper()

	key := make([]byte, size)
	_, err := rand.Read(key)
	require.NoError(t, err)

	return hex.EncodeToString(key)
}

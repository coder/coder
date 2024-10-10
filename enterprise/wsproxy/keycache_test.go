package wsproxy_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/wsproxy"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestCryptoKeyCache(t *testing.T) {
	t.Parallel()

	t.Run("Signing", func(t *testing.T) {
		t.Parallel()

		t.Run("HitsCache", func(t *testing.T) {
			t.Parallel()
			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = slogtest.Make(t, nil)
				clock  = quartz.NewMock(t)
			)

			now := clock.Now().UTC()
			expected := codersdk.CryptoKey{
				Feature:  codersdk.CryptoKeyFeatureWorkspaceApp,
				Secret:   "key2",
				Sequence: 2,
				StartsAt: now,
			}

			ff := &fakeFetcher{
				keys: []codersdk.CryptoKey{expected},
			}

			cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, ff, withClock(clock))
			require.NoError(t, err)

			got, err := cache.Signing(ctx)
			require.NoError(t, err)
			require.Equal(t, expected, got)
			require.Equal(t, 1, ff.called)
		})

		t.Run("MissesCache", func(t *testing.T) {
			t.Parallel()
			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = slogtest.Make(t, nil)
				clock  = quartz.NewMock(t)
			)

			ff := &fakeFetcher{
				keys: []codersdk.CryptoKey{},
			}

			cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, ff, withClock(clock))
			require.NoError(t, err)

			expected := codersdk.CryptoKey{
				Feature:  codersdk.CryptoKeyFeatureWorkspaceApp,
				Secret:   "key1",
				Sequence: 12,
				StartsAt: clock.Now().UTC(),
			}
			ff.keys = []codersdk.CryptoKey{expected}

			got, err := cache.Signing(ctx)
			require.NoError(t, err)
			require.Equal(t, expected, got)
			// 1 on startup + missing cache.
			require.Equal(t, 2, ff.called)

			// Ensure the cache gets hit this time.
			got, err = cache.Signing(ctx)
			require.NoError(t, err)
			require.Equal(t, expected, got)
			// 1 on startup + missing cache.
			require.Equal(t, 2, ff.called)
		})

		t.Run("IgnoresInvalid", func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = slogtest.Make(t, nil)
				clock  = quartz.NewMock(t)
			)
			now := clock.Now().UTC()
			expected := codersdk.CryptoKey{
				Feature:  codersdk.CryptoKeyFeatureWorkspaceApp,
				Secret:   "key1",
				Sequence: 1,
				StartsAt: clock.Now().UTC(),
			}

			ff := &fakeFetcher{
				keys: []codersdk.CryptoKey{
					expected,
					{
						Feature:   codersdk.CryptoKeyFeatureWorkspaceApp,
						Secret:    "key2",
						Sequence:  2,
						StartsAt:  now.Add(-time.Second),
						DeletesAt: now,
					},
				},
			}

			cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, ff, withClock(clock))
			require.NoError(t, err)

			got, err := cache.Signing(ctx)
			require.NoError(t, err)
			require.Equal(t, expected, got)
			require.Equal(t, 1, ff.called)
		})

		t.Run("KeyNotFound", func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = slogtest.Make(t, nil)
				clock  = quartz.NewMock(t)
			)

			ff := &fakeFetcher{
				keys: []codersdk.CryptoKey{},
			}

			cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, ff, withClock(clock))
			require.NoError(t, err)

			_, err = cache.Signing(ctx)
			require.ErrorIs(t, err, cryptokeys.ErrKeyNotFound)
		})
	})

	t.Run("Verifying", func(t *testing.T) {
		t.Parallel()

		t.Run("HitsCache", func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = slogtest.Make(t, nil)
				clock  = quartz.NewMock(t)
			)

			now := clock.Now().UTC()
			expected := codersdk.CryptoKey{
				Feature:  codersdk.CryptoKeyFeatureWorkspaceApp,
				Secret:   "key1",
				Sequence: 12,
				StartsAt: now,
			}
			ff := &fakeFetcher{
				keys: []codersdk.CryptoKey{
					expected,
					{
						Feature:  codersdk.CryptoKeyFeatureWorkspaceApp,
						Secret:   "key2",
						Sequence: 13,
						StartsAt: now,
					},
				},
			}

			cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, ff, withClock(clock))
			require.NoError(t, err)

			got, err := cache.Verifying(ctx, expected.Sequence)
			require.NoError(t, err)
			require.Equal(t, expected, got)
			require.Equal(t, 1, ff.called)
		})

		t.Run("MissesCache", func(t *testing.T) {
			t.Parallel()
			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = slogtest.Make(t, nil)
				clock  = quartz.NewMock(t)
			)

			ff := &fakeFetcher{
				keys: []codersdk.CryptoKey{},
			}

			cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, ff, withClock(clock))
			require.NoError(t, err)

			expected := codersdk.CryptoKey{
				Feature:  codersdk.CryptoKeyFeatureWorkspaceApp,
				Secret:   "key1",
				Sequence: 12,
				StartsAt: clock.Now().UTC(),
			}
			ff.keys = []codersdk.CryptoKey{expected}

			got, err := cache.Verifying(ctx, expected.Sequence)
			require.NoError(t, err)
			require.Equal(t, expected, got)
			require.Equal(t, 2, ff.called)

			// Ensure the cache gets hit this time.
			got, err = cache.Verifying(ctx, expected.Sequence)
			require.NoError(t, err)
			require.Equal(t, expected, got)
			require.Equal(t, 2, ff.called)
		})

		t.Run("AllowsBeforeStartsAt", func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = slogtest.Make(t, nil)
				clock  = quartz.NewMock(t)
			)

			now := clock.Now().UTC()
			expected := codersdk.CryptoKey{
				Feature:  codersdk.CryptoKeyFeatureWorkspaceApp,
				Secret:   "key1",
				Sequence: 12,
				StartsAt: now.Add(-time.Second),
			}

			ff := &fakeFetcher{
				keys: []codersdk.CryptoKey{
					expected,
				},
			}

			cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, ff, withClock(clock))
			require.NoError(t, err)

			got, err := cache.Verifying(ctx, expected.Sequence)
			require.NoError(t, err)
			require.Equal(t, expected, got)
			require.Equal(t, 1, ff.called)
		})

		t.Run("KeyInvalid", func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = slogtest.Make(t, nil)
				clock  = quartz.NewMock(t)
			)

			now := clock.Now().UTC()
			expected := codersdk.CryptoKey{
				Feature:   codersdk.CryptoKeyFeatureWorkspaceApp,
				Secret:    "key1",
				Sequence:  12,
				StartsAt:  now.Add(-time.Second),
				DeletesAt: now,
			}

			ff := &fakeFetcher{
				keys: []codersdk.CryptoKey{
					expected,
				},
			}

			cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, ff, withClock(clock))
			require.NoError(t, err)

			_, err = cache.Verifying(ctx, expected.Sequence)
			require.ErrorIs(t, err, cryptokeys.ErrKeyInvalid)
			require.Equal(t, 1, ff.called)
		})

		t.Run("KeyNotFound", func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = slogtest.Make(t, nil)
				clock  = quartz.NewMock(t)
			)

			ff := &fakeFetcher{
				keys: []codersdk.CryptoKey{},
			}

			cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, ff, withClock(clock))
			require.NoError(t, err)

			_, err = cache.Verifying(ctx, 1)
			require.ErrorIs(t, err, cryptokeys.ErrKeyNotFound)
		})
	})

	t.Run("CacheRefreshes", func(t *testing.T) {
		t.Parallel()

		var (
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = slogtest.Make(t, nil)
			clock  = quartz.NewMock(t)
		)

		now := clock.Now().UTC()
		expected := codersdk.CryptoKey{
			Feature:   codersdk.CryptoKeyFeatureWorkspaceApp,
			Secret:    "key1",
			Sequence:  12,
			StartsAt:  now,
			DeletesAt: now.Add(time.Minute * 10),
		}
		ff := &fakeFetcher{
			keys: []codersdk.CryptoKey{
				expected,
			},
		}

		cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, ff, withClock(clock))
		require.NoError(t, err)

		got, err := cache.Signing(ctx)
		require.NoError(t, err)
		require.Equal(t, expected, got)
		require.Equal(t, 1, ff.called)

		newKey := codersdk.CryptoKey{
			Feature:  codersdk.CryptoKeyFeatureWorkspaceApp,
			Secret:   "key2",
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
		got, err = cache.Signing(ctx)
		require.NoError(t, err)
		require.Equal(t, newKey, got)
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
			logger = slogtest.Make(t, nil)
			clock  = quartz.NewMock(t)
		)

		now := clock.Now().UTC()
		expected := codersdk.CryptoKey{
			Feature:   codersdk.CryptoKeyFeatureWorkspaceApp,
			Secret:    "key1",
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
		cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, ff, withClock(clock))
		require.NoError(t, err)

		_, wait := clock.AdvanceNext()
		trapped := trap.MustWait(ctx)

		newKey := codersdk.CryptoKey{
			Feature:  codersdk.CryptoKeyFeatureWorkspaceApp,
			Secret:   "key2",
			Sequence: 13,
			StartsAt: now,
		}
		ff.keys = []codersdk.CryptoKey{newKey}

		_, err = cache.Verifying(ctx, newKey.Sequence)
		require.NoError(t, err)
		require.Equal(t, 2, ff.called)

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
			logger = slogtest.Make(t, nil)
			clock  = quartz.NewMock(t)
		)

		now := clock.Now()
		expected := codersdk.CryptoKey{
			Feature:  codersdk.CryptoKeyFeatureWorkspaceApp,
			Secret:   "key1",
			Sequence: 12,
			StartsAt: now,
		}
		ff := &fakeFetcher{
			keys: []codersdk.CryptoKey{
				expected,
			},
		}

		cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, ff, withClock(clock))
		require.NoError(t, err)

		got, err := cache.Signing(ctx)
		require.NoError(t, err)
		require.Equal(t, expected, got)
		require.Equal(t, 1, ff.called)

		got, err = cache.Verifying(ctx, expected.Sequence)
		require.NoError(t, err)
		require.Equal(t, expected, got)
		require.Equal(t, 1, ff.called)

		cache.Close()

		_, err = cache.Signing(ctx)
		require.ErrorIs(t, err, cryptokeys.ErrClosed)

		_, err = cache.Verifying(ctx, expected.Sequence)
		require.ErrorIs(t, err, cryptokeys.ErrClosed)
	})
}

type fakeFetcher struct {
	keys   []codersdk.CryptoKey
	called int
}

func (f *fakeFetcher) Fetch(_ context.Context) ([]codersdk.CryptoKey, error) {
	f.called++
	return f.keys, nil
}

func withClock(clock quartz.Clock) func(*wsproxy.CryptoKeyCache) {
	return func(cache *wsproxy.CryptoKeyCache) {
		cache.Clock = clock
	}
}

package wsproxy_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/wsproxy"
	"github.com/coder/coder/v2/enterprise/wsproxy/wsproxysdk"
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

			fc := newFakeCoderd(t, []codersdk.CryptoKey{
				{
					Feature:  codersdk.CryptoKeyFeatureWorkspaceApp,
					Secret:   "key1",
					Sequence: 1,
					StartsAt: now,
				},
				// Should be ignored since it hasn't breached its starts_at time yet.
				{
					Feature:  codersdk.CryptoKeyFeatureWorkspaceApp,
					Secret:   "key3",
					Sequence: 3,
					StartsAt: now.Add(time.Second * 2),
				},
				expected,
			})

			cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, wsproxysdk.New(fc.url), withClock(clock))
			require.NoError(t, err)

			got, err := cache.Signing(ctx)
			require.NoError(t, err)
			require.Equal(t, expected, got)
			require.Equal(t, 1, fc.called)
		})

		t.Run("MissesCache", func(t *testing.T) {
			t.Parallel()
			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = slogtest.Make(t, nil)
				clock  = quartz.NewMock(t)
			)

			fc := newFakeCoderd(t, []codersdk.CryptoKey{})

			cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, wsproxysdk.New(fc.url), withClock(clock))
			require.NoError(t, err)

			expected := codersdk.CryptoKey{
				Feature:  codersdk.CryptoKeyFeatureWorkspaceApp,
				Secret:   "key1",
				Sequence: 12,
				StartsAt: clock.Now().UTC(),
			}
			fc.keys = []codersdk.CryptoKey{expected}

			got, err := cache.Signing(ctx)
			require.NoError(t, err)
			require.Equal(t, expected, got)
			// 1 on startup + missing cache.
			require.Equal(t, 2, fc.called)

			// Ensure the cache gets hit this time.
			got, err = cache.Signing(ctx)
			require.NoError(t, err)
			require.Equal(t, expected, got)
			// 1 on startup + missing cache.
			require.Equal(t, 2, fc.called)
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

			fc := newFakeCoderd(t, []codersdk.CryptoKey{
				expected,
				{
					Feature:   codersdk.CryptoKeyFeatureWorkspaceApp,
					Secret:    "key2",
					Sequence:  2,
					StartsAt:  now.Add(-time.Second),
					DeletesAt: now,
				},
			})

			cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, wsproxysdk.New(fc.url), withClock(clock))
			require.NoError(t, err)

			got, err := cache.Signing(ctx)
			require.NoError(t, err)
			require.Equal(t, expected, got)
			require.Equal(t, 1, fc.called)
		})

		t.Run("KeyNotFound", func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = slogtest.Make(t, nil)
				clock  = quartz.NewMock(t)
			)

			fc := newFakeCoderd(t, []codersdk.CryptoKey{})

			cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, wsproxysdk.New(fc.url), withClock(clock))
			require.NoError(t, err)

			_, err = cache.Verifying(ctx, 1)
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
			fc := newFakeCoderd(t, []codersdk.CryptoKey{
				expected,
				{
					Feature:  codersdk.CryptoKeyFeatureWorkspaceApp,
					Secret:   "key2",
					Sequence: 13,
					StartsAt: now,
				},
			})

			cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, wsproxysdk.New(fc.url), withClock(clock))
			require.NoError(t, err)

			got, err := cache.Verifying(ctx, expected.Sequence)
			require.NoError(t, err)
			require.Equal(t, expected, got)
			require.Equal(t, 1, fc.called)
		})

		t.Run("MissesCache", func(t *testing.T) {
			t.Parallel()
			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = slogtest.Make(t, nil)
				clock  = quartz.NewMock(t)
			)

			fc := newFakeCoderd(t, []codersdk.CryptoKey{})

			cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, wsproxysdk.New(fc.url), withClock(clock))
			require.NoError(t, err)

			expected := codersdk.CryptoKey{
				Feature:  codersdk.CryptoKeyFeatureWorkspaceApp,
				Secret:   "key1",
				Sequence: 12,
				StartsAt: clock.Now().UTC(),
			}
			fc.keys = []codersdk.CryptoKey{expected}

			got, err := cache.Verifying(ctx, expected.Sequence)
			require.NoError(t, err)
			require.Equal(t, expected, got)
			require.Equal(t, 2, fc.called)

			// Ensure the cache gets hit this time.
			got, err = cache.Verifying(ctx, expected.Sequence)
			require.NoError(t, err)
			require.Equal(t, expected, got)
			require.Equal(t, 2, fc.called)
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

			fc := newFakeCoderd(t, []codersdk.CryptoKey{
				expected,
			})

			cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, wsproxysdk.New(fc.url), withClock(clock))
			require.NoError(t, err)

			got, err := cache.Verifying(ctx, expected.Sequence)
			require.NoError(t, err)
			require.Equal(t, expected, got)
			require.Equal(t, 1, fc.called)
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

			fc := newFakeCoderd(t, []codersdk.CryptoKey{
				expected,
			})

			cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, wsproxysdk.New(fc.url), withClock(clock))
			require.NoError(t, err)

			_, err = cache.Verifying(ctx, expected.Sequence)
			require.ErrorIs(t, err, cryptokeys.ErrKeyInvalid)
			require.Equal(t, 1, fc.called)
		})

		t.Run("KeyNotFound", func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				logger = slogtest.Make(t, nil)
				clock  = quartz.NewMock(t)
			)

			fc := newFakeCoderd(t, []codersdk.CryptoKey{})

			cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, wsproxysdk.New(fc.url), withClock(clock))
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
		fc := newFakeCoderd(t, []codersdk.CryptoKey{
			expected,
		})

		cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, wsproxysdk.New(fc.url), withClock(clock))
		require.NoError(t, err)

		got, err := cache.Signing(ctx)
		require.NoError(t, err)
		require.Equal(t, expected, got)
		require.Equal(t, 1, fc.called)

		newKey := codersdk.CryptoKey{
			Feature:  codersdk.CryptoKeyFeatureWorkspaceApp,
			Secret:   "key2",
			Sequence: 13,
			StartsAt: now,
		}
		fc.keys = []codersdk.CryptoKey{newKey}

		// The ticker should fire and cause a request to coderd.
		dur, advance := clock.AdvanceNext()
		advance.MustWait(ctx)
		require.Equal(t, 2, fc.called)
		require.Equal(t, time.Minute*10, dur)

		// Assert hits cache.
		got, err = cache.Signing(ctx)
		require.NoError(t, err)
		require.Equal(t, newKey, got)
		require.Equal(t, 2, fc.called)

		// We check again to ensure the timer has been reset.
		_, advance = clock.AdvanceNext()
		advance.MustWait(ctx)
		require.Equal(t, 3, fc.called)
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
		fc := newFakeCoderd(t, []codersdk.CryptoKey{
			expected,
		})

		// Create a trap that blocks when the refresh timer fires.
		trap := clock.Trap().Now("refresh")
		cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, wsproxysdk.New(fc.url), withClock(clock))
		require.NoError(t, err)

		_, wait := clock.AdvanceNext()
		trapped := trap.MustWait(ctx)

		newKey := codersdk.CryptoKey{
			Feature:  codersdk.CryptoKeyFeatureWorkspaceApp,
			Secret:   "key2",
			Sequence: 13,
			StartsAt: now,
		}
		fc.keys = []codersdk.CryptoKey{newKey}

		_, err = cache.Verifying(ctx, newKey.Sequence)
		require.NoError(t, err)
		require.Equal(t, 2, fc.called)

		trapped.Release()
		wait.MustWait(ctx)
		require.Equal(t, 2, fc.called)
		trap.Close()

		// The next timer should fire in 10 minutes.
		dur, wait := clock.AdvanceNext()
		wait.MustWait(ctx)
		require.Equal(t, time.Minute*10, dur)
		require.Equal(t, 3, fc.called)
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
		fc := newFakeCoderd(t, []codersdk.CryptoKey{
			expected,
		})

		cache, err := wsproxy.NewCryptoKeyCache(ctx, logger, wsproxysdk.New(fc.url), withClock(clock))
		require.NoError(t, err)

		got, err := cache.Signing(ctx)
		require.NoError(t, err)
		require.Equal(t, expected, got)
		require.Equal(t, 1, fc.called)

		got, err = cache.Verifying(ctx, expected.Sequence)
		require.NoError(t, err)
		require.Equal(t, expected, got)
		require.Equal(t, 1, fc.called)

		cache.Close()

		_, err = cache.Signing(ctx)
		require.ErrorIs(t, err, cryptokeys.ErrClosed)

		_, err = cache.Verifying(ctx, expected.Sequence)
		require.ErrorIs(t, err, cryptokeys.ErrClosed)
	})
}

type fakeCoderd struct {
	server *httptest.Server
	keys   []codersdk.CryptoKey
	called int
	url    *url.URL
}

func newFakeCoderd(t *testing.T, keys []codersdk.CryptoKey) *fakeCoderd {
	t.Helper()

	c := &fakeCoderd{
		keys: keys,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/workspaceproxies/me/crypto-keys", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(wsproxysdk.CryptoKeysResponse{
			CryptoKeys: c.keys,
		})
		require.NoError(t, err)
		c.called++
	})

	c.server = httptest.NewServer(mux)
	t.Cleanup(c.server.Close)

	var err error
	c.url, err = url.Parse(c.server.URL)
	require.NoError(t, err)

	return c
}

func withClock(clock quartz.Clock) func(*wsproxy.CryptoKeyCache) {
	return func(cache *wsproxy.CryptoKeyCache) {
		cache.Clock = clock
	}
}

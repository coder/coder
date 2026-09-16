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
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
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
			olderKey := codersdk.CryptoKey{
				Feature:  codersdk.CryptoKeyFeatureTailnetResume,
				Secret:   generateKey(t, 64),
				Sequence: 1,
				StartsAt: now,
			}

			ff := &fakeFetcher{
				keys: []codersdk.CryptoKey{expected, olderKey},
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

		trapped.MustRelease(ctx)
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

	// A failed on-demand fetch must not leave the cache in a permanently
	// "fetching" state. Subsequent callers must be able to trigger a new fetch
	// and receive a result rather than blocking forever.
	t.Run("FetchErrorDoesNotWedge", func(t *testing.T) {
		t.Parallel()

		var (
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = testutil.Logger(t)
			clock  = quartz.NewMock(t)
		)

		now := clock.Now().UTC()
		ff := &fakeFetcher{
			keys: []codersdk.CryptoKey{},
		}

		cache, err := cryptokeys.NewSigningCache(ctx, logger, ff, codersdk.CryptoKeyFeatureTailnetResume, cryptokeys.WithCacheClock(clock))
		require.NoError(t, err)
		defer cache.Close()
		require.Equal(t, 1, ff.called)

		// Simulate a request whose fetch fails, for example a transient
		// database error.
		ff.err = xerrors.New("fetch failed")
		_, err = cache.VerifyingKey(ctx, "13")
		require.Error(t, err)
		require.Equal(t, 2, ff.called)

		// The fetcher recovers and the key now exists.
		newKey := codersdk.CryptoKey{
			Feature:  codersdk.CryptoKeyFeatureTailnetResume,
			Secret:   generateKey(t, 64),
			Sequence: 13,
			StartsAt: now,
		}
		ff.err = nil
		ff.keys = []codersdk.CryptoKey{newKey}

		done := make(chan fetchResult, 1)
		go func() {
			key, err := cache.VerifyingKey(ctx, keyID(newKey))
			done <- fetchResult{key: key, err: err}
		}()

		// Fails with "context expired" if the cache is wedged.
		res := testutil.TryReceive(ctx, t, done)
		require.NoError(t, res.err)
		require.Equal(t, decodedSecret(t, newKey), res.key)
		require.Equal(t, 3, ff.called)
	})

	// A failed background refresh must not leave the cache in a permanently
	// "fetching" state, and the refresh timer must be re-armed so the cache
	// recovers on its own.
	t.Run("RefreshErrorDoesNotWedge", func(t *testing.T) {
		t.Parallel()

		var (
			ctx = testutil.Context(t, testutil.WaitShort)
			// The failed refresh logs at error level by design.
			logger = slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
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
			keys: []codersdk.CryptoKey{expected},
		}

		cache, err := cryptokeys.NewSigningCache(ctx, logger, ff, codersdk.CryptoKeyFeatureTailnetResume, cryptokeys.WithCacheClock(clock))
		require.NoError(t, err)
		defer cache.Close()
		require.Equal(t, 1, ff.called)

		// The refresh timer fires and the fetch fails.
		ff.err = xerrors.New("fetch failed")
		dur, advance := clock.AdvanceNext()
		advance.MustWait(ctx)
		require.Equal(t, time.Minute*10, dur)
		require.Equal(t, 2, ff.called)

		// The timer must be re-armed despite the failure.
		_, ok := clock.Peek()
		require.True(t, ok, "refresh timer was not re-armed after a failed refresh")

		// Cached keys remain usable.
		ff.err = nil
		key, err := cache.VerifyingKey(ctx, keyID(expected))
		require.NoError(t, err)
		require.Equal(t, decodedSecret(t, expected), key)

		// An uncached key must trigger a new fetch rather than block forever.
		newKey := codersdk.CryptoKey{
			Feature:  codersdk.CryptoKeyFeatureTailnetResume,
			Secret:   generateKey(t, 64),
			Sequence: 13,
			StartsAt: now,
		}
		ff.keys = []codersdk.CryptoKey{expected, newKey}

		done := make(chan fetchResult, 1)
		go func() {
			key, err := cache.VerifyingKey(ctx, keyID(newKey))
			done <- fetchResult{key: key, err: err}
		}()

		// Fails with "context expired" if the cache is wedged.
		res := testutil.TryReceive(ctx, t, done)
		require.NoError(t, res.err)
		require.Equal(t, decodedSecret(t, newKey), res.key)
		require.Equal(t, 3, ff.called)
	})

	// A waiter blocked on another caller's fetch must be woken when that fetch
	// fails, retry the fetch itself, and succeed. Only the caller whose fetch
	// failed observes the error.
	t.Run("WaiterRetriesAfterFetchError", func(t *testing.T) {
		t.Parallel()

		var (
			ctx    = testutil.Context(t, testutil.WaitShort)
			logger = testutil.Logger(t)
			clock  = quartz.NewMock(t)
		)

		bf := newBlockingFetcher()
		// Initial fetch in the constructor returns no keys.
		bf.results <- fetchResult{}
		cache, err := cryptokeys.NewSigningCache(ctx, logger, bf, codersdk.CryptoKeyFeatureTailnetResume, cryptokeys.WithCacheClock(clock))
		require.NoError(t, err)
		defer cache.Close()
		testutil.RequireReceive(ctx, t, bf.started)

		// Caller A misses and starts a fetch that we hold open.
		aDone := make(chan error, 1)
		go func() {
			_, err := cache.VerifyingKey(ctx, "13")
			aDone <- err
		}()
		testutil.RequireReceive(ctx, t, bf.started)

		// Caller B misses and waits on A's fetch. SigningKey reads the clock
		// under the cache mutex right before waiting, so trapping that call
		// guarantees B holds the mutex until it enters cond.Wait. A cannot
		// re-acquire the mutex to finish until B is waiting.
		trap := clock.Trap().Now()
		bDone := make(chan fetchResult, 1)
		go func() {
			_, key, err := cache.SigningKey(ctx)
			bDone <- fetchResult{key: key, err: err}
		}()
		trap.MustWait(ctx).MustRelease(ctx)
		trap.Close()

		// A's fetch fails.
		bf.results <- fetchResult{err: xerrors.New("fetch failed")}
		require.Error(t, testutil.RequireReceive(ctx, t, aDone))

		// B must wake, retry the fetch, and get the key.
		testutil.RequireReceive(ctx, t, bf.started)
		expected := codersdk.CryptoKey{
			Feature:  codersdk.CryptoKeyFeatureTailnetResume,
			Secret:   generateKey(t, 64),
			Sequence: 13,
			StartsAt: clock.Now().UTC(),
		}
		bf.results <- fetchResult{keys: []codersdk.CryptoKey{expected}}
		res := testutil.RequireReceive(ctx, t, bDone)
		require.NoError(t, res.err)
		require.Equal(t, decodedSecret(t, expected), res.key)
	})
}

type fakeFetcher struct {
	keys   []codersdk.CryptoKey
	err    error
	called int
}

func (f *fakeFetcher) Fetch(_ context.Context, _ codersdk.CryptoKeyFeature) ([]codersdk.CryptoKey, error) {
	f.called++
	if f.err != nil {
		return nil, f.err
	}
	return f.keys, nil
}

type fetchResult struct {
	keys []codersdk.CryptoKey
	key  interface{}
	err  error
}

// blockingFetcher signals on started for each Fetch call and blocks until the
// test supplies a result, letting tests interleave concurrent callers.
type blockingFetcher struct {
	started chan struct{}
	results chan fetchResult
}

func newBlockingFetcher() *blockingFetcher {
	return &blockingFetcher{
		started: make(chan struct{}, 8),
		results: make(chan fetchResult, 1),
	}
}

func (f *blockingFetcher) Fetch(ctx context.Context, _ codersdk.CryptoKeyFeature) ([]codersdk.CryptoKey, error) {
	f.started <- struct{}{}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-f.results:
		return res.keys, res.err
	}
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

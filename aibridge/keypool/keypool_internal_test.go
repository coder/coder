package keypool

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/metrics"
	codertestutil "github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestNewKeyPool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		keys         []string
		expectedKeys []string
		expectedErr  error
	}{
		{"nil_keys", nil, nil, ErrNoKeys},
		{"empty_keys", []string{}, nil, ErrNoKeys},
		{"single_key", []string{"key-0"}, []string{"key-0"}, nil},
		{"multiple_keys", []string{"key-0", "key-1", "key-2"}, []string{"key-0", "key-1", "key-2"}, nil},
		{"duplicate_keys", []string{"key-0", "key-1", "key-0"}, nil, ErrDuplicateKey},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool, err := New("test-provider", tc.keys, quartz.NewMock(t), nil)
			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, pool)

			// Verify all keys are returned in order and valid.
			walker := pool.Walker()
			for _, expected := range tc.expectedKeys {
				key, keyPoolErr := walker.Next()
				require.Nil(t, keyPoolErr)
				assert.Equal(t, expected, key.Value())
				assert.Equal(t, KeyStateValid, key.State())
			}

			// No more keys available.
			_, keyPoolErr := walker.Next()
			require.Equal(t, &Error{Kind: ErrorKindRateLimited}, keyPoolErr, "expected rate-limited exhaustion after walker returned every valid key")
		})
	}
}

func TestState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		setup         func(t *testing.T, pool *Pool, clk *quartz.Mock) *Key
		expectedState KeyState
	}{
		{
			// Fresh key is valid.
			name: "fresh_key_is_valid",
			setup: func(t *testing.T, pool *Pool, _ *quartz.Mock) *Key {
				key, keyPoolErr := pool.Walker().Next()
				require.Nil(t, keyPoolErr)
				return key
			},
			expectedState: KeyStateValid,
		},
		{
			// Active cooldown makes the key temporary.
			name: "active_cooldown_is_temporary",
			setup: func(t *testing.T, pool *Pool, _ *quartz.Mock) *Key {
				key, keyPoolErr := pool.Walker().Next()
				require.Nil(t, keyPoolErr)
				key.markTemporary(60 * time.Second)
				return key
			},
			expectedState: KeyStateTemporary,
		},
		{
			// Expired cooldown returns the key to valid.
			name: "expired_cooldown_is_valid",
			setup: func(t *testing.T, pool *Pool, clk *quartz.Mock) *Key {
				key, keyPoolErr := pool.Walker().Next()
				require.Nil(t, keyPoolErr)
				key.markTemporary(30 * time.Second)
				clk.Advance(35 * time.Second)
				return key
			},
			expectedState: KeyStateValid,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clk := quartz.NewMock(t)
			pool, err := New("test-provider", []string{"key-0"}, clk, nil)
			require.NoError(t, err)

			key := tc.setup(t, pool, clk)

			assert.Equal(t, tc.expectedState, key.State())
		})
	}
}

func TestMarkTemporary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		cooldown           time.Duration
		setup              func(t *testing.T, pool *Pool, clk *quartz.Mock) *Key
		expectedState      KeyState
		expectedTransition bool
	}{
		{
			// valid -> temporary: key becomes unavailable.
			name:     "valid_to_temporary",
			cooldown: 60 * time.Second,
			setup: func(t *testing.T, pool *Pool, _ *quartz.Mock) *Key {
				key, keyPoolErr := pool.Walker().Next()
				require.Nil(t, keyPoolErr)
				return key
			},
			expectedState:      KeyStateTemporary,
			expectedTransition: true,
		},
		{
			// temporary -> temporary: new cooldown is longer,
			// so the deadline is extended.
			name:     "temporary_to_temporary_extends_cooldown",
			cooldown: 60 * time.Second,
			setup: func(t *testing.T, pool *Pool, _ *quartz.Mock) *Key {
				key, keyPoolErr := pool.Walker().Next()
				require.Nil(t, keyPoolErr)
				key.markTemporary(10 * time.Second)
				return key
			},
			expectedState:      KeyStateTemporary,
			expectedTransition: false,
		},
		{
			// temporary -> temporary: new cooldown is shorter,
			// so the existing longer deadline is preserved.
			name:     "temporary_to_temporary_keeps_longer_cooldown",
			cooldown: 10 * time.Second,
			setup: func(t *testing.T, pool *Pool, _ *quartz.Mock) *Key {
				key, keyPoolErr := pool.Walker().Next()
				require.Nil(t, keyPoolErr)
				key.markTemporary(60 * time.Second)
				return key
			},
			expectedState:      KeyStateTemporary,
			expectedTransition: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clk := quartz.NewMock(t)
			pool, err := New("test-provider", []string{"key-0", "key-1"}, clk, nil)
			require.NoError(t, err)

			key := tc.setup(t, pool, clk)
			transition := key.markTemporary(tc.cooldown)

			assert.Equal(t, tc.expectedState, key.State())
			assert.Equal(t, tc.expectedTransition, transition)
		})
	}
}

// markNextByStatus walks to the next available key and cools it down via
// the given HTTP status (401 or 429), so the resulting cooldown carries the
// same reason the failover path records at runtime.
func markNextByStatus(t *testing.T, pool *Pool, walker *Walker, status int) {
	t.Helper()
	key, keyPoolErr := walker.Next()
	require.Nil(t, keyPoolErr)
	resp := &http.Response{StatusCode: status, Header: make(http.Header)}
	pool.MarkKeyOnStatus(context.Background(), key, resp, slogtest.Make(t, nil))
}

func TestWalkerNext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		keys          []string
		setup         func(t *testing.T, pool *Pool)
		advance       time.Duration
		expectedValid []string
		expectedErr   *Error
	}{
		{
			// Given: key-0: valid, key-1: valid, key-2: valid.
			// Then: key-0: valid, key-1: valid, key-2: valid.
			name:          "all_keys_valid",
			keys:          []string{"key-0", "key-1", "key-2"},
			setup:         func(_ *testing.T, _ *Pool) {},
			expectedValid: []string{"key-0", "key-1", "key-2"},
			expectedErr:   &Error{Kind: ErrorKindRateLimited},
		},
		{
			// Given: key-0: temporary, key-1: valid, key-2: valid.
			// Then: key-0: temporary, key-1: valid, key-2: valid.
			name: "skips_temporary_keys",
			keys: []string{"key-0", "key-1", "key-2"},
			setup: func(t *testing.T, pool *Pool) {
				key, keyPoolErr := pool.Walker().Next()
				require.Nil(t, keyPoolErr)
				key.markTemporary(60 * time.Second)
			},
			expectedValid: []string{"key-1", "key-2"},
			expectedErr:   &Error{Kind: ErrorKindRateLimited},
		},
		{
			// Given: key-0: temporary (30s), key-1: valid.
			// When: 35s pass.
			// Then: key-0: valid, key-1: valid.
			name: "expired_temporary_is_available",
			keys: []string{"key-0", "key-1"},
			setup: func(t *testing.T, pool *Pool) {
				key, keyPoolErr := pool.Walker().Next()
				require.Nil(t, keyPoolErr)
				key.markTemporary(30 * time.Second)
			},
			advance:       35 * time.Second,
			expectedValid: []string{"key-0", "key-1"},
			expectedErr:   &Error{Kind: ErrorKindRateLimited},
		},
		{
			// Given: key-0: temporary (zero, default 60s), key-1: valid.
			// When: 50s pass.
			// Then: key-0: temporary, key-1: valid.
			name: "default_cooldown_not_expired",
			keys: []string{"key-0", "key-1"},
			setup: func(t *testing.T, pool *Pool) {
				key, keyPoolErr := pool.Walker().Next()
				require.Nil(t, keyPoolErr)
				key.markTemporary(0)
			},
			advance:       50 * time.Second,
			expectedValid: []string{"key-1"},
			expectedErr:   &Error{Kind: ErrorKindRateLimited},
		},
		{
			// Given: key-0: temporary (zero, default 60s), key-1: valid.
			// When: 65s pass.
			// Then: key-0: valid, key-1: valid.
			name: "default_cooldown_expired",
			keys: []string{"key-0", "key-1"},
			setup: func(t *testing.T, pool *Pool) {
				key, keyPoolErr := pool.Walker().Next()
				require.Nil(t, keyPoolErr)
				key.markTemporary(0)
			},
			advance:       65 * time.Second,
			expectedValid: []string{"key-0", "key-1"},
			expectedErr:   &Error{Kind: ErrorKindRateLimited},
		},
		{
			// Given: key-0: temporary (negative, default 60s), key-1: valid.
			// When: 65s pass.
			// Then: key-0: valid, key-1: valid.
			name: "negative_cooldown_uses_default",
			keys: []string{"key-0", "key-1"},
			setup: func(t *testing.T, pool *Pool) {
				key, keyPoolErr := pool.Walker().Next()
				require.Nil(t, keyPoolErr)
				key.markTemporary(-10 * time.Second)
			},
			advance:       65 * time.Second,
			expectedValid: []string{"key-0", "key-1"},
			expectedErr:   &Error{Kind: ErrorKindRateLimited},
		},
		{
			// Given: key-0: temporary (60s), then marked again with shorter cooldown (10s).
			// When: 15s pass (past 10s, but not 60s).
			// Then: key-0: temporary, 45s remaining.
			name: "shorter_cooldown_preserves_longer_not_expired",
			keys: []string{"key-0"},
			setup: func(t *testing.T, pool *Pool) {
				key, keyPoolErr := pool.Walker().Next()
				require.Nil(t, keyPoolErr)
				key.markTemporary(60 * time.Second)
				key.markTemporary(10 * time.Second)
			},
			advance:       15 * time.Second,
			expectedValid: []string{},
			expectedErr:   &Error{Kind: ErrorKindRateLimited, RetryAfter: 45 * time.Second},
		},
		{
			// Given: key-0: temporary (60s), then marked again with shorter cooldown (10s).
			// When: 65s pass (past the original 60s).
			// Then: key-0: valid.
			name: "shorter_cooldown_preserves_longer_expired",
			keys: []string{"key-0"},
			setup: func(t *testing.T, pool *Pool) {
				key, keyPoolErr := pool.Walker().Next()
				require.Nil(t, keyPoolErr)
				key.markTemporary(60 * time.Second)
				key.markTemporary(10 * time.Second)
			},
			advance:       65 * time.Second,
			expectedValid: []string{"key-0"},
			expectedErr:   &Error{Kind: ErrorKindRateLimited},
		},
		{
			// Given: key-0: temporary (60s), key-1: temporary (10s), key-2: temporary (30s).
			// Then: key-0: temporary, key-1: temporary, key-2: temporary.
			// Smallest remaining cooldown is reported on exhaustion.
			name: "smallest_cooldown_across_temporary_keys",
			keys: []string{"key-0", "key-1", "key-2"},
			setup: func(t *testing.T, pool *Pool) {
				walker := pool.Walker()
				key0, keyPoolErr := walker.Next()
				require.Nil(t, keyPoolErr)
				key0.markTemporary(60 * time.Second)
				key1, keyPoolErr := walker.Next()
				require.Nil(t, keyPoolErr)
				key1.markTemporary(10 * time.Second)
				key2, keyPoolErr := walker.Next()
				require.Nil(t, keyPoolErr)
				key2.markTemporary(30 * time.Second)
			},
			expectedValid: []string{},
			expectedErr:   &Error{Kind: ErrorKindRateLimited, RetryAfter: 10 * time.Second},
		},
		{
			// Given: key-0: temporary, key-1: temporary.
			// Then: key-0: temporary, key-1: temporary.
			name: "all_temporary_exhausted",
			keys: []string{"key-0", "key-1"},
			setup: func(t *testing.T, pool *Pool) {
				walker := pool.Walker()
				key0, keyPoolErr := walker.Next()
				require.Nil(t, keyPoolErr)
				key0.markTemporary(60 * time.Second)
				key1, keyPoolErr := walker.Next()
				require.Nil(t, keyPoolErr)
				key1.markTemporary(60 * time.Second)
			},
			expectedValid: []string{},
			expectedErr:   &Error{Kind: ErrorKindRateLimited, RetryAfter: 60 * time.Second},
		},
		{
			// Given: key-0: temporary (401), key-1: temporary (401).
			// Then: key-0: temporary, key-1: temporary.
			// Every cooldown is an auth failure, so exhaustion is unauthorized.
			name: "all_unauthorized_exhausted",
			keys: []string{"key-0", "key-1"},
			setup: func(t *testing.T, pool *Pool) {
				walker := pool.Walker()
				markNextByStatus(t, pool, walker, http.StatusUnauthorized)
				markNextByStatus(t, pool, walker, http.StatusUnauthorized)
			},
			expectedValid: []string{},
			expectedErr:   &Error{Kind: ErrorKindUnauthorized, RetryAfter: 60 * time.Second},
		},
		{
			// Given: key-0: temporary (401), key-1: temporary (429).
			// Then: key-0: temporary, key-1: temporary.
			// A rate limit anywhere in the pool wins, so exhaustion is
			// rate-limited despite the auth failure.
			name: "mixed_unauthorized_and_rate_limited_exhausted",
			keys: []string{"key-0", "key-1"},
			setup: func(t *testing.T, pool *Pool) {
				walker := pool.Walker()
				markNextByStatus(t, pool, walker, http.StatusUnauthorized)
				markNextByStatus(t, pool, walker, http.StatusTooManyRequests)
			},
			expectedValid: []string{},
			expectedErr:   &Error{Kind: ErrorKindRateLimited, RetryAfter: 60 * time.Second},
		},
	}

	const providerName = "test-provider"

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clk := quartz.NewMock(t)
			reg := prometheus.NewRegistry()
			m := metrics.NewMetrics(reg)
			pool, err := New(providerName, tc.keys, clk, m)
			require.NoError(t, err)

			tc.setup(t, pool)

			// Simulate time passing between setup and the walk.
			if tc.advance > 0 {
				clk.Advance(tc.advance)
			}

			walker := pool.Walker()
			for _, expectedKey := range tc.expectedValid {
				key, keyPoolErr := walker.Next()
				require.Nil(t, keyPoolErr)
				assert.Equal(t, expectedKey, key.Value())
			}

			// After all expected keys, the walker should be exhausted.
			_, keyPoolErr := walker.Next()
			require.Equal(t, tc.expectedErr, keyPoolErr)

			// The walker hands out one attempt per valid key before
			// exhaustion.
			assert.Equal(t, len(tc.expectedValid), walker.Attempts())

			// Exhaustion records one event whose outcome reflects the error kind.
			wantOutcome := "rate_limited"
			if tc.expectedErr.Kind == ErrorKindUnauthorized {
				wantOutcome = "auth_failed"
			}
			gathered, err := reg.Gather()
			require.NoError(t, err)
			for _, outcome := range []string{"rate_limited", "auth_failed"} {
				if outcome == wantOutcome {
					assert.True(t, codertestutil.PromCounterHasValue(t, gathered, 1, "key_pool_exhaustions_total", outcome, providerName))
				} else {
					assert.False(t, codertestutil.PromCounterGathered(t, gathered, "key_pool_exhaustions_total", outcome, providerName))
				}
			}
		})
	}
}

// TestKeyConcurrent exercises the documented concurrent-safety
// contract by hammering a single key with concurrent cooldown updates
// and asserting the resulting state honors the pool's invariants.
func TestKeyConcurrent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// run is called concurrently from numGoroutines, each
		// with its own index.
		run func(idx int, key *Key)
		// verify asserts the final state. May advance the clock.
		verify func(t *testing.T, key *Key, clk *quartz.Mock)
	}{
		{
			// Half of the goroutines mark the key as temporary
			// with 60s, the other half with 10s. The longer
			// cooldown must win regardless of ordering.
			name: "longer_cooldown_wins",
			run: func(idx int, key *Key) {
				if idx%2 == 0 {
					key.markTemporary(60 * time.Second)
				} else {
					key.markTemporary(10 * time.Second)
				}
			},
			verify: func(t *testing.T, key *Key, clk *quartz.Mock) {
				// At 50s the 60s cooldown is still active.
				clk.Advance(50 * time.Second)
				assert.Equal(t, KeyStateTemporary, key.State())
				// At 65s the 60s cooldown has expired.
				clk.Advance(15 * time.Second)
				assert.Equal(t, KeyStateValid, key.State())
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			clk := quartz.NewMock(t)
			pool, err := New("test-provider", []string{"key-0"}, clk, nil)
			require.NoError(t, err)
			key, keyPoolErr := pool.Walker().Next()
			require.Nil(t, keyPoolErr)

			const numGoroutines = 10
			var wg sync.WaitGroup
			for r := range numGoroutines {
				wg.Go(func() {
					tc.run(r, key)
				})
			}
			wg.Wait()

			tc.verify(t, key, clk)
		})
	}
}

// TestWalkerIndependence simulates two requests using the same pool. The first
// request marks key-0 and key-1 temporary, then gets key-2. The second request
// sees the updated pool state and also gets key-2.
func TestWalkerIndependence(t *testing.T) {
	t.Parallel()

	clk := quartz.NewMock(t)
	pool, err := New("test-provider", []string{"key-0", "key-1", "key-2"}, clk, nil)
	require.NoError(t, err)

	walker := pool.Walker()
	for _, expected := range []string{"key-0", "key-1"} {
		key, keyPoolErr := walker.Next()
		require.Nil(t, keyPoolErr)
		assert.Equal(t, expected, key.Value())
		key.markTemporary(60 * time.Second)
	}

	key, keyPoolErr := walker.Next()
	require.Nil(t, keyPoolErr)
	assert.Equal(t, "key-2", key.Value())

	key, keyPoolErr = pool.Walker().Next()
	require.Nil(t, keyPoolErr)
	assert.Equal(t, "key-2", key.Value())
}

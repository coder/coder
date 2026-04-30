package keypool_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/keypool"
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
		{"nil_keys", nil, nil, keypool.ErrNoKeys},
		{"empty_keys", []string{}, nil, keypool.ErrNoKeys},
		{"single_key", []string{"key-0"}, []string{"key-0"}, nil},
		{"multiple_keys", []string{"key-0", "key-1", "key-2"}, []string{"key-0", "key-1", "key-2"}, nil},
		{"duplicate_keys", []string{"key-0", "key-1", "key-0"}, nil, keypool.ErrDuplicateKey},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool, err := keypool.New(tc.keys, quartz.NewMock(t))
			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, pool)

			// Verify all keys are returned in order and valid.
			walker := pool.Walker()
			for _, expected := range tc.expectedKeys {
				key, err := walker.Next()
				require.NoError(t, err)
				assert.Equal(t, expected, key.Value())
				assert.Equal(t, keypool.KeyStateValid, key.State())
			}

			// No more keys available.
			_, err = walker.Next()
			require.ErrorIs(t, err, keypool.ErrAllKeysExhausted)
		})
	}
}

func TestState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		setup         func(t *testing.T, pool *keypool.Pool, clk *quartz.Mock) *keypool.Key
		expectedState keypool.KeyState
	}{
		{
			// Fresh key is valid.
			name: "fresh_key_is_valid",
			setup: func(t *testing.T, pool *keypool.Pool, _ *quartz.Mock) *keypool.Key {
				key, err := pool.Walker().Next()
				require.NoError(t, err)
				return key
			},
			expectedState: keypool.KeyStateValid,
		},
		{
			// Active cooldown makes the key temporary.
			name: "active_cooldown_is_temporary",
			setup: func(t *testing.T, pool *keypool.Pool, _ *quartz.Mock) *keypool.Key {
				key, err := pool.Walker().Next()
				require.NoError(t, err)
				key.MarkTemporary(60 * time.Second)
				return key
			},
			expectedState: keypool.KeyStateTemporary,
		},
		{
			// Expired cooldown returns the key to valid.
			name: "expired_cooldown_is_valid",
			setup: func(t *testing.T, pool *keypool.Pool, clk *quartz.Mock) *keypool.Key {
				key, err := pool.Walker().Next()
				require.NoError(t, err)
				key.MarkTemporary(30 * time.Second)
				clk.Advance(35 * time.Second)
				return key
			},
			expectedState: keypool.KeyStateValid,
		},
		{
			// Permanent key is permanent.
			name: "permanent_key",
			setup: func(t *testing.T, pool *keypool.Pool, _ *quartz.Mock) *keypool.Key {
				key, err := pool.Walker().Next()
				require.NoError(t, err)
				key.MarkPermanent()
				return key
			},
			expectedState: keypool.KeyStatePermanent,
		},
		{
			// Permanent takes precedence over active cooldown.
			name: "permanent_with_cooldown_is_permanent",
			setup: func(t *testing.T, pool *keypool.Pool, _ *quartz.Mock) *keypool.Key {
				key, err := pool.Walker().Next()
				require.NoError(t, err)
				key.MarkTemporary(60 * time.Second)
				key.MarkPermanent()
				return key
			},
			expectedState: keypool.KeyStatePermanent,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clk := quartz.NewMock(t)
			pool, err := keypool.New([]string{"key-0"}, clk)
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
		setup              func(t *testing.T, pool *keypool.Pool, clk *quartz.Mock) *keypool.Key
		expectedState      keypool.KeyState
		expectedTransition bool
	}{
		{
			// valid -> temporary: key becomes unavailable.
			name:     "valid_to_temporary",
			cooldown: 60 * time.Second,
			setup: func(t *testing.T, pool *keypool.Pool, _ *quartz.Mock) *keypool.Key {
				key, err := pool.Walker().Next()
				require.NoError(t, err)
				return key
			},
			expectedState:      keypool.KeyStateTemporary,
			expectedTransition: true,
		},
		{
			// temporary -> temporary: new cooldown is longer,
			// so the deadline is extended.
			name:     "temporary_to_temporary_extends_cooldown",
			cooldown: 60 * time.Second,
			setup: func(t *testing.T, pool *keypool.Pool, _ *quartz.Mock) *keypool.Key {
				key, err := pool.Walker().Next()
				require.NoError(t, err)
				key.MarkTemporary(10 * time.Second)
				return key
			},
			expectedState:      keypool.KeyStateTemporary,
			expectedTransition: false,
		},
		{
			// temporary -> temporary: new cooldown is shorter,
			// so the existing longer deadline is preserved.
			name:     "temporary_to_temporary_keeps_longer_cooldown",
			cooldown: 10 * time.Second,
			setup: func(t *testing.T, pool *keypool.Pool, _ *quartz.Mock) *keypool.Key {
				key, err := pool.Walker().Next()
				require.NoError(t, err)
				key.MarkTemporary(60 * time.Second)
				return key
			},
			expectedState:      keypool.KeyStateTemporary,
			expectedTransition: false,
		},
		{
			// permanent -> permanent: no-op, permanent is irreversible.
			name:     "permanent_to_temporary_is_no_op",
			cooldown: 60 * time.Second,
			setup: func(t *testing.T, pool *keypool.Pool, _ *quartz.Mock) *keypool.Key {
				key, err := pool.Walker().Next()
				require.NoError(t, err)
				key.MarkPermanent()
				return key
			},
			expectedState:      keypool.KeyStatePermanent,
			expectedTransition: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clk := quartz.NewMock(t)
			pool, err := keypool.New([]string{"key-0", "key-1"}, clk)
			require.NoError(t, err)

			key := tc.setup(t, pool, clk)
			transition := key.MarkTemporary(tc.cooldown)

			assert.Equal(t, tc.expectedState, key.State())
			assert.Equal(t, tc.expectedTransition, transition)
		})
	}
}

func TestMarkPermanent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		setup              func(t *testing.T, pool *keypool.Pool) *keypool.Key
		expectedState      keypool.KeyState
		expectedTransition bool
	}{
		{
			// valid -> permanent: key becomes permanently unavailable.
			name: "valid_to_permanent",
			setup: func(t *testing.T, pool *keypool.Pool) *keypool.Key {
				key, err := pool.Walker().Next()
				require.NoError(t, err)
				return key
			},
			expectedState:      keypool.KeyStatePermanent,
			expectedTransition: true,
		},
		{
			// temporary -> permanent: escalation from rate limit
			// to auth failure.
			name: "temporary_to_permanent",
			setup: func(t *testing.T, pool *keypool.Pool) *keypool.Key {
				key, err := pool.Walker().Next()
				require.NoError(t, err)
				key.MarkTemporary(60 * time.Second)
				return key
			},
			expectedState:      keypool.KeyStatePermanent,
			expectedTransition: true,
		},
		{
			// permanent -> permanent: no-op, already permanent.
			name: "permanent_to_permanent",
			setup: func(t *testing.T, pool *keypool.Pool) *keypool.Key {
				key, err := pool.Walker().Next()
				require.NoError(t, err)
				key.MarkPermanent()
				return key
			},
			expectedState:      keypool.KeyStatePermanent,
			expectedTransition: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clk := quartz.NewMock(t)
			pool, err := keypool.New([]string{"key-0", "key-1"}, clk)
			require.NoError(t, err)

			key := tc.setup(t, pool)
			transition := key.MarkPermanent()

			assert.Equal(t, tc.expectedState, key.State())
			assert.Equal(t, tc.expectedTransition, transition)
		})
	}
}

func TestWalkerNext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		keys        []string
		setup       func(t *testing.T, pool *keypool.Pool)
		advance     time.Duration
		expectValid []string
	}{
		{
			// Given: key-0: valid, key-1: valid, key-2: valid.
			// Then: key-0: valid, key-1: valid, key-2: valid.
			name:        "all_keys_valid",
			keys:        []string{"key-0", "key-1", "key-2"},
			setup:       func(_ *testing.T, _ *keypool.Pool) {},
			expectValid: []string{"key-0", "key-1", "key-2"},
		},
		{
			// Given: key-0: temporary, key-1: valid, key-2: valid.
			// Then: key-0: temporary, key-1: valid, key-2: valid.
			name: "skips_temporary_keys",
			keys: []string{"key-0", "key-1", "key-2"},
			setup: func(t *testing.T, pool *keypool.Pool) {
				key, err := pool.Walker().Next()
				require.NoError(t, err)
				key.MarkTemporary(60 * time.Second)
			},
			expectValid: []string{"key-1", "key-2"},
		},
		{
			// Given: key-0: permanent, key-1: permanent, key-2: valid.
			// Then: key-0: permanent, key-1: permanent, key-2: valid.
			name: "skips_permanent_keys",
			keys: []string{"key-0", "key-1", "key-2"},
			setup: func(t *testing.T, pool *keypool.Pool) {
				walker := pool.Walker()
				key0, err := walker.Next()
				require.NoError(t, err)
				key0.MarkPermanent()
				key1, err := walker.Next()
				require.NoError(t, err)
				key1.MarkPermanent()
			},
			expectValid: []string{"key-2"},
		},
		{
			// Given: key-0: temporary (30s), key-1: valid.
			// When: 35s pass.
			// Then: key-0: valid, key-1: valid.
			name: "expired_temporary_is_available",
			keys: []string{"key-0", "key-1"},
			setup: func(t *testing.T, pool *keypool.Pool) {
				key, err := pool.Walker().Next()
				require.NoError(t, err)
				key.MarkTemporary(30 * time.Second)
			},
			advance:     35 * time.Second,
			expectValid: []string{"key-0", "key-1"},
		},
		{
			// Given: key-0: temporary (zero, default 60s), key-1: valid.
			// When: 50s pass.
			// Then: key-0: temporary, key-1: valid.
			name: "default_cooldown_not_expired",
			keys: []string{"key-0", "key-1"},
			setup: func(t *testing.T, pool *keypool.Pool) {
				key, err := pool.Walker().Next()
				require.NoError(t, err)
				key.MarkTemporary(0)
			},
			advance:     50 * time.Second,
			expectValid: []string{"key-1"},
		},
		{
			// Given: key-0: temporary (zero, default 60s), key-1: valid.
			// When: 65s pass.
			// Then: key-0: valid, key-1: valid.
			name: "default_cooldown_expired",
			keys: []string{"key-0", "key-1"},
			setup: func(t *testing.T, pool *keypool.Pool) {
				key, err := pool.Walker().Next()
				require.NoError(t, err)
				key.MarkTemporary(0)
			},
			advance:     65 * time.Second,
			expectValid: []string{"key-0", "key-1"},
		},
		{
			// Given: key-0: temporary (negative, default 60s), key-1: valid.
			// When: 65s pass.
			// Then: key-0: valid, key-1: valid.
			name: "negative_cooldown_uses_default",
			keys: []string{"key-0", "key-1"},
			setup: func(t *testing.T, pool *keypool.Pool) {
				key, err := pool.Walker().Next()
				require.NoError(t, err)
				key.MarkTemporary(-10 * time.Second)
			},
			advance:     65 * time.Second,
			expectValid: []string{"key-0", "key-1"},
		},
		{
			// Given: key-0: temporary (60s), then marked again with shorter cooldown (10s).
			// When: 15s pass (past 10s, but not 60s).
			// Then: key-0: temporary.
			name: "shorter_cooldown_preserves_longer_not_expired",
			keys: []string{"key-0"},
			setup: func(t *testing.T, pool *keypool.Pool) {
				key, err := pool.Walker().Next()
				require.NoError(t, err)
				key.MarkTemporary(60 * time.Second)
				key.MarkTemporary(10 * time.Second)
			},
			advance:     15 * time.Second,
			expectValid: []string{},
		},
		{
			// Given: key-0: temporary (60s), then marked again with shorter cooldown (10s).
			// When: 65s pass (past the original 60s).
			// Then: key-0: valid.
			name: "shorter_cooldown_preserves_longer_expired",
			keys: []string{"key-0"},
			setup: func(t *testing.T, pool *keypool.Pool) {
				key, err := pool.Walker().Next()
				require.NoError(t, err)
				key.MarkTemporary(60 * time.Second)
				key.MarkTemporary(10 * time.Second)
			},
			advance:     65 * time.Second,
			expectValid: []string{"key-0"},
		},
		{
			// Given: key-0: temporary, key-1: temporary.
			// Then: key-0: temporary, key-1: temporary.
			name: "all_temporary_exhausted",
			keys: []string{"key-0", "key-1"},
			setup: func(t *testing.T, pool *keypool.Pool) {
				walker := pool.Walker()
				key0, err := walker.Next()
				require.NoError(t, err)
				key0.MarkTemporary(60 * time.Second)
				key1, err := walker.Next()
				require.NoError(t, err)
				key1.MarkTemporary(60 * time.Second)
			},
			expectValid: []string{},
		},
		{
			// Given: key-0: permanent, key-1: permanent.
			// Then: key-0: permanent, key-1: permanent.
			name: "all_permanent_exhausted",
			keys: []string{"key-0", "key-1"},
			setup: func(t *testing.T, pool *keypool.Pool) {
				walker := pool.Walker()
				key0, err := walker.Next()
				require.NoError(t, err)
				key0.MarkPermanent()
				key1, err := walker.Next()
				require.NoError(t, err)
				key1.MarkPermanent()
			},
			expectValid: []string{},
		},
		{
			// Given: key-0: permanent, key-1: temporary, key-2: permanent.
			// Then: key-0: permanent, key-1: temporary, key-2: permanent.
			name: "mixed_states_exhausted",
			keys: []string{"key-0", "key-1", "key-2"},
			setup: func(t *testing.T, pool *keypool.Pool) {
				walker := pool.Walker()
				key0, err := walker.Next()
				require.NoError(t, err)
				key0.MarkPermanent()
				key1, err := walker.Next()
				require.NoError(t, err)
				key1.MarkTemporary(60 * time.Second)
				key2, err := walker.Next()
				require.NoError(t, err)
				key2.MarkPermanent()
			},
			expectValid: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clk := quartz.NewMock(t)
			pool, err := keypool.New(tc.keys, clk)
			require.NoError(t, err)

			tc.setup(t, pool)

			// Simulate time passing between setup and the walk.
			if tc.advance > 0 {
				clk.Advance(tc.advance)
			}

			walker := pool.Walker()
			for _, expectedKey := range tc.expectValid {
				key, err := walker.Next()
				require.NoError(t, err)
				assert.Equal(t, expectedKey, key.Value())
			}

			// After all expected keys, the walker should be exhausted.
			_, err = walker.Next()
			require.ErrorIs(t, err, keypool.ErrAllKeysExhausted)
		})
	}
}

// TestWalkerIndependence simulates two requests using the same
// pool. The first request marks key-0 temporary and key-1
// permanent, then gets key-2. The second request sees the
// updated pool state and also gets key-2.
func TestWalkerIndependence(t *testing.T) {
	t.Parallel()

	clk := quartz.NewMock(t)
	pool, err := keypool.New([]string{"key-0", "key-1", "key-2"}, clk)
	require.NoError(t, err)

	walker := pool.Walker()

	// First attempt: get key-0.
	key, err := walker.Next()
	require.NoError(t, err)
	assert.Equal(t, "key-0", key.Value())

	// Simulate 429: mark key-0 temporary.
	key.MarkTemporary(60 * time.Second)

	// Second attempt: walker advances to key-1.
	key, err = walker.Next()
	require.NoError(t, err)
	assert.Equal(t, "key-1", key.Value())

	// Simulate 401: mark key-1 permanent.
	key.MarkPermanent()

	// Third attempt: walker advances to key-2.
	key, err = walker.Next()
	require.NoError(t, err)
	assert.Equal(t, "key-2", key.Value())

	// A new walker should skip key-0 (temporary) and key-1
	// (permanent), and return key-2.
	key2, err := pool.Walker().Next()
	require.NoError(t, err)
	assert.Equal(t, "key-2", key2.Value())
}

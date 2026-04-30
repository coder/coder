package keypool_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/quartz"
)

func TestMarkKeyOnStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		statusCode       int
		headers          map[string]string
		expectedReturn   bool
		expectedState    keypool.KeyState
		expectedCooldown time.Duration
	}{
		{
			// 429 with standard Retry-After header (seconds).
			name:             "429_with_retry_after_seconds",
			statusCode:       http.StatusTooManyRequests,
			headers:          map[string]string{"Retry-After": "5"},
			expectedReturn:   true,
			expectedState:    keypool.KeyStateTemporary,
			expectedCooldown: 5 * time.Second,
		},
		{
			// 429 with retry-after-ms header (milliseconds).
			name:             "429_with_retry_after_ms",
			statusCode:       http.StatusTooManyRequests,
			headers:          map[string]string{"retry-after-ms": "1500"},
			expectedReturn:   true,
			expectedState:    keypool.KeyStateTemporary,
			expectedCooldown: 1500 * time.Millisecond,
		},
		{
			// 429 without headers falls back to default cooldown.
			name:             "429_no_headers_uses_default",
			statusCode:       http.StatusTooManyRequests,
			expectedReturn:   true,
			expectedState:    keypool.KeyStateTemporary,
			expectedCooldown: 60 * time.Second,
		},
		{
			name:           "401_marks_permanent",
			statusCode:     http.StatusUnauthorized,
			expectedReturn: true,
			expectedState:  keypool.KeyStatePermanent,
		},
		{
			name:           "403_marks_permanent",
			statusCode:     http.StatusForbidden,
			expectedReturn: true,
			expectedState:  keypool.KeyStatePermanent,
		},
		{
			name:           "200_does_not_mark",
			statusCode:     http.StatusOK,
			expectedReturn: false,
			expectedState:  keypool.KeyStateValid,
		},
		{
			name:           "500_does_not_mark",
			statusCode:     http.StatusInternalServerError,
			expectedReturn: false,
			expectedState:  keypool.KeyStateValid,
		},
		{
			// 529 is the Anthropic overloaded status, handled by
			// the circuit breaker, not key failover.
			name:           "529_does_not_mark",
			statusCode:     529,
			expectedReturn: false,
			expectedState:  keypool.KeyStateValid,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clk := quartz.NewMock(t)
			pool, err := keypool.New([]string{"key-0"}, clk)
			require.NoError(t, err)
			key, err := pool.Walker().Next()
			require.NoError(t, err)

			resp := &http.Response{Header: make(http.Header)}
			for k, v := range tc.headers {
				resp.Header.Set(k, v)
			}

			got := keypool.MarkKeyOnStatus(
				context.Background(),
				key,
				tc.statusCode,
				resp,
				// 401 and 403 cases legitimately log at error
				// level when marking a key permanent.
				slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
				"test",
			)

			assert.Equal(t, tc.expectedReturn, got)
			assert.Equal(t, tc.expectedState, key.State())

			// Verify cooldown was set to the expected duration:
			// advancing by exactly that amount returns the key
			// to valid.
			if tc.expectedCooldown > 0 {
				clk.Advance(tc.expectedCooldown)
				assert.Equal(t, keypool.KeyStateValid, key.State())
			}
		})
	}
}

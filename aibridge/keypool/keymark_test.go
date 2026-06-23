package keypool_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	codertestutil "github.com/coder/coder/v2/testutil"
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
		// expectedReason is the transition metric's reason label, or
		// empty when no transition is expected.
		expectedReason string
	}{
		{
			// 429 with standard Retry-After header (seconds).
			name:             "429_with_retry_after_seconds",
			statusCode:       http.StatusTooManyRequests,
			headers:          map[string]string{"Retry-After": "5"},
			expectedReturn:   true,
			expectedState:    keypool.KeyStateTemporary,
			expectedCooldown: 5 * time.Second,
			expectedReason:   "rate_limited",
		},
		{
			// 429 with retry-after-ms header (milliseconds).
			name:             "429_with_retry_after_ms",
			statusCode:       http.StatusTooManyRequests,
			headers:          map[string]string{"retry-after-ms": "1500"},
			expectedReturn:   true,
			expectedState:    keypool.KeyStateTemporary,
			expectedCooldown: 1500 * time.Millisecond,
			expectedReason:   "rate_limited",
		},
		{
			// 429 without headers falls back to default cooldown.
			name:             "429_no_headers_uses_default",
			statusCode:       http.StatusTooManyRequests,
			expectedReturn:   true,
			expectedState:    keypool.KeyStateTemporary,
			expectedCooldown: 60 * time.Second,
			expectedReason:   "rate_limited",
		},
		{
			name:           "401_marks_permanent",
			statusCode:     http.StatusUnauthorized,
			expectedReturn: true,
			expectedState:  keypool.KeyStatePermanent,
			expectedReason: "unauthorized",
		},
		{
			name:           "403_marks_permanent",
			statusCode:     http.StatusForbidden,
			expectedReturn: true,
			expectedState:  keypool.KeyStatePermanent,
			expectedReason: "forbidden",
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

	const providerName = "test-provider"

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clk := quartz.NewMock(t)
			reg := prometheus.NewRegistry()
			m := metrics.NewMetrics(reg)
			pool, err := keypool.New(providerName, []string{"key-0"}, clk, m)
			require.NoError(t, err)
			key, keyPoolErr := pool.Walker().Next()
			require.Nil(t, keyPoolErr)

			resp := &http.Response{
				StatusCode: tc.statusCode,
				Header:     make(http.Header),
			}
			for k, v := range tc.headers {
				resp.Header.Set(k, v)
			}

			got := pool.MarkKeyOnStatus(
				context.Background(),
				key,
				resp,
				// 401 and 403 cases legitimately log at error
				// level when marking a key permanent.
				slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			)

			assert.Equal(t, tc.expectedReturn, got)
			assert.Equal(t, tc.expectedState, key.State())

			gathered, err := reg.Gather()
			require.NoError(t, err)
			// A state transition records one event under its reason,
			// and other reasons record none.
			for _, reason := range []string{"rate_limited", "unauthorized", "forbidden"} {
				if reason == tc.expectedReason {
					assert.True(t, codertestutil.PromCounterHasValue(t, gathered, 1, "key_pool_state_transitions_total", providerName, reason))
				} else {
					assert.False(t, codertestutil.PromCounterGathered(t, gathered, "key_pool_state_transitions_total", providerName, reason))
				}
			}

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

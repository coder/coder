package intercept_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/keypool"
)

func TestResponseErrorFromKeyPool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		keyPoolErr         *keypool.Error
		expectedStatus     int
		expectedRetryAfter time.Duration
	}{
		{
			// Permanent: 502 api_error.
			name:           "permanent_returns_502",
			keyPoolErr:     &keypool.Error{Kind: keypool.ErrorKindPermanent},
			expectedStatus: http.StatusBadGateway,
		},
		{
			// Rate-limited with no cooldown: 429, no Retry-After.
			name:               "rate_limited_zero_retry_after",
			keyPoolErr:         &keypool.Error{Kind: keypool.ErrorKindRateLimited},
			expectedStatus:     http.StatusTooManyRequests,
			expectedRetryAfter: 0,
		},
		{
			// Rate-limited with cooldown: 429, Retry-After set.
			name:               "rate_limited_with_retry_after",
			keyPoolErr:         &keypool.Error{Kind: keypool.ErrorKindRateLimited, RetryAfter: 5 * time.Second},
			expectedStatus:     http.StatusTooManyRequests,
			expectedRetryAfter: 5 * time.Second,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := intercept.ResponseErrorFromKeyPool(tc.keyPoolErr)
			require.NotNil(t, got)
			assert.Equal(t, tc.expectedStatus, got.StatusCode)
			assert.Equal(t, tc.expectedRetryAfter, got.RetryAfter)
		})
	}
}

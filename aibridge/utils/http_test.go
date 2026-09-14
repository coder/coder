package utils_test

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/utils"
)

func TestNewJSONErrorResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		retryAfter time.Duration
		body       []byte
		// Empty string means the header should be absent.
		expectRetryAfter string
	}{
		{
			// Permanent exhaustion: 502 with no Retry-After.
			name:             "permanent_no_retry_after",
			status:           http.StatusBadGateway,
			retryAfter:       0,
			body:             []byte(`{"error":"permanent"}`),
			expectRetryAfter: "",
		},
		{
			// Transient exhaustion with zero retryAfter: no Retry-After.
			name:             "transient_no_retry_after",
			status:           http.StatusTooManyRequests,
			retryAfter:       0,
			body:             []byte(`{"error":"rate"}`),
			expectRetryAfter: "",
		},
		{
			// Transient exhaustion: 429 with Retry-After in seconds.
			name:             "transient_with_retry_after",
			status:           http.StatusTooManyRequests,
			retryAfter:       60 * time.Second,
			body:             []byte(`{"error":"rate"}`),
			expectRetryAfter: "60",
		},
		{
			// Transient exhaustion with negative retryAfter: Retry-After header omitted.
			name:             "transient_negative_retry_after",
			status:           http.StatusTooManyRequests,
			retryAfter:       -1 * time.Second,
			body:             []byte(`{"error":"rate"}`),
			expectRetryAfter: "",
		},
		{
			// Transient exhaustion with 500ms retryAfter rounds up to Retry-After: 1.
			name:             "transient_under_one_second_rounds_up",
			status:           http.StatusTooManyRequests,
			retryAfter:       500 * time.Millisecond,
			body:             []byte(`{"error":"rate"}`),
			expectRetryAfter: "1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp := utils.NewJSONErrorResponse(tc.status, tc.retryAfter, tc.body)
			require.NotNil(t, resp)

			assert.Equal(t, tc.status, resp.StatusCode)
			assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			assert.Equal(t, int64(len(tc.body)), resp.ContentLength)

			if tc.expectRetryAfter == "" {
				assert.Empty(t, resp.Header.Get("Retry-After"))
			} else {
				assert.Equal(t, tc.expectRetryAfter, resp.Header.Get("Retry-After"))
			}

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			assert.Equal(t, tc.body, body)
		})
	}
}

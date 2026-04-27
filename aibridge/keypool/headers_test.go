package keypool_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/aibridge/keypool"
)

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		headers  map[string]string
		expected time.Duration
	}{
		{
			name:     "no_headers",
			headers:  nil,
			expected: 0,
		},
		{
			name:     "standard_retry_after_seconds",
			headers:  map[string]string{"Retry-After": "60"},
			expected: 60 * time.Second,
		},
		{
			name:     "openai_retry_after_ms",
			headers:  map[string]string{"retry-after-ms": "2500"},
			expected: 2500 * time.Millisecond,
		},
		{
			name: "prefers_retry_after_ms_over_standard",
			headers: map[string]string{
				"retry-after-ms": "1500",
				"Retry-After":    "30",
			},
			expected: 1500 * time.Millisecond,
		},
		{
			name:     "falls_back_to_standard_when_ms_invalid",
			headers:  map[string]string{"retry-after-ms": "invalid", "Retry-After": "10"},
			expected: 10 * time.Second,
		},
		{
			name:     "zero_seconds_returns_zero",
			headers:  map[string]string{"Retry-After": "0"},
			expected: 0,
		},
		{
			name:     "negative_seconds_returns_zero",
			headers:  map[string]string{"Retry-After": "-5"},
			expected: 0,
		},
		{
			name:     "negative_ms_returns_zero",
			headers:  map[string]string{"retry-after-ms": "-100"},
			expected: 0,
		},
		{
			name:     "whitespace_trimmed",
			headers:  map[string]string{"Retry-After": " 30 "},
			expected: 30 * time.Second,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resp := &http.Response{Header: make(http.Header)}
			for key, val := range tc.headers {
				resp.Header.Set(key, val)
			}
			assert.Equal(t, tc.expected, keypool.ParseRetryAfter(resp))
		})
	}

	t.Run("nil_response", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, time.Duration(0), keypool.ParseRetryAfter(nil))
	})
}

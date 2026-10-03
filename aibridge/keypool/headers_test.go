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
		name        string
		headers     map[string]string
		nilResponse bool
		expected    time.Duration
	}{
		// nil response.
		{
			name:        "nil_response",
			nilResponse: true,
			expected:    0,
		},
		// No headers set.
		{
			name:     "no_headers",
			headers:  nil,
			expected: 0,
		},
		// retry-after-ms (OpenAI, preferred).
		{
			name:     "openai_retry_after_ms",
			headers:  map[string]string{"retry-after-ms": "2500"},
			expected: 2500 * time.Millisecond,
		},
		{
			name:     "whitespace_trimmed_ms",
			headers:  map[string]string{"retry-after-ms": " 1500 "},
			expected: 1500 * time.Millisecond,
		},
		{
			name:     "negative_ms_returns_zero",
			headers:  map[string]string{"retry-after-ms": "-100"},
			expected: 0,
		},
		// Retry-After (standard, seconds).
		{
			name:     "standard_retry_after_seconds",
			headers:  map[string]string{"Retry-After": "60"},
			expected: 60 * time.Second,
		},
		{
			name:     "whitespace_trimmed_seconds",
			headers:  map[string]string{"Retry-After": " 30 "},
			expected: 30 * time.Second,
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
		// Both headers set: precedence and fallback.
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
			name:     "zero_ms_falls_back_to_standard",
			headers:  map[string]string{"retry-after-ms": "0", "Retry-After": "5"},
			expected: 5 * time.Second,
		},
		{
			name:     "zero_ms_and_zero_seconds_return_zero",
			headers:  map[string]string{"retry-after-ms": "0", "Retry-After": "0"},
			expected: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var resp *http.Response
			if !tc.nilResponse {
				resp = &http.Response{Header: make(http.Header)}
				for key, val := range tc.headers {
					resp.Header.Set(key, val)
				}
			}
			assert.Equal(t, tc.expected, keypool.ParseRetryAfter(resp))
		})
	}
}

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

func TestActorHeaders(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		header string
		want   bool
	}{
		{name: "ID", header: utils.ActorHeaderPrefix + "-ID", want: true},
		{name: "MetadataCaseInsensitive", header: "x-ai-bridge-actor-metadata-name", want: true},
		{name: "Other", header: "X-AI-Bridge-Request-ID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, utils.IsActorHeader(tc.header))
		})
	}
}

func TestNewStreamingTransport(t *testing.T) {
	t.Parallel()

	transport := utils.NewStreamingTransport()
	require.Equal(t, 100, transport.MaxIdleConns)
	require.Equal(t, 90*time.Second, transport.IdleConnTimeout)
	require.Equal(t, 10*time.Second, transport.TLSHandshakeTimeout)
	require.Equal(t, time.Second, transport.ExpectContinueTimeout)
	require.Zero(t, transport.ResponseHeaderTimeout)
	require.False(t, transport.DisableCompression)
}

func TestStripCoderHeaders(t *testing.T) {
	t.Parallel()

	headers := http.Header{
		"Coder-Session-Token":                    {"secret"},
		"cOdEr-Custom":                           {"secret"},
		"X-Coder-AI-Governance-Token":            {"secret"},
		"x-CoDeR-Agent-Firewall-Session-Id":      {"secret"},
		"X-Coder-Agent-Firewall-Sequence-Number": {"42"},
		"Authorization":                          {"Bearer provider"},
		"X-Api-Key":                              {"provider-key"},
		"User-Agent":                             {"client/1.0"},
		"Accept-Encoding":                        {"gzip"},
		"X-AI-Bridge-Actor-Id":                   {"actor"},
	}

	utils.StripCoderHeaders(headers)

	require.Equal(t, http.Header{
		"Authorization":        {"Bearer provider"},
		"X-Api-Key":            {"provider-key"},
		"User-Agent":           {"client/1.0"},
		"Accept-Encoding":      {"gzip"},
		"X-AI-Bridge-Actor-Id": {"actor"},
	}, headers)
}

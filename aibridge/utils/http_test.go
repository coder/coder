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

	require.True(t, utils.IsActorHeader(utils.ActorHeaderPrefix+"-ID"))
	require.True(t, utils.IsActorHeader("x-ai-bridge-actor-metadata-name"))
	require.False(t, utils.IsActorHeader("X-AI-Bridge-Request-ID"))
}

func TestNewStreamingTransport(t *testing.T) {
	t.Parallel()

	transport := utils.NewStreamingTransport()
	require.Nil(t, transport.DialContext)
	require.Equal(t, 100, transport.MaxIdleConns)
	require.Equal(t, 90*time.Second, transport.IdleConnTimeout)
	require.Equal(t, 10*time.Second, transport.TLSHandshakeTimeout)
	require.Equal(t, time.Second, transport.ExpectContinueTimeout)
	require.Zero(t, transport.ResponseHeaderTimeout)
	require.False(t, transport.DisableCompression)
}

func TestPrepareClientHeaders(t *testing.T) {
	t.Parallel()

	t.Run("nil input returns empty header", func(t *testing.T) {
		t.Parallel()

		result := utils.PrepareClientHeaders(nil)
		require.Empty(t, result)
	})

	t.Run("hop-by-hop headers are removed", func(t *testing.T) {
		t.Parallel()

		input := http.Header{
			"Connection":        {"keep-alive"},
			"Keep-Alive":        {"timeout=5"},
			"Transfer-Encoding": {"chunked"},
			"Upgrade":           {"websocket"},
			"X-Custom":          {"preserved"},
		}

		result := utils.PrepareClientHeaders(input)

		assert.Empty(t, result.Get("Connection"))
		assert.Empty(t, result.Get("Keep-Alive"))
		assert.Empty(t, result.Get("Transfer-Encoding"))
		assert.Empty(t, result.Get("Upgrade"))
		assert.Equal(t, "preserved", result.Get("X-Custom"))
	})

	t.Run("non-forwarded headers are removed", func(t *testing.T) {
		t.Parallel()

		input := http.Header{
			"Host":            {"example.com"},
			"Accept-Encoding": {"gzip"},
			"Content-Length":  {"42"},
			"X-Custom":        {"preserved"},
		}

		result := utils.PrepareClientHeaders(input)

		assert.Empty(t, result.Get("Host"))
		assert.Empty(t, result.Get("Accept-Encoding"))
		assert.Empty(t, result.Get("Content-Length"))
		assert.Equal(t, "preserved", result.Get("X-Custom"))
	})

	t.Run("auth headers are removed", func(t *testing.T) {
		t.Parallel()

		input := http.Header{
			"Authorization": {"Bearer coder-session-token"},
			"X-Api-Key":     {"sk-client-key"},
			"X-Custom":      {"preserved"},
		}

		result := utils.PrepareClientHeaders(input)

		assert.Empty(t, result.Get("Authorization"))
		assert.Empty(t, result.Get("X-Api-Key"))
		assert.Equal(t, "preserved", result.Get("X-Custom"))
	})

	t.Run("proxy headers are removed", func(t *testing.T) {
		t.Parallel()

		input := http.Header{
			"X-Forwarded-For":   {"203.0.113.50"},
			"X-Forwarded-Host":  {"app.example.com"},
			"X-Forwarded-Proto": {"https"},
			"X-Forwarded-Port":  {"443"},
			"Forwarded":         {"for=203.0.113.50;proto=https"},
			"X-Custom":          {"preserved"},
		}

		result := utils.PrepareClientHeaders(input)

		assert.Empty(t, result.Get("X-Forwarded-For"))
		assert.Empty(t, result.Get("X-Forwarded-Host"))
		assert.Empty(t, result.Get("X-Forwarded-Proto"))
		assert.Empty(t, result.Get("X-Forwarded-Port"))
		assert.Empty(t, result.Get("Forwarded"))
		assert.Equal(t, "preserved", result.Get("X-Custom"))
	})

	t.Run("multi-value headers are preserved", func(t *testing.T) {
		t.Parallel()

		input := http.Header{
			"X-Custom": {"value-1", "value-2"},
		}

		result := utils.PrepareClientHeaders(input)

		require.Equal(t, []string{"value-1", "value-2"}, result["X-Custom"])
	})

	t.Run("input is not mutated", func(t *testing.T) {
		t.Parallel()

		input := http.Header{
			"Connection": {"keep-alive"},
			"X-Custom":   {"preserved"},
		}
		originalCopy := input.Clone()

		_ = utils.PrepareClientHeaders(input)

		require.Equal(t, originalCopy, input)
	})

	t.Run("agent firewall headers are removed", func(t *testing.T) {
		t.Parallel()

		input := http.Header{
			"X-Coder-Agent-Firewall-Session-Id":      {"e5f6a7b8-1234-5678-9abc-def012345678"},
			"X-Coder-Agent-Firewall-Sequence-Number": {"42"},
			"X-Custom":                               {"preserved"},
		}

		result := utils.PrepareClientHeaders(input)

		assert.Empty(t, result.Get("X-Coder-Agent-Firewall-Session-Id"))
		assert.Empty(t, result.Get("X-Coder-Agent-Firewall-Sequence-Number"))
		assert.Equal(t, "preserved", result.Get("X-Custom"))
	})
}

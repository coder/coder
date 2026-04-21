package intercept_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/aibridge/intercept"
)

func TestPrepareClientHeaders(t *testing.T) {
	t.Parallel()

	t.Run("nil input returns empty header", func(t *testing.T) {
		t.Parallel()

		result := intercept.PrepareClientHeaders(nil)
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

		result := intercept.PrepareClientHeaders(input)

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

		result := intercept.PrepareClientHeaders(input)

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

		result := intercept.PrepareClientHeaders(input)

		assert.Empty(t, result.Get("Authorization"))
		assert.Empty(t, result.Get("X-Api-Key"))
		assert.Equal(t, "preserved", result.Get("X-Custom"))
	})

	t.Run("multi-value headers are preserved", func(t *testing.T) {
		t.Parallel()

		input := http.Header{
			"X-Custom": {"value-1", "value-2"},
		}

		result := intercept.PrepareClientHeaders(input)

		require.Equal(t, []string{"value-1", "value-2"}, result["X-Custom"])
	})

	t.Run("input is not mutated", func(t *testing.T) {
		t.Parallel()

		input := http.Header{
			"Connection": {"keep-alive"},
			"X-Custom":   {"preserved"},
		}
		originalCopy := input.Clone()

		_ = intercept.PrepareClientHeaders(input)

		require.Equal(t, originalCopy, input)
	})
}

func TestBuildUpstreamHeaders(t *testing.T) {
	t.Parallel()

	t.Run("preserves auth from SDK", func(t *testing.T) {
		t.Parallel()

		sdkHeader := http.Header{
			"Authorization": {"Bearer sk-provider-key"},
		}
		clientHeaders := http.Header{
			"Authorization": {"Bearer coder-session-token"},
			"User-Agent":    {"claude-code/1.0"},
		}

		result := intercept.BuildUpstreamHeaders(sdkHeader, clientHeaders, "Authorization")

		assert.Equal(t, "Bearer sk-provider-key", result.Get("Authorization"))
		assert.Equal(t, "claude-code/1.0", result.Get("User-Agent"))
	})

	t.Run("preserves X-Api-Key from SDK and strips client Authorization", func(t *testing.T) {
		t.Parallel()

		sdkHeader := http.Header{
			"X-Api-Key": {"sk-ant-provider-key"},
		}
		clientHeaders := http.Header{
			"X-Api-Key":      {"sk-ant-client-key"},
			"Authorization":  {"Bearer coder-session-token"},
			"Anthropic-Beta": {"prompt-caching-2024-07-31"},
		}

		result := intercept.BuildUpstreamHeaders(sdkHeader, clientHeaders, "X-Api-Key")

		assert.Equal(t, "sk-ant-provider-key", result.Get("X-Api-Key"))
		assert.Empty(t, result.Get("Authorization"))
		assert.Equal(t, "prompt-caching-2024-07-31", result.Get("Anthropic-Beta"))
	})

	t.Run("preserves actor headers from SDK", func(t *testing.T) {
		t.Parallel()

		sdkHeader := http.Header{
			"Authorization":                   {"Bearer sk-key"},
			"X-Ai-Bridge-Actor-Id":            {"user-123"},
			"X-Ai-Bridge-Actor-Metadata-Name": {"alice"},
		}
		clientHeaders := http.Header{
			"Authorization": {"Bearer coder-token"},
			"User-Agent":    {"claude-code/1.0"},
		}

		result := intercept.BuildUpstreamHeaders(sdkHeader, clientHeaders, "Authorization")

		assert.Equal(t, "Bearer sk-key", result.Get("Authorization"))
		assert.Equal(t, "user-123", result.Get("X-Ai-Bridge-Actor-Id"))
		assert.Equal(t, "alice", result.Get("X-Ai-Bridge-Actor-Metadata-Name"))
		assert.Equal(t, "claude-code/1.0", result.Get("User-Agent"))
	})

	t.Run("strips hop-by-hop and transport headers", func(t *testing.T) {
		t.Parallel()

		sdkHeader := http.Header{
			"Authorization": {"Bearer sk-key"},
		}
		clientHeaders := http.Header{
			"Connection":        {"keep-alive"},
			"Host":              {"bridge.example.com"},
			"Content-Length":    {"99"},
			"Accept-Encoding":   {"gzip"},
			"Transfer-Encoding": {"chunked"},
			"User-Agent":        {"claude-code/1.0"},
		}

		result := intercept.BuildUpstreamHeaders(sdkHeader, clientHeaders, "Authorization")

		assert.Empty(t, result.Get("Connection"))
		assert.Empty(t, result.Get("Host"))
		assert.Empty(t, result.Get("Content-Length"))
		assert.Empty(t, result.Get("Accept-Encoding"))
		assert.Empty(t, result.Get("Transfer-Encoding"))
		assert.Equal(t, "claude-code/1.0", result.Get("User-Agent"))
	})

	t.Run("empty auth header in SDK is not injected", func(t *testing.T) {
		t.Parallel()

		sdkHeader := http.Header{}
		clientHeaders := http.Header{
			"User-Agent": {"claude-code/1.0"},
		}

		result := intercept.BuildUpstreamHeaders(sdkHeader, clientHeaders, "Authorization")

		assert.Empty(t, result.Get("Authorization"))
		assert.Equal(t, "claude-code/1.0", result.Get("User-Agent"))
	})

	t.Run("does not mutate inputs", func(t *testing.T) {
		t.Parallel()

		sdkHeader := http.Header{
			"Authorization": {"Bearer sk-key"},
		}
		clientHeaders := http.Header{
			"Authorization": {"Bearer coder-token"},
			"Connection":    {"keep-alive"},
		}
		sdkCopy := sdkHeader.Clone()
		clientCopy := clientHeaders.Clone()

		_ = intercept.BuildUpstreamHeaders(sdkHeader, clientHeaders, "Authorization")

		require.Equal(t, sdkCopy, sdkHeader)
		require.Equal(t, clientCopy, clientHeaders)
	})
}

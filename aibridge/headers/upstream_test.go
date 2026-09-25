package headers_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/headers"
	"github.com/coder/coder/v2/aibridge/recorder"
)

func TestPrepareClientHeaders(t *testing.T) {
	t.Parallel()

	t.Run("nil input returns empty header", func(t *testing.T) {
		t.Parallel()

		result := headers.PrepareClientHeaders(nil)
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

		result := headers.PrepareClientHeaders(input)

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

		result := headers.PrepareClientHeaders(input)

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

		result := headers.PrepareClientHeaders(input)

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

		result := headers.PrepareClientHeaders(input)

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

		result := headers.PrepareClientHeaders(input)

		require.Equal(t, []string{"value-1", "value-2"}, result["X-Custom"])
	})

	t.Run("input is not mutated", func(t *testing.T) {
		t.Parallel()

		input := http.Header{
			"Connection": {"keep-alive"},
			"X-Custom":   {"preserved"},
		}
		originalCopy := input.Clone()

		_ = headers.PrepareClientHeaders(input)

		require.Equal(t, originalCopy, input)
	})

	t.Run("agent firewall headers are removed", func(t *testing.T) {
		t.Parallel()

		input := http.Header{
			"X-Coder-Agent-Firewall-Session-Id":      {"e5f6a7b8-1234-5678-9abc-def012345678"},
			"X-Coder-Agent-Firewall-Sequence-Number": {"42"},
			"X-Custom":                               {"preserved"},
		}

		result := headers.PrepareClientHeaders(input)

		assert.Empty(t, result.Get("X-Coder-Agent-Firewall-Session-Id"))
		assert.Empty(t, result.Get("X-Coder-Agent-Firewall-Sequence-Number"))
		assert.Equal(t, "preserved", result.Get("X-Custom"))
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

		result := headers.BuildUpstreamHeaders(sdkHeader, clientHeaders, "Authorization", false, nil)

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

		result := headers.BuildUpstreamHeaders(sdkHeader, clientHeaders, "X-Api-Key", false, nil)

		assert.Equal(t, "sk-ant-provider-key", result.Get("X-Api-Key"))
		assert.Empty(t, result.Get("Authorization"))
		assert.Equal(t, "prompt-caching-2024-07-31", result.Get("Anthropic-Beta"))
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

		result := headers.BuildUpstreamHeaders(sdkHeader, clientHeaders, "Authorization", false, nil)

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

		result := headers.BuildUpstreamHeaders(sdkHeader, clientHeaders, "Authorization", false, nil)

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

		_ = headers.BuildUpstreamHeaders(sdkHeader, clientHeaders, "Authorization", false, nil)

		require.Equal(t, sdkCopy, sdkHeader)
		require.Equal(t, clientCopy, clientHeaders)
	})

	t.Run("authenticated actor overrides client and SDK values", func(t *testing.T) {
		t.Parallel()

		sdkHeaders := http.Header{
			"Authorization":                       {"Bearer provider-key"},
			"X-Ai-Bridge-Actor-Id":                {"sdk-id"},
			"X-Ai-Bridge-Actor-Metadata-Username": {"sdk-name"},
		}
		clientHeaders := http.Header{
			"X-Ai-Bridge-Actor-Id":                {"client-id"},
			"X-Ai-Bridge-Actor-Metadata-Username": {"client-name"},
		}
		sdkCopy, clientCopy := sdkHeaders.Clone(), clientHeaders.Clone()
		actor := &context.Actor{ID: "user-123", Metadata: recorder.Metadata{"Username": "alice"}}

		result := headers.BuildUpstreamHeaders(sdkHeaders, clientHeaders, "Authorization", true, actor)

		require.Equal(t, []string{"user-123"}, result.Values(headers.ActorIDHeader()))
		require.Equal(t, []string{"alice"}, result.Values(headers.ActorMetadataHeader("Username")))
		require.Equal(t, "Bearer provider-key", result.Get("Authorization"))
		require.Equal(t, sdkCopy, sdkHeaders)
		require.Equal(t, clientCopy, clientHeaders)
	})

	t.Run("actor forwarding with nil client headers", func(t *testing.T) {
		t.Parallel()

		actor := &context.Actor{ID: "user-123"}
		for _, tc := range []struct {
			name  string
			send  bool
			actor *context.Actor
			want  string
		}{
			{name: "enabled", send: true, actor: actor, want: actor.ID},
			{name: "disabled", actor: actor},
			{name: "nil actor", send: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				result := headers.BuildUpstreamHeaders(nil, nil, "Authorization", tc.send, tc.actor)
				require.Equal(t, tc.want, result.Get(headers.ActorIDHeader()))
			})
		}
	})
}

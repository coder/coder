package chatdebug_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
)

func TestRedactHeaders(t *testing.T) {
	t.Parallel()

	t.Run("nil input", func(t *testing.T) {
		t.Parallel()

		require.Nil(t, chatdebug.RedactHeaders(nil))
	})

	t.Run("empty header", func(t *testing.T) {
		t.Parallel()

		redacted := chatdebug.RedactHeaders(http.Header{})
		require.NotNil(t, redacted)
		require.Empty(t, redacted)
	})

	t.Run("authorization redacted and others preserved", func(t *testing.T) {
		t.Parallel()

		headers := http.Header{
			"Authorization": {"Bearer secret-token"},
			"Accept":        {"application/json"},
		}

		redacted := chatdebug.RedactHeaders(headers)
		require.Equal(t, chatdebug.RedactedValue, redacted["Authorization"])
		require.Equal(t, "application/json", redacted["Accept"])
	})

	t.Run("multi-value headers are flattened", func(t *testing.T) {
		t.Parallel()

		headers := http.Header{
			"Accept": {"application/json", "text/plain"},
		}

		redacted := chatdebug.RedactHeaders(headers)
		require.Equal(t, "application/json, text/plain", redacted["Accept"])
	})

	t.Run("header name matching is case insensitive", func(t *testing.T) {
		t.Parallel()

		lowerAuthorization := "authorization"
		upperAuthorization := "AUTHORIZATION"
		headers := http.Header{
			lowerAuthorization: {"lower"},
			upperAuthorization: {"upper"},
		}

		redacted := chatdebug.RedactHeaders(headers)
		require.Equal(t, chatdebug.RedactedValue, redacted[lowerAuthorization])
		require.Equal(t, chatdebug.RedactedValue, redacted[upperAuthorization])
	})

	t.Run("token and secret substrings are redacted", func(t *testing.T) {
		t.Parallel()

		traceHeader := "X-Trace-ID"
		headers := http.Header{
			"X-Custom-Token":  {"abc"},
			"X-Custom-Secret": {"def"},
			traceHeader:       {"trace"},
		}

		redacted := chatdebug.RedactHeaders(headers)
		require.Equal(t, chatdebug.RedactedValue, redacted["X-Custom-Token"])
		require.Equal(t, chatdebug.RedactedValue, redacted["X-Custom-Secret"])
		require.Equal(t, "trace", redacted[traceHeader])
	})

	t.Run("known safe rate limit headers containing token are not redacted", func(t *testing.T) {
		t.Parallel()

		headers := http.Header{
			"Anthropic-Ratelimit-Tokens-Limit":     {"1000000"},
			"Anthropic-Ratelimit-Tokens-Remaining": {"999000"},
			"Anthropic-Ratelimit-Tokens-Reset":     {"2026-03-31T08:55:26Z"},
			"X-RateLimit-Limit-Tokens":             {"120000"},
			"X-RateLimit-Remaining-Tokens":         {"119500"},
			"X-RateLimit-Reset-Tokens":             {"12ms"},
		}

		redacted := chatdebug.RedactHeaders(headers)
		require.Equal(t, "1000000", redacted["Anthropic-Ratelimit-Tokens-Limit"])
		require.Equal(t, "999000", redacted["Anthropic-Ratelimit-Tokens-Remaining"])
		require.Equal(t, "2026-03-31T08:55:26Z", redacted["Anthropic-Ratelimit-Tokens-Reset"])
		require.Equal(t, "120000", redacted["X-RateLimit-Limit-Tokens"])
		require.Equal(t, "119500", redacted["X-RateLimit-Remaining-Tokens"])
		require.Equal(t, "12ms", redacted["X-RateLimit-Reset-Tokens"])
	})

	t.Run("rate limit headers with secret fragments are still redacted", func(t *testing.T) {
		t.Parallel()

		headers := http.Header{
			"X-RateLimit-Token":   {"secret-token"},
			"X-RateLimit-Api-Key": {"secret-key"},
		}

		redacted := chatdebug.RedactHeaders(headers)
		require.Equal(t, chatdebug.RedactedValue, redacted["X-RateLimit-Token"])
		require.Equal(t, chatdebug.RedactedValue, redacted["X-RateLimit-Api-Key"])
	})

	t.Run("original header is not modified", func(t *testing.T) {
		t.Parallel()

		headers := http.Header{
			"Authorization": {"Bearer keep-me"},
			"X-Test":        {"value"},
		}

		redacted := chatdebug.RedactHeaders(headers)
		redacted["X-Test"] = "changed"

		require.Equal(t, []string{"Bearer keep-me"}, headers["Authorization"])
		require.Equal(t, []string{"value"}, headers["X-Test"])
		require.Equal(t, chatdebug.RedactedValue, redacted["Authorization"])
	})
	t.Run("api-key header variants are redacted", func(t *testing.T) {
		t.Parallel()

		headers := http.Header{
			"X-Goog-Api-Key": {"secret"},
			"X-Api_Key":      {"other-secret"},
			"X-Safe":         {"ok"},
		}

		redacted := chatdebug.RedactHeaders(headers)
		require.Equal(t, chatdebug.RedactedValue, redacted["X-Goog-Api-Key"])
		require.Equal(t, chatdebug.RedactedValue, redacted["X-Api_Key"])
		require.Equal(t, "ok", redacted["X-Safe"])
	})
}

func TestRedactJSONSecrets(t *testing.T) {
	t.Parallel()

	t.Run("redacts top level secret fields", func(t *testing.T) {
		t.Parallel()

		input := []byte(`{"api_key":"abc","token":"def","password":"ghi","safe":"ok"}`)
		redacted := chatdebug.RedactJSONSecrets(input)
		require.JSONEq(t, `{"api_key":"[REDACTED]","token":"[REDACTED]","password":"[REDACTED]","safe":"ok"}`, string(redacted))
	})

	t.Run("redacts nested objects", func(t *testing.T) {
		t.Parallel()

		input := []byte(`{"outer":{"nested_secret":"abc","safe":1},"keep":true}`)
		redacted := chatdebug.RedactJSONSecrets(input)
		require.JSONEq(t, `{"outer":{"nested_secret":"[REDACTED]","safe":1},"keep":true}`, string(redacted))
	})

	t.Run("redacts arrays of objects", func(t *testing.T) {
		t.Parallel()

		input := []byte(`[{"token":"abc"},{"value":1,"credentials":{"access_key":"def"}}]`)
		redacted := chatdebug.RedactJSONSecrets(input)
		require.JSONEq(t, `[{"token":"[REDACTED]"},{"value":1,"credentials":"[REDACTED]"}]`, string(redacted))
	})

	t.Run("concatenated JSON is unchanged", func(t *testing.T) {
		t.Parallel()

		input := []byte(`{"token":"abc"}{"safe":"ok"}`)
		require.Equal(t, input, chatdebug.RedactJSONSecrets(input))
	})

	t.Run("non JSON input is unchanged", func(t *testing.T) {
		t.Parallel()

		input := []byte("not json")
		require.Equal(t, input, chatdebug.RedactJSONSecrets(input))
	})

	t.Run("empty input is unchanged", func(t *testing.T) {
		t.Parallel()

		input := []byte{}
		require.Equal(t, input, chatdebug.RedactJSONSecrets(input))
	})

	t.Run("JSON without sensitive keys is unchanged", func(t *testing.T) {
		t.Parallel()

		input := []byte(`{"safe":"ok","nested":{"value":1}}`)
		require.Equal(t, input, chatdebug.RedactJSONSecrets(input))
	})

	t.Run("key matching is case insensitive", func(t *testing.T) {
		t.Parallel()

		input := []byte(`{"API_KEY":"abc","Token":"def","PASSWORD":"ghi"}`)
		redacted := chatdebug.RedactJSONSecrets(input)
		require.JSONEq(t, `{"API_KEY":"[REDACTED]","Token":"[REDACTED]","PASSWORD":"[REDACTED]"}`, string(redacted))
	})
}

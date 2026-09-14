package chatdebug_test

import (
	"net/http"
	"strings"
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
			"X-Auth-Token":    {"abc"},
			"X-Custom-Secret": {"def"},
			"X-Bearer":        {"ghi"},
			traceHeader:       {"trace"},
		}

		redacted := chatdebug.RedactHeaders(headers)
		require.Equal(t, chatdebug.RedactedValue, redacted["X-Auth-Token"])
		require.Equal(t, chatdebug.RedactedValue, redacted["X-Custom-Secret"])
		require.Equal(t, chatdebug.RedactedValue, redacted["X-Bearer"])
		require.Equal(t, "trace", redacted[traceHeader])
	})

	t.Run("known safe rate limit headers containing token are not redacted", func(t *testing.T) {
		t.Parallel()

		headers := http.Header{
			"Anthropic-Ratelimit-Tokens-Limit":            {"1000000"},
			"Anthropic-Ratelimit-Tokens-Remaining":        {"999000"},
			"Anthropic-Ratelimit-Tokens-Reset":            {"2026-03-31T08:55:26Z"},
			"Anthropic-Ratelimit-Input-Tokens-Limit":      {"200000"},
			"Anthropic-Ratelimit-Input-Tokens-Remaining":  {"199000"},
			"Anthropic-Ratelimit-Input-Tokens-Reset":      {"2026-03-31T08:55:26Z"},
			"Anthropic-Ratelimit-Output-Tokens-Limit":     {"80000"},
			"Anthropic-Ratelimit-Output-Tokens-Remaining": {"79500"},
			"Anthropic-Ratelimit-Output-Tokens-Reset":     {"2026-03-31T08:55:26Z"},
			"Anthropic-Priority-Input-Tokens-Limit":       {"10000"},
			"Anthropic-Priority-Input-Tokens-Remaining":   {"9618"},
			"Anthropic-Priority-Input-Tokens-Reset":       {"2026-03-31T08:55:26Z"},
			"Anthropic-Priority-Output-Tokens-Limit":      {"10000"},
			"Anthropic-Priority-Output-Tokens-Remaining":  {"6000"},
			"Anthropic-Priority-Output-Tokens-Reset":      {"2026-03-31T08:55:26Z"},
			"Anthropic-Fast-Input-Tokens-Limit":           {"50000"},
			"Anthropic-Fast-Input-Tokens-Remaining":       {"49000"},
			"Anthropic-Fast-Input-Tokens-Reset":           {"2026-03-31T08:55:26Z"},
			"Anthropic-Fast-Output-Tokens-Limit":          {"25000"},
			"Anthropic-Fast-Output-Tokens-Remaining":      {"24000"},
			"Anthropic-Fast-Output-Tokens-Reset":          {"2026-03-31T08:55:26Z"},
			"X-RateLimit-Limit-Tokens":                    {"120000"},
			"X-RateLimit-Remaining-Tokens":                {"119500"},
			"X-RateLimit-Reset-Tokens":                    {"12ms"},
		}

		redacted := chatdebug.RedactHeaders(headers)
		require.Equal(t, "1000000", redacted["Anthropic-Ratelimit-Tokens-Limit"])
		require.Equal(t, "999000", redacted["Anthropic-Ratelimit-Tokens-Remaining"])
		require.Equal(t, "2026-03-31T08:55:26Z", redacted["Anthropic-Ratelimit-Tokens-Reset"])
		require.Equal(t, "200000", redacted["Anthropic-Ratelimit-Input-Tokens-Limit"])
		require.Equal(t, "199000", redacted["Anthropic-Ratelimit-Input-Tokens-Remaining"])
		require.Equal(t, "2026-03-31T08:55:26Z", redacted["Anthropic-Ratelimit-Input-Tokens-Reset"])
		require.Equal(t, "80000", redacted["Anthropic-Ratelimit-Output-Tokens-Limit"])
		require.Equal(t, "79500", redacted["Anthropic-Ratelimit-Output-Tokens-Remaining"])
		require.Equal(t, "2026-03-31T08:55:26Z", redacted["Anthropic-Ratelimit-Output-Tokens-Reset"])
		require.Equal(t, "10000", redacted["Anthropic-Priority-Input-Tokens-Limit"])
		require.Equal(t, "9618", redacted["Anthropic-Priority-Input-Tokens-Remaining"])
		require.Equal(t, "2026-03-31T08:55:26Z", redacted["Anthropic-Priority-Input-Tokens-Reset"])
		require.Equal(t, "10000", redacted["Anthropic-Priority-Output-Tokens-Limit"])
		require.Equal(t, "6000", redacted["Anthropic-Priority-Output-Tokens-Remaining"])
		require.Equal(t, "2026-03-31T08:55:26Z", redacted["Anthropic-Priority-Output-Tokens-Reset"])
		require.Equal(t, "50000", redacted["Anthropic-Fast-Input-Tokens-Limit"])
		require.Equal(t, "49000", redacted["Anthropic-Fast-Input-Tokens-Remaining"])
		require.Equal(t, "2026-03-31T08:55:26Z", redacted["Anthropic-Fast-Input-Tokens-Reset"])
		require.Equal(t, "25000", redacted["Anthropic-Fast-Output-Tokens-Limit"])
		require.Equal(t, "24000", redacted["Anthropic-Fast-Output-Tokens-Remaining"])
		require.Equal(t, "2026-03-31T08:55:26Z", redacted["Anthropic-Fast-Output-Tokens-Reset"])
		require.Equal(t, "120000", redacted["X-RateLimit-Limit-Tokens"])
		require.Equal(t, "119500", redacted["X-RateLimit-Remaining-Tokens"])
		require.Equal(t, "12ms", redacted["X-RateLimit-Reset-Tokens"])
	})

	t.Run("non-standard headers with api-key pattern are redacted", func(t *testing.T) {
		t.Parallel()

		headers := http.Header{
			"X-Custom-Api-Key":       {"secret-key"},
			"X-Custom-Secret":        {"secret-val"},
			"X-Custom-Session-Token": {"session-id"},
		}

		redacted := chatdebug.RedactHeaders(headers)
		require.Equal(t, chatdebug.RedactedValue, redacted["X-Custom-Api-Key"])
		require.Equal(t, chatdebug.RedactedValue, redacted["X-Custom-Secret"])
		require.Equal(t, chatdebug.RedactedValue, redacted["X-Custom-Session-Token"])
	})

	t.Run("rate limit headers with token in name are preserved", func(t *testing.T) {
		t.Parallel()

		// Rate-limit headers containing "token" should NOT be redacted
		// because they carry usage/limit counts, not credentials.
		headers := http.Header{
			"X-Ratelimit-Limit-Tokens":     {"1000000"},
			"X-Ratelimit-Remaining-Tokens": {"999000"},
		}

		redacted := chatdebug.RedactHeaders(headers)
		require.Equal(t, "1000000", redacted["X-Ratelimit-Limit-Tokens"])
		require.Equal(t, "999000", redacted["X-Ratelimit-Remaining-Tokens"])
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

	t.Run("plain token headers are redacted", func(t *testing.T) {
		t.Parallel()

		// Headers like "Token" or "X-Token" should be redacted
		// even without auth/session/access qualifiers.
		headers := http.Header{
			"Token":   {"my-secret-token"},
			"X-Token": {"another-secret"},
			"X-Safe":  {"ok"},
		}

		redacted := chatdebug.RedactHeaders(headers)
		require.Equal(t, chatdebug.RedactedValue, redacted["Token"])
		require.Equal(t, chatdebug.RedactedValue, redacted["X-Token"])
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

	t.Run("redacts security_token exact key", func(t *testing.T) {
		t.Parallel()

		input := []byte(`{"security_token":"s3cret","securityToken":"tok","safe":"ok"}`)
		redacted := chatdebug.RedactJSONSecrets(input)
		require.JSONEq(t, `{"security_token":"[REDACTED]","securityToken":"[REDACTED]","safe":"ok"}`, string(redacted))
	})

	t.Run("preserves LLM token usage fields", func(t *testing.T) {
		t.Parallel()

		input := []byte(`{"input_tokens":100,"output_tokens":50,"prompt_tokens":80,"completion_tokens":20,"reasoning_tokens":10,"cache_creation_input_tokens":5,"cache_read_input_tokens":3,"total_tokens":150,"max_tokens":4096,"max_output_tokens":2048}`)
		redacted := chatdebug.RedactJSONSecrets(input)
		// All usage/limit fields should be preserved, not redacted.
		require.Equal(t, input, redacted)
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

	t.Run("concatenated JSON is replaced with diagnostic", func(t *testing.T) {
		t.Parallel()

		input := []byte(`{"token":"abc"}{"safe":"ok"}`)
		result := chatdebug.RedactJSONSecrets(input)
		require.Contains(t, string(result), "extra JSON values")
	})

	t.Run("non JSON input is replaced with diagnostic", func(t *testing.T) {
		t.Parallel()

		input := []byte("not json")
		result := chatdebug.RedactJSONSecrets(input)
		require.Contains(t, string(result), "not valid JSON")
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

	t.Run("camelCase token field names are redacted", func(t *testing.T) {
		t.Parallel()

		// Providers may use camelCase (e.g. accessToken, refreshToken).
		// These should be redacted even though they don't match the
		// snake_case originals exactly.
		input := []byte(`{"accessToken":"abc","refreshToken":"def","authToken":"ghi","input_tokens":100,"output_tokens":50}`)
		redacted := chatdebug.RedactJSONSecrets(input)
		require.JSONEq(t, `{"accessToken":"[REDACTED]","refreshToken":"[REDACTED]","authToken":"[REDACTED]","input_tokens":100,"output_tokens":50}`, string(redacted))
	})
}

func TestRedactNDJSONSecrets(t *testing.T) {
	t.Parallel()

	t.Run("empty input", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, chatdebug.RedactNDJSONSecrets(nil))
		require.Empty(t, chatdebug.RedactNDJSONSecrets([]byte{}))
	})

	t.Run("redacts secrets in each line", func(t *testing.T) {
		t.Parallel()
		input := []byte("{\"api_key\":\"sk-123\",\"safe\":\"ok\"}\n{\"token\":\"tok-456\",\"data\":\"value\"}\n")
		redacted := chatdebug.RedactNDJSONSecrets(input)
		lines := strings.Split(string(redacted), "\n")
		require.JSONEq(t, `{"api_key":"[REDACTED]","safe":"ok"}`, lines[0])
		require.JSONEq(t, `{"token":"[REDACTED]","data":"value"}`, lines[1])
	})

	t.Run("preserves lines without secrets", func(t *testing.T) {
		t.Parallel()
		input := []byte("{\"safe\":\"ok\"}\n{\"data\":\"value\"}\n")
		redacted := chatdebug.RedactNDJSONSecrets(input)
		require.Equal(t, string(input), string(redacted))
	})

	t.Run("handles malformed lines with fail-closed", func(t *testing.T) {
		t.Parallel()
		input := []byte("{\"safe\":\"ok\"}\nnot-json\n{\"token\":\"secret\"}\n")
		redacted := chatdebug.RedactNDJSONSecrets(input)
		lines := strings.Split(string(redacted), "\n")
		require.JSONEq(t, `{"safe":"ok"}`, lines[0])
		require.Contains(t, lines[1], "not valid JSON")
		require.JSONEq(t, `{"token":"[REDACTED]"}`, lines[2])
	})

	t.Run("handles single line without trailing newline", func(t *testing.T) {
		t.Parallel()
		input := []byte(`{"api_key":"secret","value":"ok"}`)
		redacted := chatdebug.RedactNDJSONSecrets(input)
		require.JSONEq(t, `{"api_key":"[REDACTED]","value":"ok"}`, string(redacted))
	})
}

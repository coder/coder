package chatdebug

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"golang.org/x/xerrors"
)

// RedactedValue replaces sensitive values in debug payloads.
const RedactedValue = "[REDACTED]"

var sensitiveHeaderNames = map[string]struct{}{
	"authorization":       {},
	"x-api-key":           {},
	"api-key":             {},
	"proxy-authorization": {},
	"cookie":              {},
	"set-cookie":          {},
}

// sensitiveJSONKeyFragments triggers redaction for JSON keys containing
// these substrings. Notably, "token" is intentionally absent because it
// false-positively redacts LLM token-usage fields (input_tokens,
// output_tokens, prompt_tokens, completion_tokens, reasoning_tokens,
// cache_creation_input_tokens, cache_read_input_tokens, etc.). Auth-
// related token fields are caught by the exact-match set below.
var sensitiveJSONKeyFragments = []string{
	"secret",
	"password",
	"authorization",
	"credential",
}

// sensitiveJSONKeyExact matches auth-related token/key field names
// without false-positiving on LLM usage counters. Includes both
// snake_case originals and their camelCase-lowered equivalents
// (e.g. "accessToken" → "accesstoken") so that providers using
// either convention are caught.
var sensitiveJSONKeyExact = map[string]struct{}{
	"token":          {},
	"access_token":   {},
	"accesstoken":    {},
	"refresh_token":  {},
	"refreshtoken":   {},
	"id_token":       {},
	"idtoken":        {},
	"api_token":      {},
	"apitoken":       {},
	"api_key":        {},
	"apikey":         {},
	"api-key":        {},
	"x-api-key":      {},
	"auth_token":     {},
	"authtoken":      {},
	"bearer_token":   {},
	"bearertoken":    {},
	"session_token":  {},
	"sessiontoken":   {},
	"security_token": {},
	"securitytoken":  {},
	"private_key":    {},
	"privatekey":     {},
	"signing_key":    {},
	"signingkey":     {},
	"secret_key":     {},
	"secretkey":      {},
}

// RedactHeaders returns a flattened copy of h with sensitive values redacted.
func RedactHeaders(h http.Header) map[string]string {
	if h == nil {
		return nil
	}

	redacted := make(map[string]string, len(h))
	for name, values := range h {
		if isSensitiveName(name) {
			redacted[name] = RedactedValue
			continue
		}
		redacted[name] = strings.Join(values, ", ")
	}
	return redacted
}

// RedactJSONSecrets redacts sensitive JSON values by key name. When
// the input is not valid JSON (truncated body, HTML error page, etc.)
// the raw bytes are replaced entirely with a diagnostic placeholder
// to avoid leaking credentials from malformed payloads.
func RedactJSONSecrets(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		// Cannot parse: replace entirely to prevent credential leaks
		// from non-JSON error responses (HTML pages, partial bodies).
		return []byte(`{"error":"chatdebug: body is not valid JSON, redacted for safety"}`)
	}
	if err := consumeJSONEOF(decoder); err != nil {
		return []byte(`{"error":"chatdebug: body contains extra JSON values, redacted for safety"}`)
	}

	redacted, changed := redactJSONValue(value)
	if !changed {
		return data
	}

	encoded, err := json.Marshal(redacted)
	if err != nil {
		return data
	}
	return encoded
}

// RedactNDJSONSecrets redacts sensitive values in newline-delimited
// JSON (NDJSON) payloads. Each non-empty line is treated as an
// independent JSON document and redacted individually. Lines that
// fail to parse are replaced with a diagnostic placeholder.
func RedactNDJSONSecrets(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	lines := bytes.Split(data, []byte("\n"))
	changed := false
	for i, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		redacted := RedactJSONSecrets(trimmed)
		if !bytes.Equal(redacted, trimmed) {
			lines[i] = redacted
			changed = true
		}
	}
	if !changed {
		return data
	}
	return bytes.Join(lines, []byte("\n"))
}

func consumeJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return xerrors.New("chatdebug: extra JSON values")
	}
	return err
}

var safeRateLimitHeaderNames = map[string]struct{}{
	"anthropic-ratelimit-requests-limit":     {},
	"anthropic-ratelimit-requests-remaining": {},
	"anthropic-ratelimit-requests-reset":     {},
	"anthropic-ratelimit-tokens-limit":       {},
	"anthropic-ratelimit-tokens-remaining":   {},
	"anthropic-ratelimit-tokens-reset":       {},
	"x-ratelimit-limit-requests":             {},
	"x-ratelimit-limit-tokens":               {},
	"x-ratelimit-remaining-requests":         {},
	"x-ratelimit-remaining-tokens":           {},
	"x-ratelimit-reset-requests":             {},
	"x-ratelimit-reset-tokens":               {},
}

// isSensitiveName reports whether a name (header or query parameter)
// looks like a credential-carrying key. Exact-match headers are
// checked first, then the rate-limit allowlist, then substring
// patterns for API keys and auth tokens.
func isSensitiveName(name string) bool {
	lowerName := strings.ToLower(name)
	if _, ok := sensitiveHeaderNames[lowerName]; ok {
		return true
	}
	if _, ok := safeRateLimitHeaderNames[lowerName]; ok {
		return false
	}
	if strings.Contains(lowerName, "api-key") ||
		strings.Contains(lowerName, "api_key") ||
		strings.Contains(lowerName, "apikey") {
		return true
	}
	// Catch any header containing "token" (e.g. Token, X-Token,
	// X-Auth-Token).  Safe rate-limit headers like
	// x-ratelimit-remaining-tokens are already allowlisted above
	// and will not reach this point.
	if strings.Contains(lowerName, "token") {
		return true
	}
	return strings.Contains(lowerName, "secret") ||
		strings.Contains(lowerName, "bearer")
}

func isSensitiveJSONKey(key string) bool {
	lowerKey := strings.ToLower(key)
	if _, ok := sensitiveJSONKeyExact[lowerKey]; ok {
		return true
	}
	for _, fragment := range sensitiveJSONKeyFragments {
		if strings.Contains(lowerKey, fragment) {
			return true
		}
	}
	return false
}

func redactJSONValue(value any) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		changed := false
		for key, child := range typed {
			if isSensitiveJSONKey(key) {
				if current, ok := child.(string); ok && current == RedactedValue {
					continue
				}
				typed[key] = RedactedValue
				changed = true
				continue
			}

			redactedChild, childChanged := redactJSONValue(child)
			if childChanged {
				typed[key] = redactedChild
				changed = true
			}
		}
		return typed, changed
	case []any:
		changed := false
		for i, child := range typed {
			redactedChild, childChanged := redactJSONValue(child)
			if childChanged {
				typed[i] = redactedChild
				changed = true
			}
		}
		return typed, changed
	default:
		return value, false
	}
}

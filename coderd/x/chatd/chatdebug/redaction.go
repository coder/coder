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

var sensitiveJSONKeyFragments = []string{
	"token",
	"secret",
	"key",
	"password",
	"authorization",
	"credential",
}

// RedactHeaders returns a flattened copy of h with sensitive values redacted.
func RedactHeaders(h http.Header) map[string]string {
	if h == nil {
		return nil
	}

	redacted := make(map[string]string, len(h))
	for name, values := range h {
		if isSensitiveHeaderName(name) {
			redacted[name] = RedactedValue
			continue
		}
		redacted[name] = strings.Join(values, ", ")
	}
	return redacted
}

// RedactJSONSecrets redacts sensitive JSON values by key name.
func RedactJSONSecrets(data []byte) []byte {
	if len(data) == 0 {
		return data
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return data
	}
	if err := consumeJSONEOF(decoder); err != nil {
		return data
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

func isSensitiveHeaderName(name string) bool {
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
	return strings.Contains(lowerName, "token") ||
		strings.Contains(lowerName, "secret")
}

func isSensitiveJSONKey(key string) bool {
	lowerKey := strings.ToLower(key)
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

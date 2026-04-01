package chatdebug

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
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
	if err := decoder.Decode(&extra); err != io.EOF {
		return err
	}
	return nil
}

func isSensitiveHeaderName(name string) bool {
	lowerName := strings.ToLower(name)
	if _, ok := sensitiveHeaderNames[lowerName]; ok {
		return true
	}
	if strings.Contains(lowerName, "ratelimit") {
		return false
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

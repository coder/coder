package chaterror

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"charm.land/fantasy"
)

// transportErrorPrefix matches the Anthropic SDK transport error format
// `METHOD "URL": <status> <status text> ` that wraps the real provider
// response body. The SDK emits this when it cannot parse a non-Anthropic
// response, so AWS Bedrock errors arrive through aibridge with this
// wrapper, hiding the useful message inside a trailing JSON body.
var transportErrorPrefix = regexp.MustCompile(`^[A-Z]+ "[^"]+": \d{3} `)

type providerErrorDetails struct {
	detail     string
	statusCode int
	retryAfter time.Duration
}

func extractProviderErrorDetails(err error) providerErrorDetails {
	var providerErr *fantasy.ProviderError
	if !errors.As(err, &providerErr) {
		return providerErrorDetails{}
	}

	return providerErrorDetails{
		detail:     providerErrorDetail(providerErr),
		statusCode: providerErr.StatusCode,
		retryAfter: retryAfterFromHeaders(providerErr.ResponseHeaders),
	}
}

func providerErrorDetail(providerErr *fantasy.ProviderError) string {
	if detail := providerErrorResponseMessage(providerErr.ResponseBody); detail != "" {
		return detail
	}
	// The Message fallback can also be the SDK transport wrapper (e.g. for
	// Bedrock via aibridge), so unwrap it for the same clean detail.
	return unwrapTransportErrorMessage(strings.TrimSpace(providerErr.Message))
}

// providerErrorResponseMessage extracts the human-readable message from a
// provider error response body after stripping any dumped HTTP status line
// and headers. It understands both the top-level `{"message":...}` shape
// used by many providers and the nested `{"error":{"message":...}}`
// envelope. When the extracted message is itself an SDK-formatted transport
// error wrapper, the clean inner provider message is returned. For
// non-JSON text/plain bodies (e.g. aibridge's budget errors) it returns
// the first line of the body.
func providerErrorResponseMessage(responseDump []byte) string {
	if len(responseDump) == 0 || len(responseDump) > 64*1024 {
		return ""
	}
	headers, body := splitResponseDump(responseDump)
	if msg := unwrapTransportErrorMessage(jsonErrorMessage(body)); msg != "" {
		return msg
	}
	return unwrapTransportErrorMessage(plainTextErrorMessage(headers, body))
}

// plainTextErrorMessage returns the trimmed first line of a plain-text
// error body. The result is user-facing, so it requires a text/plain
// Content-Type (excluding proxy HTML and other opaque bodies) and rejects
// valid JSON (which jsonErrorMessage already handled; passing it through
// would leak raw JSON).
func plainTextErrorMessage(headers, body []byte) string {
	if !headersDeclareTextPlain(headers) || json.Valid(body) {
		return ""
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(body)), "\n")
	return strings.TrimSpace(line)
}

// headersDeclareTextPlain reports whether a dumped HTTP header block
// declares Content-Type text/plain, ignoring media-type parameters such
// as charset.
func headersDeclareTextPlain(headers []byte) bool {
	for line := range strings.Lines(string(headers)) {
		name, value, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "Content-Type") {
			continue
		}
		mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
		return err == nil && mediaType == "text/plain"
	}
	return false
}

// unwrapTransportErrorMessage extracts the clean provider message from an
// SDK-formatted wrapper such as:
//
//	POST "https://bedrock-runtime...": 400 Bad Request {"message":"..."}
//
// When the trailing body is JSON with a top-level "message" or a nested
// "error.message", that inner message is returned. Otherwise msg is
// returned unchanged.
func unwrapTransportErrorMessage(msg string) string {
	loc := transportErrorPrefix.FindStringIndex(msg)
	if loc == nil {
		return msg
	}
	// Search for the JSON body after the matched prefix so a brace inside
	// the URL or status text cannot be mistaken for the body.
	rest := msg[loc[1]:]
	start := strings.IndexByte(rest, '{')
	if start < 0 {
		return msg
	}
	if inner := jsonErrorMessage([]byte(rest[start:])); inner != "" {
		return inner
	}
	return msg
}

// jsonErrorMessage parses both the nested `{"error":{"message":...}}`
// envelope and the top-level `{"message":...}` shape used by many
// providers, preferring the nested form when present.
func jsonErrorMessage(body []byte) string {
	var env struct {
		Message string          `json:"message"`
		Error   json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return ""
	}
	// Prefer the nested error.message when error is an object carrying one.
	// error may also be a non-object (string, array, number); tolerate that
	// and fall back to the top-level message instead of dropping it.
	if len(env.Error) > 0 {
		var inner struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(env.Error, &inner); err == nil {
			if m := strings.TrimSpace(inner.Message); m != "" {
				return m
			}
		}
	}
	return strings.TrimSpace(env.Message)
}

// splitResponseDump separates a dumped HTTP response into its header block
// and body. It returns non-dump payloads whole as the body: fantasy's
// Google adapter stores a raw message in ResponseBody, and a blank line
// inside that message is not a header/body separator.
func splitResponseDump(responseDump []byte) (headers, body []byte) {
	if !bytes.HasPrefix(responseDump, []byte("HTTP/")) {
		return nil, responseDump
	}
	if h, b, ok := bytes.Cut(responseDump, []byte("\r\n\r\n")); ok {
		return h, b
	}
	if h, b, ok := bytes.Cut(responseDump, []byte("\n\n")); ok {
		return h, b
	}
	return nil, responseDump
}

func retryAfterFromHeaders(headers map[string]string) time.Duration {
	if len(headers) == 0 {
		return 0
	}

	// Prefer retry-after-ms (OpenAI convention, milliseconds)
	// over the standard retry-after (seconds or HTTP-date).
	for key, value := range headers {
		if strings.EqualFold(key, "retry-after-ms") {
			ms, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err == nil && ms > 0 {
				return time.Duration(ms * float64(time.Millisecond))
			}
		}
	}

	for key, value := range headers {
		if strings.EqualFold(key, "retry-after") {
			v := strings.TrimSpace(value)
			if seconds, err := strconv.ParseFloat(v, 64); err == nil {
				if seconds > 0 {
					return time.Duration(seconds * float64(time.Second))
				}
				return 0
			}
			if retryAt, err := http.ParseTime(v); err == nil {
				if d := time.Until(retryAt); d > 0 {
					return d
				}
			}
			return 0
		}
	}

	return 0
}

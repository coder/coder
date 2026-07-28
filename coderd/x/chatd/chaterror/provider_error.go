package chaterror

import (
	"bytes"
	"encoding/json"
	"errors"
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
// error wrapper, the clean inner provider message is returned.
func providerErrorResponseMessage(responseDump []byte) string {
	if len(responseDump) == 0 || len(responseDump) > 64*1024 {
		return ""
	}
	body := providerErrorResponseBody(responseDump)
	return unwrapTransportErrorMessage(jsonErrorMessage(body))
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

func providerErrorResponseBody(responseDump []byte) []byte {
	if _, body, ok := bytes.Cut(responseDump, []byte("\r\n\r\n")); ok {
		return body
	}
	if _, body, ok := bytes.Cut(responseDump, []byte("\n\n")); ok {
		return body
	}
	return responseDump
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

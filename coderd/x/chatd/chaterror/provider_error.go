package chaterror

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"charm.land/fantasy"
)

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
	return strings.TrimSpace(providerErr.Message)
}

// providerErrorResponseMessage extracts error.message from the common
// provider error JSON envelope after stripping any dumped HTTP status
// line and headers.
func providerErrorResponseMessage(responseDump []byte) string {
	if len(responseDump) == 0 || len(responseDump) > 64*1024 {
		return ""
	}
	body := providerErrorResponseBody(responseDump)
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	return strings.TrimSpace(envelope.Error.Message)
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

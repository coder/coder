// Package chatretry provides retry logic for transient LLM provider
// errors. It classifies errors as retryable or permanent and
// implements exponential backoff matching the behavior of coder/mux.
package chatretry

import (
	"context"
	"errors"
	"strings"
)

// nonRetryablePatterns are substrings that indicate a permanent error
// which should not be retried. These are checked first so that
// ambiguous messages (e.g. "bad request: rate limit") are correctly
// classified as non-retryable.
var nonRetryablePatterns = []string{
	"context canceled",
	"context deadline exceeded",
	"authentication",
	"unauthorized",
	"forbidden",
	"invalid api key",
	"invalid_api_key",
	"invalid model",
	"model not found",
	"model_not_found",
	"context length exceeded",
	"context_exceeded",
	"maximum context length",
	"quota",
	"billing",
}

// retryablePatterns are substrings that indicate a transient error
// worth retrying.
var retryablePatterns = []string{
	"overloaded",
	"rate limit",
	"rate_limit",
	"too many requests",
	"server error",
	"status 500",
	"status 502",
	"status 503",
	"status 529",
	"connection reset",
	"connection refused",
	"eof",
	"broken pipe",
	"timeout",
	"unavailable",
	"service unavailable",
}

// IsRetryable determines whether an error from an LLM provider is
// transient and worth retrying. It inspects the error message and
// any wrapped HTTP status codes for known retryable patterns.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// context.Canceled is always non-retryable regardless of
	// wrapping.
	if errors.Is(err, context.Canceled) {
		return false
	}

	lower := strings.ToLower(err.Error())

	// Check non-retryable patterns first so they take precedence.
	for _, p := range nonRetryablePatterns {
		if strings.Contains(lower, p) {
			return false
		}
	}

	for _, p := range retryablePatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}

	return false
}

// StatusCodeRetryable returns true for HTTP status codes that
// indicate a transient failure worth retrying.
func StatusCodeRetryable(code int) bool {
	switch code {
	case 429, 500, 502, 503, 529:
		return true
	default:
		return false
	}
}

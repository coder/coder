// Package interceptionerror categorizes terminal AI Gateway interception errors.
package interceptionerror

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/coder/coder/v2/aibridge/circuitbreaker"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// maxRecordedMessageBytes bounds error messages persisted with interception
// records so provider payloads cannot create unbounded database values.
const maxRecordedMessageBytes = 1024

// Categorizer maps provider-specific terminal errors to recorder error types.
type Categorizer interface {
	CategorizeError(error) *recorder.ErrorType
}

// statusCategorizer optionally maps provider-specific HTTP status codes that
// have no portable meaning across providers.
type statusCategorizer interface {
	CategorizeStatus(int) *recorder.ErrorType
}

// Categorize maps a terminal error to a recorder error type and bounded message.
// When err is nil, provider-specific status mappings are consulted before the
// HTTP fallback. Provider-specific errors are delegated after gateway-owned
// context, circuit, and key-pool errors.
func Categorize(c Categorizer, err error, status int) (recorder.ErrorType, string) {
	if err == nil {
		if status < http.StatusBadRequest {
			return "", ""
		}
		if statusCategorizer, ok := c.(statusCategorizer); ok {
			if errorType := statusCategorizer.CategorizeStatus(status); errorType != nil {
				return *errorType, statusMessage(status)
			}
		}
		return recorder.ErrorTypeFromStatus(status), statusMessage(status)
	}

	message := err.Error()
	if len(message) > maxRecordedMessageBytes {
		message = strings.ToValidUTF8(message[:maxRecordedMessageBytes], "")
	}

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return recorder.ErrorTypeTimeout, message
	case errors.Is(err, context.Canceled):
		// The caller went away. This is not an upstream failure, but the
		// interception did not complete, so record unknown rather than success.
		return recorder.ErrorTypeUnknown, message
	case errors.Is(err, circuitbreaker.ErrCircuitOpen):
		// Circuit-open responses are HTTP 503, but the sentinel itself carries no
		// status and must be classified directly.
		return recorder.ErrorTypeServerError, message
	}

	// Key-pool errors take precedence over provider delegation because the pool
	// masks the client response, for example by rendering permanent failures as
	// HTTP 502, which would otherwise hide the cause.
	if poolErr, ok := errors.AsType[*keypool.Error](err); ok {
		switch poolErr.Kind {
		case keypool.ErrorKindRateLimited:
			return recorder.ErrorTypeRateLimited, message
		case keypool.ErrorKindPermanent, keypool.ErrorKindUnauthorized:
			return recorder.ErrorTypeUnauthorized, message
		default:
			return recorder.ErrorTypeUnknown, message
		}
	}

	if c != nil {
		if errorType := c.CategorizeError(err); errorType != nil {
			return *errorType, message
		}
	}
	return recorder.ErrorTypeUnknown, message
}

func statusMessage(status int) string {
	if message := http.StatusText(status); message != "" {
		return message
	}
	return fmt.Sprintf("HTTP status %d", status)
}

package aibridge

import (
	"context"
	"errors"
	"strings"

	"github.com/coder/coder/v2/aibridge/circuitbreaker"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// maxRecordedErrorMessageBytes caps the raw upstream error message persisted on
// the interception record to avoid storing unbounded provider payloads.
const maxRecordedErrorMessageBytes = 1024

// errorCategorizer categorizes a provider's own terminal errors. It is
// implemented by provider.Provider.
type errorCategorizer interface {
	CategorizeError(err error) *recorder.ErrorType
}

// categorizeInterceptionError maps a terminal interception error to a recorder
// error type and a truncated raw message. It returns the empty ErrorType and an
// empty message when err is nil (the interception succeeded).
//
// Provider-agnostic failures (circuit breaker, key-pool exhaustion) are handled
// here; anything provider-specific is delegated to the provider, which owns the
// knowledge of its SDK errors and response envelopes.
func categorizeInterceptionError(c errorCategorizer, err error) (recorder.ErrorType, string) {
	if err == nil {
		return "", ""
	}
	msg := err.Error()
	if len(msg) > maxRecordedErrorMessageBytes {
		msg = strings.ToValidUTF8(msg[:maxRecordedErrorMessageBytes], "")
	}

	// Go context errors. These originate in the gateway or the caller, not
	// upstream, so they are classified before any provider delegation.
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return recorder.ErrorTypeTimeout, msg
	case errors.Is(err, context.Canceled):
		// The caller went away before the interception completed. This is not
		// an upstream failure, but the interception did not succeed either, so
		// it is recorded as unknown rather than dropped.
		return recorder.ErrorTypeUnknown, msg
	}

	// Circuit breaker. It responds with 503 Service Unavailable when open, but
	// returns a sentinel error that carries no HTTP status of its own.
	if errors.Is(err, circuitbreaker.ErrCircuitOpen) {
		return recorder.ErrorTypeServerError, msg
	}

	// Centralized key-pool failover. Checked before delegating because the pool
	// masks the client response (e.g. permanent failures become 502), which
	// would otherwise hide the cause.
	var keyPoolErr *keypool.Error
	if errors.As(err, &keyPoolErr) {
		switch keyPoolErr.Kind {
		case keypool.ErrorKindRateLimited:
			return recorder.ErrorTypeRateLimited, msg
		case keypool.ErrorKindPermanent:
			return recorder.ErrorTypeUnauthorized, msg
		default:
			return recorder.ErrorTypeUnknown, msg
		}
	}

	// Anything provider-specific is delegated to the provider, which owns the
	// knowledge of its SDK errors and response envelopes.
	if cat := c.CategorizeError(err); cat != nil {
		return *cat, msg
	}
	return recorder.ErrorTypeUnknown, msg
}

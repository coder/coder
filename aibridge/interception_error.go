package aibridge

import (
	"github.com/coder/coder/v2/aibridge/interceptionerror"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// errorCategorizer categorizes a provider's own terminal errors. It is
// implemented by provider.Provider.
type errorCategorizer = interceptionerror.Categorizer

// categorizeInterceptionError maps a terminal interception error to a recorder
// error type and a truncated raw message. It returns the empty ErrorType and an
// empty message when err is nil (the interception succeeded).
//
// Provider-agnostic failures (circuit breaker, key-pool exhaustion) are handled
// here; anything provider-specific is delegated to the provider, which owns the
// knowledge of its SDK errors and response envelopes.
func categorizeInterceptionError(c errorCategorizer, err error) (recorder.ErrorType, string) {
	return interceptionerror.Categorize(c, err)
}

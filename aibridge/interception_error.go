package aibridge

import (
	"github.com/coder/coder/v2/aibridge/interceptionerror"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// errorCategorizer categorizes a provider's own terminal errors. It is
// implemented by provider.Provider.
type errorCategorizer = interceptionerror.Categorizer

// categorizeInterceptionError maps a terminal interception error to a recorder
// error type and a truncated raw message. It returns empty values when the
// interception succeeded.
func categorizeInterceptionError(c errorCategorizer, err error) (recorder.ErrorType, string) {
	return interceptionerror.Categorize(c, err, 0)
}

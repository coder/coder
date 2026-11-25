// Package extractors derives recording facts from the bytes proxy mode
// forwards, without changing them.
package extractors

import (
	"github.com/coder/coder/v2/aibridge/recorder"
)

// Extractor provides a way to extract facts from a request and response body.
type Extractor interface {
	// ExtractRequest adds Model and CorrelatingToolCallID to the start record.
	ExtractRequest(reqBody []byte, start *recorder.InterceptionRecord) error
}

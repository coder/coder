// Package extractors derives recording facts from the bytes proxy mode
// forwards, without changing them.
package extractors

import (
	"context"
	"mime"
	"net/http"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/recorder"
)

// Extractor provides a way to extract facts from a request and response body.
// It is request-scoped. Each phase runs at most once, request first.
type Extractor interface {
	// ExtractRequest adds Model and CorrelatingToolCallID to the start record.
	ExtractRequest(reqBody []byte, start *recorder.InterceptionRecord) error
	// ExtractResponse returns the recording facts of the final upstream
	// response. body is the completed capture, partial if capture failed, and
	// nil if no response was received, which yields an empty result. On error,
	// the result holds the facts extracted before the failure.
	ExtractResponse(ctx context.Context, statusCode int, header http.Header, body []byte) (ResponseResult, error)
}

// ResponseResult holds the facts extracted from a response. The caller stamps
// records with the interception ID and creation time.
type ResponseResult struct {
	// Prompt is nil when the request had no prompt or the response has no
	// message ID.
	Prompt   *recorder.PromptUsageRecord
	Tokens   []recorder.TokenUsageRecord
	Tools    []recorder.ToolUsageRecord
	Thoughts []recorder.ModelThoughtRecord

	// ResponseError is a failure reported by the upstream response.
	// Extraction failures are returned separately by ExtractResponse.
	ResponseError error
}

// streamingFromContentType reports whether the response content type declares
// a streamed (SSE) or blocking (JSON) response. It returns an error when the
// content type is missing, malformed, or unsupported.
func streamingFromContentType(header http.Header) (streaming bool, err error) {
	contentType := header.Get("Content-Type")
	if contentType == "" {
		return false, xerrors.New("missing response content type")
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false, xerrors.Errorf("parse response content type %q: %w", contentType, err)
	}
	switch {
	case mediaType == "text/event-stream":
		return true, nil
	case mediaType == "application/json", strings.HasSuffix(mediaType, "+json"):
		return false, nil
	default:
		return false, xerrors.Errorf("unsupported response content type %q", contentType)
	}
}

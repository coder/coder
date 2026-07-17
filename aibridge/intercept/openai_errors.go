package intercept

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"

	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/utils"
)

// OpenAI error type and code constants used by the chatcompletions
// and responses interceptors. The OpenAI Go SDK does not expose
// these as typed constants, so we define our own.
// See https://platform.openai.com/docs/guides/error-codes.
const (
	OpenAIErrTypeError     = "error"
	OpenAIErrTypeAPI       = "api_error"
	OpenAIErrTypeRateLimit = "rate_limit_error"

	OpenAIErrCodeServer    = "server_error"
	OpenAIErrCodeRateLimit = "rate_limit_exceeded"
)

var _ error = &ResponseError{}

// ResponseError is the OpenAI-shaped error envelope returned to
// clients. StatusCode and RetryAfter map to HTTP headers, not JSON
// fields. The chatcompletions and responses interceptors both
// use the same response error format.
type ResponseError struct {
	ErrorObject *shared.ErrorObject `json:"error"`
	StatusCode  int                 `json:"-"`
	RetryAfter  time.Duration       `json:"-"`
}

// NewResponseError builds a ResponseError with the OpenAI-shaped
// envelope. errType and code should be one of the OpenAIErrType*
// and OpenAIErrCode* constants defined above.
func NewResponseError(msg, errType, code string, status int, retryAfter time.Duration) *ResponseError {
	return &ResponseError{
		ErrorObject: &shared.ErrorObject{
			Code:    code,
			Message: msg,
			Type:    errType,
		},
		StatusCode: status,
		RetryAfter: retryAfter,
	}
}

func (e *ResponseError) Error() string {
	if e.ErrorObject == nil {
		return ""
	}
	return e.ErrorObject.Message
}

// ToResponse marshals e into an *http.Response shaped for the
// OpenAI API.
func (e *ResponseError) ToResponse() *http.Response {
	body, err := json.Marshal(e)
	if err != nil {
		body = []byte(`{"error":{"type":"error","message":"error marshaling upstream error","code":"server_error"}}`)
	}
	return utils.NewJSONErrorResponse(e.StatusCode, e.RetryAfter, body)
}

// ResponseErrorFromKeyPool translates a *keypool.Error into
// a developer-facing ResponseError shaped for the OpenAI API.
func ResponseErrorFromKeyPool(keyPoolErr *keypool.Error) *ResponseError {
	if keyPoolErr == nil {
		return nil
	}
	switch keyPoolErr.Kind {
	case keypool.ErrorKindPermanent:
		return NewResponseError(
			keyPoolErr.Error(),
			OpenAIErrTypeAPI,
			OpenAIErrCodeServer,
			http.StatusBadGateway,
			keyPoolErr.RetryAfter,
		)
	case keypool.ErrorKindRateLimited:
		return NewResponseError(
			keyPoolErr.Error(),
			OpenAIErrTypeRateLimit,
			OpenAIErrCodeRateLimit,
			http.StatusTooManyRequests,
			keyPoolErr.RetryAfter,
		)
	default:
		// Fall back to a generic 502.
		return NewResponseError(
			keyPoolErr.Error(),
			OpenAIErrTypeAPI,
			OpenAIErrCodeServer,
			http.StatusBadGateway,
			keyPoolErr.RetryAfter,
		)
	}
}

// ResponseErrorFromAPIError converts an OpenAI SDK error into a
// ResponseError. Returns nil if err is not an *openai.Error.
func ResponseErrorFromAPIError(err error) *ResponseError {
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		return nil
	}
	return NewResponseError(apiErr.Message, apiErr.Type, apiErr.Code, apiErr.StatusCode, keypool.ParseRetryAfter(apiErr.Response))
}

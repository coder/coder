package httperror

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

type Responder interface {
	Response() (int, codersdk.Response)
}

func IsResponder(err error) (Responder, bool) {
	var responseErr Responder
	if errors.As(err, &responseErr) {
		return responseErr, true
	}
	return nil, false
}

func NewResponseError(status int, resp codersdk.Response) error {
	return &responseError{
		status:   status,
		response: resp,
	}
}

// WrapResponseError returns an HTTP response error that unwraps to cause.
func WrapResponseError(status int, resp codersdk.Response, cause error) error {
	return &responseError{
		status:   status,
		response: resp,
		cause:    cause,
	}
}

// StatusCode returns the status that WriteResponseError writes for err.
func StatusCode(err error) int {
	if responseErr, ok := IsResponder(err); ok {
		code, _ := responseErr.Response()
		return code
	}
	return http.StatusInternalServerError
}

func WriteResponseError(ctx context.Context, rw http.ResponseWriter, err error) {
	if responseErr, ok := IsResponder(err); ok {
		code, resp := responseErr.Response()

		httpapi.Write(ctx, rw, code, resp)
		return
	}

	httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
		Message: "Internal server error",
		Detail:  err.Error(),
	})
}

type responseError struct {
	status   int
	response codersdk.Response
	cause    error
}

var (
	_ error     = (*responseError)(nil)
	_ Responder = (*responseError)(nil)
)

func (e *responseError) Error() string {
	return fmt.Sprintf("%s: %s", e.response.Message, e.response.Detail)
}

func (e *responseError) Unwrap() error {
	return e.cause
}

func (e *responseError) Status() int {
	return e.status
}

func (e *responseError) Response() (int, codersdk.Response) {
	return e.status, e.response
}

var ErrResourceNotFound = NewResponseError(http.StatusNotFound, httpapi.ResourceNotFoundResponse)

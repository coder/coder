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

// NewResponseWithError is like NewResponseError but preserves the underlying
// error in the chain so callers traversing with errors.As / errors.Is can
// inspect it. Use this when you want to surface a Responder for
// httperror.WriteResponseError while keeping the typed cause reachable for
// upstream logic (for example, ReadModifyUpdate's pq.Error retry detection).
func NewResponseWithError(status int, resp codersdk.Response, err error) error {
	return &responseError{
		status:   status,
		response: resp,
		inner:    err,
	}
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
	// inner is the wrapped underlying error, exposed via Unwrap so that
	// errors.As / errors.Is callers can drill through this layer. Nil for
	// errors constructed via NewResponseError, which means errors.As against
	// a non-Responder target stops here.
	inner error
}

var (
	_ error     = (*responseError)(nil)
	_ Responder = (*responseError)(nil)
)

func (e *responseError) Error() string {
	return fmt.Sprintf("%s: %s", e.response.Message, e.response.Detail)
}

func (e *responseError) Status() int {
	return e.status
}

func (e *responseError) Response() (int, codersdk.Response) {
	return e.status, e.response
}

// Unwrap returns the underlying error supplied at construction time, or nil
// for response errors built without an underlying cause. errors.As / errors.Is
// treat a nil Unwrap as the end of the chain.
func (e *responseError) Unwrap() error {
	return e.inner
}

var ErrResourceNotFound = NewResponseError(http.StatusNotFound, httpapi.ResourceNotFoundResponse)

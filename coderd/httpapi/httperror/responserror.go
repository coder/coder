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

// workspaceBuildResponder is implemented by errors that can render
// themselves as a workspace-build-shaped 4xx envelope. It is consumed
// by WriteWorkspaceBuildError. The build envelope is structurally a
// superset of codersdk.Response and carries per-validation Kind
// discriminators so the frontend can route entries.
//
// The interface is unexported because the only producer is
// *dynamicparameters.DiagnosticError (via its BuildResponse method) and
// the only consumer is WriteWorkspaceBuildError in this package.
// Callers that need to assert the kinded-envelope contract in tests
// should use errors.As against the concrete producer.
type workspaceBuildResponder interface {
	BuildResponse() (int, codersdk.WorkspaceBuildErrorResponse)
}

func isWorkspaceBuildResponder(err error) (workspaceBuildResponder, bool) {
	var responseErr workspaceBuildResponder
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

var ErrResourceNotFound = NewResponseError(http.StatusNotFound, httpapi.ResourceNotFoundResponse)

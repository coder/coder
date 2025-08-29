package httperror

import (
	"errors"
	"fmt"

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

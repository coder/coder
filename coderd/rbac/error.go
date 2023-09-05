package rbac

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/topdown"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/httpapi/httpapiconstraints"
)

const (
	// errUnauthorized is the error message that should be returned to
	// clients when an action is forbidden. It is intentionally vague to prevent
	// disclosing information that a client should not have access to.
	errUnauthorized = "rbac: forbidden"
)

// UnauthorizedError is the error type for authorization errors
type UnauthorizedError struct {
	// internal is the internal error that should never be shown to the client.
	// It is only for debugging purposes.
	internal error

	// These fields are for debugging purposes.
	subject Subject
	action  Action
	// Note only the object type is set for partial execution.
	object Object

	output rego.ResultSet
}

// Ensure we implement the IsUnauthorized interface.
var _ httpapiconstraints.IsUnauthorizedError = (*UnauthorizedError)(nil)

// IsUnauthorized implements the IsUnauthorized interface.
func (UnauthorizedError) IsUnauthorized() bool {
	return true
}

// IsUnauthorizedError is a convenience function to check if err is UnauthorizedError.
// It is equivalent to errors.As(err, &UnauthorizedError{}).
func IsUnauthorizedError(err error) bool {
	return errors.As(err, &UnauthorizedError{})
}

// ForbiddenWithInternal creates a new error that will return a simple
// "forbidden" to the client, logging internally the more detailed message
// provided.
func ForbiddenWithInternal(internal error, subject Subject, action Action, object Object, output rego.ResultSet) *UnauthorizedError {
	return &UnauthorizedError{
		internal: internal,
		subject:  subject,
		action:   action,
		object:   object,
		output:   output,
	}
}

func (e UnauthorizedError) Unwrap() error {
	return e.internal
}

func (e *UnauthorizedError) longError() string {
	return fmt.Sprintf(
		"%s: (subject: %v), (action: %v), (object: %v), (output: %v)",
		errUnauthorized, e.subject, e.action, e.object, e.output,
	)
}

// Error implements the error interface.
func (e UnauthorizedError) Error() string {
	if flag.Lookup("test.v") != nil {
		return e.longError()
	}
	return errUnauthorized
}

// Internal allows the internal error message to be logged.
func (e *UnauthorizedError) Internal() error {
	return e.internal
}

func (e *UnauthorizedError) SetInternal(err error) {
	e.internal = err
}

func (e *UnauthorizedError) Input() map[string]interface{} {
	return map[string]interface{}{
		"subject": e.subject,
		"action":  e.action,
		"object":  e.object,
	}
}

// Output contains the results of the Rego query for debugging.
func (e *UnauthorizedError) Output() rego.ResultSet {
	return e.output
}

// As implements the errors.As interface.
func (*UnauthorizedError) As(target interface{}) bool {
	if _, ok := target.(*UnauthorizedError); ok {
		return true
	}
	return false
}

// correctCancelError will return the correct error for a canceled context. This
// is because rego changes a canceled context to a topdown.CancelErr. This error
// is not helpful if the code is "canceled". To make the error conform with the
// rest of our canceled errors, we will convert the error to a context.Canceled
// error. No good information is lost, as the topdown.CancelErr provides the
// location of the query that was canceled, which does not matter.
func correctCancelError(err error) error {
	e := new(topdown.Error)
	if xerrors.As(err, &e) || e.Code == topdown.CancelErr {
		return context.Canceled
	}
	return err
}

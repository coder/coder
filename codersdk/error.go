package codersdk

import (
	"fmt"
	"net"

	"golang.org/x/xerrors"
)

// Response represents a generic HTTP response.
type Response struct {
	// Message is an actionable message that depicts actions the request took.
	// These messages should be fully formed sentences with proper punctuation.
	// Examples:
	// - "A user has been created."
	// - "Failed to create a user."
	Message string `json:"message"`
	// Detail is a debug message that provides further insight into why the
	// action failed. This information can be technical and a regular golang
	// err.Error() text.
	// - "database: too many open connections"
	// - "stat: too many open files"
	Detail string `json:"detail,omitempty"`
	// Validations are form field-specific friendly error messages. They will be
	// shown on a form field in the UI. These can also be used to add additional
	// context if there is a set of errors in the primary 'Message'.
	Validations []ValidationError `json:"validations,omitempty"`
}

// ValidationError represents a scoped error to a user input.
type ValidationError struct {
	Field  string `json:"field" validate:"required"`
	Detail string `json:"detail" validate:"required"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("field: %s detail: %s", e.Field, e.Detail)
}

var _ error = (*ValidationError)(nil)

// IsConnectionErr is a convenience function for checking if the source of an
// error is due to a 'connection refused', 'no such host', etc.
func IsConnectionErr(err error) bool {
	var (
		// E.g. no such host
		dnsErr *net.DNSError
		// Eg. connection refused
		opErr *net.OpError
	)

	return xerrors.As(err, &dnsErr) || xerrors.As(err, &opErr)
}

func AsError(err error) (*Error, bool) {
	var e *Error
	return e, xerrors.As(err, &e)
}

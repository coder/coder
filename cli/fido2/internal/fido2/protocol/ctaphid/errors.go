package ctaphid

import (
	"errors"
)

// Common errors returned by the CTAPHID package.
var (
	ErrMessageTooLarge        = errors.New("ctaphid: message payload too large")
	ErrUnexpectedCommand      = errors.New("ctaphid: unexpected command")
	ErrInvalidResponseMessage = errors.New("ctaphid: invalid response message")
)

// CTAPError represents a CTAP2 error response from the authenticator.
// It wraps the failed command and the status code returned.
type CTAPError struct {
	Command    Command
	StatusCode StatusCode
}

// newCTAPError creates a new CTAPError.
func newCTAPError(cmd Command, code StatusCode) *CTAPError {
	return &CTAPError{
		Command:    cmd,
		StatusCode: code,
	}
}

// Error returns the string representation of the error.
func (e *CTAPError) Error() string {
	return e.Command.String() + " failed (" + e.StatusCode.String() + ")"
}

// Unwrap returns an error representing the status code.
func (e *CTAPError) Unwrap() error {
	return errors.New(e.StatusCode.String())
}

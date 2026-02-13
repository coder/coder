package fido2

import "errors"

// Common errors returned by the fido2 package.
var (
	// ErrPinUvAuthTokenRequired is returned when a PIN or UV auth token is required for an operation.
	ErrPinUvAuthTokenRequired = errors.New("fido2: pinUvAuthToken required")
	// ErrNotSupported is returned when an operation or extension is not supported by the device.
	ErrNotSupported = errors.New("fido2: not supported")
	// ErrSyntaxError is returned when there is a syntax error in the request or response.
	ErrSyntaxError = errors.New("fido2: syntax error")
	// ErrInvalidSaltSize is returned when the salt size for HMAC-secret or PRF is invalid.
	ErrInvalidSaltSize = errors.New("fido2: invalid salt size")
	// ErrPinNotSet is returned when an operation requires a PIN to be set but it is not.
	ErrPinNotSet = errors.New("fido2: pin not set")
)

// ErrorWithMessage represents an error with an additional descriptive message.
type ErrorWithMessage struct {
	Message string
	Err     error
}

// newErrorMessage creates a new ErrorWithMessage.
func newErrorMessage(err error, msg string) *ErrorWithMessage {
	return &ErrorWithMessage{
		Message: msg,
		Err:     err,
	}
}

// Error returns the string representation of the error.
func (m *ErrorWithMessage) Error() string {
	if m.Message != "" {
		return m.Err.Error() + " (" + m.Message + ")"
	}
	return m.Err.Error()
}

// Unwrap returns the underlying error.
func (m *ErrorWithMessage) Unwrap() error {
	return m.Err
}

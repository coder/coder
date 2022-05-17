package rbac

import "github.com/open-policy-agent/opa/rego"

const (
	// errUnauthorized is the error message that should be returned to
	// clients when an action is forbidden. It is intentionally vague to prevent
	// disclosing information that a client should not have access to.
	errUnauthorized = "forbidden"
)

// UnauthorizedError is the error type for authorization errors
type UnauthorizedError struct {
	// internal is the internal error that should never be shown to the client.
	// It is only for debugging purposes.
	internal error
	input    map[string]interface{}
	output   rego.ResultSet
}

// ForbiddenWithInternal creates a new error that will return a simple
// "forbidden" to the client, logging internally the more detailed message
// provided.
func ForbiddenWithInternal(internal error, input map[string]interface{}, output rego.ResultSet) *UnauthorizedError {
	if input == nil {
		input = map[string]interface{}{}
	}
	return &UnauthorizedError{
		internal: internal,
		input:    input,
		output:   output,
	}
}

// Error implements the error interface.
func (UnauthorizedError) Error() string {
	return errUnauthorized
}

// Internal allows the internal error message to be logged.
func (e *UnauthorizedError) Internal() error {
	return e.internal
}

func (e *UnauthorizedError) Input() map[string]interface{} {
	return e.input
}

// Output contains the results of the Rego query for debugging.
func (e *UnauthorizedError) Output() rego.ResultSet {
	return e.output
}

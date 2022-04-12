package rbac

const (
	// UnauthorizedErrorMessage is the error message that should be returned to
	// clients when an action is forbidden. It is intentionally vague to prevent
	// disclosing information that a client should not have access to.
	UnauthorizedErrorMessage = "unauthorized"
)

// Unauthorized is the error type for authorization errors
type Unauthorized struct {
	// internal is the internal error that should never be shown to the client.
	// It is only for debugging purposes.
	internal error
	input    map[string]interface{}
}

// ForbiddenWithInternal creates a new error that will return a simple
// "forbidden" to the client, logging internally the more detailed message
// provided.
func ForbiddenWithInternal(internal error, input map[string]interface{}) *Unauthorized {
	if input == nil {
		input = map[string]interface{}{}
	}
	return &Unauthorized{
		internal: internal,
		input:    input,
	}
}

// Error implements the error interface.
func (e *Unauthorized) Error() string {
	return UnauthorizedErrorMessage
}

// Internal allows the internal error message to be logged.
func (e *Unauthorized) Internal() error {
	return e.internal
}

func (e *Unauthorized) Input() map[string]interface{} {
	return e.input
}

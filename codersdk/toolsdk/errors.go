package toolsdk

// PublicError describes a tool failure that can be shown to the caller.
// Message must contain only user-facing information, never internal error
// details. Cause is retained for diagnostics and error matching.
type PublicError struct {
	Message string
	Cause   error
}

// Error returns the message and any underlying diagnostic details.
func (e *PublicError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

// Unwrap returns the underlying error, if any.
func (e *PublicError) Unwrap() error {
	return e.Cause
}

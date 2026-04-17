package mcp

import "context"

// requestorKey is an unexported type so only this package can set/read it.
type requestorKey struct{}

// Requestor describes who initiated an MCP HTTP request. It is attached to
// the request context by the HTTP entrypoint and consumed by tool handlers
// for structured logging.
type Requestor struct {
	UserID    string
	Username  string
	Email     string
	APIKeyID  string
	RequestID string
	UserAgent string
}

// WithRequestor returns a copy of ctx carrying the provided Requestor.
func WithRequestor(ctx context.Context, r Requestor) context.Context {
	return context.WithValue(ctx, requestorKey{}, r)
}

// RequestorFromContext returns the Requestor stored on ctx, if any.
func RequestorFromContext(ctx context.Context) (Requestor, bool) {
	r, ok := ctx.Value(requestorKey{}).(Requestor)
	return r, ok
}

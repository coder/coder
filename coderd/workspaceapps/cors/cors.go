package cors

import "context"

type AppCORSBehavior string

const (
	AppCORSBehaviorSimple   AppCORSBehavior = "simple"
	AppCORSBehaviorPassthru AppCORSBehavior = "passthru"
)

type contextKeyBehavior struct{}

// WithBehavior sets the CORS behavior for the given context.
func WithBehavior(ctx context.Context, behavior AppCORSBehavior) context.Context {
	return context.WithValue(ctx, contextKeyBehavior{}, behavior)
}

// HasBehavior returns true if the given context has the specified CORS behavior.
func HasBehavior(ctx context.Context, behavior AppCORSBehavior) bool {
	val := ctx.Value(contextKeyBehavior{})
	b, ok := val.(AppCORSBehavior)
	return ok && b == behavior
}

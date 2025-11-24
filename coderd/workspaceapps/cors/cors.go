package cors

import (
	"context"

	"github.com/coder/coder/v2/codersdk"
)

type contextKeyBehavior struct{}

// WithBehavior sets the CORS behavior for the given context.
func WithBehavior(ctx context.Context, behavior codersdk.CORSBehavior) context.Context {
	return context.WithValue(ctx, contextKeyBehavior{}, behavior)
}

// HasBehavior returns true if the given context has the specified CORS behavior.
func HasBehavior(ctx context.Context, behavior codersdk.CORSBehavior) bool {
	val := ctx.Value(contextKeyBehavior{})
	b, ok := val.(codersdk.CORSBehavior)
	return ok && b == behavior
}

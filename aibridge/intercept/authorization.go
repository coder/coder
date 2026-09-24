package intercept

import "context"

// AuthorizationErrorKind identifies the outcome of a request-scoped model
// authorization check.
type AuthorizationErrorKind uint8

const (
	AuthorizationErrorAuthentication AuthorizationErrorKind = iota + 1
	AuthorizationErrorPolicy
	AuthorizationErrorEvaluation
	AuthorizationErrorMalformed
)

// AuthorizationError is returned by a request authorization hook.
type AuthorizationError struct {
	Kind AuthorizationErrorKind
	Err  error
}

func (e *AuthorizationError) Error() string {
	if e == nil || e.Err == nil {
		return "authorization failed"
	}
	return e.Err.Error()
}

func (e *AuthorizationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// RequestAuthorizer authorizes the exact provider invocation identifier.
type RequestAuthorizer func(ctx context.Context, providerName, invocationModel string) error

type requestAuthorizerContextKey struct{}

// WithRequestAuthorizer attaches a request-scoped authorization hook.
func WithRequestAuthorizer(ctx context.Context, authorizer RequestAuthorizer) context.Context {
	return context.WithValue(ctx, requestAuthorizerContextKey{}, authorizer)
}

func RequestAuthorizerFromContext(ctx context.Context) RequestAuthorizer {
	if authorizer, ok := ctx.Value(requestAuthorizerContextKey{}).(RequestAuthorizer); ok {
		return authorizer
	}
	return nil
}

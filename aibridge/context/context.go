package context

import (
	"context"

	"github.com/coder/coder/v2/aibridge/recorder"
)

type (
	actorContextKey struct{}
)

type Actor struct {
	ID string
	// Email is kept out of Metadata so it is never recorded or logged. It is
	// only read when forwarding actor headers upstream.
	Email    string
	Metadata recorder.Metadata
}

func AsActor(ctx context.Context, actorID, email string, metadata recorder.Metadata) context.Context {
	return context.WithValue(ctx, actorContextKey{}, &Actor{ID: actorID, Email: email, Metadata: metadata})
}

func ActorFromContext(ctx context.Context) *Actor {
	a, ok := ctx.Value(actorContextKey{}).(*Actor)
	if !ok {
		return nil
	}

	return a
}

// ActorIDFromContext safely extracts the actor ID from the context.
// Returns an empty string if no actor is found.
func ActorIDFromContext(ctx context.Context) string {
	if actor := ActorFromContext(ctx); actor != nil {
		return actor.ID
	}
	return ""
}

type createAdmissionContextKey struct{}

// CreateAdmissionFunc decides whether one Responses WebSocket
// response.create for model may be forwarded upstream. It returns nil to
// admit the create. A refusal is an *intercept.ResponseError carrying the
// status, type, code, and message relayed to the client, which keeps its
// connection.
type CreateAdmissionFunc func(ctx context.Context, model string) error

// WithCreateAdmission returns a copy of ctx carrying admit. Only the gateway
// sets it, for every authorized request, so each create of a WebSocket the
// request opens is checked like a separate HTTP request.
func WithCreateAdmission(ctx context.Context, admit CreateAdmissionFunc) context.Context {
	return context.WithValue(ctx, createAdmissionContextKey{}, admit)
}

// CreateAdmissionFromContext returns the function attached by
// [WithCreateAdmission], or nil when none was attached.
func CreateAdmissionFromContext(ctx context.Context) CreateAdmissionFunc {
	admit, _ := ctx.Value(createAdmissionContextKey{}).(CreateAdmissionFunc)
	return admit
}

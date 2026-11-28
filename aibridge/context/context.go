package context

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

type (
	actorContextKey          struct{}
	interceptionIDContextKey struct{}
)

// ErrMissingAPIKeyID indicates that the context has no actor with an API-key ID.
var ErrMissingAPIKeyID = xerrors.New("missing authenticated API-key ID")

// Actor is the authenticated identity attached to an AI Gateway request.
type Actor struct {
	ID       uuid.UUID
	APIKeyID string
	Username string
	// Email is only used for the opt-in email actor header forwarding. It is kept
	// out of interception recordings.
	Email string
}

// AsActor returns a context containing a copy of the authenticated actor.
func AsActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, actorContextKey{}, &actor)
}

// ActorFromContext returns the authenticated actor, or nil when absent.
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
	if actor := ActorFromContext(ctx); actor != nil && actor.ID != uuid.Nil {
		return actor.ID.String()
	}
	return ""
}

// APIKeyIDFromContext returns the actor's API-key ID, or ErrMissingAPIKeyID if
// the actor is absent or its API-key ID is empty.
func APIKeyIDFromContext(ctx context.Context) (string, error) {
	actor := ActorFromContext(ctx)
	if actor == nil || actor.APIKeyID == "" {
		return "", ErrMissingAPIKeyID
	}
	return actor.APIKeyID, nil
}

// WithInterceptionID returns a context with the interception_id
// log field set.
func WithInterceptionID(ctx context.Context, id string) context.Context {
	ctx = slog.With(ctx, slog.F("interception_id", id))
	return context.WithValue(ctx, interceptionIDContextKey{}, id)
}

// HasInterceptionID reports whether ctx already carries the
// interception_id field.
func HasInterceptionID(ctx context.Context) bool {
	_, ok := ctx.Value(interceptionIDContextKey{}).(string)
	return ok
}

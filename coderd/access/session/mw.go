package session

import (
	"context"
	"net/http"

	"github.com/coder/coder/coderd/database"
)

type actorContextKey struct{}

// APIKey returns the API key from the ExtractAPIKey handler.
func RequestActor(r *http.Request) Actor {
	actor, ok := r.Context().Value(actorContextKey{}).(Actor)
	if !ok {
		panic("developer error: ExtractActor middleware not provided")
	}
	return actor
}

// ExtractActor determines the Actor from the request. It will try to get the
// following actors in order:
//   1. UserActor
//   2. AnonymousActor
func ExtractActor(db database.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			var (
				ctx = r.Context()
				act Actor
			)

			// Try to get a UserActor.
			act, ok := userActorFromRequest(ctx, db, rw, r)
			if !ok {
				return
			}

			// TODO: Dean - WorkspaceActor, SatelliteActor etc.

			// Fallback to an AnonymousActor.
			if act == nil {
				act = Anon
			}

			ctx = context.WithValue(ctx, actorContextKey{}, act)
			next.ServeHTTP(rw, r.WithContext(ctx))
			return
		})
	}
}

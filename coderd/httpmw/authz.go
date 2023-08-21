package httpmw

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

// AsAuthzSystem is a chained handler that temporarily sets the dbauthz context
// to System for the inner handlers, and resets the context afterwards.
//
// TODO: Refactor the middleware functions to not require this.
// This is a bit of a kludge for now as some middleware functions require
// usage as a system user in some cases, but not all cases. To avoid large
// refactors, we use this middleware to temporarily set the context to a system.
func AsAuthzSystem(mws ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	chain := chi.Chain(mws...)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			before, beforeExists := dbauthz.ActorFromContext(r.Context())
			if !beforeExists {
				// AsRemoveActor will actually remove the actor from the context.
				before = dbauthz.AsRemoveActor
			}

			// nolint:gocritic // AsAuthzSystem needs to do this.
			r = r.WithContext(dbauthz.AsSystemRestricted(ctx))
			chain.Handler(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				r = r.WithContext(dbauthz.As(r.Context(), before))
				next.ServeHTTP(rw, r)
			})).ServeHTTP(rw, r)
		})
	}
}

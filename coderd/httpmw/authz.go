package httpmw

import (
	"net/http"

	"github.com/coder/coder/coderd/database/dbauthz"

	"github.com/go-chi/chi/v5"
)

// AsAuthzSystem is a bit of a kludge for now. Some middleware functions require
// usage as a system user in some cases, but not all cases. To avoid large
// refactors, we use this middleware to temporarily set the context to a system.
//
// TODO: Refact the middleware functions to not require this.
func AsAuthzSystem(mws ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	chain := chi.Chain(mws...)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			before, _ := dbauthz.ActorFromContext(r.Context())

			r = r.WithContext(dbauthz.AsSystem(ctx))
			chain.Handler(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				r = r.WithContext(dbauthz.As(r.Context(), before))
				next.ServeHTTP(rw, r)
			})).ServeHTTP(rw, r)
		})
	}
}

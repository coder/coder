//go:build !slim

package httpmw

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
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

// RecordAuthzChecks enables recording all the authorization checks that
// occurred in the processing of a request. This is mostly helpful for debugging
// and understanding what permissions are required for a given action.
//
// Can either be toggled on by a deployment wide configuration value, or opt-in on
// a per-request basis by setting the `x-record-authz-checks` header to a truthy value.
//
// Requires using a Recorder Authorizer.
//
//nolint:revive
func RecordAuthzChecks(always bool) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if enabled, _ := strconv.ParseBool(r.Header.Get("x-record-authz-checks")); enabled || always {
				r = r.WithContext(rbac.WithAuthzCheckRecorder(r.Context()))
			}

			next.ServeHTTP(rw, r)
		})
	}
}

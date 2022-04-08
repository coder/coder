package httpmw

import (
	"fmt"
	"net/http"

	"github.com/coder/coder/coderd/access/session"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
)

// RequestActor reexports session.RequestActor for your convenience.
func RequestActor(r *http.Request) session.Actor {
	return session.RequestActor(r)
}

// ExtractActor reexports session.ExtractActor for your convenience.
func ExtractActor(db database.Store) func(http.Handler) http.Handler {
	return session.ExtractActor(db)
}

// RequireAuthentication returns a 401 Unauthorized response if the request
// doesn't have an actor. If you want to require a specific actor type, you
// should use the sibling middleware RequireActor() below.
//
// Depends on session.ExtractActor middleware.
func RequireAuthentication() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if session.RequestActor(r) == nil {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: "authentication required",
				})
				return
			}

			next.ServeHTTP(rw, r)
		})
	}
}

// RequireActor returns a 401 Unauthorized response if the request doesn't have
// an actor or the request's actor type doesn't match the provided type. If you
// don't require a specific actor type, you should use the sibling middleware
// RequireAuthentication() above.
//
// Depends on session.ExtractActor middleware.
func RequireActor(actorType session.ActorType) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			act := session.RequestActor(r)
			if act == nil {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: "authentication required",
				})
				return
			}
			if act.Type() != actorType {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: fmt.Sprintf(
						"only %q actors can access this endpoint (currently %q)",
						actorType,
						act.Type(),
					),
				})
				return
			}

			next.ServeHTTP(rw, r)
		})
	}
}

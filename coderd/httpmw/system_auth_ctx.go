package httpmw

import (
	"net/http"

	"github.com/coder/coder/coderd/authzquery"
	"github.com/coder/coder/coderd/rbac"
)

// SystemAuthCtx sets the system auth context for the request.
// Use sparingly.
func SystemAuthCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		ctx := authzquery.WithAuthorizeSystemContext(r.Context(), rbac.RolesAdminSystem())
		next.ServeHTTP(rw, r.WithContext(ctx))
	})
}

package httpmw

import (
	"net/http"
	"time"

	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/coder/coder/v2/coderd/jwtutils"
)

const healthcheckSubject = "healthcheck"

// HealthcheckOrSessionAuth returns middleware that grants access to
// the wrapped handler if the request carries a valid healthcheck JWT
// signed with verifyKey. When the JWT is absent or invalid, the
// request is passed through sessionAuth and rbacAuth so that regular
// session-token-based auth (with RBAC) still works for human callers.
//
// This is used for /debug/ws, which the background healthcheck
// runner needs to reach without a real API key. The runner sends a
// short-lived JWT in the Coder-Session-Token header instead.
func HealthcheckOrSessionAuth(
	verifyKey jwtutils.VerifyKeyProvider,
	sessionAuth func(http.Handler) http.Handler,
	rbacAuth func(http.Handler) http.Handler,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			token := APITokenFromRequest(r)
			if token != "" {
				var claims jwtutils.RegisteredClaims
				err := jwtutils.Verify(
					r.Context(), verifyKey, token, &claims,
					jwtutils.WithVerifyExpected(jwt.Expected{
						Subject: healthcheckSubject,
						Time:    time.Now(),
					}),
				)
				if err == nil {
					// Valid healthcheck JWT; skip session auth
					// and RBAC. The echo endpoint exposes no
					// sensitive data.
					next.ServeHTTP(rw, r)
					return
				}
			}

			// Not a valid healthcheck JWT. Fall through to
			// normal session auth + RBAC.
			sessionAuth(rbacAuth(next)).ServeHTTP(rw, r)
		})
	}
}

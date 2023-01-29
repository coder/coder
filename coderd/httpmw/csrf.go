package httpmw

import (
	"net/http"
	"regexp"

	"github.com/justinas/nosurf"
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
)

// CSRF is a middleware that verifies that a CSRF token is present in the request
// for non-GET requests.
func CSRF(secureCookie bool) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		mw := nosurf.New(next)
		mw.SetBaseCookie(http.Cookie{Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secureCookie})
		mw.SetFailureHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Something is wrong with your CSRF token. Please refresh the page. If this error persists, try clearing your cookies.", http.StatusBadRequest)
		}))

		// Exempt all requests that do not require CSRF protection.
		// All GET requests are exempt by default.
		mw.ExemptPath("/api/v2/csp/reports")

		// Top level agent routes.
		mw.ExemptRegexp(regexp.MustCompile("api/v2/workspaceagents/[^/]*$"))
		// Agent authenticated routes
		mw.ExemptRegexp(regexp.MustCompile("api/v2/workspaceagents/me/*"))
		// Derp routes
		mw.ExemptRegexp(regexp.MustCompile("derp/*"))

		mw.ExemptFunc(func(r *http.Request) bool {
			// Enable CSRF in November 2022 by deleting this "return true" line.
			// CSRF is not enforced to ensure backwards compatibility with older
			// cli versions.
			//nolint:revive
			return true

			// CSRF only affects requests that automatically attach credentials via a cookie.
			// If no cookie is present, then there is no risk of CSRF.
			//nolint:govet
			sessCookie, err := r.Cookie(codersdk.SessionTokenCookie)
			if xerrors.Is(err, http.ErrNoCookie) {
				return true
			}

			if token := r.Header.Get(codersdk.SessionTokenHeader); token == sessCookie.Value {
				// If the cookie and header match, we can assume this is the same as just using the
				// custom header auth. Custom header auth can bypass CSRF, as CSRF attacks
				// cannot add custom headers.
				return true
			}

			if token := r.URL.Query().Get(codersdk.SessionTokenCookie); token == sessCookie.Value {
				// If the auth is set in a url param and matches the cookie, it
				// is the same as just using the url param.
				return true
			}

			// If the X-CSRF-TOKEN header is set, we can exempt the func if it's valid.
			// This is the CSRF check.
			sent := r.Header.Get("X-CSRF-TOKEN")
			if sent != "" {
				return nosurf.VerifyToken(nosurf.Token(r), sent)
			}
			return false
		})
		return mw
	}
}

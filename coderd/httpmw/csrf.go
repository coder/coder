package httpmw

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/justinas/nosurf"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// CSRF is a middleware that verifies that a CSRF token is present in the request
// for non-GET requests.
// If enforce is false, then CSRF enforcement is disabled. We still want
// to include the CSRF middleware because it will set the CSRF cookie.
func CSRF(cookieCfg codersdk.HTTPCookieConfig) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		mw := nosurf.New(next)
		mw.SetBaseCookie(*cookieCfg.Apply(&http.Cookie{Path: "/", HttpOnly: true}))
		mw.SetFailureHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sessCookie, err := r.Cookie(codersdk.SessionTokenCookie)
			if err == nil &&
				r.Header.Get(codersdk.SessionTokenHeader) != "" &&
				r.Header.Get(codersdk.SessionTokenHeader) != sessCookie.Value {
				// If a user is using header authentication and cookie auth, but the values
				// do not match, the cookie value takes priority.
				// At the very least, return a more helpful error to the user.
				http.Error(w,
					fmt.Sprintf("CSRF error encountered. Authentication via %q cookie and %q header detected, but the values do not match. "+
						"To resolve this issue ensure the values used in both match, or only use one of the authentication methods. "+
						"You can also try clearing your cookies if this error persists.",
						codersdk.SessionTokenCookie, codersdk.SessionTokenHeader),
					http.StatusBadRequest)
				return
			}

			http.Error(w, "Something is wrong with your CSRF token. Please refresh the page. If this error persists, try clearing your cookies.", http.StatusBadRequest)
		}))

		// Exempt all requests that do not require CSRF protection.
		// All GET requests are exempt by default.
		//
		// Exemptions are exact-path matches ONLY. Unanchored regex
		// exemptions were removed because nosurf matches them as
		// substrings of the request path: a pattern like "derp/*"
		// exempted every /api path merely containing "derp", including
		// attacker-influenced segments such as usernames, and nosurf
		// short-circuits before BOTH the token check and its same-origin
		// validation on exempt paths.
		//
		// The removed exemptions were redundant:
		//   - Agent, workspace-proxy, and provisioner-daemon requests
		//     authenticate via headers/PSK and carry no session cookie,
		//     so the ExemptFunc below already exempts them.
		//   - /derp and /scim are not under /api, so the ExemptFunc
		//     prefix check already exempts them.
		//   - The dashboard sends X-CSRF-TOKEN on every request, so
		//     browser flows on the previously exempted routes (e.g.
		//     devcontainer recreate) pass the standard CSRF checks.
		mw.ExemptPath("/api/v2/csp/reports")
		mw.ExemptPath("/api/v2/users/first")

		mw.ExemptFunc(func(r *http.Request) bool {
			// Enforce CSRF on API routes and the OAuth2 authorize
			// endpoint. The authorize endpoint serves a browser consent
			// form whose POST must be CSRF-protected to prevent
			// cross-site authorization code theft (coder/security#121).
			if !strings.HasPrefix(r.URL.Path, "/api") &&
				!strings.HasPrefix(r.URL.Path, "/oauth2/authorize") {
				return true
			}

			// CSRF only affects requests that automatically attach credentials via a cookie.
			// If no cookie is present, then there is no risk of CSRF.
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

			if r.Header.Get(codersdk.ProvisionerDaemonPSK) != "" {
				// If present, the provisioner daemon also is providing an api key
				// that will make them exempt from CSRF. But this is still useful
				// for enumerating the external auths.
				return true
			}

			if r.Header.Get(codersdk.ProvisionerDaemonKey) != "" {
				// If present, the provisioner daemon also is providing an api key
				// that will make them exempt from CSRF. But this is still useful
				// for enumerating the external auths.
				return true
			}

			// RFC 6750 Bearer Token authentication is exempt from CSRF
			// as it uses custom headers that cannot be set by malicious sites
			if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
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

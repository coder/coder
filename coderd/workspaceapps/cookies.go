package workspaceapps

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// AppConnectSessionTokenCookieName returns the cookie name for the session
// token for the given access method.
func AppConnectSessionTokenCookieName(accessMethod AccessMethod) string {
	if accessMethod == AccessMethodSubdomain {
		return codersdk.SubdomainAppSessionTokenCookie
	}
	return codersdk.PathAppSessionTokenCookie
}

// AppConnectSessionTokenFromRequest returns the session token from the request
// if it exists. The access method is used to determine which cookie name to
// use.
//
// We use different cookie names for path apps and for subdomain apps to avoid
// both being set and sent to the server at the same time and the server using
// the wrong value.
//
// We use different cookie names for:
// - path apps on primary access URL: coder_session_token
// - path apps on proxies: coder_path_app_session_token
// - subdomain apps: coder_subdomain_app_session_token
//
// First we try the default function to get a token from request, which supports
// query parameters, the Coder-Session-Token header and the coder_session_token
// cookie.
//
// Then we try the specific cookie name for the access method.
func AppConnectSessionTokenFromRequest(r *http.Request, accessMethod AccessMethod) string {
	// Try the default function first.
	token := httpmw.APITokenFromRequest(r)
	if token != "" {
		return token
	}

	// Then try the specific cookie name for the access method.
	cookie, err := r.Cookie(AppConnectSessionTokenCookieName(accessMethod))
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	return ""
}

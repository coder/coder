package workspaceapps

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

type AppCookies struct {
	PathAppSessionToken      string
	SubdomainAppSessionToken string
	SignedAppToken           string
}

// NewAppCookies returns the cookie names for the app session token for the
// given hostname. The subdomain cookie is unique per workspace proxy and is
// based on a hash of the workspace proxy subdomain hostname. See
// SubdomainAppSessionTokenCookie for more details.
func NewAppCookies(hostname string) AppCookies {
	return AppCookies{
		PathAppSessionToken:      codersdk.PathAppSessionTokenCookie,
		SubdomainAppSessionToken: SubdomainAppSessionTokenCookie(hostname),
		SignedAppToken:           codersdk.SignedAppTokenCookie,
	}
}

// CookieNameForAccessMethod returns the cookie name for the long-lived session
// token for the given access method.
func (c AppCookies) CookieNameForAccessMethod(accessMethod AccessMethod) string {
	if accessMethod == AccessMethodSubdomain {
		return c.SubdomainAppSessionToken
	}
	// Path-based and terminal apps are on the same domain:
	return c.PathAppSessionToken
}

// SubdomainAppSessionTokenCookie returns the cookie name for the subdomain app
// session token. This is unique per workspace proxy and is based on a hash of
// the workspace proxy subdomain hostname.
//
// The reason the cookie needs to be unique per workspace proxy is to avoid
// cookies from one proxy (e.g. the primary) being sent on requests to a
// different proxy underneath the wildcard.
//
// E.g. `*.dev.coder.com` and `*.sydney.dev.coder.com`
//
// If you have an expired cookie on the primary proxy (valid for
// `*.dev.coder.com`), your browser will send it on all requests to the Sydney
// proxy as it's underneath the wildcard.
//
// By using a unique cookie name per workspace proxy, we can avoid this issue.
func SubdomainAppSessionTokenCookie(hostname string) string {
	hash := sha256.Sum256([]byte(hostname))
	// 16 bytes of uniqueness is probably enough.
	str := hex.EncodeToString(hash[:16])
	return codersdk.SubdomainAppSessionTokenCookie + "_" + str
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
// - subdomain apps: coder_subdomain_app_session_token_{unique_hash}
//
// First we try the default function to get a token from request, which supports
// query parameters, the Coder-Session-Token header and the coder_session_token
// cookie.
//
// Then we try the specific cookie name for the access method.
func (c AppCookies) TokenFromRequest(r *http.Request, accessMethod AccessMethod) string {
	// Try the default function first.
	token := httpmw.APITokenFromRequest(r)
	if token != "" {
		return token
	}

	// Then try the specific cookie name for the access method.
	cookie, err := r.Cookie(c.CookieNameForAccessMethod(accessMethod))
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	return ""
}

package httpapi

import (
	"net/textproto"
	"strings"

	"github.com/coder/coder/v2/codersdk"
)

// StripCoderCookies removes the session token from the cookie header provided.
func StripCoderCookies(header string) string {
	header = textproto.TrimString(header)
	cookies := []string{}

	var part string
	for len(header) > 0 { // continue since we have rest
		part, header, _ = strings.Cut(header, ";")
		part = textproto.TrimString(part)
		if part == "" {
			continue
		}
		name, _, _ := strings.Cut(part, "=")
		if name == codersdk.SessionTokenCookie ||
			name == codersdk.OAuth2StateCookie ||
			name == codersdk.OAuth2RedirectCookie ||
			name == codersdk.PathAppSessionTokenCookie ||
			// This uses a prefix check because the subdomain cookie is unique
			// per workspace proxy and is based on a hash of the workspace proxy
			// subdomain hostname. See the workspaceapps package for more
			// details.
			strings.HasPrefix(name, codersdk.SubdomainAppSessionTokenCookie) ||
			name == codersdk.SignedAppTokenCookie {
			continue
		}
		cookies = append(cookies, part)
	}
	return strings.Join(cookies, "; ")
}

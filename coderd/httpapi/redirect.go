package httpapi

import (
	"net/url"
	"strings"
)

// SafeRedirectPath reduces a redirect URL down to a safe, relative path. The
// scheme and host are dropped to prevent redirecting to another origin. Opaque
// URLs (e.g. `javascript:`, `data:`) are rejected outright and default to /.
func SafeRedirectPath(u string) string {
	uri, err := url.Parse(u)
	if err != nil || uri.Opaque != "" {
		return "/"
	}

	// A path with two or more leading slashes (e.g. "///evil.com") is
	// interpreted by some browsers as protocol-relative, so collapse any
	// leading slashes down to exactly one.
	path := "/" + strings.TrimLeft(uri.EscapedPath(), "/")
	if uri.RawQuery != "" {
		return path + "?" + uri.RawQuery
	}
	return path
}

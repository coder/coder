package httpapi

import (
	"net/url"
	"strings"
)

// SafeRedirectPath reduces a redirect URL down to a safe, relative path plus
// query string local to this application. Any scheme and host are dropped,
// since preserving them would allow an open redirect to another site. Opaque
// URLs (e.g. "javascript:..." or "data:...") are rejected outright and
// collapse to "/", since their content isn't a hierarchical path we can
// safely reduce.
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

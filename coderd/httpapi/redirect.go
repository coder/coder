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

	// A path with 2 or more leading slashes (e.g. "//evil.com") is interpreted as
	// protocol-relative, so make sure there is exactly one.
	path := "/" + strings.TrimLeft(uri.EscapedPath(), "/")
	if uri.RawQuery != "" {
		path += "?" + uri.RawQuery
	}
	// We're specifically checking Fragment instead of RawFragment here because
	// RawFragment is only populated when the parser needs to preserve a
	// non-default escaping, so it is empty for plain-alphanumeric fragments like
	// "#wooble". EscapedFragment handles escaping correctly in either case.
	if uri.Fragment != "" {
		path += "#" + uri.EscapedFragment()
	}
	return path
}

// Package xurl contains small helpers extending the standard net/url
// package.
package xurl

import (
	"net/netip"
	"net/url"
	"strings"
	"unicode"
)

// ContainsEncodedPath reports whether u's path is unsafe to forward to
// another server with its escaping intact, because that server could resolve
// it to a different path than the one it was matched on.
//
// [net/http.ServeMux] matches routes on the escaped path and only cleans
// literal dot segments, while upstreams decode percent-encoded unreserved
// characters (RFC 3986 section 6.2.2.2) and then remove dot segments. A path
// such as /v1/models/%2e%2e/files therefore matches a /v1/models/ route
// locally but is served as /v1/files upstream.
//
// Each escaped segment is decoded exactly once. It returns true if a segment
// fails to decode, or if a decoded segment:
//   - contains a "." or ".." element, also when split on an escaped "/" or "\"
//     for components that decode reserved characters,
//   - still contains "%", which only arises from multiple encoding and would
//     let a component that decodes twice produce a dot segment,
//   - contains control characters.
//
// Escaped separators in ordinary segments, such as /models/org%2Fmodel, are
// allowed.
func ContainsEncodedPath(u *url.URL) bool {
	for _, seg := range strings.Split(u.EscapedPath(), "/") {
		decoded, err := url.PathUnescape(seg)
		if err != nil {
			return true
		}
		if strings.ContainsRune(decoded, '%') || strings.ContainsFunc(decoded, unicode.IsControl) {
			return true
		}
		for _, part := range strings.FieldsFunc(decoded, isPathSeparator) {
			if part == "." || part == ".." {
				return true
			}
		}
	}
	return false
}

func isPathSeparator(r rune) bool {
	return r == '/' || r == '\\'
}

// IsHTTP reports whether u uses the http or https scheme.
func IsHTTP(u *url.URL) bool {
	return strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https")
}

// SameOrigin reports whether two URLs have the same scheme, host, and port.
func SameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		sameHost(a.Hostname(), b.Hostname()) &&
		portOrDefault(a) == portOrDefault(b)
}

// sameHost compares IP addresses by value, so "::1" equals "0:0:0:0:0:0:0:1", and other hosts case-insensitively.
func sameHost(a, b string) bool {
	addrA, errA := netip.ParseAddr(a)
	addrB, errB := netip.ParseAddr(b)
	if errA == nil && errB == nil {
		return addrA == addrB
	}
	return strings.EqualFold(a, b)
}

func portOrDefault(u *url.URL) string {
	if port := u.Port(); port != "" {
		if trimmed := strings.TrimLeft(port, "0"); trimmed != "" {
			return trimmed
		}
		return "0"
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

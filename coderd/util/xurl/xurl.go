// Package xurl contains small helpers extending the standard net/url
// package.
package xurl

import (
	"net/netip"
	"net/url"
	"strings"
	"unicode"

	"github.com/coder/safedial"
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

// IsPublicHTTPS reports whether u is an https URL whose host could be
// reachable from the public internet: the host is not localhost and is
// not an IP literal in a range that safedial blocks (loopback, private,
// link-local, multicast, carrier-grade NAT, documentation, and other
// special-use ranges). Host names are not resolved.
func IsPublicHTTPS(u *url.URL) bool {
	if u == nil || u.Scheme != "https" {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return safedial.CheckAddr(ip) == nil
	}
	return true
}

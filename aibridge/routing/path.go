package routing

import (
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"golang.org/x/xerrors"
)

// ErrInvalidForwardPath is returned by [ValidateForwardPath] when a request
// path could resolve outside the route it was matched on once an upstream
// decodes it.
var ErrInvalidForwardPath = xerrors.New("invalid request path")

// ValidateForwardPath rejects request paths that are unsafe to forward
// upstream with their escaping intact.
//
// [http.ServeMux] matches routes on the escaped path and only cleans literal
// dot segments, while upstreams decode percent-encoded unreserved characters
// (RFC 3986 section 6.2.2.2) and then remove dot segments. A path such as
// /v1/models/%2e%2e/files therefore matches an allowlisted route here but is
// served as /v1/files upstream.
//
// Each escaped segment is decoded exactly once. The path is rejected if a
// decoded segment:
//   - contains a "." or ".." element, also when split on an escaped "/" or "\"
//     for components that decode reserved characters,
//   - still contains "%", which only arises from multiple encoding and would
//     let a component that decodes twice produce a dot segment,
//   - contains control characters.
//
// Escaped separators in ordinary segments, such as /models/org%2Fmodel, are
// allowed.
func ValidateForwardPath(u *url.URL) error {
	for _, seg := range strings.Split(u.EscapedPath(), "/") {
		decoded, err := url.PathUnescape(seg)
		if err != nil {
			return ErrInvalidForwardPath
		}
		if strings.ContainsRune(decoded, '%') || strings.ContainsFunc(decoded, unicode.IsControl) {
			return ErrInvalidForwardPath
		}
		for _, part := range strings.FieldsFunc(decoded, isPathSeparator) {
			if part == "." || part == ".." {
				return ErrInvalidForwardPath
			}
		}
	}
	return nil
}

// RejectInvalidForwardPath wraps next and responds with 400 to requests whose
// path fails [ValidateForwardPath], before any routing or upstream request.
func RejectInvalidForwardPath(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := ValidateForwardPath(r.URL); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isPathSeparator(r rune) bool {
	return r == '/' || r == '\\'
}

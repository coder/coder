package routing

import (
	"net/url"
	"strings"

	"golang.org/x/xerrors"
)

// ValidateForwardPath rejects decoded traversal segments before a request is
// dispatched upstream. Escaped separators are preserved for forwarding, while
// nested decoded components are still checked for dot segments.
func ValidateForwardPath(u *url.URL) error {
	escapedPath := u.EscapedPath()
	for segment := range strings.SplitSeq(escapedPath, "/") {
		decoded, err := url.PathUnescape(segment)
		if err != nil {
			return xerrors.Errorf("decode path segment: %w", err)
		}
		for nested := range strings.SplitSeq(decoded, "/") {
			if nested == "." || nested == ".." {
				return xerrors.New("path contains a traversal segment")
			}
		}
	}
	return nil
}

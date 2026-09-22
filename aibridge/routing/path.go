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
	decodedPath, err := url.PathUnescape(u.EscapedPath())
	if err != nil {
		// EscapedPath currently guarantees valid escaping. Keep this fail-safe
		// so a future URL representation cannot bypass traversal validation.
		return xerrors.Errorf("decode path: %w", err)
	}
	for segment := range strings.SplitSeq(decodedPath, "/") {
		if segment == "." || segment == ".." {
			return xerrors.New("path contains a traversal segment")
		}
	}
	return nil
}

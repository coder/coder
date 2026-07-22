package codersdk

import (
	"net/url"
	"path"
	"strings"

	"golang.org/x/xerrors"
)

// IconURLValid validates an optional user-supplied icon reference.
// Only deployment-relative paths (for example "/emojis/1f4bb.png" or
// "/icon/aws.svg") are accepted. Absolute and protocol-relative URLs
// are rejected so rendering an icon never causes a viewer's browser
// to request an attacker-controlled host, which would disclose the
// viewer's IP address (Cure53 CDM-02-006).
func IconURLValid(str string) error {
	if str == "" {
		return nil
	}
	// Browsers follow the WHATWG URL parser, which treats
	// backslashes in http(s) URLs as slashes, so "/\evil.com" is
	// fetched as "//evil.com". net/url does not, so reject
	// backslashes outright rather than misparse them.
	if strings.Contains(str, `\`) {
		return xerrors.New("must not contain backslashes")
	}
	u, err := url.Parse(str)
	if err != nil {
		return xerrors.New("must be a valid URL")
	}
	// Host catches protocol-relative "//host/path" references, and
	// Opaque catches scheme:opaque forms such as "javascript:" and
	// "data:".
	if u.Scheme != "" || u.Opaque != "" || u.User != nil || u.Host != "" {
		return xerrors.New("must be a relative path, not an absolute URL")
	}
	if !strings.HasPrefix(u.Path, "/") {
		return xerrors.New("must be an absolute path starting with /")
	}
	if cleaned := path.Clean(u.Path); cleaned != u.Path {
		return xerrors.Errorf("must be a normalized path, e.g. %q", cleaned)
	}
	return nil
}

package provisioner

import (
	"net/url"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/xerrors"
)

// blockedExternalURLSchemes run script or read local content in the browser
// instead of opening an app.
var blockedExternalURLSchemes = []string{"javascript", "vbscript", "data", "blob", "file", "filesystem"}

// ValidateExternalURL validates that value is a URL with a parseable scheme and
// host, a scheme that isn't blocked, and no control characters.
func ValidateExternalURL(value string) error {
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return xerrors.New("must not contain control characters")
	}

	u, err := url.Parse(value)
	if err != nil {
		return xerrors.Errorf("parse URL: %w", err)
	}

	if u.String() == "" {
		return xerrors.New(`must include a scheme and host`)
	}

	if u.Scheme == "" {
		return xerrors.New(`must include a scheme, for example "https://"`)
	}

	if slices.Contains(blockedExternalURLSchemes, u.Scheme) {
		return xerrors.Errorf("%q URLs are not allowed", u.Scheme)
	}

	if u.Host == "" || u.Hostname() == "" {
		return xerrors.Errorf("%q URLs must include a host", u.Scheme)
	}

	return nil
}

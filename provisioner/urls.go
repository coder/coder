package provisioner

import (
	"net/url"

	"golang.org/x/xerrors"
)

// ValidateExternalURL validates that value is a URL has a parseable scheme and host defined
func ValidateExternalURL(value string) error {
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

	if u.Host == "" || u.Hostname() == "" {
		return xerrors.Errorf("%q URLs must include a host", u.Scheme)
	}

	return nil
}

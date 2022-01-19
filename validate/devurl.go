package validate

import (
	"regexp"
	"strings"

	"golang.org/x/xerrors"
)

// NOTE: disallowing leading and trailing hyphens to avoid semantic confusion with hyphen used as separator.
// Disallowing leading and trailing underscores to avoid potential clashes with mDNS-related stuff.
var devURLValidNameRx = regexp.MustCompile("^[a-zA-Z]([a-zA-Z0-9_-]{0,41}[a-zA-Z0-9])?$")
var devURLInvalidLenMsg = "invalid devurl name %q: names may not be more than 64 characters in length."
var devURLInvalidNameMsg = "invalid devurl name %q: names must begin with a letter, followed by zero or more letters," +
	" digits, hyphens, or underscores, and end with a letter or digit."

const (
	// DevURLDelimiter is the separator for parts of a DevURL.
	// eg. kyle--test--name.cdr.co
	DevURLDelimiter = "--"
)

// DevURLName only validates the name of the devurl, not the fully resolved subdomain.
func DevURLName(name string) error {
	if len(name) == 0 {
		return nil
	}
	if len(name) > 43 {
		return xerrors.Errorf(devURLInvalidLenMsg, name)
	}
	if name != "" && !devURLValidNameRx.MatchString(name) {
		return xerrors.Errorf(devURLInvalidNameMsg, name)
	}
	if strings.Contains(name, DevURLDelimiter) {
		return xerrors.Errorf(devURLInvalidNameMsg, name)
	}
	return nil
}

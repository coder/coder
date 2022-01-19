package validate

import (
	"regexp"

	"golang.org/x/xerrors"
)

const (
	// UsernameMaxLength is the maximum length a username can be.
	UsernameMaxLength = 32
)

// Matches alphanumeric usernames with `-`, but not consecutively.
var usernameRx = regexp.MustCompile("^[a-zA-Z0-9]+(?:-[a-zA-Z0-9]+)*$")

var ErrInvalidUsernameRegex = xerrors.Errorf("username must conform to regex %v", usernameRx.String())
var ErrUsernameTooLong = xerrors.Errorf("usernames must be a maximum length of %d", UsernameMaxLength)

// Username validates the string provided to be a valid Coder username.
// Coder usernames follow GitHub's username rules. Here are the rules:
// 1. Must be alphanumeric.
// 2. Minimum length of 1, maximum of 32.
// 3. Cannot start with a hyphen.
// 4. Cannot include consecutive hyphens.
func Username(s string) error {
	if len(s) > UsernameMaxLength {
		return ErrUsernameTooLong
	}
	if len(s) < 1 {
		return ErrInvalidUsernameRegex
	}
	if !usernameRx.MatchString(s) {
		return ErrInvalidUsernameRegex
	}
	return nil
}

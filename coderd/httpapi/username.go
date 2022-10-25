package httpapi

import (
	"regexp"
	"strings"

	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"
)

var (
	UsernameValidRegex = regexp.MustCompile("^[a-zA-Z0-9]+(?:-[a-zA-Z0-9]+)*$")
	usernameReplace    = regexp.MustCompile("[^a-zA-Z0-9-]*")
)

// UsernameValid returns whether the input string is a valid username.
func UsernameValid(str string) error {
	if len(str) > 32 {
		return xerrors.New("must be <= 32 characters")
	}
	if len(str) < 1 {
		return xerrors.New("must be >= 1 character")
	}
	matched := UsernameValidRegex.MatchString(str)
	if !matched {
		return xerrors.New("must be alphanumeric with hyphens")
	}
	return nil
}

// UsernameFrom returns a best-effort username from the provided string.
//
// It first attempts to validate the incoming string, which will
// be returned if it is valid. It then will attempt to extract
// the username from an email address. If no success happens during
// these steps, a random username will be returned.
func UsernameFrom(str string) string {
	if valid := UsernameValid(str); valid == nil {
		return str
	}
	emailAt := strings.LastIndex(str, "@")
	if emailAt >= 0 {
		str = str[:emailAt]
	}
	str = usernameReplace.ReplaceAllString(str, "")
	if valid := UsernameValid(str); valid == nil {
		return str
	}
	return strings.ReplaceAll(namesgenerator.GetRandomName(1), "_", "-")
}

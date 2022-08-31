package httpapi

import (
	"regexp"
	"strings"

	"github.com/moby/moby/pkg/namesgenerator"
)

var (
	UsernameValidRegex = regexp.MustCompile("^[a-zA-Z0-9]+(?:-[a-zA-Z0-9]+)*$")
	usernameReplace    = regexp.MustCompile("[^a-zA-Z0-9-]*")
)

// UsernameValid returns whether the input string is a valid username.
func UsernameValid(str string) bool {
	if len(str) > 32 {
		return false
	}
	if len(str) < 1 {
		return false
	}
	return UsernameValidRegex.MatchString(str)
}

// UsernameFrom returns a best-effort username from the provided string.
//
// It first attempts to validate the incoming string, which will
// be returned if it is valid. It then will attempt to extract
// the username from an email address. If no success happens during
// these steps, a random username will be returned.
func UsernameFrom(str string) string {
	if UsernameValid(str) {
		return str
	}
	emailAt := strings.LastIndex(str, "@")
	if emailAt >= 0 {
		str = str[:emailAt]
	}
	str = usernameReplace.ReplaceAllString(str, "")
	if UsernameValid(str) {
		return str
	}
	return strings.ReplaceAll(namesgenerator.GetRandomName(1), "_", "-")
}

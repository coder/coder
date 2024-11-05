package codersdk

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"
)

var (
	UsernameValidRegex = regexp.MustCompile("^[a-zA-Z0-9]+(?:-[a-zA-Z0-9]+)*$")
	usernameReplace    = regexp.MustCompile("[^a-zA-Z0-9-]*")

	templateVersionName = regexp.MustCompile(`^[a-zA-Z0-9]+(?:[_.-]{1}[a-zA-Z0-9]+)*$`)
	templateDisplayName = regexp.MustCompile(`^[^\s](.*[^\s])?$`)
)

// UsernameFrom returns a best-effort username from the provided string.
//
// It first attempts to validate the incoming string, which will
// be returned if it is valid. It then will attempt to extract
// the username from an email address. If no success happens during
// these steps, a random username will be returned.
func UsernameFrom(str string) string {
	if valid := NameValid(str); valid == nil {
		return str
	}
	emailAt := strings.LastIndex(str, "@")
	if emailAt >= 0 {
		str = str[:emailAt]
	}
	str = usernameReplace.ReplaceAllString(str, "")
	if valid := NameValid(str); valid == nil {
		return str
	}
	return strings.ReplaceAll(namesgenerator.GetRandomName(1), "_", "-")
}

// NameValid returns whether the input string is a valid name.
// It is a generic validator for any name (user, workspace, template, role name, etc.).
func NameValid(str string) error {
	if len(str) > 32 {
		return xerrors.New("must be <= 32 characters")
	}
	if len(str) < 1 {
		return xerrors.New("must be >= 1 character")
	}
	// Avoid conflicts with routes like /templates/new and /groups/create.
	if str == "new" || str == "create" {
		return xerrors.Errorf("cannot use %q as a name", str)
	}
	matched := UsernameValidRegex.MatchString(str)
	if !matched {
		return xerrors.New("must be alphanumeric with hyphens")
	}
	return nil
}

// TemplateVersionNameValid returns whether the input string is a valid template version name.
func TemplateVersionNameValid(str string) error {
	if len(str) > 64 {
		return xerrors.New("must be <= 64 characters")
	}
	matched := templateVersionName.MatchString(str)
	if !matched {
		return xerrors.New("must be alphanumeric with underscores and dots")
	}
	return nil
}

// DisplayNameValid returns whether the input string is a valid template display name.
func DisplayNameValid(str string) error {
	if len(str) == 0 {
		return nil // empty display_name is correct
	}
	if len(str) > 64 {
		return xerrors.New("must be <= 64 characters")
	}
	matched := templateDisplayName.MatchString(str)
	if !matched {
		return xerrors.New("must be alphanumeric with spaces")
	}
	return nil
}

// UserRealNameValid returns whether the input string is a valid real user name.
func UserRealNameValid(str string) error {
	if len(str) > 128 {
		return xerrors.New("must be <= 128 characters")
	}

	if strings.TrimSpace(str) != str {
		return xerrors.New("must not have leading or trailing whitespace")
	}
	return nil
}

// GroupNameValid returns whether the input string is a valid group name.
func GroupNameValid(str string) error {
	// We want to support longer names for groups to allow users to sync their
	// group names with their identity providers without manual mapping. Related
	// to: https://github.com/coder/coder/issues/15184
	limit := 255
	if len(str) > limit {
		return xerrors.New(fmt.Sprintf("must be <= %d characters", limit))
	}
	// Avoid conflicts with routes like /groups/new and /groups/create.
	if str == "new" || str == "create" {
		return xerrors.Errorf("cannot use %q as a name", str)
	}
	matched := UsernameValidRegex.MatchString(str)
	if !matched {
		return xerrors.New("must be alphanumeric with hyphens")
	}
	return nil
}

// NormalizeUserRealName normalizes a user name such that it will pass
// validation by UserRealNameValid. This is done to avoid blocking
// little  Bobby  Whitespace  from using Coder.
func NormalizeRealUsername(str string) string {
	s := strings.TrimSpace(str)
	if len(s) > 128 {
		s = s[:128]
	}
	return s
}

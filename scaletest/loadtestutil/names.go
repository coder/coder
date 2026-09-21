package loadtestutil

import (
	"fmt"
	"strings"

	"github.com/coder/coder/v2/cryptorand"
)

const (
	// Prefix for all scaletest resources (users and workspaces)
	ScaleTestPrefix = "scaletest"

	// Email domain for scaletest users
	EmailDomain = "@scaletest.local"

	DefaultRandLength = 8
)

// GenerateUserIdentifier generates a username and email for scale testing.
// The username follows the pattern: scaletest-<random>-<id>
// The email follows the pattern: <random>-<id>@scaletest.local
func GenerateUserIdentifier(id string) (username, email string, err error) {
	return GenerateUserIdentifierWithPrefix(ScaleTestPrefix+"-", id)
}

// GenerateUserIdentifierWithPrefix generates a username and email for scale
// testing using a caller-supplied username prefix. The username follows the
// pattern: <prefix><random>-<id>. Callers are expected to keep the "scaletest-"
// root in prefix so users created with a custom prefix are still discovered by
// IsScaleTestUser and the scaletest cleanup command; a caller-chosen infix
// is inserted between the root and the random suffix (for example prefix
// "scaletest-asdf-" yields scaletest-asdf-<random>-<id>). The email keeps the
// scaletest domain regardless (<random>-<id>@scaletest.local).
func GenerateUserIdentifierWithPrefix(prefix, id string) (username, email string, err error) {
	randStr, err := cryptorand.String(DefaultRandLength)
	if err != nil {
		return "", "", err
	}

	username = fmt.Sprintf("%s%s-%s", prefix, randStr, id)
	email = fmt.Sprintf("%s-%s%s", randStr, id, EmailDomain)
	return username, email, nil
}

// GenerateWorkspaceName generates a workspace name for scale testing.
// The workspace name follows the pattern: scaletest-<random>-<id>
func GenerateWorkspaceName(id string) (name string, err error) {
	randStr, err := cryptorand.String(DefaultRandLength)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%s-%s", ScaleTestPrefix, randStr, id), nil
}

// GenerateDeterministicWorkspaceName generates a deterministic workspace name
// for scale testing without a random component. This is useful when the
// workspace name needs to be known before the workspace is created, such as
// for pre-creating channels keyed by workspace name.
// The workspace name follows the pattern: scaletest-<id>
func GenerateDeterministicWorkspaceName(id string) string {
	return fmt.Sprintf("%s-%s", ScaleTestPrefix, id)
}

// IsScaleTestUser checks if a username indicates it was created for scale testing.
func IsScaleTestUser(username, email string) bool {
	return strings.HasPrefix(username, ScaleTestPrefix+"-") ||
		strings.HasSuffix(email, EmailDomain)
}

// IsScaleTestWorkspace checks if a workspace name indicates it was created for scale testing.
func IsScaleTestWorkspace(workspaceName, ownerName string) bool {
	return strings.HasPrefix(workspaceName, ScaleTestPrefix+"-") ||
		strings.HasPrefix(ownerName, ScaleTestPrefix+"-")
}

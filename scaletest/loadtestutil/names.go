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
	randStr, err := cryptorand.String(DefaultRandLength)
	if err != nil {
		return "", "", err
	}

	username = fmt.Sprintf("%s-%s-%s", ScaleTestPrefix, randStr, id)
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

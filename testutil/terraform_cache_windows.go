//go:build windows

package testutil

import (
	"testing"
)

// CacheTFProviders is a no-op on Windows.
// Terraform provider caching is only supported on Linux and macOS due to
// platform-specific filesystem operations and Terraform's provider mirror behavior.
// On Windows, tests will download providers normally during terraform init.
func CacheTFProviders(t *testing.T, rootDir string, testName string, templateFiles map[string]string) string {
	t.Helper()
	t.Log("Terraform provider caching is not supported on Windows; providers will be downloaded normally")
	return ""
}

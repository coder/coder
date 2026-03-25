package testutil

import (
	"testing"

	"github.com/coder/coder/v2/coderd/util/namesgenerator"
)

// GetRandomName returns a random name with a unique suffix, truncated to 32
// characters to fit common name length limits in tests.
func GetRandomName(t testing.TB) string {
	t.Helper()
	return namesgenerator.UniqueName()
}

// GetRandomNameHyphenated is like GetRandomName but uses hyphens instead of
// underscores.
func GetRandomNameHyphenated(t testing.TB) string {
	t.Helper()
	return namesgenerator.UniqueNameWith("-")
}

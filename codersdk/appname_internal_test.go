package codersdk

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Lookups fold input through NormalizeAppName before probing, so map keys
// that are not already normalized can never match.
func TestAppNameFamilyKeysAreNormalized(t *testing.T) {
	t.Parallel()
	for name := range appNameFamilies {
		require.Equal(t, NormalizeAppName(name), name)
	}
}

// A family with no destination in AttributedAppFamilies silently drops its
// sessions from usage reporting, so adding one must fail here first.
func TestEveryFamilyIsAttributed(t *testing.T) {
	t.Parallel()
	for appName, family := range appNameFamilies {
		require.Contains(t, AttributedAppFamilies, family,
			"app %q maps to family %q, which usage reporting cannot report", appName, family)
	}
}

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
		require.NotEmpty(t, name)
		require.Equal(t, NormalizeAppName(name), name)
	}
}

// Families reach the queries as jsonb values and metric labels, so a blank
// entry would attribute sessions to an unusable name. AppFamilyUnknown is the
// fold destination for unregistered apps, not something to register.
func TestAppNameFamilyValuesAreUsable(t *testing.T) {
	t.Parallel()
	for appName, family := range appNameFamilies {
		require.NotEmpty(t, family, "app %q has no family", appName)
		require.NotEqual(t, AppFamilyUnknown, family,
			"app %q must not register the unknown family", appName)
		require.Equal(t, NormalizeAppName(string(family)), string(family),
			"family %q must be normalized", family)
	}
}

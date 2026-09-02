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

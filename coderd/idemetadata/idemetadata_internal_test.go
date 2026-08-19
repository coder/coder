package idemetadata

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Lookups fold input through canonicalKey before probing, so map keys that
// are not already canonical can never match.
func TestFamilyKeysAreCanonical(t *testing.T) {
	t.Parallel()
	for name := range families {
		require.Equal(t, canonicalKey(name), name)
	}
}

// A family with no destination in AttributedFamilies silently drops its
// sessions from usage reporting, so adding one must fail here first.
func TestEveryFamilyIsAttributed(t *testing.T) {
	t.Parallel()
	for appName, family := range families {
		require.Contains(t, AttributedFamilies, family,
			"app %q maps to family %q, which usage reporting cannot report", appName, family)
	}
}

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

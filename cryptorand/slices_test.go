package cryptorand_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cryptorand"
)

func TestRandomElement(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()

		s := []string{}
		v, err := cryptorand.Element(s)
		require.Error(t, err)
		require.ErrorContains(t, err, "slice must have at least one element")
		require.Empty(t, v)
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		// Generate random slices of ints and strings
		var (
			ints    = make([]int, 20)
			strings = make([]string, 20)
		)
		for i := range ints {
			v, err := cryptorand.Intn(1024)
			require.NoError(t, err, "generate random int for test slice")
			ints[i] = v
		}
		for i := range strings {
			v, err := cryptorand.String(10)
			require.NoError(t, err, "generate random string for test slice")
			strings[i] = v
		}

		// Get a random value from each 20 times.
		for i := 0; i < 20; i++ {
			iv, err := cryptorand.Element(ints)
			require.NoError(t, err, "unexpected error from Element(ints)")
			t.Logf("random int slice element: %v", iv)
			require.Contains(t, ints, iv)

			sv, err := cryptorand.Element(strings)
			require.NoError(t, err, "unexpected error from Element(strings)")
			t.Logf("random string slice element: %v", sv)
			require.Contains(t, strings, sv)
		}
	})
}

package cryptorand_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cryptorand"
)

func TestInt63(t *testing.T) {
	t.Parallel()

	for i := 0; i < 20; i++ {
		v, err := cryptorand.Int63()
		require.NoError(t, err, "unexpected error from Int63")
		t.Logf("value: %v <- random?", v)
		require.True(t, v >= 0, "values must be positive")
	}
}

func TestIntn(t *testing.T) {
	t.Parallel()

	for i := 0; i < 20; i++ {
		v, err := cryptorand.Intn(100)
		require.NoError(t, err, "unexpected error from Intn")
		t.Logf("value: %v <- random?", v)
		require.GreaterOrEqual(t, v, 0, "values must be positive")
		require.True(t, v < 100, "values must be less than 100")
	}

	// Ensure Intn works for int larger than 32 bits
	_, err := cryptorand.Intn(1 << 35)
	require.NoError(t, err, "expected Intn to work for 64-bit int")

	// Expect a panic if max is negative
	require.PanicsWithValue(t, "invalid argument to Intn", func() {
		cryptorand.Intn(0)
	})
}

func TestFloat64(t *testing.T) {
	t.Parallel()

	for i := 0; i < 20; i++ {
		v, err := cryptorand.Float64()
		require.NoError(t, err, "unexpected error from Float64")
		t.Logf("value: %v <- random?", v)
		require.True(t, v >= 0.0, "values must be positive")
		require.True(t, v < 1.0, "values must be less than 1.0")
	}
}

package cryptorand_test

import (
	"crypto/rand"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cryptorand"
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

func TestUint64(t *testing.T) {
	t.Parallel()

	for i := 0; i < 20; i++ {
		v, err := cryptorand.Uint64()
		require.NoError(t, err, "unexpected error from Uint64")
		t.Logf("value: %v <- random?", v)
	}
}

func TestInt31(t *testing.T) {
	t.Parallel()

	for i := 0; i < 20; i++ {
		v, err := cryptorand.Int31()
		require.NoError(t, err, "unexpected error from Int31")
		t.Logf("value: %v <- random?", v)
		require.True(t, v >= 0, "values must be positive")
	}
}

func TestUnbiasedModulo32(t *testing.T) {
	t.Parallel()
	const mod = 7
	dist := [mod]uint32{}

	_, err := cryptorand.UnbiasedModulo32(0, mod)
	require.NoError(t, err)

	for i := 0; i < 1000; i++ {
		b := [4]byte{}
		_, _ = rand.Read(b[:])
		v, err := cryptorand.UnbiasedModulo32(binary.BigEndian.Uint32(b[:]), mod)
		require.NoError(t, err, "unexpected error from UnbiasedModulo32")
		dist[v]++
	}

	t.Logf("dist: %+v <- evenly distributed?", dist)
}

func TestUint32(t *testing.T) {
	t.Parallel()

	for i := 0; i < 20; i++ {
		v, err := cryptorand.Uint32()
		require.NoError(t, err, "unexpected error from Uint32")
		t.Logf("value: %v <- random?", v)
	}
}

func TestInt(t *testing.T) {
	t.Parallel()

	for i := 0; i < 20; i++ {
		v, err := cryptorand.Int()
		require.NoError(t, err, "unexpected error from Int")
		t.Logf("value: %v <- random?", v)
		require.True(t, v >= 0, "values must be positive")
	}
}

func TestInt63n(t *testing.T) {
	t.Parallel()

	for i := 0; i < 20; i++ {
		v, err := cryptorand.Int63n(1 << 35)
		require.NoError(t, err, "unexpected error from Int63n")
		t.Logf("value: %v <- random?", v)
		require.True(t, v >= 0, "values must be positive")
		require.True(t, v < 1<<35, "values must be less than 1<<35")
	}

	// Expect a panic if max is negative
	require.PanicsWithValue(t, "invalid argument to Int63n", func() {
		cryptorand.Int63n(0)
	})
}

func TestInt31n(t *testing.T) {
	t.Parallel()

	for i := 0; i < 20; i++ {
		v, err := cryptorand.Int31n(100)
		require.NoError(t, err, "unexpected error from Int31n")
		t.Logf("value: %v <- random?", v)
		require.True(t, v >= 0, "values must be positive")
		require.True(t, v < 100, "values must be less than 100")
	}
}

func TestIntn(t *testing.T) {
	t.Parallel()

	for i := 0; i < 20; i++ {
		v, err := cryptorand.Intn(100)
		require.NoError(t, err, "unexpected error from Intn")
		t.Logf("value: %v <- random?", v)
		require.True(t, v >= 0, "values must be positive")
		require.True(t, v < 100, "values must be less than 100")
	}

	// Ensure Intn works for int larger than 32 bits
	_, err := cryptorand.Intn(1 << 35)
	require.NoError(t, err, "expected Intn to work for 64-bit int")

	// Expect a panic if max is negative
	require.PanicsWithValue(t, "n must be a positive nonzero number", func() {
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

func TestFloat32(t *testing.T) {
	t.Parallel()

	for i := 0; i < 20; i++ {
		v, err := cryptorand.Float32()
		require.NoError(t, err, "unexpected error from Float32")
		t.Logf("value: %v <- random?", v)
		require.True(t, v >= 0.0, "values must be positive")
		require.True(t, v < 1.0, "values must be less than 1.0")
	}
}

func TestDuration(t *testing.T) {
	t.Parallel()

	for i := 0; i < 20; i++ {
		v, err := cryptorand.Duration()
		require.NoError(t, err, "unexpected error from Duration")
		t.Logf("value: %v <- random?", v)
		require.True(t, v >= 0.0, "values must be positive")
	}
}

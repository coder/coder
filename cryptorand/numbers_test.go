package cryptorand_test

import (
	"crypto/rand"
	"encoding/binary"
	"testing"

	"github.com/coder/coder/cryptorand"
	"github.com/stretchr/testify/require"
)

func TestInt63(t *testing.T) {
	t.Parallel()

	for i := 0; i < 20; i++ {
		v, err := cryptorand.Int63()
		require.NoError(t, err, "unexpected error from Int63")
		t.Logf("value: %v <- random?", v)
		require.True(t, v >= 0, "values must be positive")

		v = cryptorand.MustInt63()
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

		v = cryptorand.MustUint64()
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

		v = cryptorand.MustInt31()
		t.Logf("value: %v <- random?", v)
		require.True(t, v >= 0, "values must be positive")
	}
}

func TestUnbiasedModulo32(t *testing.T) {
	t.Parallel()
	const mod = 7
	dist := [mod]uint32{}

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

		v = cryptorand.MustUint32()
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

		v = cryptorand.MustInt()
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

		v = cryptorand.MustInt63n(1 << 40)
		t.Logf("value: %v <- random?", v)
		require.True(t, v >= 0, "values must be positive")
		require.True(t, v < 1<<40, "values must be less than 1<<40")
	}
}

func TestInt31n(t *testing.T) {
	t.Parallel()

	for i := 0; i < 20; i++ {
		v, err := cryptorand.Int31n(100)
		require.NoError(t, err, "unexpected error from Int31n")
		t.Logf("value: %v <- random?", v)
		require.True(t, v >= 0, "values must be positive")
		require.True(t, v < 100, "values must be less than 100")

		v = cryptorand.MustInt31n(500)
		t.Logf("value: %v <- random?", v)
		require.True(t, v >= 0, "values must be positive")
		require.True(t, v < 500, "values must be less than 500")
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

		v = cryptorand.MustIntn(500)
		t.Logf("value: %v <- random?", v)
		require.True(t, v >= 0, "values must be positive")
		require.True(t, v < 500, "values must be less than 500")

		v = cryptorand.MustIntn(1 << 40)
		t.Logf("value: %v <- random?", v)
		require.True(t, v >= 0, "values must be positive")
		require.True(t, v < 1<<40, "values must be less than 1<<40")
	}
}

func TestFloat64(t *testing.T) {
	t.Parallel()

	for i := 0; i < 20; i++ {
		v, err := cryptorand.Float64()
		require.NoError(t, err, "unexpected error from Float64")
		t.Logf("value: %v <- random?", v)
		require.True(t, v >= 0.0, "values must be positive")
		require.True(t, v < 1.0, "values must be less than 1.0")

		v = cryptorand.MustFloat64()
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

		v = cryptorand.MustFloat32()
		t.Logf("value: %v <- random?", v)
		require.True(t, v >= 0.0, "values must be positive")
		require.True(t, v < 1.0, "values must be less than 1.0")
	}
}

func TestBool(t *testing.T) {
	t.Parallel()

	const iterations = 10000
	trueCount := 0

	for i := 0; i < iterations; i += 2 {
		v, err := cryptorand.Bool()
		require.NoError(t, err, "unexpected error from Bool")
		if v {
			trueCount++
		}

		v = cryptorand.MustBool()
		if v {
			trueCount++
		}
	}

	percentage := (float64(trueCount) / iterations) * 100
	t.Logf("number of true values: %d of %d total (%.2f%%)", trueCount, iterations, percentage)
	require.True(t, percentage > 48, "expected more than 48 percent of values to be true")
	require.True(t, percentage < 52, "expected less than 52 percent of values to be true")
}

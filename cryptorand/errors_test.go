package cryptorand_test

import (
	"crypto/rand"
	"io"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cryptorand"
)

// TestRandError checks that the code handles errors when reading from
// the rand.Reader.
//
// This test replaces the global rand.Reader, so cannot be parallelized
//
//nolint:paralleltest
func TestRandError(t *testing.T) {
	origReader := rand.Reader
	t.Cleanup(func() {
		rand.Reader = origReader
	})

	rand.Reader = iotest.ErrReader(io.ErrShortBuffer)

	t.Run("Int63", func(t *testing.T) {
		_, err := cryptorand.Int63()
		require.ErrorIs(t, err, io.ErrShortBuffer, "expected Int63 error")
	})

	t.Run("Uint64", func(t *testing.T) {
		_, err := cryptorand.Uint64()
		require.ErrorIs(t, err, io.ErrShortBuffer, "expected Uint64 error")
	})

	t.Run("Int31", func(t *testing.T) {
		_, err := cryptorand.Int31()
		require.ErrorIs(t, err, io.ErrShortBuffer, "expected Int31 error")
	})

	t.Run("Int31n", func(t *testing.T) {
		_, err := cryptorand.Int31n(100)
		require.ErrorIs(t, err, io.ErrShortBuffer, "expected Int31n error")
	})

	t.Run("Uint32", func(t *testing.T) {
		_, err := cryptorand.Uint32()
		require.ErrorIs(t, err, io.ErrShortBuffer, "expected Uint32 error")
	})

	t.Run("Int", func(t *testing.T) {
		_, err := cryptorand.Int()
		require.ErrorIs(t, err, io.ErrShortBuffer, "expected Int error")
	})

	t.Run("Intn_32bit", func(t *testing.T) {
		_, err := cryptorand.Intn(100)
		require.ErrorIs(t, err, io.ErrShortBuffer, "expected Intn error")
	})

	t.Run("Intn_64bit", func(t *testing.T) {
		_, err := cryptorand.Intn(int(1 << 35))
		require.ErrorIs(t, err, io.ErrShortBuffer, "expected Intn error")
	})

	t.Run("Float64", func(t *testing.T) {
		_, err := cryptorand.Float64()
		require.ErrorIs(t, err, io.ErrShortBuffer, "expected Float64 error")
	})

	t.Run("Float32", func(t *testing.T) {
		_, err := cryptorand.Float32()
		require.ErrorIs(t, err, io.ErrShortBuffer, "expected Float32 error")
	})

	t.Run("Bool", func(t *testing.T) {
		_, err := cryptorand.Bool()
		require.ErrorIs(t, err, io.ErrShortBuffer, "expected Bool error")
	})

	t.Run("StringCharset", func(t *testing.T) {
		_, err := cryptorand.HexString(10)
		require.ErrorIs(t, err, io.ErrShortBuffer, "expected HexString error")
	})
}

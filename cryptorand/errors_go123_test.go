//go:build !go1.24

package cryptorand_test

import (
	"crypto/rand"
	"io"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cryptorand"
)

// TestRandError_pre_Go1_24 checks that the code handles errors when
// reading from the rand.Reader.
//
// This test replaces the global rand.Reader, so cannot be parallelized
//
//nolint:paralleltest
func TestRandError_pre_Go1_24(t *testing.T) {
	origReader := rand.Reader
	t.Cleanup(func() {
		rand.Reader = origReader
	})

	rand.Reader = iotest.ErrReader(io.ErrShortBuffer)

	// Testing `rand.Reader.Read` for errors will panic in Go 1.24 and later.
	t.Run("StringCharset", func(t *testing.T) {
		_, err := cryptorand.HexString(10)
		require.ErrorIs(t, err, io.ErrShortBuffer, "expected HexString error")
	})
}

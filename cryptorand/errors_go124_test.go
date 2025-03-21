//go:build go1.24

package cryptorand_test

import (
	"testing"

	"github.com/coder/coder/v2/cryptorand"
)

// TestRandError_Go1_24 verifies that cryptorand functions don't panic in Go 1.24+
// In Go 1.24+ we can't replace the rand.Reader to test error cases, but we can at
// least verify the functions don't panic.
func TestRandError_Go1_24(t *testing.T) {
	t.Run("StringCharset", func(t *testing.T) {
		_, err := cryptorand.HexString(10)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})
}
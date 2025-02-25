package agentrsa_test

import (
	"crypto/rsa"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/agent/agentrsa"
)

func TestGenerateDeterministicKey(t *testing.T) {
	t.Parallel()

	key1 := agentrsa.GenerateDeterministicKey(1234)
	key2 := agentrsa.GenerateDeterministicKey(1234)

	assert.Equal(t, key1, key2)
	assert.EqualExportedValues(t, key1, key2)
}

var result *rsa.PrivateKey

func BenchmarkGenerateDeterministicKey(b *testing.B) {
	var r *rsa.PrivateKey

	for range b.N {
		// always record the result of DeterministicPrivateKey to prevent
		// the compiler eliminating the function call.
		r = agentrsa.GenerateDeterministicKey(rand.Int64())
	}

	// always store the result to a package level variable
	// so the compiler cannot eliminate the Benchmark itself.
	result = r
}

func FuzzGenerateDeterministicKey(f *testing.F) {
	testcases := []int64{0, 1234, 1010101010}
	for _, tc := range testcases {
		f.Add(tc) // Use f.Add to provide a seed corpus
	}
	f.Fuzz(func(t *testing.T, seed int64) {
		key1 := agentrsa.GenerateDeterministicKey(seed)
		key2 := agentrsa.GenerateDeterministicKey(seed)
		assert.Equal(t, key1, key2)
		assert.EqualExportedValues(t, key1, key2)
	})
}

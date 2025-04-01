package cryptorand

import (
	"crypto/rand"
	"encoding/binary"
	insecurerand "math/rand"
)

type cryptoSource struct {
	err error
}

func (*cryptoSource) Seed(_ int64) {
	// Intentionally disregard seed
}

func (c *cryptoSource) Int63() int64 {
	var n int64
	err := binary.Read(rand.Reader, binary.BigEndian, &n)
	if err != nil {
		c.err = err
	}
	// The sign bit must be cleared to ensure the final value is non-negative.
	n &= 0x7fffffffffffffff
	return n
}

func (c *cryptoSource) Uint64() uint64 {
	var n uint64
	err := binary.Read(rand.Reader, binary.BigEndian, &n)
	if err != nil {
		c.err = err
	}
	return n
}

// secureRand returns a cryptographically secure random number generator.
func secureRand() (*insecurerand.Rand, *cryptoSource) {
	var cs cryptoSource
	//nolint:gosec
	return insecurerand.New(&cs), &cs
}

// Int64 returns a non-negative random 63-bit integer as a int64.
func Int63() (int64, error) {
	rng, cs := secureRand()
	return rng.Int63(), cs.err
}

// Intn returns a non-negative integer in [0,maxVal) as an int.
func Intn(maxVal int) (int, error) {
	rng, cs := secureRand()
	return rng.Intn(maxVal), cs.err
}

// Float64 returns a random number in [0.0,1.0) as a float64.
func Float64() (float64, error) {
	rng, cs := secureRand()
	return rng.Float64(), cs.err
}

package cryptorand

import (
	"crypto/rand"
	"encoding/binary"

	"golang.org/x/xerrors"
)

// Most of this code is inspired by math/rand, so shares similar
// functions and implementations, but uses crypto/rand to generate
// random Int63 data.

// Int64 returns a non-negative random 63-bit integer as a int64.
func Int63() (int64, error) {
	var i int64
	err := binary.Read(rand.Reader, binary.BigEndian, &i)
	if err != nil {
		return 0, xerrors.Errorf("read binary: %w", err)
	}

	if i < 0 {
		return -i, nil
	}
	return i, nil
}

// Uint64 returns a random 64-bit integer as a uint64.
func Uint64() (uint64, error) {
	upper, err := Int63()
	if err != nil {
		return 0, xerrors.Errorf("read upper: %w", err)
	}

	lower, err := Int63()
	if err != nil {
		return 0, xerrors.Errorf("read lower: %w", err)
	}

	return uint64(lower)>>31 | uint64(upper)<<32, nil
}

// Int31 returns a non-negative random 31-bit integer as a int32.
func Int31() (int32, error) {
	i, err := Int63()
	if err != nil {
		return 0, err
	}

	return int32(i >> 32), nil
}

// Uint32 returns a 32-bit value as a uint32.
func Uint32() (uint32, error) {
	i, err := Int63()
	if err != nil {
		return 0, err
	}

	return uint32(i >> 31), nil
}

// Int returns a non-negative random integer as a int.
func Int() (int, error) {
	i, err := Int63()
	if err != nil {
		return 0, err
	}

	if i < 0 {
		return int(-i), nil
	}
	return int(i), nil
}

// Int63n returns a non-negative random integer in [0,n) as a int64.
func Int63n(n int64) (int64, error) {
	if n <= 0 {
		panic("invalid argument to Int63n")
	}

	max := int64((1 << 63) - 1 - (1<<63)%uint64(n))
	i, err := Int63()
	if err != nil {
		return 0, err
	}

	for i > max {
		i, err = Int63()
		if err != nil {
			return 0, err
		}
	}

	return i % n, nil
}

// Int31n returns a non-negative integer in [0,n) as a int32.
func Int31n(n int32) (int32, error) {
	i, err := Uint32()
	if err != nil {
		return 0, err
	}

	return UnbiasedModulo32(i, n)
}

// UnbiasedModulo32 uniformly modulos v by n over a sufficiently large data
// set, regenerating v if necessary. n must be > 0. All input bits in v must be
// fully random, you cannot cast a random uint8/uint16 for input into this
// function.
func UnbiasedModulo32(v uint32, n int32) (int32, error) {
	prod := uint64(v) * uint64(n)
	low := uint32(prod)
	if low < uint32(n) {
		thresh := uint32(-n) % uint32(n)
		for low < thresh {
			var err error
			v, err = Uint32()
			if err != nil {
				return 0, err
			}
			prod = uint64(v) * uint64(n)
			low = uint32(prod)
		}
	}
	return int32(prod >> 32), nil
}

// Intn returns a non-negative integer in [0,n) as a int.
func Intn(n int) (int, error) {
	if n <= 0 {
		panic("n must be a positive nonzero number")
	}

	if n <= 1<<31-1 {
		i, err := Int31n(int32(n))
		if err != nil {
			return 0, err
		}

		return int(i), nil
	}

	i, err := Int63n(int64(n))
	if err != nil {
		return 0, err
	}

	return int(i), nil
}

// Float64 returns a random number in [0.0,1.0) as a float64.
func Float64() (float64, error) {
again:
	i, err := Int63n(1 << 53)
	if err != nil {
		return 0, err
	}

	f := (float64(i) / (1 << 53))
	if f == 1 {
		goto again
	}

	return f, nil
}

// Float32 returns a random number in [0.0,1.0) as a float32.
func Float32() (float32, error) {
again:
	i, err := Float64()
	if err != nil {
		return 0, err
	}

	f := float32(i)
	if f == 1 {
		goto again
	}

	return f, nil
}

// Bool returns a random true/false value as a bool.
func Bool() (bool, error) {
	i, err := Uint64()
	if err != nil {
		return false, err
	}

	// True if the least significant bit is 1
	return i&1 == 1, nil
}

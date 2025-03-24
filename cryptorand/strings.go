package cryptorand

import (
	"crypto/rand"
	"encoding/binary"
	"strings"

	"golang.org/x/xerrors"
)

// Charsets
const (
	// Numeric includes decimal numbers (0-9)
	Numeric = "0123456789"

	// Upper is uppercase characters in the Latin alphabet
	Upper = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

	// Lower is lowercase characters in the Latin alphabet
	Lower = "abcdefghijklmnopqrstuvwxyz"

	// Alpha is upper or lowercase alphabetic characters
	Alpha = Upper + Lower

	// Default is uppercase, lowercase, or numeric characters
	Default = Numeric + Alpha

	// Hex is hexadecimal lowercase characters
	Hex = "0123456789abcdef"

	// Human creates strings which are easily distinguishable from
	// others created with the same charset. It contains most lowercase
	// alphanumeric characters without 0,o,i,1,l.
	Human = "23456789abcdefghjkmnpqrstuvwxyz"
)

// unbiasedModulo32 uniformly modulos v by n over a sufficiently large data
// set, regenerating v if necessary. n must be > 0. All input bits in v must be
// fully random, you cannot cast a random uint8/uint16 for input into this
// function.
//
// See more details on this algorithm here:
// https://lemire.me/blog/2016/06/27/a-fast-alternative-to-the-modulo-reduction/
//
//nolint:varnamelen
func unbiasedModulo32(v uint32, n int32) (int32, error) {
	prod := uint64(v) * uint64(n) // #nosec G115 -- uint32 to uint64 is always safe
	low := uint32(prod)           // #nosec G115 -- truncation is intentional for the algorithm
	if low < uint32(n) {          // #nosec G115 -- int32 to uint32 is safe for positive n (we require n > 0)
		thresh := uint32(-n) % uint32(n) // #nosec G115 -- int32 to uint32 after negation is an acceptable pattern here
		for low < thresh {
			err := binary.Read(rand.Reader, binary.BigEndian, &v)
			if err != nil {
				return 0, err
			}
			prod = uint64(v) * uint64(n) // #nosec G115 -- uint32 to uint64 is always safe
			low = uint32(prod)           // #nosec G115 -- truncation is intentional for the algorithm
		}
	}
	return int32(prod >> 32), nil // #nosec G115 -- proper range is guaranteed by the algorithm
}

// StringCharset generates a random string using the provided charset and size.
func StringCharset(charSetStr string, size int) (string, error) {
	if size == 0 {
		return "", nil
	}

	if len(charSetStr) == 0 {
		return "", xerrors.Errorf("charSetStr must not be empty")
	}

	charSet := []rune(charSetStr)

	// We pre-allocate the entropy to amortize the crypto/rand syscall overhead.
	entropy := make([]byte, 4*size)

	_, err := rand.Read(entropy)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	buf.Grow(size)

	for i := 0; i < size; i++ {
		r := binary.BigEndian.Uint32(entropy[:4]) // #nosec G115 -- not a conversion, just reading bytes as uint32
		entropy = entropy[4:]

		// Charset length is limited by string size, so conversion to int32 is safe
		ci, err := unbiasedModulo32(
			r,
			int32(len(charSet)), // #nosec G115 -- int to int32 is safe for charset length
		)
		if err != nil {
			return "", err
		}

		_, _ = buf.WriteRune(charSet[ci])
	}

	return buf.String(), nil
}

// String returns a random string using Default.
func String(size int) (string, error) {
	return StringCharset(Default, size)
}

// HexString returns a hexadecimal string of given length.
func HexString(size int) (string, error) {
	return StringCharset(Hex, size)
}

// Sha1String returns a 40-character hexadecimal string, which matches
// the length of a SHA-1 hash (160 bits).
func Sha1String() (string, error) {
	return StringCharset(Hex, 40)
}

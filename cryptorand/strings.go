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

// StringCharset generates a random string using the provided charset and size.
func StringCharset(charSetStr string, size int) (string, error) {
	charSet := []rune(charSetStr)

	if len(charSet) == 0 || size == 0 {
		return "", nil
	}

	// This buffer facilitates pre-emptively creation of random uint32s
	// to reduce syscall overhead.
	ibuf := make([]byte, 4*size)

	_, err := rand.Read(ibuf)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	buf.Grow(size)

	for i := 0; i < size; i++ {
		count, err := UnbiasedModulo32(
			binary.BigEndian.Uint32(ibuf[i*4:(i+1)*4]),
			int32(len(charSet)),
		)
		if err != nil {
			return "", err
		}

		_, _ = buf.WriteRune(charSet[count])
	}

	return buf.String(), nil
}

// MustStringCharset generates a random string of the given length, using the
// provided charset. It will panic if an error occurs.
func MustStringCharset(charSet string, size int) string {
	s, err := StringCharset(charSet, size)
	must(err)
	return s
}

// String returns a random string using Default.
func String(size int) (string, error) {
	return StringCharset(Default, size)
}

// MustString generates a random string of the given length, using
// the Default charset. It will panic if an error occurs.
func MustString(size int) string {
	s, err := String(size)
	must(err)
	return s
}

// HexString returns a hexadecimal string of given length.
func HexString(size int) (string, error) {
	return StringCharset(Hex, size)
}

// MustHexString generates a random hexadecimal string of the given
// length. It will panic if an error occurs.
func MustHexString(size int) string {
	s, err := HexString(size)
	must(err)
	return s
}

// Sha1String returns a 40-character hexadecimal string, which matches the
// length of a SHA-1 hash (160 bits).
func Sha1String() (string, error) {
	return StringCharset(Hex, 40)
}

// MustSha1String returns a 40-character hexadecimal string, which matches the
// length of a SHA-1 hash (160 bits). It will panic if an error occurs.
func MustSha1String() string {
	s, err := Sha1String()
	must(err)
	return s
}

// must is a utility function that panics with the given error if
// err is non-nil.
func must(err error) {
	if err != nil {
		panic(xerrors.Errorf("crand: %w", err))
	}
}

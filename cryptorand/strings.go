package cryptorand

import (
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

// StringCharset generates a random string using the provided charset and size
func StringCharset(charSetStr string, size int) (string, error) {
	charSet := []rune(charSetStr)

	if size == 0 {
		return "", nil
	}

	if len(charSet) == 0 {
		return "", xerrors.Errorf("charSetStr must not be empty")
	}

	var buf strings.Builder
	buf.Grow(size)

	for i := 0; i < size; i++ {
		ci, err := Intn(len(charSet))
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

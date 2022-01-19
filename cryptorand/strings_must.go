package cryptorand

// MustStringCharset generates a random string of the given length,
// using the provided charset. It will panic if an error occurs.
func MustStringCharset(charSet string, size int) string {
	s, err := StringCharset(charSet, size)
	must(err)
	return s
}

// MustString generates a random string of the given length, using
// the Default charset. It will panic if an error occurs.
func MustString(size int) string {
	s, err := String(size)
	must(err)
	return s
}

// MustHexString generates a random hexadecimal string of the given
// length. It will panic if an error occurs.
func MustHexString(size int) string {
	s, err := HexString(size)
	must(err)
	return s
}

// MustSha1String returns a 20-character hexadecimal string, which
// matches the length of a SHA-1 hash (160 bits). It will panic if
// an error occurs.
func MustSha1String() string {
	s, err := Sha1String()
	must(err)
	return s
}

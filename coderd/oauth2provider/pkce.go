package oauth2provider

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

// VerifyPKCE verifies that the code_verifier matches the code_challenge
// using the S256 method as specified in RFC 7636.
func VerifyPKCE(challenge, verifier string) bool {
	if challenge == "" || verifier == "" {
		return false
	}

	// S256: BASE64URL-ENCODE(SHA256(ASCII(code_verifier))) == code_challenge
	h := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])
	return subtle.ConstantTimeCompare([]byte(challenge), []byte(computed)) == 1
}

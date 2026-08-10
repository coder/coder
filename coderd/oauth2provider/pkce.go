package oauth2provider

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
)

// PKCE code verifier bounds from RFC 7636 §4.1.
const (
	pkceVerifierMinLength = 43
	pkceVerifierMaxLength = 128
)

// ValidPKCEFormat reports whether s meets RFC 7636 §4.1: 43 to 128 characters
// of the unreserved set [A-Za-z0-9-._~]. RFC 7636 gives code_verifier and
// code_challenge the same ABNF, so this check applies to both: a code_verifier
// directly, and a code_challenge because the S256 method that produces it
// (base64url(SHA256(verifier))) always yields a string within these bounds.
//
// The length floor is the whole point. PKCE is the only client authentication
// some clients have, and the challenge travels in the authorization request
// URL while the code travels in the redirect, both of which land in browser
// history, referrer headers, and proxy logs. An attacker holding those
// brute-forces the verifier offline at whatever entropy the client chose,
// where no server-side rate limit applies. A client that sends a
// one-character verifier has set a one-character password, and the server
// should refuse it rather than accept whatever the client picked. The same
// bound on code_challenge keeps a malformed value from being persisted
// verbatim and failing late, at token exchange, instead of at the
// authorization request where RFC 7636 §4.4.1 expects it to be rejected.
func ValidPKCEFormat(s string) bool {
	if len(s) < pkceVerifierMinLength || len(s) > pkceVerifierMaxLength {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z',
			r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '-', r == '.', r == '_', r == '~':
		default:
			return false
		}
	}
	return true
}

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

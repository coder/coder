package oauth2providertest

import (
	"crypto/sha256"
	"encoding/base64"
)

// Test constants for OAuth2 testing
const (
	// TestRedirectURI is the standard test redirect URI
	TestRedirectURI = "http://localhost:9876/callback"

	// TestResourceURI is used for testing resource parameter
	TestResourceURI = "https://api.example.com"

	// InvalidCodeVerifier is well-formed (43 characters, RFC 7636 §4.1's
	// unreserved set) but does not hash to any issued challenge, so it
	// exercises the PKCE comparison failure (invalid_grant) rather than the
	// length/charset check (invalid_request).
	InvalidCodeVerifier = "wrong-verifier-that-is-well-formed-43-chars"

	// MalformedCodeVerifier is below RFC 7636 §4.1's 43-character floor, so
	// it exercises the length/charset check (invalid_request) instead of the
	// PKCE comparison.
	MalformedCodeVerifier = "too-short"
)

// OAuth2ErrorTypes contains standard OAuth2 error codes
var OAuth2ErrorTypes = struct {
	InvalidRequest       string
	InvalidClient        string
	InvalidGrant         string
	UnauthorizedClient   string
	UnsupportedGrantType string
	InvalidScope         string
}{
	InvalidRequest:       "invalid_request",
	InvalidClient:        "invalid_client",
	InvalidGrant:         "invalid_grant",
	UnauthorizedClient:   "unauthorized_client",
	UnsupportedGrantType: "unsupported_grant_type",
	InvalidScope:         "invalid_scope",
}

// GenerateCodeChallenge creates an S256 code challenge from a verifier
func GenerateCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

package identityprovidertest

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

	// Test PKCE values from RFC 7636 examples
	TestCodeVerifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	TestCodeChallenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	// Alternative PKCE values for multi-test scenarios
	TestCodeVerifier2 = "3641a2d12d66101249cdf7a79c000c1f8c05d2e72842b98070b8a09b1c1eb0a95"

	// Invalid PKCE verifier for negative testing
	InvalidCodeVerifier = "wrong-verifier"
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

// TestCodeChallenge2 is the generated challenge for TestCodeVerifier2
var TestCodeChallenge2 = GenerateCodeChallenge(TestCodeVerifier2)

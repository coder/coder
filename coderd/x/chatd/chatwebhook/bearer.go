package chatwebhook

import (
	"net/http"
	"strings"
	"time"
)

// bearerVerifier compares the token of an "Authorization: Bearer TOKEN"
// header with the secret. It does not read the body.
type bearerVerifier struct{}

func (bearerVerifier) Verify(header http.Header, _ []byte, secret string, _ time.Time) error {
	// An empty secret would accept the empty token of "Bearer ".
	if secret == "" {
		return reject("secret is empty")
	}
	value, ok := singleHeader(header, "Authorization")
	if !ok {
		return reject("authorization header missing or repeated")
	}
	// RFC 7235: the authentication scheme is case-insensitive.
	const prefixLen = len("Bearer ")
	if len(value) <= prefixLen || strings.ToLower(value[:prefixLen]) != "bearer " {
		return reject("authorization header is not a bearer token")
	}
	if !secretsEqual(value[prefixLen:], secret) {
		return reject("token does not match")
	}
	return nil
}

func (bearerVerifier) GenerateSecret() (string, bool, error) {
	return generateSecret()
}

func (bearerVerifier) ValidateSecret(string) error {
	return invalidSecret("coder creates the secret for this scheme")
}

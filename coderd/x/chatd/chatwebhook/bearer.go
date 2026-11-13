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
	value, ok := singleHeader(header, "Authorization")
	if !ok {
		return reject("authorization header missing or repeated")
	}
	// RFC 6750 section 2.1: credentials = "Bearer" 1*SP b64token. RFC 7235:
	// the authentication scheme is case-insensitive. Only spaces separate
	// the scheme from the token. Anything after the token is part of it.
	const scheme = "Bearer"
	if len(value) <= len(scheme) || strings.ToLower(value[:len(scheme)]) != "bearer" || value[len(scheme)] != ' ' {
		return reject("authorization header is not a bearer token")
	}
	// A token is required, so "Bearer " never matches an empty secret.
	token := strings.TrimLeft(value[len(scheme):], " ")
	if token == "" {
		return reject("bearer token is empty")
	}
	if !secretsEqual(token, secret) {
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

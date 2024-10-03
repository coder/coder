package jwtutils

import (
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

const (
	keyIDHeaderKey = "kid"
)

// Claims defines the payload for a JWT. Most callers
// should embed jwt.Claims
type Claims interface {
	Validate(jwt.Expected) error
}

// ParseOptions are options for parsing a JWT.
type ParseOptions struct {
	RegisteredClaims jwt.Expected

	// The following are only used for JWSs.
	SignatureAlgorithm jose.SignatureAlgorithm

	// The following should only be used for JWEs.
	KeyAlgorithm               jose.KeyAlgorithm
	ContentEncryptionAlgorithm jose.ContentEncryption
}

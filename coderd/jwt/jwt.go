package jwt

import (
	"github.com/go-jose/go-jose/v4"
	jjwt "github.com/go-jose/go-jose/v4/jwt"
)

// Claims defines the payload for a JWT. Most callers
// should ember go-jose/jwt.Claims
type Claims interface {
	Validate(jjwt.Expected) error
}

// ParseOptions are options for parsing a JWT.
type ParseOptions struct {
	RegisteredClaims jjwt.Expected

	// The following are only used for JWSs.
	SignatureAlgorithm jose.SignatureAlgorithm

	// The following should only be used for JWEs.
	KeyAlgorithm               jose.KeyAlgorithm
	ContentEncryptionAlgorithm jose.ContentEncryption
}

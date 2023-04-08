package systemaccounttokenprovider

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
)

var (
	ErrInvalidSystemAccountToken = errors.New("invalid system account token")
	ErrExpiredSystemAccountToken = errors.New("system account token is expired")
)

// SystemAccountTokenPrefix is the prefix string for a system account JWT token
const SystemAccountTokenPrefix = "sysacct"

// SystemAccountTokenClaims defines the claims for a system account JWT token
type SystemAccountTokenClaims struct {
	jwt.StandardClaims
	SystemAccountID string `json:"sysacct_id,omitempty"`
}

// SystemAccountTokenProvider is an interface for a system account JWT token provider
type SystemAccountTokenProvider interface {
	CreateSystemAccountJWTToken(systemAccountID string) (string, error)
	ValidateSystemAccountJWTToken(tokenString string) (string, error)
}

// systemAccountTokenProvider is the default implementation of the SystemAccountTokenProvider interface
type systemAccountTokenProvider struct {
	secretKey      string
	expirationTime int64
	clock          func() time.Time
}

// NewSystemAccountTokenProvider creates a new SystemAccountTokenProvider instance with the given secret key, expiration time, and clock function
func NewSystemAccountTokenProvider(secretKey string, expirationTime int64, clock func() time.Time) SystemAccountTokenProvider {
	return &systemAccountTokenProvider{
		secretKey:      secretKey,
		expirationTime: expirationTime,
		clock:          clock,
	}
}

// CreateSystemAccountJWTToken creates a new system account JWT token with the given system account ID, secret key, and expiration time, and concatenates the prefix string with the encoded token string.
func (p *systemAccountTokenProvider) CreateSystemAccountJWTToken(systemAccountID string) (string, error) {
	// Create the token claims
	claims := &SystemAccountTokenClaims{
		SystemAccountID: systemAccountID,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: p.clock().Unix() + p.expirationTime,
			IssuedAt:  p.clock().Unix(),
			NotBefore: p.clock().Unix(),
		},
	}

	// Create a new token object, specifying signing method and the claims
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign the token using the secret key
	tokenString, err := token.SignedString([]byte(p.secretKey))
	if err != nil {
		return "", err
	}

	// Concatenate the prefix and the encoded token string
	prefixedTokenString := SystemAccountTokenPrefix + "." + tokenString

	return prefixedTokenString, nil
}

// ValidateSystemAccountJWTToken validates and decodes the given system account JWT token string, and returns the decoded system account ID.
func (p *systemAccountTokenProvider) ValidateSystemAccountJWTToken(prefixedTokenString string) (string, error) {
	// Split the prefixed token string into prefix and encoded token string
	parts := strings.SplitN(prefixedTokenString, ".", 2)
	if len(parts) != 2 || parts[0] != SystemAccountTokenPrefix {
		return "", ErrInvalidSystemAccountToken
	}
	tokenString := parts[1]

	// Parse the token
	token, err := jwt.ParseWithClaims(tokenString, &SystemAccountTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		// Return the secret key
		return []byte(p.secretKey), nil
	})
	if err != nil {
		if ve, ok := err.(*jwt.ValidationError); ok {
			if ve.Errors&jwt.ValidationErrorExpired != 0 {
				return "", errors.New("system account token is expired")
			}
			return "", ErrInvalidSystemAccountToken
		}
		return "", err
	}

	// Validate the token claims
	if claims, ok := token.Claims.(*SystemAccountTokenClaims); ok && token.Valid {
		systemAccountID, err := base64.StdEncoding.DecodeString(claims.SystemAccountID)
		if err != nil {
			return "", ErrInvalidSystemAccountToken
		}
		return string(systemAccountID), nil
	}

	return "", ErrInvalidSystemAccountToken
}

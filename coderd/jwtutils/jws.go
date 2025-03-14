package jwtutils
import (
	"fmt"
	"errors"
	"context"
	"encoding/json"
	"time"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)
var ErrMissingKeyID = errors.New("missing key ID")
const (
	keyIDHeaderKey = "kid"
)
// RegisteredClaims is a convenience type for embedding jwt.Claims. It should be
// preferred over embedding jwt.Claims directly since it will ensure that certain fields are set.
type RegisteredClaims jwt.Claims
func (r RegisteredClaims) Validate(e jwt.Expected) error {
	if r.Expiry == nil {
		return fmt.Errorf("expiry is required")
	}
	if e.Time.IsZero() {
		return fmt.Errorf("expected time is required")
	}
	return (jwt.Claims(r)).Validate(e)
}
// Claims defines the payload for a JWT. Most callers
// should embed jwt.Claims
type Claims interface {
	Validate(jwt.Expected) error
}
const (
	SigningAlgo = jose.HS512
)
type SigningKeyManager interface {
	SigningKeyProvider
	VerifyKeyProvider
}
type SigningKeyProvider interface {
	SigningKey(ctx context.Context) (id string, key interface{}, err error)
}
type VerifyKeyProvider interface {
	VerifyingKey(ctx context.Context, id string) (key interface{}, err error)
}
// Sign signs a token and returns it as a string.
func Sign(ctx context.Context, s SigningKeyProvider, claims Claims) (string, error) {
	id, key, err := s.SigningKey(ctx)
	if err != nil {
		return "", fmt.Errorf("get signing key: %w", err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{
		Algorithm: SigningAlgo,
		Key:       key,
	}, &jose.SignerOptions{
		ExtraHeaders: map[jose.HeaderKey]interface{}{
			keyIDHeaderKey: id,
		},
	})
	if err != nil {
		return "", fmt.Errorf("new signer: %w", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	signed, err := signer.Sign(payload)
	if err != nil {
		return "", fmt.Errorf("sign payload: %w", err)
	}
	compact, err := signed.CompactSerialize()
	if err != nil {
		return "", fmt.Errorf("compact serialize: %w", err)
	}
	return compact, nil
}
// VerifyOptions are options for verifying a JWT.
type VerifyOptions struct {
	RegisteredClaims   jwt.Expected
	SignatureAlgorithm jose.SignatureAlgorithm
}
func WithVerifyExpected(expected jwt.Expected) func(*VerifyOptions) {
	return func(opts *VerifyOptions) {
		opts.RegisteredClaims = expected
	}
}
// Verify verifies that a token was signed by the provided key. It unmarshals into the provided claims.
func Verify(ctx context.Context, v VerifyKeyProvider, token string, claims Claims, opts ...func(*VerifyOptions)) error {
	options := VerifyOptions{
		RegisteredClaims: jwt.Expected{
			Time: time.Now(),
		},
		SignatureAlgorithm: SigningAlgo,
	}
	for _, opt := range opts {
		opt(&options)
	}
	object, err := jose.ParseSigned(token, []jose.SignatureAlgorithm{options.SignatureAlgorithm})
	if err != nil {
		return fmt.Errorf("parse JWS: %w", err)
	}
	if len(object.Signatures) != 1 {
		return errors.New("expected 1 signature")
	}
	signature := object.Signatures[0]
	if signature.Header.Algorithm != string(SigningAlgo) {
		return fmt.Errorf("expected JWS algorithm to be %q, got %q", SigningAlgo, object.Signatures[0].Header.Algorithm)
	}
	kid := signature.Header.KeyID
	if kid == "" {
		return ErrMissingKeyID
	}
	key, err := v.VerifyingKey(ctx, kid)
	if err != nil {
		return fmt.Errorf("key with id %q: %w", kid, err)
	}
	payload, err := object.Verify(key)
	if err != nil {
		return fmt.Errorf("verify payload: %w", err)
	}
	err = json.Unmarshal(payload, &claims)
	if err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}
	return claims.Validate(options.RegisteredClaims)
}
// StaticKey fulfills the SigningKeycache and EncryptionKeycache interfaces. Useful for testing.
type StaticKey struct {
	ID  string
	Key interface{}
}
func (s StaticKey) SigningKey(_ context.Context) (string, interface{}, error) {
	return s.ID, s.Key, nil
}
func (s StaticKey) VerifyingKey(_ context.Context, id string) (interface{}, error) {
	if id != s.ID {
		return nil, fmt.Errorf("invalid id %q", id)
	}
	return s.Key, nil
}
func (s StaticKey) EncryptingKey(_ context.Context) (string, interface{}, error) {
	return s.ID, s.Key, nil
}
func (s StaticKey) DecryptingKey(_ context.Context, id string) (interface{}, error) {
	if id != s.ID {
		return nil, fmt.Errorf("invalid id %q", id)
	}
	return s.Key, nil
}
func (StaticKey) Close() error {
	return nil
}

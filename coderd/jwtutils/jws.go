package jwtutils

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/xerrors"
)

const (
	keyIDHeaderKey = "kid"
)

// Claims defines the payload for a JWT. Most callers
// should embed jwt.Claims
type Claims interface {
	Validate(jwt.Expected) error
}

const (
	signingAlgo = jose.HS512
)

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
		return "", xerrors.Errorf("get signing key: %w", err)
	}

	signer, err := jose.NewSigner(jose.SigningKey{
		Algorithm: signingAlgo,
		Key:       key,
	}, &jose.SignerOptions{
		ExtraHeaders: map[jose.HeaderKey]interface{}{
			keyIDHeaderKey: id,
		},
	})
	if err != nil {
		return "", xerrors.Errorf("new signer: %w", err)
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", xerrors.Errorf("marshal claims: %w", err)
	}

	signed, err := signer.Sign(payload)
	if err != nil {
		return "", xerrors.Errorf("sign payload: %w", err)
	}

	compact, err := signed.CompactSerialize()
	if err != nil {
		return "", xerrors.Errorf("compact serialize: %w", err)
	}

	return compact, nil
}

// VerifyOptions are options for verifying a JWT.
type VerifyOptions struct {
	RegisteredClaims   jwt.Expected
	SignatureAlgorithm jose.SignatureAlgorithm
}

// Verify verifies that a token was signed by the provided key. It unmarshals into the provided claims.
func Verify(ctx context.Context, v VerifyKeyProvider, token string, claims Claims, opts ...func(*VerifyOptions)) error {
	options := VerifyOptions{
		RegisteredClaims: jwt.Expected{
			Time: time.Now(),
		},
		SignatureAlgorithm: signingAlgo,
	}

	for _, opt := range opts {
		opt(&options)
	}

	object, err := jose.ParseSigned(token, []jose.SignatureAlgorithm{options.SignatureAlgorithm})
	if err != nil {
		return xerrors.Errorf("parse JWS: %w", err)
	}

	if len(object.Signatures) != 1 {
		return xerrors.New("expected 1 signature")
	}

	signature := object.Signatures[0]

	if signature.Header.Algorithm != string(signingAlgo) {
		return xerrors.Errorf("expected JWS algorithm to be %q, got %q", signingAlgo, object.Signatures[0].Header.Algorithm)
	}

	kid := signature.Header.KeyID
	if kid == "" {
		return xerrors.Errorf("expected %q header to be a string", keyIDHeaderKey)
	}

	key, err := v.VerifyingKey(ctx, kid)
	if err != nil {
		return xerrors.Errorf("key with id %q: %w", kid, err)
	}

	payload, err := object.Verify(key)
	if err != nil {
		return xerrors.Errorf("verify payload: %w", err)
	}

	err = json.Unmarshal(payload, &claims)
	if err != nil {
		return xerrors.Errorf("unmarshal payload: %w", err)
	}

	return claims.Validate(options.RegisteredClaims)
}

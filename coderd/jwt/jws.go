package jwt

import (
	"encoding/json"
	"time"

	"github.com/go-jose/go-jose/v4"
	jjwt "github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/xerrors"
)

const (
	defaultSigningAlgo = jose.HS512
	featureHeaderKey   = "feat"
	keyIDHeaderKey     = "kid"
)

// Sign signs a token and returns it as a string.
func Sign(claims Claims, keyFn SecuringKeyFn) (string, error) {
	kid, key, err := keyFn()
	if err != nil {
		return "", xerrors.Errorf("get key: %w", err)
	}

	signer, err := jose.NewSigner(jose.SigningKey{
		Algorithm: defaultSigningAlgo,
		Key:       key,
	}, &jose.SignerOptions{
		ExtraHeaders: map[jose.HeaderKey]interface{}{
			keyIDHeaderKey: kid,
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

// Verify verifies that a token was signed by the provided key. It unmarshals into the provided claims.
func Verify(token string, claims Claims, keyFn ParseKeyFunc, opts ...func(*ParseOptions)) error {
	options := ParseOptions{
		RegisteredClaims: jjwt.Expected{
			Time: time.Now(),
		},
		SignatureAlgorithm: defaultSigningAlgo,
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

	if signature.Header.Algorithm != string(defaultSigningAlgo) {
		return xerrors.Errorf("expected token signing algorithm to be %q, got %q", defaultSigningAlgo, object.Signatures[0].Header.Algorithm)
	}

	key, err := keyFn(signature.Header)
	if err != nil {
		return xerrors.Errorf("get key: %w", err)
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

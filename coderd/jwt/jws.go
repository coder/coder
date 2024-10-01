package jwt

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"

	"github.com/go-jose/go-jose/v4"
	jjwt "github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/cryptokeys"
)

type Claims interface {
	Validate(jjwt.Expected) error
}

const (
	defaultSigningAlgo = jose.HS512
	featureHeaderKey   = "feat"
	keyIDHeaderKey     = "kid"
)

type SecuringKeyFn func() (id string, key interface{}, err error)

func KeycacheSecure(ctx context.Context, keys cryptokeys.Keycache) SecuringKeyFn {
	return func() (id string, key interface{}, err error) {
		signing, err := keys.Signing(ctx)
		if err != nil {
			return "", nil, xerrors.Errorf("get signing key: %w", err)
		}

		decoded, err := hex.DecodeString(signing.Secret)
		if err != nil {
			return "", nil, xerrors.Errorf("decode signing key: %w", err)
		}

		return strconv.FormatInt(int64(signing.Sequence), 10), decoded, nil
	}
}

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

type KeyFunc func(jose.Header) (interface{}, error)

func KeycacheVerify(ctx context.Context, keys cryptokeys.Keycache) KeyFunc {
	return func(header jose.Header) (interface{}, error) {
		sequenceStr := header.KeyID
		if sequenceStr == "" {
			return nil, xerrors.Errorf("expected %q header to be a string", keyIDHeaderKey)
		}

		sequence, err := strconv.ParseInt(sequenceStr, 10, 32)
		if err != nil {
			return nil, xerrors.Errorf("parse sequence: %w", err)
		}

		key, err := keys.Verifying(ctx, int32(sequence))
		if err != nil {
			return nil, xerrors.Errorf("version: %w", err)
		}

		decoded, err := hex.DecodeString(key.Secret)
		if err != nil {
			return nil, xerrors.Errorf("decode key: %w", err)
		}

		return decoded, nil
	}
}

type ParseOptions struct {
	RegisteredClaims jjwt.Expected

	// The following are only used for JWSs.
	SignatureAlgorithm jose.SignatureAlgorithm

	// The following should only be used for JWEs.
	KeyAlgorithm               jose.KeyAlgorithm
	ContentEncryptionAlgorithm jose.ContentEncryption
}

func Verify(token string, claims Claims, keyFn KeyFunc, opts ...func(*ParseOptions)) error {
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

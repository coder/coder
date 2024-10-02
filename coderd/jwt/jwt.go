package jwt

import (
	"context"
	"encoding/hex"
	"strconv"

	"github.com/go-jose/go-jose/v4"
	jjwt "github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/cryptokeys"
)

type Claims interface {
	Validate(jjwt.Expected) error
}

// SecuringKeyFn returns a key for signing or encrypting.
type SecuringKeyFn func() (id string, key interface{}, err error)

// SecuringKeyFromCache returns the appropriate key for signing or encrypting.
func SecuringKeyFromCache(ctx context.Context, keys cryptokeys.Keycache) SecuringKeyFn {
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

// ParseKeyFunc returns a key for verifying or decrypting a token.
type ParseKeyFunc func(jose.Header) (interface{}, error)

// ParseKeyFromCache returns the appropriate key to decrypt or verify a token.
func ParseKeyFromCache(ctx context.Context, keys cryptokeys.Keycache) ParseKeyFunc {
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

// ParseOptions are options for parsing a JWT.
type ParseOptions struct {
	RegisteredClaims jjwt.Expected

	// The following are only used for JWSs.
	SignatureAlgorithm jose.SignatureAlgorithm

	// The following should only be used for JWEs.
	KeyAlgorithm               jose.KeyAlgorithm
	ContentEncryptionAlgorithm jose.ContentEncryption
}

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
	encryptKeyAlgo     = jose.A256GCMKW
	encryptContentAlgo = jose.A256GCM
)

type EncryptKeyProvider interface {
	EncryptingKey(ctx context.Context) (id string, key interface{}, err error)
}

type DecryptKeyProvider interface {
	DecryptingKey(ctx context.Context, id string) (key interface{}, err error)
}

// Encrypt encrypts a token and returns it as a string.
func Encrypt(ctx context.Context, e EncryptKeyProvider, claims Claims) (string, error) {
	id, key, err := e.EncryptingKey(ctx)
	if err != nil {
		return "", xerrors.Errorf("encrypting key: %w", err)
	}

	encrypter, err := jose.NewEncrypter(
		encryptContentAlgo,
		jose.Recipient{
			Algorithm: encryptKeyAlgo,
			Key:       key,
		},
		&jose.EncrypterOptions{
			Compression: jose.DEFLATE,
			ExtraHeaders: map[jose.HeaderKey]interface{}{
				keyIDHeaderKey: id,
			},
		},
	)
	if err != nil {
		return "", xerrors.Errorf("initialize encrypter: %w", err)
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", xerrors.Errorf("marshal payload: %w", err)
	}

	encrypted, err := encrypter.Encrypt(payload)
	if err != nil {
		return "", xerrors.Errorf("encrypt: %w", err)
	}

	compact, err := encrypted.CompactSerialize()
	if err != nil {
		return "", xerrors.Errorf("compact serialize: %w", err)
	}

	return compact, nil
}

func WithDecryptExpected(expected jwt.Expected) func(*DecryptOptions) {
	return func(opts *DecryptOptions) {
		opts.RegisteredClaims = expected
	}
}

// DecryptOptions are options for decrypting a JWE.
type DecryptOptions struct {
	RegisteredClaims           jwt.Expected
	KeyAlgorithm               jose.KeyAlgorithm
	ContentEncryptionAlgorithm jose.ContentEncryption
}

// Decrypt decrypts the token using the provided key. It unmarshals into the provided claims.
func Decrypt(ctx context.Context, d DecryptKeyProvider, token string, claims Claims, opts ...func(*DecryptOptions)) error {
	options := DecryptOptions{
		RegisteredClaims: jwt.Expected{
			Time: time.Now(),
		},
		KeyAlgorithm:               encryptKeyAlgo,
		ContentEncryptionAlgorithm: encryptContentAlgo,
	}

	for _, opt := range opts {
		opt(&options)
	}

	object, err := jose.ParseEncrypted(token,
		[]jose.KeyAlgorithm{options.KeyAlgorithm},
		[]jose.ContentEncryption{options.ContentEncryptionAlgorithm},
	)
	if err != nil {
		return xerrors.Errorf("parse jwe: %w", err)
	}

	if object.Header.Algorithm != string(encryptKeyAlgo) {
		return xerrors.Errorf("expected JWE algorithm to be %q, got %q", encryptKeyAlgo, object.Header.Algorithm)
	}

	kid := object.Header.KeyID
	if kid == "" {
		return ErrMissingKeyID
	}

	key, err := d.DecryptingKey(ctx, kid)
	if err != nil {
		return xerrors.Errorf("key with id %q: %w", kid, err)
	}

	decrypted, err := object.Decrypt(key)
	if err != nil {
		return xerrors.Errorf("decrypt: %w", err)
	}

	if err := json.Unmarshal(decrypted, &claims); err != nil {
		return xerrors.Errorf("unmarshal: %w", err)
	}

	return claims.Validate(options.RegisteredClaims)
}

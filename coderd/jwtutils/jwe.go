package jwtutils

import (
	"context"
	"encoding/base64"
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

type EncryptKeyer interface {
	EncryptingKey(ctx context.Context) (id string, key interface{}, err error)
}

type DecryptKeyer interface {
	DecryptingKey(ctx context.Context, id string) (key interface{}, err error)
}

// Encrypt encrypts a token and returns it as a string.
func Encrypt(ctx context.Context, e EncryptKeyer, claims Claims) (string, error) {
	id, key, err := e.EncryptingKey(ctx)
	if err != nil {
		return "", xerrors.Errorf("get signing key: %w", err)
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

	serialized := []byte(encrypted.FullSerialize())
	return base64.RawURLEncoding.EncodeToString(serialized), nil
}

// Decrypt decrypts the token using the provided key. It unmarshals into the provided claims.
func Decrypt(ctx context.Context, d DecryptKeyer, token string, claims Claims, opts ...func(*ParseOptions)) error {
	options := ParseOptions{
		RegisteredClaims: jwt.Expected{
			Time: time.Now(),
		},
		KeyAlgorithm:               encryptKeyAlgo,
		ContentEncryptionAlgorithm: encryptContentAlgo,
	}

	for _, opt := range opts {
		opt(&options)
	}

	encrypted, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return xerrors.Errorf("decode: %w", err)
	}

	object, err := jose.ParseEncrypted(string(encrypted),
		[]jose.KeyAlgorithm{options.KeyAlgorithm},
		[]jose.ContentEncryption{options.ContentEncryptionAlgorithm},
	)
	if err != nil {
		return xerrors.Errorf("parse jwe: %w", err)
	}

	if object.Header.Algorithm != string(encryptKeyAlgo) {
		return xerrors.Errorf("expected JWE algorithm to be %q, got %q", encryptKeyAlgo, object.Header.Algorithm)
	}

	sequenceStr := object.Header.KeyID
	if sequenceStr == "" {
		return xerrors.Errorf("expected %q header to be a string", keyIDHeaderKey)
	}

	key, err := d.DecryptingKey(ctx, sequenceStr)
	if err != nil {
		return xerrors.Errorf("version: %w", err)
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

package jwt

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/go-jose/go-jose/v4"
	jjwt "github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/xerrors"
)

const (
	encryptKeyAlgo     = jose.A256GCMKW
	encryptContentAlgo = jose.A256GCM
)

func Encrypt(claims Claims, keyFn SecuringKeyFn) (string, error) {
	kid, key, err := keyFn()
	if err != nil {
		return "", xerrors.Errorf("get key: %w", err)
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
				keyIDHeaderKey: kid,
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

func Decrypt(token string, claims Claims, keyFn KeyFunc, opts ...func(*ParseOptions)) error {
	options := ParseOptions{
		RegisteredClaims: jjwt.Expected{
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
		return xerrors.Errorf("parse encrypted API key: %w", err)
	}

	if object.Header.Algorithm != string(encryptKeyAlgo) {
		return xerrors.Errorf("expected API key encryption algorithm to be %q, got %q", encryptKeyAlgo, object.Header.Algorithm)
	}

	key, err := keyFn(object.Header)
	if err != nil {
		return xerrors.Errorf("get key: %w", err)
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

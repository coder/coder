package jwt

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"

	"github.com/go-jose/go-jose/v4"
	jjwt "github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/cryptokeys"
)

const (
	encryptKeyAlgo     = jose.A256GCMKW
	encryptContentAlgo = jose.A256GCM
)

// Encrypt encrypts a token and returns it as a string.
func Encrypt(ctx context.Context, keys cryptokeys.Keycache, claims Claims) (string, error) {
	signing, err := keys.Signing(ctx)
	if err != nil {
		return "", xerrors.Errorf("get signing key: %w", err)
	}

	decoded, err := hex.DecodeString(signing.Secret)
	if err != nil {
		return "", xerrors.Errorf("decode signing key: %w", err)
	}

	encrypter, err := jose.NewEncrypter(
		encryptContentAlgo,
		jose.Recipient{
			Algorithm: encryptKeyAlgo,
			Key:       decoded,
		},
		&jose.EncrypterOptions{
			Compression: jose.DEFLATE,
			ExtraHeaders: map[jose.HeaderKey]interface{}{
				keyIDHeaderKey: strconv.FormatInt(int64(signing.Sequence), 10),
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
func Decrypt(ctx context.Context, keys cryptokeys.Keycache, token string, claims Claims, opts ...func(*ParseOptions)) error {
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

	sequenceStr := object.Header.KeyID
	if sequenceStr == "" {
		return xerrors.Errorf("expected %q header to be a string", keyIDHeaderKey)
	}

	sequence, err := strconv.ParseInt(sequenceStr, 10, 32)
	if err != nil {
		return xerrors.Errorf("parse sequence: %w", err)
	}

	key, err := keys.Verifying(ctx, int32(sequence))
	if err != nil {
		return xerrors.Errorf("version: %w", err)
	}

	decoded, err := hex.DecodeString(key.Secret)
	if err != nil {
		return xerrors.Errorf("decode key: %w", err)
	}

	decrypted, err := object.Decrypt(decoded)
	if err != nil {
		return xerrors.Errorf("decrypt: %w", err)
	}

	if err := json.Unmarshal(decrypted, &claims); err != nil {
		return xerrors.Errorf("unmarshal: %w", err)
	}

	return claims.Validate(options.RegisteredClaims)
}

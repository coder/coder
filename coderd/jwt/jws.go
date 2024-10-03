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

const (
	defaultSigningAlgo = jose.HS512
	keyIDHeaderKey     = "kid"
)

// Sign signs a token and returns it as a string.
func Sign(ctx context.Context, keys cryptokeys.Keycache, claims Claims) (string, error) {
	signing, err := keys.Signing(ctx)
	if err != nil {
		return "", xerrors.Errorf("get signing key: %w", err)
	}

	decoded, err := hex.DecodeString(signing.Secret)
	if err != nil {
		return "", xerrors.Errorf("decode signing key: %w", err)
	}

	signer, err := jose.NewSigner(jose.SigningKey{
		Algorithm: defaultSigningAlgo,
		Key:       decoded,
	}, &jose.SignerOptions{
		ExtraHeaders: map[jose.HeaderKey]interface{}{
			keyIDHeaderKey: strconv.FormatInt(int64(signing.Sequence), 10),
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
func Verify(ctx context.Context, keys cryptokeys.Keycache, token string, claims Claims, opts ...func(*ParseOptions)) error {
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

	sequenceStr := signature.Header.KeyID
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

	payload, err := object.Verify(decoded)
	if err != nil {
		return xerrors.Errorf("verify payload: %w", err)
	}

	err = json.Unmarshal(payload, &claims)
	if err != nil {
		return xerrors.Errorf("unmarshal payload: %w", err)
	}

	return claims.Validate(options.RegisteredClaims)
}

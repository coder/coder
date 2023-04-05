package workspaceapps

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
)

const tokenSigningAlgorithm = jose.HS512

// SignedToken is the struct data contained inside a workspace app JWE. It
// contains the details of the workspace app that the token is valid for to
// avoid database queries.
type SignedToken struct {
	// Request details.
	Request `json:"request"`

	// Trusted resolved details.
	Expiry      time.Time `json:"expiry"` // set by GenerateToken if unset
	UserID      uuid.UUID `json:"user_id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	AgentID     uuid.UUID `json:"agent_id"`
	AppURL      string    `json:"app_url"`
}

// MatchesRequest returns true if the token matches the request. Any token that
// does not match the request should be considered invalid.
func (t SignedToken) MatchesRequest(req Request) bool {
	return t.AccessMethod == req.AccessMethod &&
		t.BasePath == req.BasePath &&
		t.UsernameOrID == req.UsernameOrID &&
		t.WorkspaceNameOrID == req.WorkspaceNameOrID &&
		t.AgentNameOrID == req.AgentNameOrID &&
		t.AppSlugOrPort == req.AppSlugOrPort
}

// SigningKey is used for signing and encrypting app tokens and API keys.
// TODO: we cannot use the same key for signing and encrypting with two
// different algorithms, that's a security issue. We should use a different key
// for each.
// OR
// We get rid of signing and use encryption for both api keys and tickets.
// Do this by switching to JWE.
type SigningKey [64]byte

func KeyFromString(str string) (SigningKey, error) {
	var key SigningKey
	decoded, err := hex.DecodeString(str)
	if err != nil {
		return key, xerrors.Errorf("decode key: %w", err)
	}
	if len(decoded) != len(key) {
		return key, xerrors.Errorf("expected key to be %d bytes, got %d", len(key), len(decoded))
	}
	copy(key[:], decoded)

	return key, nil
}

// SignToken generates a signed workspace app token with the given payload. If
// the payload doesn't have an expiry, it will be set to the current time plus
// the default expiry.
func (k SigningKey) SignToken(payload SignedToken) (string, error) {
	if payload.Expiry.IsZero() {
		payload.Expiry = time.Now().Add(DefaultTokenExpiry)
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", xerrors.Errorf("marshal payload to JSON: %w", err)
	}

	signer, err := jose.NewSigner(jose.SigningKey{
		Algorithm: tokenSigningAlgorithm,
		Key:       k[:],
	}, nil)
	if err != nil {
		return "", xerrors.Errorf("create signer: %w", err)
	}

	signedObject, err := signer.Sign(payloadBytes)
	if err != nil {
		return "", xerrors.Errorf("sign payload: %w", err)
	}

	serialized, err := signedObject.CompactSerialize()
	if err != nil {
		return "", xerrors.Errorf("serialize JWS: %w", err)
	}

	return serialized, nil
}

// VerifySignedToken parses a signed workspace app token with the given key and
// returns the payload. If the token is invalid or expired, an error is
// returned.
func (k SigningKey) VerifySignedToken(str string) (SignedToken, error) {
	object, err := jose.ParseSigned(str)
	if err != nil {
		return SignedToken{}, xerrors.Errorf("parse JWS: %w", err)
	}
	if len(object.Signatures) != 1 {
		return SignedToken{}, xerrors.New("expected 1 signature")
	}
	if object.Signatures[0].Header.Algorithm != string(tokenSigningAlgorithm) {
		return SignedToken{}, xerrors.Errorf("expected token signing algorithm to be %q, got %q", tokenSigningAlgorithm, object.Signatures[0].Header.Algorithm)
	}

	output, err := object.Verify(k[:])
	if err != nil {
		return SignedToken{}, xerrors.Errorf("verify JWS: %w", err)
	}

	var tok SignedToken
	err = json.Unmarshal(output, &tok)
	if err != nil {
		return SignedToken{}, xerrors.Errorf("unmarshal payload: %w", err)
	}
	if tok.Expiry.Before(time.Now()) {
		return SignedToken{}, xerrors.New("signed app token expired")
	}

	return tok, nil
}

type EncryptedAPIKeyPayload struct {
	APIKey    string    `json:"api_key"`
	ExpiresAt time.Time `json:"expires_at"`
}

// EncryptAPIKey encrypts an API key for subdomain token smuggling.
func (k SigningKey) EncryptAPIKey(payload EncryptedAPIKeyPayload) (string, error) {
	if payload.APIKey == "" {
		return "", xerrors.New("API key is empty")
	}
	if payload.ExpiresAt.IsZero() {
		// Very short expiry as these keys are only used once as part of an
		// automatic redirection flow.
		payload.ExpiresAt = database.Now().Add(time.Minute)
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", xerrors.Errorf("marshal payload: %w", err)
	}

	// JWEs seem to apply a nonce themselves.
	encrypter, err := jose.NewEncrypter(
		jose.A256GCM,
		jose.Recipient{
			Algorithm: jose.A256GCMKW,
			Key:       k[:32],
		},
		&jose.EncrypterOptions{
			Compression: jose.DEFLATE,
		},
	)
	if err != nil {
		return "", xerrors.Errorf("initializer jose encrypter: %w", err)
	}
	encryptedObject, err := encrypter.Encrypt(payloadBytes)
	if err != nil {
		return "", xerrors.Errorf("encrypt jwe: %w", err)
	}

	encrypted := encryptedObject.FullSerialize()
	return base64.RawURLEncoding.EncodeToString([]byte(encrypted)), nil
}

// DecryptAPIKey undoes EncryptAPIKey and is used in the subdomain app handler.
func (k SigningKey) DecryptAPIKey(encryptedAPIKey string) (string, error) {
	encrypted, err := base64.RawURLEncoding.DecodeString(encryptedAPIKey)
	if err != nil {
		return "", xerrors.Errorf("base64 decode encrypted API key: %w", err)
	}

	object, err := jose.ParseEncrypted(string(encrypted))
	if err != nil {
		return "", xerrors.Errorf("parse encrypted API key: %w", err)
	}

	// Decrypt using the hashed secret.
	decrypted, err := object.Decrypt(k[:32])
	if err != nil {
		return "", xerrors.Errorf("decrypt API key: %w", err)
	}

	// Unmarshal the payload.
	var payload EncryptedAPIKeyPayload
	if err := json.Unmarshal(decrypted, &payload); err != nil {
		return "", xerrors.Errorf("unmarshal decrypted payload: %w", err)
	}

	// Validate expiry.
	if payload.ExpiresAt.Before(database.Now()) {
		return "", xerrors.New("encrypted API key expired")
	}

	return payload.APIKey, nil
}

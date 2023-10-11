package workspaceapps

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/codersdk"
)

const (
	tokenSigningAlgorithm     = jose.HS512
	apiKeyEncryptionAlgorithm = jose.A256GCMKW
)

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
	tokenBasePath := t.Request.BasePath
	if !strings.HasSuffix(tokenBasePath, "/") {
		tokenBasePath += "/"
	}
	reqBasePath := req.BasePath
	if !strings.HasSuffix(reqBasePath, "/") {
		reqBasePath += "/"
	}

	return t.AccessMethod == req.AccessMethod &&
		tokenBasePath == reqBasePath &&
		t.Prefix == req.Prefix &&
		t.UsernameOrID == req.UsernameOrID &&
		t.WorkspaceNameOrID == req.WorkspaceNameOrID &&
		t.AgentNameOrID == req.AgentNameOrID &&
		t.AppSlugOrPort == req.AppSlugOrPort
}

// SecurityKey is used for signing and encrypting app tokens and API keys.
//
// The first 64 bytes of the key are used for signing tokens with HMAC-SHA256,
// and the last 32 bytes are used for encrypting API keys with AES-256-GCM.
// We use a single key for both operations to avoid having to store and manage
// two keys.
type SecurityKey [96]byte

func (k SecurityKey) String() string {
	return hex.EncodeToString(k[:])
}

func (k SecurityKey) signingKey() []byte {
	return k[:64]
}

func (k SecurityKey) encryptionKey() []byte {
	return k[64:]
}

func KeyFromString(str string) (SecurityKey, error) {
	var key SecurityKey
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
func (k SecurityKey) SignToken(payload SignedToken) (string, error) {
	if payload.Expiry.IsZero() {
		payload.Expiry = time.Now().Add(DefaultTokenExpiry)
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", xerrors.Errorf("marshal payload to JSON: %w", err)
	}

	signer, err := jose.NewSigner(jose.SigningKey{
		Algorithm: tokenSigningAlgorithm,
		Key:       k.signingKey(),
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
func (k SecurityKey) VerifySignedToken(str string) (SignedToken, error) {
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

	output, err := object.Verify(k.signingKey())
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
func (k SecurityKey) EncryptAPIKey(payload EncryptedAPIKeyPayload) (string, error) {
	if payload.APIKey == "" {
		return "", xerrors.New("API key is empty")
	}
	if payload.ExpiresAt.IsZero() {
		// Very short expiry as these keys are only used once as part of an
		// automatic redirection flow.
		payload.ExpiresAt = dbtime.Now().Add(time.Minute)
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", xerrors.Errorf("marshal payload: %w", err)
	}

	// JWEs seem to apply a nonce themselves.
	encrypter, err := jose.NewEncrypter(
		jose.A256GCM,
		jose.Recipient{
			Algorithm: apiKeyEncryptionAlgorithm,
			Key:       k.encryptionKey(),
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
func (k SecurityKey) DecryptAPIKey(encryptedAPIKey string) (string, error) {
	encrypted, err := base64.RawURLEncoding.DecodeString(encryptedAPIKey)
	if err != nil {
		return "", xerrors.Errorf("base64 decode encrypted API key: %w", err)
	}

	object, err := jose.ParseEncrypted(string(encrypted))
	if err != nil {
		return "", xerrors.Errorf("parse encrypted API key: %w", err)
	}
	if object.Header.Algorithm != string(apiKeyEncryptionAlgorithm) {
		return "", xerrors.Errorf("expected API key encryption algorithm to be %q, got %q", apiKeyEncryptionAlgorithm, object.Header.Algorithm)
	}

	// Decrypt using the hashed secret.
	decrypted, err := object.Decrypt(k.encryptionKey())
	if err != nil {
		return "", xerrors.Errorf("decrypt API key: %w", err)
	}

	// Unmarshal the payload.
	var payload EncryptedAPIKeyPayload
	if err := json.Unmarshal(decrypted, &payload); err != nil {
		return "", xerrors.Errorf("unmarshal decrypted payload: %w", err)
	}

	// Validate expiry.
	if payload.ExpiresAt.Before(dbtime.Now()) {
		return "", xerrors.New("encrypted API key expired")
	}

	return payload.APIKey, nil
}

// FromRequest returns the signed token from the request, if it exists and is
// valid. The caller must check that the token matches the request.
func FromRequest(r *http.Request, key SecurityKey) (*SignedToken, bool) {
	// Get all signed app tokens from the request. This includes the query
	// parameter and all matching cookies sent with the request. If there are
	// somehow multiple signed app token cookies, we want to try all of them
	// (up to 4). The first one that is valid is used.
	//
	// Browsers will send all cookies in the request, even if there are multiple
	// with the same name on different paths.
	//
	// If using a query parameter the request MUST be a terminal request. We use
	// this to support cross-domain terminal access for the web terminal.
	var (
		tokens        = []string{}
		hasQueryParam = false
	)
	if q := r.URL.Query().Get(codersdk.SignedAppTokenQueryParameter); q != "" {
		hasQueryParam = true
		tokens = append(tokens, q)
	}
	for _, cookie := range r.Cookies() {
		if cookie.Name == codersdk.SignedAppTokenCookie {
			tokens = append(tokens, cookie.Value)
		}
	}

	if len(tokens) > 4 {
		tokens = tokens[:4]
	}

	for _, tokenStr := range tokens {
		token, err := key.VerifySignedToken(tokenStr)
		if err == nil {
			req := token.Request.Normalize()
			if hasQueryParam && req.AccessMethod != AccessMethodTerminal {
				// The request must be a terminal request if we're using a
				// query parameter.
				return nil, false
			}

			err := req.Validate()
			if err == nil {
				// The request has a valid signed app token, which is a valid
				// token signed by us. The caller must check that it matches
				// the request.
				return &token, true
			}
		}
	}

	return nil, false
}

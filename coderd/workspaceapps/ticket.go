package workspaceapps

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"gopkg.in/square/go-jose.v2"
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

// GenerateToken generates a signed workspace app token with the given key and
// payload. If the payload doesn't have an expiry, it will be set to the current
// time plus the default expiry.
func GenerateToken(key []byte, payload SignedToken) (string, error) {
	if payload.Expiry.IsZero() {
		payload.Expiry = time.Now().Add(DefaultTokenExpiry)
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", xerrors.Errorf("marshal payload to JSON: %w", err)
	}

	signer, err := jose.NewSigner(jose.SigningKey{
		Algorithm: tokenSigningAlgorithm,
		Key:       key,
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

// ParseToken parses a signed workspace app token with the given key and returns
// the payload. If the token is invalid or expired, an error is returned.
func ParseToken(key []byte, str string) (SignedToken, error) {
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

	output, err := object.Verify(key)
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

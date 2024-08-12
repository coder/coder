package tailnet

import (
	"encoding/json"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/coder/coder/v2/tailnet/proto"
)

const (
	DefaultResumeTokenExpiry = 24 * time.Hour

	resumeTokenSigningAlgorithm = jose.HS512
)

var InsecureTestResumeTokenProvider ResumeTokenProvider = ResumeTokenKeyProvider{
	key:    [64]byte{1},
	expiry: time.Hour,
}

type ResumeTokenProvider interface {
	GenerateResumeToken(peerID uuid.UUID) (*proto.RefreshResumeTokenResponse, error)
	ParseResumeToken(token string) (uuid.UUID, error)
}

type ResumeTokenKeyProvider struct {
	key    [64]byte
	expiry time.Duration
}

func NewResumeTokenKeyProvider(key [64]byte, expiry time.Duration) ResumeTokenProvider {
	if expiry <= 0 {
		expiry = DefaultResumeTokenExpiry
	}
	return ResumeTokenKeyProvider{
		key:    key,
		expiry: DefaultResumeTokenExpiry,
	}
}

type resumeTokenPayload struct {
	PeerID uuid.UUID `json:"peer_id"`
	Expiry time.Time `json:"expiry"`
}

func (p ResumeTokenKeyProvider) GenerateResumeToken(peerID uuid.UUID) (*proto.RefreshResumeTokenResponse, error) {
	payload := resumeTokenPayload{
		PeerID: peerID,
		Expiry: time.Now().Add(p.expiry),
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, xerrors.Errorf("marshal payload to JSON: %w", err)
	}

	signer, err := jose.NewSigner(jose.SigningKey{
		Algorithm: resumeTokenSigningAlgorithm,
		Key:       p.key[:],
	}, nil)
	if err != nil {
		return nil, xerrors.Errorf("create signer: %w", err)
	}

	signedObject, err := signer.Sign(payloadBytes)
	if err != nil {
		return nil, xerrors.Errorf("sign payload: %w", err)
	}

	serialized, err := signedObject.CompactSerialize()
	if err != nil {
		return nil, xerrors.Errorf("serialize JWS: %w", err)
	}

	return &proto.RefreshResumeTokenResponse{
		Token:     serialized,
		RefreshIn: durationpb.New(p.expiry / 2),
		ExpiresAt: timestamppb.New(payload.Expiry),
	}, nil
}

// VerifySignedToken parses a signed workspace app token with the given key and
// returns the payload. If the token is invalid or expired, an error is
// returned.
func (p ResumeTokenKeyProvider) ParseResumeToken(str string) (uuid.UUID, error) {
	object, err := jose.ParseSigned(str)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("parse JWS: %w", err)
	}
	if len(object.Signatures) != 1 {
		return uuid.Nil, xerrors.New("expected 1 signature")
	}
	if object.Signatures[0].Header.Algorithm != string(resumeTokenSigningAlgorithm) {
		return uuid.Nil, xerrors.Errorf("expected token signing algorithm to be %q, got %q", resumeTokenSigningAlgorithm, object.Signatures[0].Header.Algorithm)
	}

	output, err := object.Verify(p.key[:])
	if err != nil {
		return uuid.Nil, xerrors.Errorf("verify JWS: %w", err)
	}

	var tok resumeTokenPayload
	err = json.Unmarshal(output, &tok)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("unmarshal payload: %w", err)
	}
	if tok.Expiry.Before(time.Now()) {
		return uuid.Nil, xerrors.New("signed app token expired")
	}

	return tok.PeerID, nil
}

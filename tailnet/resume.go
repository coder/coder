package tailnet
import (
	"fmt"
	"errors"
	"context"
	"crypto/rand"
	"time"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/quartz"
)
const (
	DefaultResumeTokenExpiry = 24 * time.Hour
)
// NewInsecureTestResumeTokenProvider returns a ResumeTokenProvider that uses a
// random key with short expiry for testing purposes. If any errors occur while
// generating the key, the function panics.
func NewInsecureTestResumeTokenProvider() ResumeTokenProvider {
	key, err := GenerateResumeTokenSigningKey()
	if err != nil {
		panic(err)
	}
	return NewResumeTokenKeyProvider(jwtutils.StaticKey{
		ID:  uuid.New().String(),
		Key: key[:],
	}, quartz.NewReal(), time.Hour)
}
type ResumeTokenProvider interface {
	GenerateResumeToken(ctx context.Context, peerID uuid.UUID) (*proto.RefreshResumeTokenResponse, error)
	VerifyResumeToken(ctx context.Context, token string) (uuid.UUID, error)
}
type ResumeTokenSigningKey [64]byte
func GenerateResumeTokenSigningKey() (ResumeTokenSigningKey, error) {
	var key ResumeTokenSigningKey
	_, err := rand.Read(key[:])
	if err != nil {
		return key, fmt.Errorf("generate random key: %w", err)
	}
	return key, nil
}
type ResumeTokenKeyProvider struct {
	key    jwtutils.SigningKeyManager
	clock  quartz.Clock
	expiry time.Duration
}
func NewResumeTokenKeyProvider(key jwtutils.SigningKeyManager, clock quartz.Clock, expiry time.Duration) ResumeTokenProvider {
	if expiry <= 0 {
		expiry = DefaultResumeTokenExpiry
	}
	return ResumeTokenKeyProvider{
		key:    key,
		clock:  clock,
		expiry: expiry,
	}
}
func (p ResumeTokenKeyProvider) GenerateResumeToken(ctx context.Context, peerID uuid.UUID) (*proto.RefreshResumeTokenResponse, error) {
	exp := p.clock.Now().Add(p.expiry)
	payload := jwtutils.RegisteredClaims{
		Subject: peerID.String(),
		Expiry:  jwt.NewNumericDate(exp),
	}
	token, err := jwtutils.Sign(ctx, p.key, payload)
	if err != nil {
		return nil, fmt.Errorf("sign payload: %w", err)
	}
	return &proto.RefreshResumeTokenResponse{
		Token:     token,
		RefreshIn: durationpb.New(p.expiry / 2),
		ExpiresAt: timestamppb.New(exp),
	}, nil
}
// VerifyResumeToken parses a signed tailnet resume token with the given key and
// returns the payload. If the token is invalid or expired, an error is
// returned.
func (p ResumeTokenKeyProvider) VerifyResumeToken(ctx context.Context, str string) (uuid.UUID, error) {
	var tok jwt.Claims
	err := jwtutils.Verify(ctx, p.key, str, &tok, jwtutils.WithVerifyExpected(jwt.Expected{
		Time: p.clock.Now(),
	}))
	if err != nil {
		return uuid.Nil, fmt.Errorf("verify payload: %w", err)
	}
	parsed, err := uuid.Parse(tok.Subject)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse peerID from token: %w", err)
	}
	return parsed, nil
}

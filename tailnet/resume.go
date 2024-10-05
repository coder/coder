package tailnet

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/tailnet/proto"
	"github.com/coder/quartz"
)

const (
	DefaultResumeTokenExpiry = 24 * time.Hour

	resumeTokenSigningAlgorithm = jose.HS512
)

// resumeTokenSigningKeyID is a fixed key ID for the resume token signing key.
// If/when we add support for multiple keys (e.g. key rotation), this will move
// to the database instead.
var resumeTokenSigningKeyID = uuid.MustParse("97166747-9309-4d7f-9071-a230e257c2a4")

// NewInsecureTestResumeTokenProvider returns a ResumeTokenProvider that uses a
// random key with short expiry for testing purposes. If any errors occur while
// generating the key, the function panics.
func NewInsecureTestResumeTokenProvider() ResumeTokenProvider {
	key, err := GenerateResumeTokenSigningKey()
	if err != nil {
		panic(err)
	}
	return NewResumeTokenKeyProvider(jwtutils.StaticKeyManager{
		ID:  uuid.New().String(),
		Key: key,
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
		return key, xerrors.Errorf("generate random key: %w", err)
	}
	return key, nil
}

type ResumeTokenSigningKeyDatabaseStore interface {
	GetCoordinatorResumeTokenSigningKey(ctx context.Context) (string, error)
	UpsertCoordinatorResumeTokenSigningKey(ctx context.Context, key string) error
}

// ResumeTokenSigningKeyFromDatabase retrieves the coordinator resume token
// signing key from the database. If the key is not found, a new key is
// generated and inserted into the database.
func ResumeTokenSigningKeyFromDatabase(ctx context.Context, db ResumeTokenSigningKeyDatabaseStore) (ResumeTokenSigningKey, error) {
	var resumeTokenKey ResumeTokenSigningKey
	resumeTokenKeyStr, err := db.GetCoordinatorResumeTokenSigningKey(ctx)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return resumeTokenKey, xerrors.Errorf("get coordinator resume token key: %w", err)
	}
	if decoded, err := hex.DecodeString(resumeTokenKeyStr); err != nil || len(decoded) != len(resumeTokenKey) {
		newKey, err := GenerateResumeTokenSigningKey()
		if err != nil {
			return resumeTokenKey, xerrors.Errorf("generate fresh coordinator resume token key: %w", err)
		}

		resumeTokenKeyStr = hex.EncodeToString(newKey[:])
		err = db.UpsertCoordinatorResumeTokenSigningKey(ctx, resumeTokenKeyStr)
		if err != nil {
			return resumeTokenKey, xerrors.Errorf("insert freshly generated coordinator resume token key to database: %w", err)
		}
	}

	resumeTokenKeyBytes, err := hex.DecodeString(resumeTokenKeyStr)
	if err != nil {
		return resumeTokenKey, xerrors.Errorf("decode coordinator resume token key from database: %w", err)
	}
	if len(resumeTokenKeyBytes) != len(resumeTokenKey) {
		return resumeTokenKey, xerrors.Errorf("coordinator resume token key in database is not the correct length, expect %d got %d", len(resumeTokenKey), len(resumeTokenKeyBytes))
	}
	copy(resumeTokenKey[:], resumeTokenKeyBytes)
	if resumeTokenKey == [64]byte{} {
		return resumeTokenKey, xerrors.Errorf("coordinator resume token key in database is empty")
	}
	return resumeTokenKey, nil
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
		expiry: DefaultResumeTokenExpiry,
	}
}

type resumeTokenPayload struct {
	jwt.Claims
	PeerID uuid.UUID `json:"sub"`
	Expiry int64     `json:"exp"`
}

func (p ResumeTokenKeyProvider) GenerateResumeToken(ctx context.Context, peerID uuid.UUID) (*proto.RefreshResumeTokenResponse, error) {
	exp := p.clock.Now().Add(p.expiry)
	payload := resumeTokenPayload{
		PeerID: peerID,
		Claims: jwt.Claims{
			Expiry: jwt.NewNumericDate(exp),
		},
	}

	token, err := jwtutils.Sign(ctx, p.key, payload)
	if err != nil {
		return nil, xerrors.Errorf("sign payload: %w", err)
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
	var tok resumeTokenPayload
	err := jwtutils.Verify(ctx, p.key, str, &tok, jwtutils.WithVerifyExpected(jwt.Expected{
		Time: p.clock.Now(),
	}))
	if err != nil {
		return uuid.Nil, xerrors.Errorf("verify payload: %w", err)
	}
	return tok.PeerID, nil
}

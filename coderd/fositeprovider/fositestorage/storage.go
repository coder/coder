package fositestorage

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/ory/fosite"
	"github.com/ory/fosite/handler/oauth2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

type fositeStorage interface {
	fosite.ClientManager
	oauth2.CoreStorage
	// TODO: Add support for database transactions.
	//storage.Transactional
}

var _ fositeStorage = (*Storage)(nil)

type Storage struct {
	db database.Store

	// TODO: Remove the memory store entirely and implement all methods to use the database.
	//storage.MemoryStore
}

func New(db database.Store) *Storage {
	return &Storage{
		db: db,
		//MemoryStore: *storage.NewMemoryStore(),
	}
}

func (s Storage) CreateAuthorizeCodeSession(ctx context.Context, code string, request fosite.Requester) error {
	_, err := s.db.InsertOAuth2ProviderAppCode(ctx, database.InsertOAuth2ProviderAppCodeParams{
		ID:        uuid.New(),
		CreatedAt: dbtime.Now(),
		// TODO: Configurable expiration?  Ten minutes matches GitHub.
		// This timeout is only for the code that will be exchanged for the
		// access token, not the access token itself.  It does not need to be
		// long-lived because normally it will be exchanged immediately after it
		// is received.  If the application does wait before exchanging the
		// token (for example suppose they ask the user to confirm and the user
		// has left) then they can just retry immediately and get a new code.
		ExpiresAt:           dbtime.Now().Add(time.Duration(10) * time.Minute),
		SecretPrefix:        nil,
		HashedSecret:        nil,
		AppID:               uuid.UUID{},
		UserID:              uuid.UUID{},
		ResourceUri:         sql.NullString{},
		CodeChallenge:       sql.NullString{},
		CodeChallengeMethod: sql.NullString{},
	})
	if err != nil {
		return xerrors.Errorf("insert oauth code: %w", err)
	}
	return nil
}

func (s Storage) GetAuthorizeCodeSession(ctx context.Context, code string, session fosite.Session) (request fosite.Requester, err error) {
	//TODO implement me
	panic("implement me")
}

func (s Storage) InvalidateAuthorizeCodeSession(ctx context.Context, code string) (err error) {
	//TODO implement me
	panic("implement me")
}

func (s Storage) CreateAccessTokenSession(ctx context.Context, signature string, request fosite.Requester) (err error) {
	//TODO implement me
	panic("implement me")
}

func (s Storage) GetAccessTokenSession(ctx context.Context, signature string, session fosite.Session) (request fosite.Requester, err error) {
	//TODO implement me
	panic("implement me")
}

func (s Storage) DeleteAccessTokenSession(ctx context.Context, signature string) (err error) {
	//TODO implement me
	panic("implement me")
}

func (s Storage) CreateRefreshTokenSession(ctx context.Context, signature string, accessSignature string, request fosite.Requester) (err error) {
	//TODO implement me
	panic("implement me")
}

func (s Storage) GetRefreshTokenSession(ctx context.Context, signature string, session fosite.Session) (request fosite.Requester, err error) {
	//TODO implement me
	panic("implement me")
}

func (s Storage) DeleteRefreshTokenSession(ctx context.Context, signature string) (err error) {
	//TODO implement me
	panic("implement me")
}

func (s Storage) RotateRefreshToken(ctx context.Context, requestID string, refreshTokenSignature string) (err error) {
	//TODO implement me
	panic("implement me")
}

func (s Storage) GetClient(ctx context.Context, id string) (fosite.Client, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fosite.ErrNotFound
	}

	app, err := s.db.GetOAuth2ProviderAppByID(dbauthz.AsSystemRestricted(ctx), uid)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return nil, fosite.ErrNotFound
		}
		return nil, fosite.ErrorToRFC6749Error(err)
	}

	secrets, err := s.db.GetOAuth2ProviderAppSecretsByAppID(dbauthz.AsSystemRestricted(ctx), app.ID)
	if err != nil {
		return nil, fosite.ErrorToRFC6749Error(err)
	}

	return Client{
		App:     app,
		Secrets: secrets,
	}, nil
}

func (s Storage) ClientAssertionJWTValid(ctx context.Context, jti string) error {
	//TODO implement me
	panic("implement me")
}

func (s Storage) SetClientAssertionJWT(ctx context.Context, jti string, exp time.Time) error {
	//TODO implement me
	panic("implement me")
}

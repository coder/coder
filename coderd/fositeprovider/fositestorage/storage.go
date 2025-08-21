package fositestorage

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/ory/fosite"
	"github.com/ory/fosite/handler/oauth2"
	"github.com/ory/fosite/storage"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

type fositeStorage interface {
	fosite.ClientManager
	oauth2.CoreStorage
}

var _ fositeStorage = (*Storage)(nil)

type Storage struct {
	db database.Store

	// TODO: Remove the memory store entirely and implement all methods to use the database.
	storage.MemoryStore
}

func New(db database.Store) *Storage {
	return &Storage{
		db:          db,
		MemoryStore: *storage.NewMemoryStore(),
	}
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

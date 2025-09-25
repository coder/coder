package fositestorage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/ory/fosite"
	"github.com/ory/fosite/handler/oauth2"
	"github.com/ory/fosite/handler/pkce"
	"github.com/ory/fosite/handler/rfc8628"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/cryptorand"
)

type fositeStorage interface {
	fosite.ClientManager
	oauth2.CoreStorage
	pkce.PKCERequestStorage
	rfc8628.RFC8628CoreStorage
	// TODO: Add support for database transactions.
	//storage.Transactional

	// Copied from Ory Keto
	// https://github.com/ory/hydra/blob/8e3a7b82e1aa54e2f2e9cefd5f9cb26ea7421e56/x/fosite_storer.go#L49-L49
	UpdateDeviceCodeSessionBySignature(ctx context.Context, requestID string, requester fosite.DeviceRequester) error
}

var _ fositeStorage = (*Storage)(nil)

type Storage struct {
	db          database.Store
	sessionByID map[uuid.UUID]*CoderSession

	// TODO: Remove the memory store entirely and implement all methods to use the database.
	//storage.MemoryStore
}

func (s Storage) UpdateDeviceCodeSessionBySignature(ctx context.Context, requestID string, requester fosite.DeviceRequester) error {
	//TODO implement me
	panic("implement me")
}

func (s Storage) CreateDeviceAuthSession(ctx context.Context, deviceCodeSignature, userCodeSignature string, request fosite.DeviceRequester) (err error) {
	//TODO implement me
	panic("implement me")
}

func (s Storage) GetDeviceCodeSession(ctx context.Context, signature string, session fosite.Session) (request fosite.DeviceRequester, err error) {
	//TODO implement me
	panic("implement me")
}

func (s Storage) InvalidateDeviceCodeSession(ctx context.Context, signature string) (err error) {
	//TODO implement me
	panic("implement me")
}

func (s Storage) GetPKCERequestSession(ctx context.Context, signature string, session fosite.Session) (fosite.Requester, error) {
	//TODO implement me
	panic("implement me")
}

func (s Storage) CreatePKCERequestSession(ctx context.Context, signature string, requester fosite.Requester) error {
	//TODO implement me
	panic("implement me")
}

func (s Storage) DeletePKCERequestSession(ctx context.Context, signature string) error {
	//TODO implement me
	panic("implement me")
}

func New(db database.Store) *Storage {
	return &Storage{
		db: db,
		// TODO: Persist to the database.
		sessionByID: map[uuid.UUID]*CoderSession{},
		//MemoryStore: *storage.NewMemoryStore(),
	}
}

func (s Storage) CreateAuthorizeCodeSession(ctx context.Context, code string, request fosite.Requester) error {
	client := request.GetClient()
	clientID, err := uuid.Parse(client.GetID())
	if err != nil {
		return xerrors.Errorf("parse client id: %w", err)
	}

	session, ok := request.GetSession().(*CoderSession)
	if !ok {
		return xerrors.Errorf("expected *CoderSession, got %T", request.GetSession())
	}

	s.sessionByID[session.ID] = session

	jsonForm, err := json.Marshal(request.GetRequestForm())
	if err != nil {
		return xerrors.Errorf("marshal form: %w", err)
	}

	_, err = s.db.InsertOAuth2ProviderAppCode(ctx, database.InsertOAuth2ProviderAppCodeParams{
		AppID:     clientID,
		ID:        uuid.New(),
		CreatedAt: request.GetRequestedAt(),
		// TODO: Is this expires correct?
		ExpiresAt:         session.GetExpiresAt(fosite.AuthorizeCode),
		UserID:            session.UserID,
		SessionID:         session.ID,
		Code:              code,
		RequestedScopes:   request.GetRequestedScopes(),
		GrantedScopes:     request.GetGrantedScopes(),
		RequestedAudience: request.GetRequestedAudience(),
		GrantedAudience:   request.GetGrantedAudience(),
		Form:              jsonForm,

		// Not used at the moment, or maybe ever again?
		SecretPrefix:        nil,
		HashedSecret:        nil,
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
	result, err := s.db.GetOAuth2ProviderAppCodeByCode(ctx, code)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return nil, fosite.ErrNotFound
		}
		return nil, xerrors.Errorf("get oauth code by code: %w", err)
	}

	cli, err := s.GetClient(ctx, result.AppID.String())
	if err != nil {
		return nil, xerrors.Errorf("get client: %w", err)
	}

	var form url.Values
	if err := json.Unmarshal(result.Form, &form); err != nil {
		return nil, xerrors.Errorf("unmarshal form: %w", err)
	}

	session, ok := s.sessionByID[result.SessionID]
	if !ok {
		return nil, xerrors.Errorf("session not found")
	}

	rq := &fosite.Request{
		ID:                result.ID.String(),
		RequestedAt:       result.CreatedAt,
		Client:            cli,
		RequestedScope:    result.RequestedScopes,
		GrantedScope:      result.GrantedScopes,
		Form:              form,
		Session:           session,
		RequestedAudience: result.RequestedAudience,
		GrantedAudience:   result.GrantedAudience,
	}

	if !result.Active {
		return rq, fosite.ErrInvalidatedAuthorizeCode
	}

	return rq, nil
}

func (s Storage) InvalidateAuthorizeCodeSession(ctx context.Context, code string) (err error) {
	return s.db.InvalidateOAuth2ProviderAppCodeByCode(ctx, code)
}

func (s Storage) CreateAccessTokenSession(ctx context.Context, signature string, request fosite.Requester) (err error) {
	now := dbtime.Now()
	client := request.GetClient().(*Client)
	session := request.GetSession().(*CoderSession)

	exp := session.ExpiresAt[fosite.AccessToken]

	tokenName := fmt.Sprintf("%s_%s_oauth_session_token", session.UserID, client.GetID())
	//key, _, err := apikey.Generate(apikey.CreateParams{
	//	UserID:          session.UserID,
	//	LoginType:       database.LoginTypeOAuth2ProviderApp,
	//	DefaultLifetime: time.Hour * 7, // TODO: Pass in this info. Do we still need this field?
	//	ExpiresAt:       exp,
	//	LifetimeSeconds: 0,
	//	// TODO: Add scopes
	//	Scope: database.APIKeyScopeAll,
	//	// For now, we only allow one active token per user+app.
	//	// TODO: This should be fixed to allow multiple tokens.
	//	TokenName: tokenName,
	//})

	// TODO: This is unfortunate. The
	//id, signature := (&TokenStrategy{}).splitPrefix(signature)
	//key.HashedSecret = []byte(signature)
	//key.ID = id

	// Grab the user roles so we can perform the exchange as the user.
	actor, _, err := httpmw.UserRBACSubject(ctx, s.db, session.UserID, rbac.ScopeAll)
	if err != nil {
		return xerrors.Errorf("fetch user actor: %w", err)
	}

	err = s.db.InTx(func(tx database.Store) error {
		ctx := dbauthz.As(ctx, actor)

		// Delete the previous key, if any.
		prevKey, err := tx.GetAPIKeyByName(ctx, database.GetAPIKeyByNameParams{
			UserID:    session.UserID,
			TokenName: tokenName,
		})
		if err == nil {
			err = tx.DeleteAPIKeyByID(ctx, prevKey.ID)
		} else if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("delete api key by name: %w", err)
		}

		id, _ := cryptorand.String(10)
		newKey, err := tx.InsertAPIKey(ctx, database.InsertAPIKeyParams{
			// TODO: Does the ID really matter now?
			// Just used to link to the oauth2 token.
			ID:              id,
			LifetimeSeconds: 0,
			// HashedSecret is an hmac and going to be the lookup value for the access token.
			HashedSecret: []byte(signature),
			IPAddress:    pqtype.Inet{},
			UserID:       session.UserID,
			LastUsed:     time.Time{},
			ExpiresAt:    exp,
			CreatedAt:    now,
			UpdatedAt:    now,
			LoginType:    database.LoginTypeOAuth2ProviderApp,
			// TODO: Use scopes from the request.
			Scopes: []database.APIKeyScope{database.APIKeyScopeAll},
			// TODO: Use a proper allow list from request
			AllowList: database.AllowList{database.AllowListWildcard()},
			TokenName: tokenName,
		})
		if err != nil {
			return xerrors.Errorf("insert oauth2 access token: %w", err)
		}

		// TODO: Is this audience correct?
		aud := ""
		if len(request.GetGrantedAudience()) > 0 {
			aud = request.GetGrantedAudience()[0]
		}

		_, err = tx.InsertOAuth2ProviderAppToken(ctx, database.InsertOAuth2ProviderAppTokenParams{
			Sessionid: session.ID,
			ID:        uuid.New(),
			CreatedAt: dbtime.Now(),
			ExpiresAt: exp,
			//HashPrefix:  []byte(refreshToken.Prefix),
			//RefreshHash: []byte(refreshToken.Hashed),
			AppSecretID: client.Secrets[0].ID, // Why do we have more than 1 secret?
			APIKeyID:    newKey.ID,
			UserID:      session.UserID,
			Audience: sql.NullString{
				String: aud,
				Valid:  aud != "",
			},
		})
		if err != nil {
			return xerrors.Errorf("insert oauth2 refresh token: %w", err)
		}
		return nil
	}, nil)

	return nil
}

func (s Storage) GetAccessTokenSession(ctx context.Context, signature string, accessRequest fosite.Session) (request fosite.Requester, err error) {
	key, err := s.db.GetAPIKeyBySignature(ctx, []byte(signature))
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return nil, fosite.ErrNotFound
		}
		return nil, xerrors.Errorf("get api key by signature: %w", err)
	}

	token, err := s.db.GetOAuth2ProviderAppTokenByAPIKeyID(ctx, key.ID)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return nil, fosite.ErrNotFound
		}
		return nil, xerrors.Errorf("get api key by signature: %w", err)
	}

	secret, err := s.db.GetOAuth2ProviderAppSecretByID(ctx, token.AppSecretID)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return nil, fosite.ErrNotFound
		}
		return nil, xerrors.Errorf("get api key by signature: %w", err)
	}

	client, err := s.GetClient(ctx, secret.AppID.String())
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return nil, fosite.ErrNotFound
		}
		return nil, xerrors.Errorf("get api key by signature: %w", err)
	}

	session := s.sessionByID[token.Sessionid]
	return &fosite.Request{
		ID:          token.ID.String(), // TODO: Is this the right ID?
		RequestedAt: token.CreatedAt,
		Client:      client,
		Form:        nil, // TODO
		Session:     session,
		// TODO: Fix all this.
		RequestedAudience: []string{token.Audience.String},
		GrantedAudience:   []string{token.Audience.String},
		RequestedScope:    slice.ToStrings(key.Scopes),
		GrantedScope:      slice.ToStrings(key.Scopes),
	}, nil
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

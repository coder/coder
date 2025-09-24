package fositestorage

import (
	"context"
	"database/sql"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/ory/fosite"
	"github.com/ory/fosite/handler/oauth2"
	"github.com/ory/fosite/handler/openid"
	"github.com/ory/fosite/handler/pkce"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/userpassword"
)

type fositeStorage interface {
	fosite.ClientManager
	oauth2.CoreStorage
	pkce.PKCERequestStorage

	// TODO: Add support for database transactions.
	//storage.Transactional
}

var _ fositeStorage = (*Storage)(nil)

type Storage struct {
	db database.Store

	// TODO: Remove the memory store entirely and implement all methods to use the database.
	//storage.MemoryStore
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
		//MemoryStore: *storage.NewMemoryStore(),
	}
}

func codePrefix(code string) []byte {
	const n = 16
	if len(code) <= n {
		return []byte(code)
	}
	return []byte(code[:n])
}

func (s Storage) CreateAuthorizeCodeSession(ctx context.Context, code string, request fosite.Requester) error {
	client := request.GetClient()
	appID, err := uuid.Parse(client.GetID())
	if err != nil {
		return fosite.ErrorToRFC6749Error(xerrors.Errorf("parse client id: %w", err))
	}

	var userID uuid.UUID
	if request.GetSession() != nil {
		if sess, ok := request.GetSession().(*openid.DefaultSession); ok && sess != nil && sess.Claims != nil {
			if sess.Claims.Subject != "" {
				if uid, e := uuid.Parse(sess.Claims.Subject); e == nil {
					userID = uid
				}
			}
		}
	}

	h, err := userpassword.Hash(code)
	if err != nil {
		return xerrors.Errorf("hash code: %w", err)
	}

	form := request.GetRequestForm()
	var resource sql.NullString
	if v := form.Get("resource"); v != "" {
		resource = sql.NullString{String: v, Valid: true}
	}
	var cc sql.NullString
	if v := form.Get("code_challenge"); v != "" {
		cc = sql.NullString{String: v, Valid: true}
	}
	var ccm sql.NullString
	if v := form.Get("code_challenge_method"); v != "" {
		ccm = sql.NullString{String: v, Valid: true}
	}

	_, err = s.db.InsertOAuth2ProviderAppCode(ctx, database.InsertOAuth2ProviderAppCodeParams{
		ID:        uuid.New(),
		CreatedAt: dbtime.Now(),
		// TODO: Configurable expiration; default to 10 minutes similar to GitHub
		ExpiresAt:           dbtime.Now().Add(10 * time.Minute),
		SecretPrefix:        codePrefix(code),
		HashedSecret:        []byte(h),
		AppID:               appID,
		UserID:              userID,
		ResourceUri:         resource,
		CodeChallenge:       cc,
		CodeChallengeMethod: ccm,
	})
	if err != nil {
		return xerrors.Errorf("insert oauth code: %w", err)
	}
	return nil
}

func (s Storage) GetAuthorizeCodeSession(ctx context.Context, code string, session fosite.Session) (request fosite.Requester, err error) {
	rec, err := s.db.GetOAuth2ProviderAppCodeByPrefix(dbauthz.AsSystemRestricted(ctx), codePrefix(code))
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return nil, fosite.ErrNotFound
		}
		return nil, fosite.ErrorToRFC6749Error(err)
	}

	if time.Now().After(rec.ExpiresAt) {
		return nil, fosite.ErrNotFound
	}

	ok, err := userpassword.Compare(string(rec.HashedSecret), code)
	if err != nil {
		return nil, fosite.ErrorToRFC6749Error(err)
	}
	if !ok {
		return nil, fosite.ErrNotFound
	}

	cli, err := s.GetClient(dbauthz.AsSystemRestricted(ctx), rec.AppID.String())
	if err != nil {
		return nil, fosite.ErrorToRFC6749Error(err)
	}

	// Populate subject into the provided session if it matches our session type.
	if session != nil {
		if sess, ok := session.(*openid.DefaultSession); ok && sess != nil && sess.Claims != nil {
			sess.Claims.Subject = rec.UserID.String()
		}
	}

	r := fosite.NewRequest()
	r.Client = cli
	r.Session = session
	r.RequestedAt = rec.CreatedAt
	// Minimal form data required for PKCE validation.
	if rec.CodeChallenge.Valid || rec.CodeChallengeMethod.Valid {
		fv := url.Values{}
		if rec.CodeChallenge.Valid {
			fv.Set("code_challenge", rec.CodeChallenge.String)
		}
		if rec.CodeChallengeMethod.Valid {
			fv.Set("code_challenge_method", rec.CodeChallengeMethod.String)
		}
		r.Form = fv
	}

	return r, nil
}

func (s Storage) InvalidateAuthorizeCodeSession(ctx context.Context, code string) (err error) {
	rec, err := s.db.GetOAuth2ProviderAppCodeByPrefix(dbauthz.AsSystemRestricted(ctx), codePrefix(code))
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			return fosite.ErrNotFound
		}
		return fosite.ErrorToRFC6749Error(err)
	}
	if derr := s.db.DeleteOAuth2ProviderAppCodeByID(dbauthz.AsSystemRestricted(ctx), rec.ID); derr != nil {
		return fosite.ErrorToRFC6749Error(derr)
	}
	return nil
}

func (s Storage) CreateAccessTokenSession(ctx context.Context, signature string, request fosite.Requester) (err error) {
	// Using JWT access tokens does not require persisting the session. If opaque tokens are enabled,
	// implement persistence here.
	return nil
}

func (s Storage) GetAccessTokenSession(ctx context.Context, signature string, session fosite.Session) (request fosite.Requester, err error) {
	// Not persisted; return not found for opaque token lookups.
	return nil, fosite.ErrNotFound
}

func (s Storage) DeleteAccessTokenSession(ctx context.Context, signature string) (err error) {
	// Nothing to delete for stateless JWT access tokens.
	return nil
}

func (s Storage) CreateRefreshTokenSession(ctx context.Context, signature string, accessSignature string, request fosite.Requester) (err error) {
	// TODO: Implement refresh token persistence when enabling refresh tokens.
	return nil
}

func (s Storage) GetRefreshTokenSession(ctx context.Context, signature string, session fosite.Session) (request fosite.Requester, err error) {
	return nil, fosite.ErrNotFound
}

func (s Storage) DeleteRefreshTokenSession(ctx context.Context, signature string) (err error) {
	return nil
}

func (s Storage) RotateRefreshToken(ctx context.Context, requestID string, refreshTokenSignature string) (err error) {
	return nil
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
	// Not tracked; treat as valid (not used before).
	return nil
}

func (s Storage) SetClientAssertionJWT(ctx context.Context, jti string, exp time.Time) error {
	// Not persisted yet.
	return nil
}

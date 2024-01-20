package identityprovider

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
)

type tokenParams struct {
	clientID     string
	clientSecret string
	code         string
	grantType    codersdk.OAuth2ProviderGrantType
	redirectURL  *url.URL
}

func extractTokenParams(r *http.Request, callbackURL string) (tokenParams, []codersdk.ValidationError, error) {
	p := httpapi.NewQueryParamParser()
	err := r.ParseForm()
	if err != nil {
		return tokenParams{}, nil, xerrors.Errorf("parse form: %w", err)
	}

	// TODO: Can we make this a URL straight out of the database?
	cb, err := url.Parse(callbackURL)
	if err != nil {
		return tokenParams{}, nil, err
	}

	p.Required("grant_type", "client_secret", "client_id", "code")

	vals := r.Form
	params := tokenParams{
		clientID:     p.String(vals, "", "client_id"),
		clientSecret: p.String(vals, "", "client_secret"),
		code:         p.String(vals, "", "code"),
		redirectURL:  p.URL(vals, cb, "redirect_uri"),
		grantType:    httpapi.ParseCustom(p, vals, "", "grant_type", httpapi.ParseEnum[codersdk.OAuth2ProviderGrantType]),
	}

	p.ErrorExcessParams(vals)
	return params, p.Errors, nil
}

func Tokens(db database.Store, defaultLifetime time.Duration) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		app := httpmw.OAuth2ProviderApp(r)

		params, validationErrs, err := extractTokenParams(r, app.CallbackURL)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Failed to validate form values.",
				Detail:  err.Error(),
			})
			return
		}
		if len(validationErrs) > 0 {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message:     "Invalid query params.",
				Validations: validationErrs,
			})
			return
		}

		var token oauth2.Token
		//nolint:gocritic,revive // More cases will be added later.
		switch params.grantType {
		// TODO: Client creds, device code, refresh.
		default:
			token, err = authorizationCodeGrant(ctx, db, app, defaultLifetime, params.clientSecret, params.code)
		}

		if err != nil && errors.Is(err, sql.ErrNoRows) {
			httpapi.Write(r.Context(), rw, http.StatusUnauthorized, codersdk.Response{
				Message: "Invalid client secret or code",
			})
			return
		}
		if err != nil {
			httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to exchange token",
				Detail:  err.Error(),
			})
			return
		}

		// Some client libraries allow this to be "application/x-www-form-urlencoded". We can implement that upon
		// request. The same libraries should also accept JSON. If implemented, choose based on "Accept" header.
		httpapi.Write(ctx, rw, http.StatusOK, token)
	}
}

func authorizationCodeGrant(ctx context.Context, db database.Store, app database.OAuth2ProviderApp, defaultLifetime time.Duration, clientSecret, code string) (oauth2.Token, error) {
	// Validate the client secret.
	secretHash := Hash(clientSecret, app.ID)
	secret, err := db.GetOAuth2ProviderAppSecretByAppIDAndSecret(
		//nolint:gocritic // Users cannot read secrets so we must use the system.
		dbauthz.AsSystemRestricted(ctx),
		database.GetOAuth2ProviderAppSecretByAppIDAndSecretParams{
			AppID:        app.ID,
			HashedSecret: secretHash[:],
		})
	if err != nil {
		return oauth2.Token{}, err
	}

	// Validate the authorization code.
	codeHash := Hash(code, app.ID)
	if err != nil {
		return oauth2.Token{}, err
	}
	dbCode, err := db.GetOAuth2ProviderAppCodeByAppIDAndSecret(
		//nolint:gocritic // There is no user yet so we must use the system.
		dbauthz.AsSystemRestricted(ctx),
		database.GetOAuth2ProviderAppCodeByAppIDAndSecretParams{
			AppID:        app.ID,
			HashedSecret: codeHash[:],
		})
	if err != nil {
		return oauth2.Token{}, err
	}

	// Ensure the code has not expired.  Make it look like no code.
	if dbCode.ExpiresAt.Before(dbtime.Now()) {
		return oauth2.Token{}, sql.ErrNoRows
	}

	// Generate a refresh token.
	// The refresh token is not currently used or exposed though as API keys can
	// already be refreshed by just using them.
	// TODO: However, should we implement the refresh grant anyway?
	// 40 characters matches the length of GitHub's client secrets.
	rawRefreshToken, err := cryptorand.String(40)
	if err != nil {
		return oauth2.Token{}, err
	}

	// Generate the API key we will swap for the code.
	// TODO: We are ignoring scopes for now.
	tokenName := fmt.Sprintf("%s_%s_oauth_session_token", dbCode.UserID, app.ID)
	key, sessionToken, err := apikey.Generate(apikey.CreateParams{
		UserID:    dbCode.UserID,
		LoginType: database.LoginTypeOAuth2ProviderApp,
		// TODO: This is just the lifetime for api keys, maybe have its own config
		//       settings. #11693
		DefaultLifetime: defaultLifetime,
		// For now, we allow only one token per app and user at a time.
		TokenName: tokenName,
	})
	if err != nil {
		return oauth2.Token{}, err
	}

	// Grab the user roles so we can perform the exchange as the user.
	//nolint:gocritic // In the token exchange, there is no user actor.
	roles, err := db.GetAuthorizationUserRoles(dbauthz.AsSystemRestricted(ctx), dbCode.UserID)
	if err != nil {
		return oauth2.Token{}, err
	}
	userSubj := rbac.Subject{
		ID:     dbCode.UserID.String(),
		Roles:  rbac.RoleNames(roles.Roles),
		Groups: roles.Groups,
		Scope:  rbac.ScopeAll,
	}

	// Do the actual token exchange in the database.
	err = db.InTx(func(tx database.Store) error {
		ctx := dbauthz.As(ctx, userSubj)
		err = tx.DeleteOAuth2ProviderAppCodeByID(ctx, dbCode.ID)
		if err != nil {
			return xerrors.Errorf("delete oauth2 app code: %w", err)
		}

		// Delete the previous key, if any.
		prevKey, err := tx.GetAPIKeyByName(ctx, database.GetAPIKeyByNameParams{
			UserID:    dbCode.UserID,
			TokenName: tokenName,
		})
		if err == nil {
			err = tx.DeleteAPIKeyByID(ctx, prevKey.ID)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("delete api key by name: %w", err)
		}

		newKey, err := tx.InsertAPIKey(ctx, key)
		if err != nil {
			return xerrors.Errorf("insert oauth2 access token: %w", err)
		}

		hashed := Hash(rawRefreshToken, app.ID)
		_, err = tx.InsertOAuth2ProviderAppToken(ctx, database.InsertOAuth2ProviderAppTokenParams{
			ID:          uuid.New(),
			CreatedAt:   dbtime.Now(),
			ExpiresAt:   key.ExpiresAt,
			RefreshHash: hashed[:],
			AppSecretID: secret.ID,
			APIKeyID:    newKey.ID,
		})
		if err != nil {
			return xerrors.Errorf("insert oauth2 refresh token: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		return oauth2.Token{}, err
	}

	return oauth2.Token{
		AccessToken: sessionToken,
		TokenType:   "Bearer",
		// TODO: Exclude until refresh grant is implemented.
		// RefreshToken: rawRefreshToken,
		// Expiry:       key.ExpiresAt,
	}, nil
}

/**
 * Hash uses argon2 to hash the secret using the ID as the salt.
 */
func Hash(secret string, id uuid.UUID) []byte {
	b := []byte(secret)
	// TODO: Expose iterations, memory, and threads as configuration values?
	return argon2.IDKey(b, []byte(id.String()), 1, 64*1024, 2, uint32(len(b)))
}

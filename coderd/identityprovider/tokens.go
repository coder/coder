package identityprovider

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"github.com/google/uuid"

	"golang.org/x/oauth2"
	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"

	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/userpassword"
	"github.com/coder/coder/v2/codersdk"
)
var (
	// errBadSecret means the user provided a bad secret.
	errBadSecret = errors.New("Invalid client secret")
	// errBadCode means the user provided a bad code.

	errBadCode = errors.New("Invalid code")
	// errBadToken means the user provided a bad token.
	errBadToken = errors.New("Invalid token")
)
type tokenParams struct {
	clientID     string
	clientSecret string
	code         string
	grantType    codersdk.OAuth2ProviderGrantType

	redirectURL  *url.URL
	refreshToken string
}
func extractTokenParams(r *http.Request, callbackURL *url.URL) (tokenParams, []codersdk.ValidationError, error) {
	p := httpapi.NewQueryParamParser()
	err := r.ParseForm()
	if err != nil {
		return tokenParams{}, nil, fmt.Errorf("parse form: %w", err)
	}

	vals := r.Form
	p.RequiredNotEmpty("grant_type")
	grantType := httpapi.ParseCustom(p, vals, "", "grant_type", httpapi.ParseEnum[codersdk.OAuth2ProviderGrantType])
	switch grantType {
	case codersdk.OAuth2ProviderGrantTypeRefreshToken:
		p.RequiredNotEmpty("refresh_token")
	case codersdk.OAuth2ProviderGrantTypeAuthorizationCode:

		p.RequiredNotEmpty("client_secret", "client_id", "code")
	}
	params := tokenParams{
		clientID:     p.String(vals, "", "client_id"),
		clientSecret: p.String(vals, "", "client_secret"),
		code:         p.String(vals, "", "code"),
		grantType:    grantType,
		redirectURL:  p.RedirectURL(vals, callbackURL, "redirect_uri"),
		refreshToken: p.String(vals, "", "refresh_token"),
	}

	p.ErrorExcessParams(vals)
	if len(p.Errors) > 0 {
		return tokenParams{}, p.Errors, fmt.Errorf("invalid query params: %w", p.Errors)
	}
	return params, nil, nil
}
// Tokens
// TODO: the sessions lifetime config passed is for coder api tokens.
// Should there be a separate config for oauth2 tokens? They are related,

// but they are not the same.
func Tokens(db database.Store, lifetimes codersdk.SessionLifetime) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		app := httpmw.OAuth2ProviderApp(r)
		callbackURL, err := url.Parse(app.CallbackURL)
		if err != nil {

			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to validate form values.",
				Detail:  err.Error(),
			})
			return
		}
		params, validationErrs, err := extractTokenParams(r, callbackURL)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{

				Message:     "Invalid query params.",
				Detail:      err.Error(),
				Validations: validationErrs,
			})
			return
		}
		var token oauth2.Token
		//nolint:gocritic,revive // More cases will be added later.
		switch params.grantType {

		// TODO: Client creds, device code.
		case codersdk.OAuth2ProviderGrantTypeRefreshToken:
			token, err = refreshTokenGrant(ctx, db, app, lifetimes, params)
		case codersdk.OAuth2ProviderGrantTypeAuthorizationCode:
			token, err = authorizationCodeGrant(ctx, db, app, lifetimes, params)
		default:
			// Grant types are validated by the parser, so getting through here means
			// the developer added a type but forgot to add a case here.
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Unhandled grant type.",

				Detail:  fmt.Sprintf("Grant type %q is unhandled", params.grantType),
			})
			return
		}
		if errors.Is(err, errBadCode) || errors.Is(err, errBadSecret) {
			httpapi.Write(r.Context(), rw, http.StatusUnauthorized, codersdk.Response{
				Message: err.Error(),
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
func authorizationCodeGrant(ctx context.Context, db database.Store, app database.OAuth2ProviderApp, lifetimes codersdk.SessionLifetime, params tokenParams) (oauth2.Token, error) {
	// Validate the client secret.
	secret, err := parseSecret(params.clientSecret)
	if err != nil {
		return oauth2.Token{}, errBadSecret
	}
	//nolint:gocritic // Users cannot read secrets so we must use the system.
	dbSecret, err := db.GetOAuth2ProviderAppSecretByPrefix(dbauthz.AsSystemRestricted(ctx), []byte(secret.prefix))
	if errors.Is(err, sql.ErrNoRows) {
		return oauth2.Token{}, errBadSecret

	}
	if err != nil {
		return oauth2.Token{}, err
	}
	equal, err := userpassword.Compare(string(dbSecret.HashedSecret), secret.secret)
	if err != nil {

		return oauth2.Token{}, fmt.Errorf("unable to compare secret: %w", err)
	}
	if !equal {
		return oauth2.Token{}, errBadSecret
	}
	// Validate the authorization code.
	code, err := parseSecret(params.code)
	if err != nil {
		return oauth2.Token{}, errBadCode
	}
	//nolint:gocritic // There is no user yet so we must use the system.
	dbCode, err := db.GetOAuth2ProviderAppCodeByPrefix(dbauthz.AsSystemRestricted(ctx), []byte(code.prefix))
	if errors.Is(err, sql.ErrNoRows) {
		return oauth2.Token{}, errBadCode
	}
	if err != nil {
		return oauth2.Token{}, err
	}
	equal, err = userpassword.Compare(string(dbCode.HashedSecret), code.secret)
	if err != nil {
		return oauth2.Token{}, fmt.Errorf("unable to compare code: %w", err)
	}

	if !equal {
		return oauth2.Token{}, errBadCode
	}
	// Ensure the code has not expired.
	if dbCode.ExpiresAt.Before(dbtime.Now()) {
		return oauth2.Token{}, errBadCode
	}
	// Generate a refresh token.
	refreshToken, err := GenerateSecret()
	if err != nil {
		return oauth2.Token{}, err
	}
	// Generate the API key we will swap for the code.
	// TODO: We are ignoring scopes for now.
	tokenName := fmt.Sprintf("%s_%s_oauth_session_token", dbCode.UserID, app.ID)
	key, sessionToken, err := apikey.Generate(apikey.CreateParams{
		UserID:          dbCode.UserID,
		LoginType:       database.LoginTypeOAuth2ProviderApp,
		DefaultLifetime: lifetimes.DefaultDuration.Value(),
		// For now, we allow only one token per app and user at a time.
		TokenName: tokenName,

	})
	if err != nil {
		return oauth2.Token{}, err
	}
	// Grab the user roles so we can perform the exchange as the user.

	actor, _, err := httpmw.UserRBACSubject(ctx, db, dbCode.UserID, rbac.ScopeAll)
	if err != nil {
		return oauth2.Token{}, fmt.Errorf("fetch user actor: %w", err)
	}
	// Do the actual token exchange in the database.
	err = db.InTx(func(tx database.Store) error {

		ctx := dbauthz.As(ctx, actor)
		err = tx.DeleteOAuth2ProviderAppCodeByID(ctx, dbCode.ID)
		if err != nil {
			return fmt.Errorf("delete oauth2 app code: %w", err)
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

			return fmt.Errorf("delete api key by name: %w", err)
		}
		newKey, err := tx.InsertAPIKey(ctx, key)
		if err != nil {
			return fmt.Errorf("insert oauth2 access token: %w", err)
		}

		_, err = tx.InsertOAuth2ProviderAppToken(ctx, database.InsertOAuth2ProviderAppTokenParams{
			ID:          uuid.New(),
			CreatedAt:   dbtime.Now(),
			ExpiresAt:   key.ExpiresAt,
			HashPrefix:  []byte(refreshToken.Prefix),
			RefreshHash: []byte(refreshToken.Hashed),
			AppSecretID: dbSecret.ID,
			APIKeyID:    newKey.ID,

		})
		if err != nil {
			return fmt.Errorf("insert oauth2 refresh token: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		return oauth2.Token{}, err
	}
	return oauth2.Token{
		AccessToken:  sessionToken,
		TokenType:    "Bearer",

		RefreshToken: refreshToken.Formatted,
		Expiry:       key.ExpiresAt,
	}, nil
}
func refreshTokenGrant(ctx context.Context, db database.Store, app database.OAuth2ProviderApp, lifetimes codersdk.SessionLifetime, params tokenParams) (oauth2.Token, error) {

	// Validate the token.
	token, err := parseSecret(params.refreshToken)
	if err != nil {
		return oauth2.Token{}, errBadToken
	}
	//nolint:gocritic // There is no user yet so we must use the system.
	dbToken, err := db.GetOAuth2ProviderAppTokenByPrefix(dbauthz.AsSystemRestricted(ctx), []byte(token.prefix))
	if errors.Is(err, sql.ErrNoRows) {
		return oauth2.Token{}, errBadToken
	}
	if err != nil {
		return oauth2.Token{}, err
	}
	equal, err := userpassword.Compare(string(dbToken.RefreshHash), token.secret)
	if err != nil {
		return oauth2.Token{}, fmt.Errorf("unable to compare token: %w", err)
	}
	if !equal {

		return oauth2.Token{}, errBadToken
	}
	// Ensure the token has not expired.
	if dbToken.ExpiresAt.Before(dbtime.Now()) {
		return oauth2.Token{}, errBadToken
	}
	// Grab the user roles so we can perform the refresh as the user.
	//nolint:gocritic // There is no user yet so we must use the system.

	prevKey, err := db.GetAPIKeyByID(dbauthz.AsSystemRestricted(ctx), dbToken.APIKeyID)
	if err != nil {
		return oauth2.Token{}, err
	}
	actor, _, err := httpmw.UserRBACSubject(ctx, db, prevKey.UserID, rbac.ScopeAll)
	if err != nil {
		return oauth2.Token{}, fmt.Errorf("fetch user actor: %w", err)
	}
	// Generate a new refresh token.
	refreshToken, err := GenerateSecret()
	if err != nil {
		return oauth2.Token{}, err
	}
	// Generate the new API key.
	// TODO: We are ignoring scopes for now.
	tokenName := fmt.Sprintf("%s_%s_oauth_session_token", prevKey.UserID, app.ID)
	key, sessionToken, err := apikey.Generate(apikey.CreateParams{
		UserID:          prevKey.UserID,
		LoginType:       database.LoginTypeOAuth2ProviderApp,
		DefaultLifetime: lifetimes.DefaultDuration.Value(),
		// For now, we allow only one token per app and user at a time.
		TokenName: tokenName,

	})
	if err != nil {
		return oauth2.Token{}, err
	}
	// Replace the token.

	err = db.InTx(func(tx database.Store) error {
		ctx := dbauthz.As(ctx, actor)
		err = tx.DeleteAPIKeyByID(ctx, prevKey.ID) // This cascades to the token.
		if err != nil {
			return fmt.Errorf("delete oauth2 app token: %w", err)
		}
		newKey, err := tx.InsertAPIKey(ctx, key)

		if err != nil {
			return fmt.Errorf("insert oauth2 access token: %w", err)
		}
		_, err = tx.InsertOAuth2ProviderAppToken(ctx, database.InsertOAuth2ProviderAppTokenParams{
			ID:          uuid.New(),

			CreatedAt:   dbtime.Now(),
			ExpiresAt:   key.ExpiresAt,
			HashPrefix:  []byte(refreshToken.Prefix),
			RefreshHash: []byte(refreshToken.Hashed),
			AppSecretID: dbToken.AppSecretID,
			APIKeyID:    newKey.ID,

		})
		if err != nil {
			return fmt.Errorf("insert oauth2 refresh token: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		return oauth2.Token{}, err
	}
	return oauth2.Token{
		AccessToken:  sessionToken,
		TokenType:    "Bearer",
		RefreshToken: refreshToken.Formatted,
		Expiry:       key.ExpiresAt,

	}, nil
}

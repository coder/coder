package oauth2provider

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

var (
	// errBadSecret means the user provided a bad secret.
	errBadSecret = xerrors.New("Invalid client secret")
	// errBadCode means the user provided a bad code.
	errBadCode = xerrors.New("Invalid code")
	// errBadToken means the user provided a bad token.
	errBadToken = xerrors.New("Invalid token")
	// errInvalidPKCE means the PKCE verification failed.
	errInvalidPKCE = xerrors.New("invalid code_verifier")
	// errInvalidResource means the resource parameter validation failed.
	errInvalidResource = xerrors.New("invalid resource parameter")
)

// tokenParams contains parsed/validated token request parameters.
type tokenParams struct {
	req         codersdk.OAuth2TokenRequest // source of truth for field values
	redirectURL *url.URL                    // parsed from req.RedirectURI
	scopes      []string                    // parsed from req.Scope
}

func extractTokenParams(r *http.Request, callbackURL *url.URL) (tokenParams, []codersdk.ValidationError, error) {
	p := httpapi.NewQueryParamParser()
	err := r.ParseForm()
	if err != nil {
		return tokenParams{}, nil, xerrors.Errorf("parse form: %w", err)
	}

	vals := r.Form
	p.RequiredNotEmpty("grant_type")
	grantType := httpapi.ParseCustom(p, vals, "", "grant_type", httpapi.ParseEnum[codersdk.OAuth2ProviderGrantType])

	// Grant-type specific validation - must be called before parsing values
	switch grantType {
	case codersdk.OAuth2ProviderGrantTypeRefreshToken:
		p.RequiredNotEmpty("refresh_token")
	case codersdk.OAuth2ProviderGrantTypeAuthorizationCode:
		p.RequiredNotEmpty("client_secret", "client_id", "code")
	}

	// Parse into SDK type (source of truth)
	req := codersdk.OAuth2TokenRequest{
		GrantType:    grantType,
		ClientID:     p.String(vals, "", "client_id"),
		ClientSecret: p.String(vals, "", "client_secret"),
		Code:         p.String(vals, "", "code"),
		RedirectURI:  p.String(vals, "", "redirect_uri"),
		RefreshToken: p.String(vals, "", "refresh_token"),
		CodeVerifier: p.String(vals, "", "code_verifier"),
		Resource:     p.String(vals, "", "resource"),
		Scope:        p.String(vals, "", "scope"),
	}

	// Parse redirect URI
	redirectURL := p.RedirectURL(vals, callbackURL, "redirect_uri")

	// Validate resource parameter syntax (RFC 8707): must be absolute URI without fragment
	if err := validateResourceParameter(req.Resource); err != nil {
		p.Errors = append(p.Errors, codersdk.ValidationError{
			Field:  "resource",
			Detail: "must be an absolute URI without fragment",
		})
	}

	p.ErrorExcessParams(vals)
	if len(p.Errors) > 0 {
		return tokenParams{}, p.Errors, xerrors.Errorf("invalid query params: %w", p.Errors)
	}
	return tokenParams{
		req:         req,
		redirectURL: redirectURL,
		scopes:      strings.Fields(strings.TrimSpace(req.Scope)),
	}, nil, nil
}

// Tokens
// Uses Sessions.DefaultDuration for access token (API key) TTL and
// Sessions.RefreshDefaultDuration for refresh token TTL.
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
			// Check for specific validation errors in priority order
			if slices.ContainsFunc(validationErrs, func(validationError codersdk.ValidationError) bool {
				return validationError.Field == "grant_type"
			}) {
				httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, codersdk.OAuth2ErrorCodeUnsupportedGrantType, "The grant type is missing or unsupported")
				return
			}

			// Check for missing required parameters for authorization_code grant
			for _, field := range []string{"code", "client_id", "client_secret"} {
				if slices.ContainsFunc(validationErrs, func(validationError codersdk.ValidationError) bool {
					return validationError.Field == field
				}) {
					httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, codersdk.OAuth2ErrorCodeInvalidRequest, fmt.Sprintf("Missing required parameter: %s", field))
					return
				}
			}
			// Generic invalid request for other validation errors
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, codersdk.OAuth2ErrorCodeInvalidRequest, "The request is missing required parameters or is otherwise malformed")
			return
		}

		var token codersdk.OAuth2TokenResponse
		//nolint:gocritic,revive // More cases will be added later.
		switch params.req.GrantType {
		// TODO: Client creds, device code.
		case codersdk.OAuth2ProviderGrantTypeRefreshToken:
			token, err = refreshTokenGrant(ctx, db, app, lifetimes, params)
		case codersdk.OAuth2ProviderGrantTypeAuthorizationCode:
			token, err = authorizationCodeGrant(ctx, db, app, lifetimes, params)
		default:
			// This should handle truly invalid grant types
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, codersdk.OAuth2ErrorCodeUnsupportedGrantType, fmt.Sprintf("The grant type %q is not supported", params.req.GrantType))
			return
		}

		if errors.Is(err, errBadSecret) {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusUnauthorized, codersdk.OAuth2ErrorCodeInvalidClient, "The client credentials are invalid")
			return
		}
		if errors.Is(err, errBadCode) {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, codersdk.OAuth2ErrorCodeInvalidGrant, "The authorization code is invalid or expired")
			return
		}
		if errors.Is(err, errInvalidPKCE) {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, codersdk.OAuth2ErrorCodeInvalidGrant, "The PKCE code verifier is invalid")
			return
		}
		if errors.Is(err, errInvalidResource) {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, codersdk.OAuth2ErrorCodeInvalidTarget, "The resource parameter is invalid")
			return
		}
		if errors.Is(err, errBadToken) {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, codersdk.OAuth2ErrorCodeInvalidGrant, "The refresh token is invalid or expired")
			return
		}
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
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

func authorizationCodeGrant(ctx context.Context, db database.Store, app database.OAuth2ProviderApp, lifetimes codersdk.SessionLifetime, params tokenParams) (codersdk.OAuth2TokenResponse, error) {
	// Validate the client secret.
	secret, err := ParseFormattedSecret(params.req.ClientSecret)
	if err != nil {
		return codersdk.OAuth2TokenResponse{}, errBadSecret
	}
	//nolint:gocritic // Users cannot read secrets so we must use the system.
	dbSecret, err := db.GetOAuth2ProviderAppSecretByPrefix(dbauthz.AsSystemRestricted(ctx), []byte(secret.Prefix))
	if errors.Is(err, sql.ErrNoRows) {
		return codersdk.OAuth2TokenResponse{}, errBadSecret
	}
	if err != nil {
		return codersdk.OAuth2TokenResponse{}, err
	}

	equalSecret := apikey.ValidateHash(dbSecret.HashedSecret, secret.Secret)
	if !equalSecret {
		return codersdk.OAuth2TokenResponse{}, errBadSecret
	}

	// Validate the authorization code.
	code, err := ParseFormattedSecret(params.req.Code)
	if err != nil {
		return codersdk.OAuth2TokenResponse{}, errBadCode
	}
	//nolint:gocritic // There is no user yet so we must use the system.
	dbCode, err := db.GetOAuth2ProviderAppCodeByPrefix(dbauthz.AsSystemRestricted(ctx), []byte(code.Prefix))
	if errors.Is(err, sql.ErrNoRows) {
		return codersdk.OAuth2TokenResponse{}, errBadCode
	}
	if err != nil {
		return codersdk.OAuth2TokenResponse{}, err
	}
	equalCode := apikey.ValidateHash(dbCode.HashedSecret, code.Secret)
	if !equalCode {
		return codersdk.OAuth2TokenResponse{}, errBadCode
	}

	// Ensure the code has not expired.
	if dbCode.ExpiresAt.Before(dbtime.Now()) {
		return codersdk.OAuth2TokenResponse{}, errBadCode
	}

	// Verify PKCE challenge if present
	if dbCode.CodeChallenge.Valid && dbCode.CodeChallenge.String != "" {
		if params.req.CodeVerifier == "" {
			return codersdk.OAuth2TokenResponse{}, errInvalidPKCE
		}
		if !VerifyPKCE(dbCode.CodeChallenge.String, params.req.CodeVerifier) {
			return codersdk.OAuth2TokenResponse{}, errInvalidPKCE
		}
	}

	// Verify resource parameter consistency (RFC 8707)
	if dbCode.ResourceUri.Valid && dbCode.ResourceUri.String != "" {
		// Resource was specified during authorization - it must match in token request
		if params.req.Resource == "" {
			return codersdk.OAuth2TokenResponse{}, errInvalidResource
		}
		if params.req.Resource != dbCode.ResourceUri.String {
			return codersdk.OAuth2TokenResponse{}, errInvalidResource
		}
	} else if params.req.Resource != "" {
		// Resource was not specified during authorization but is now provided
		return codersdk.OAuth2TokenResponse{}, errInvalidResource
	}

	// Generate a refresh token.
	refreshToken, err := GenerateSecret()
	if err != nil {
		return codersdk.OAuth2TokenResponse{}, err
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
		return codersdk.OAuth2TokenResponse{}, err
	}

	// Grab the user roles so we can perform the exchange as the user.
	actor, _, err := httpmw.UserRBACSubject(ctx, db, dbCode.UserID, rbac.ScopeAll)
	if err != nil {
		return codersdk.OAuth2TokenResponse{}, xerrors.Errorf("fetch user actor: %w", err)
	}

	// Do the actual token exchange in the database.
	// Determine refresh token expiry independently from the access token.
	refreshLifetime := lifetimes.RefreshDefaultDuration.Value()
	if refreshLifetime == 0 {
		refreshLifetime = lifetimes.DefaultDuration.Value()
	}
	refreshExpiresAt := dbtime.Now().Add(refreshLifetime)

	err = db.InTx(func(tx database.Store) error {
		ctx := dbauthz.As(ctx, actor)
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

		_, err = tx.InsertOAuth2ProviderAppToken(ctx, database.InsertOAuth2ProviderAppTokenParams{
			ID:          uuid.New(),
			CreatedAt:   dbtime.Now(),
			ExpiresAt:   refreshExpiresAt,
			HashPrefix:  []byte(refreshToken.Prefix),
			RefreshHash: refreshToken.Hashed,
			AppSecretID: dbSecret.ID,
			APIKeyID:    newKey.ID,
			UserID:      dbCode.UserID,
			Audience:    dbCode.ResourceUri,
		})
		if err != nil {
			return xerrors.Errorf("insert oauth2 refresh token: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		return codersdk.OAuth2TokenResponse{}, err
	}

	return codersdk.OAuth2TokenResponse{
		AccessToken:  sessionToken,
		TokenType:    codersdk.OAuth2TokenTypeBearer,
		RefreshToken: refreshToken.Formatted,
		ExpiresIn:    int64(time.Until(key.ExpiresAt).Seconds()),
		Expiry:       &key.ExpiresAt,
	}, nil
}

func refreshTokenGrant(ctx context.Context, db database.Store, app database.OAuth2ProviderApp, lifetimes codersdk.SessionLifetime, params tokenParams) (codersdk.OAuth2TokenResponse, error) {
	// Validate the token.
	token, err := ParseFormattedSecret(params.req.RefreshToken)
	if err != nil {
		return codersdk.OAuth2TokenResponse{}, errBadToken
	}
	//nolint:gocritic // There is no user yet so we must use the system.
	dbToken, err := db.GetOAuth2ProviderAppTokenByPrefix(dbauthz.AsSystemRestricted(ctx), []byte(token.Prefix))
	if errors.Is(err, sql.ErrNoRows) {
		return codersdk.OAuth2TokenResponse{}, errBadToken
	}
	if err != nil {
		return codersdk.OAuth2TokenResponse{}, err
	}
	equal := apikey.ValidateHash(dbToken.RefreshHash, token.Secret)
	if !equal {
		return codersdk.OAuth2TokenResponse{}, errBadToken
	}

	// Ensure the token has not expired.
	if dbToken.ExpiresAt.Before(dbtime.Now()) {
		return codersdk.OAuth2TokenResponse{}, errBadToken
	}

	// Verify resource parameter consistency for refresh tokens (RFC 8707)
	if params.req.Resource != "" {
		// If resource is provided in refresh request, it must match the original token's audience
		if !dbToken.Audience.Valid || dbToken.Audience.String != params.req.Resource {
			return codersdk.OAuth2TokenResponse{}, errInvalidResource
		}
	}

	// Grab the user roles so we can perform the refresh as the user.
	//nolint:gocritic // There is no user yet so we must use the system.
	prevKey, err := db.GetAPIKeyByID(dbauthz.AsSystemRestricted(ctx), dbToken.APIKeyID)
	if err != nil {
		return codersdk.OAuth2TokenResponse{}, err
	}

	actor, _, err := httpmw.UserRBACSubject(ctx, db, prevKey.UserID, rbac.ScopeAll)
	if err != nil {
		return codersdk.OAuth2TokenResponse{}, xerrors.Errorf("fetch user actor: %w", err)
	}

	// Generate a new refresh token.
	refreshToken, err := GenerateSecret()
	if err != nil {
		return codersdk.OAuth2TokenResponse{}, err
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
		return codersdk.OAuth2TokenResponse{}, err
	}

	// Replace the token.
	// Determine refresh token expiry independently from the access token.
	refreshLifetime := lifetimes.RefreshDefaultDuration.Value()
	if refreshLifetime == 0 {
		refreshLifetime = lifetimes.DefaultDuration.Value()
	}
	refreshExpiresAt := dbtime.Now().Add(refreshLifetime)

	err = db.InTx(func(tx database.Store) error {
		ctx := dbauthz.As(ctx, actor)
		err = tx.DeleteAPIKeyByID(ctx, prevKey.ID) // This cascades to the token.
		if err != nil {
			return xerrors.Errorf("delete oauth2 app token: %w", err)
		}

		newKey, err := tx.InsertAPIKey(ctx, key)
		if err != nil {
			return xerrors.Errorf("insert oauth2 access token: %w", err)
		}

		_, err = tx.InsertOAuth2ProviderAppToken(ctx, database.InsertOAuth2ProviderAppTokenParams{
			ID:          uuid.New(),
			CreatedAt:   dbtime.Now(),
			ExpiresAt:   refreshExpiresAt,
			HashPrefix:  []byte(refreshToken.Prefix),
			RefreshHash: refreshToken.Hashed,
			AppSecretID: dbToken.AppSecretID,
			APIKeyID:    newKey.ID,
			UserID:      dbToken.UserID,
			Audience:    dbToken.Audience,
		})
		if err != nil {
			return xerrors.Errorf("insert oauth2 refresh token: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		return codersdk.OAuth2TokenResponse{}, err
	}

	return codersdk.OAuth2TokenResponse{
		AccessToken:  sessionToken,
		TokenType:    codersdk.OAuth2TokenTypeBearer,
		RefreshToken: refreshToken.Formatted,
		ExpiresIn:    int64(time.Until(key.ExpiresAt).Seconds()),
		Expiry:       &key.ExpiresAt,
	}, nil
}

// validateResourceParameter validates that a resource parameter conforms to RFC 8707:
// must be an absolute URI without fragment component.
func validateResourceParameter(resource string) error {
	if resource == "" {
		return nil // Resource parameter is optional
	}

	u, err := url.Parse(resource)
	if err != nil {
		return xerrors.Errorf("invalid URI syntax: %w", err)
	}

	if u.Scheme == "" {
		return xerrors.New("must be an absolute URI with scheme")
	}

	if u.Fragment != "" {
		return xerrors.New("must not contain fragment component")
	}

	return nil
}

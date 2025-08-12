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
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

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
	errBadSecret = xerrors.New("invalid client secret")
	// errBadCode means the user provided a bad code.
	errBadCode = xerrors.New("invalid code")
	// errBadToken means the user provided a bad token.
	errBadToken = xerrors.New("invalid token")
	// errInvalidPKCE means the PKCE verification failed.
	errInvalidPKCE = xerrors.New("invalid code_verifier")
	// errInvalidResource means the resource parameter validation failed.
	errInvalidResource = xerrors.New("invalid resource parameter")
	// errBadDeviceCode means the user provided a bad device code.
	errBadDeviceCode = xerrors.New("invalid device code")
	// errAuthorizationPending means the user hasn't authorized the device yet.
	errAuthorizationPending = xerrors.New("authorization pending")
	// errSlowDown means the client is polling too frequently.
	errSlowDown = xerrors.New("slow down")
	// errAccessDenied means the user denied the authorization.
	errAccessDenied = xerrors.New("access denied")
	// errExpiredToken means the device code has expired.
	errExpiredToken = xerrors.New("expired token")
)

type tokenParams struct {
	clientID     string
	clientSecret string
	code         string
	grantType    codersdk.OAuth2ProviderGrantType
	redirectURL  *url.URL
	refreshToken string
	codeVerifier string // PKCE verifier
	resource     string // RFC 8707 resource for token binding
	deviceCode   string // RFC 8628 device code
}

func extractTokenParams(r *http.Request, registeredRedirectURIs []string) (tokenParams, []codersdk.ValidationError, error) {
	p := httpapi.NewQueryParamParser()
	if err := r.ParseForm(); err != nil {
		return tokenParams{}, nil, xerrors.Errorf("parse form: %w", err)
	}

	vals := r.Form
	p.RequiredNotEmpty("grant_type")
	grantType := httpapi.ParseCustom(p, vals, "", "grant_type", httpapi.ParseEnum[codersdk.OAuth2ProviderGrantType])
	switch grantType {
	case codersdk.OAuth2ProviderGrantTypeRefreshToken:
		p.RequiredNotEmpty("refresh_token")
	case codersdk.OAuth2ProviderGrantTypeAuthorizationCode:
		p.RequiredNotEmpty("client_secret", "client_id", "code", "redirect_uri")
	case codersdk.OAuth2ProviderGrantTypeDeviceCode:
		p.RequiredNotEmpty("client_id", "device_code")
	}

	params := tokenParams{
		clientID:     p.String(vals, "", "client_id"),
		clientSecret: p.String(vals, "", "client_secret"),
		code:         p.String(vals, "", "code"),
		grantType:    grantType,
		redirectURL:  nil, // Will be validated below
		refreshToken: p.String(vals, "", "refresh_token"),
		codeVerifier: p.String(vals, "", "code_verifier"),
		resource:     p.String(vals, "", "resource"),
		deviceCode:   p.String(vals, "", "device_code"),
	}

	// RFC 6749 compliant redirect URI validation for authorization code flow
	if grantType == codersdk.OAuth2ProviderGrantTypeAuthorizationCode {
		redirectURIParam := p.String(vals, "", "redirect_uri")
		params.redirectURL = validateRedirectURI(p, redirectURIParam, registeredRedirectURIs)
	}
	// Validate resource parameter syntax (RFC 8707): must be absolute URI without fragment
	if err := validateResourceParameter(params.resource); err != nil {
		p.Errors = append(p.Errors, codersdk.ValidationError{
			Field:  "resource",
			Detail: "must be an absolute URI without fragment",
		})
	}

	p.ErrorExcessParams(vals)
	if len(p.Errors) > 0 {
		return tokenParams{}, p.Errors, xerrors.Errorf("invalid query params: %w", p.Errors)
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

		// Validate that app has registered redirect URIs
		if len(app.RedirectUris) == 0 {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "OAuth2 app has no registered redirect URIs.",
			})
			return
		}

		params, validationErrs, err := extractTokenParams(r, app.RedirectUris)
		if err != nil {
			// Check for specific validation errors in priority order
			if slices.ContainsFunc(validationErrs, func(validationError codersdk.ValidationError) bool {
				return validationError.Field == "grant_type"
			}) {
				httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "unsupported_grant_type", "The grant type is missing or unsupported")
				return
			}

			// Check for missing required parameters for different grant types
			missingParams := []string{"code", "client_id", "client_secret", "device_code", "refresh_token"}
			for _, field := range missingParams {
				if slices.ContainsFunc(validationErrs, func(validationError codersdk.ValidationError) bool {
					return validationError.Field == field
				}) {
					httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_request", fmt.Sprintf("Missing required parameter: %s", field))
					return
				}
			}
			// Generic invalid request for other validation errors
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_request", "The request is missing required parameters or is otherwise malformed")
			return
		}

		var token oauth2.Token
		//nolint:gocritic,revive // More cases will be added later.
		switch params.grantType {
		case codersdk.OAuth2ProviderGrantTypeRefreshToken:
			token, err = refreshTokenGrant(ctx, db, app, lifetimes, params)
		case codersdk.OAuth2ProviderGrantTypeAuthorizationCode:
			token, err = authorizationCodeGrant(ctx, db, app, lifetimes, params)
		case codersdk.OAuth2ProviderGrantTypeDeviceCode:
			token, err = deviceCodeGrant(ctx, db, app, lifetimes, params)
		default:
			// This should handle truly invalid grant types
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "unsupported_grant_type", fmt.Sprintf("The grant type %q is not supported", params.grantType))
			return
		}

		if errors.Is(err, errBadSecret) {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusUnauthorized, "invalid_client", "The client credentials are invalid")
			return
		}
		if errors.Is(err, errBadCode) {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_grant", "The authorization code is invalid or expired")
			return
		}
		if errors.Is(err, errInvalidPKCE) {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_grant", "The PKCE code verifier is invalid")
			return
		}
		if errors.Is(err, errInvalidResource) {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_target", "The resource parameter is invalid")
			return
		}
		if errors.Is(err, errBadToken) {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_grant", "The refresh token is invalid or expired")
			return
		}
		if errors.Is(err, errBadDeviceCode) {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_grant", "The device code is invalid")
			return
		}
		if errors.Is(err, errAuthorizationPending) {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "authorization_pending", "The authorization request is still pending")
			return
		}
		if errors.Is(err, errSlowDown) {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "slow_down", "The client is polling too frequently")
			return
		}
		if errors.Is(err, errAccessDenied) {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "access_denied", "The authorization was denied by the user")
			return
		}
		if errors.Is(err, errExpiredToken) {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "expired_token", "The device authorization has expired")
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

func authorizationCodeGrant(ctx context.Context, db database.Store, app database.OAuth2ProviderApp, lifetimes codersdk.SessionLifetime, params tokenParams) (oauth2.Token, error) {
	// Validate the client secret.
	secret, err := parseFormattedSecret(params.clientSecret)
	if err != nil {
		return oauth2.Token{}, errBadSecret
	}
	//nolint:gocritic // Users cannot read secrets so we must use the system.
	dbSecret, err := db.GetOAuth2ProviderAppSecretByPrefix(dbauthz.AsSystemRestricted(ctx), []byte(secret.prefix))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return oauth2.Token{}, errBadSecret
		}
		return oauth2.Token{}, err
	}
	equal, err := userpassword.Compare(string(dbSecret.HashedSecret), secret.secret)
	if err != nil {
		return oauth2.Token{}, xerrors.Errorf("unable to compare secret: %w", err)
	}
	if !equal {
		return oauth2.Token{}, errBadSecret
	}

	// Atomically consume the authorization code (handles expiry check).
	code, err := parseFormattedSecret(params.code)
	if err != nil {
		return oauth2.Token{}, errBadCode
	}
	//nolint:gocritic // There is no user yet so we must use the system.
	dbCode, err := db.ConsumeOAuth2ProviderAppCodeByPrefix(dbauthz.AsSystemRestricted(ctx), []byte(code.prefix))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return oauth2.Token{}, errBadCode
		}
		return oauth2.Token{}, err
	}

	// Validate the code hash after atomic consumption.
	equal, err = userpassword.Compare(string(dbCode.HashedSecret), code.secret)
	if err != nil {
		return oauth2.Token{}, xerrors.Errorf("unable to compare code: %w", err)
	}
	if !equal {
		return oauth2.Token{}, errBadCode
	}

	// Verify PKCE challenge if present
	if dbCode.CodeChallenge.Valid && dbCode.CodeChallenge.String != "" {
		if params.codeVerifier == "" {
			return oauth2.Token{}, errInvalidPKCE
		}
		if !VerifyPKCE(dbCode.CodeChallenge.String, params.codeVerifier) {
			return oauth2.Token{}, errInvalidPKCE
		}
	}

	// Verify resource parameter consistency (RFC 8707)
	if dbCode.ResourceUri.Valid && dbCode.ResourceUri.String != "" {
		// Resource was specified during authorization - it must match in token request
		if params.resource == "" {
			return oauth2.Token{}, errInvalidResource
		}
		if params.resource != dbCode.ResourceUri.String {
			return oauth2.Token{}, errInvalidResource
		}
	} else if params.resource != "" {
		// Resource was not specified during authorization but is now provided
		return oauth2.Token{}, errInvalidResource
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
		return oauth2.Token{}, xerrors.Errorf("fetch user actor: %w", err)
	}

	// Do the actual token exchange in the database.
	err = db.InTx(func(tx database.Store) error {
		ctx := dbauthz.As(ctx, actor)

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
			ExpiresAt:   key.ExpiresAt,
			HashPrefix:  []byte(refreshToken.Prefix),
			RefreshHash: []byte(refreshToken.Hashed),
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
		return oauth2.Token{}, err
	}

	return oauth2.Token{
		AccessToken:  sessionToken,
		TokenType:    "Bearer",
		RefreshToken: refreshToken.Formatted,
		Expiry:       key.ExpiresAt,
		ExpiresIn:    int64(time.Until(key.ExpiresAt).Seconds()),
	}, nil
}

func refreshTokenGrant(ctx context.Context, db database.Store, app database.OAuth2ProviderApp, lifetimes codersdk.SessionLifetime, params tokenParams) (oauth2.Token, error) {
	// Validate the token.
	token, err := parseFormattedSecret(params.refreshToken)
	if err != nil {
		return oauth2.Token{}, errBadToken
	}
	//nolint:gocritic // There is no user yet so we must use the system.
	dbToken, err := db.GetOAuth2ProviderAppTokenByPrefix(dbauthz.AsSystemRestricted(ctx), []byte(token.prefix))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return oauth2.Token{}, errBadToken
		}
		return oauth2.Token{}, err
	}
	equal, err := userpassword.Compare(string(dbToken.RefreshHash), token.secret)
	if err != nil {
		return oauth2.Token{}, xerrors.Errorf("unable to compare token: %w", err)
	}
	if !equal {
		return oauth2.Token{}, errBadToken
	}

	// Ensure the token has not expired.
	if dbToken.ExpiresAt.Before(dbtime.Now()) {
		return oauth2.Token{}, errBadToken
	}

	// Verify resource parameter consistency for refresh tokens (RFC 8707)
	if params.resource != "" {
		// If resource is provided in refresh request, it must match the original token's audience
		if !dbToken.Audience.Valid || dbToken.Audience.String != params.resource {
			return oauth2.Token{}, errInvalidResource
		}
	}

	// Grab the user roles so we can perform the refresh as the user.
	//nolint:gocritic // There is no user yet so we must use the system.
	prevKey, err := db.GetAPIKeyByID(dbauthz.AsSystemRestricted(ctx), dbToken.APIKeyID)
	if err != nil {
		// API key was deleted (e.g., by token revocation), so token is invalid
		if errors.Is(err, sql.ErrNoRows) {
			return oauth2.Token{}, errBadToken
		}
		return oauth2.Token{}, err
	}

	actor, _, err := httpmw.UserRBACSubject(ctx, db, prevKey.UserID, rbac.ScopeAll)
	if err != nil {
		return oauth2.Token{}, xerrors.Errorf("fetch user actor: %w", err)
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
			return xerrors.Errorf("delete oauth2 app token: %w", err)
		}

		newKey, err := tx.InsertAPIKey(ctx, key)
		if err != nil {
			return xerrors.Errorf("insert oauth2 access token: %w", err)
		}

		_, err = tx.InsertOAuth2ProviderAppToken(ctx, database.InsertOAuth2ProviderAppTokenParams{
			ID:          uuid.New(),
			CreatedAt:   dbtime.Now(),
			ExpiresAt:   key.ExpiresAt,
			HashPrefix:  []byte(refreshToken.Prefix),
			RefreshHash: []byte(refreshToken.Hashed),
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
		return oauth2.Token{}, err
	}

	return oauth2.Token{
		AccessToken:  sessionToken,
		TokenType:    "Bearer",
		RefreshToken: refreshToken.Formatted,
		Expiry:       key.ExpiresAt,
		ExpiresIn:    int64(time.Until(key.ExpiresAt).Seconds()),
	}, nil
}

// validateRedirectURI validates redirect URI according to RFC 6749 and returns the parsed URL if valid
func validateRedirectURI(p *httpapi.QueryParamParser, redirectURIParam string, registeredRedirectURIs []string) *url.URL {
	if redirectURIParam == "" {
		return nil
	}

	// Parse the redirect URI
	redirectURL, err := url.Parse(redirectURIParam)
	if err != nil {
		p.Errors = append(p.Errors, codersdk.ValidationError{
			Field:  "redirect_uri",
			Detail: "must be a valid URL",
		})
		return nil
	}

	// RFC 6749: Exact match against registered redirect URIs
	if slices.Contains(registeredRedirectURIs, redirectURIParam) {
		return redirectURL
	}

	p.Errors = append(p.Errors, codersdk.ValidationError{
		Field:  "redirect_uri",
		Detail: "redirect_uri must exactly match one of the registered redirect URIs",
	})
	return nil
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

// parseDeviceCode parses a device code formatted like "cdr_device_prefix_secret"
func parseDeviceCode(deviceCode string) (parsedSecret, error) {
	parts := strings.Split(deviceCode, "_")
	if len(parts) != 4 {
		return parsedSecret{}, xerrors.Errorf("incorrect number of parts: %d", len(parts))
	}
	if parts[0] != "cdr" || parts[1] != "device" {
		return parsedSecret{}, xerrors.Errorf("incorrect scheme: %s_%s", parts[0], parts[1])
	}
	return parsedSecret{
		prefix: parts[2],
		secret: parts[3],
	}, nil
}

func deviceCodeGrant(ctx context.Context, db database.Store, app database.OAuth2ProviderApp, lifetimes codersdk.SessionLifetime, params tokenParams) (oauth2.Token, error) {
	// Parse the device code
	deviceCode, err := parseDeviceCode(params.deviceCode)
	if err != nil {
		return oauth2.Token{}, errBadDeviceCode
	}

	// First, look up the device code to check its status (non-consuming)
	//nolint:gocritic // System access needed for device code lookup
	dbDeviceCode, err := db.GetOAuth2ProviderDeviceCodeByPrefix(dbauthz.AsSystemRestricted(ctx), deviceCode.prefix)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return oauth2.Token{}, errBadDeviceCode
		}
		return oauth2.Token{}, err
	}

	// Check if the device code has expired
	if dbDeviceCode.ExpiresAt.Before(dbtime.Now()) {
		return oauth2.Token{}, errExpiredToken
	}

	// Verify the device code hash before checking authorization status
	equal, err := userpassword.Compare(string(dbDeviceCode.DeviceCodeHash), deviceCode.secret)
	if err != nil {
		return oauth2.Token{}, xerrors.Errorf("unable to compare device code: %w", err)
	}
	if !equal {
		return oauth2.Token{}, errBadDeviceCode
	}

	// Security: Make sure the app requesting the token is the same one that
	// initiated the device flow.
	if dbDeviceCode.ClientID != app.ID {
		return oauth2.Token{}, errBadDeviceCode
	}

	// Check authorization status before consuming
	switch dbDeviceCode.Status {
	case database.OAuth2DeviceStatusDenied:
		return oauth2.Token{}, errAccessDenied
	case database.OAuth2DeviceStatusPending:
		return oauth2.Token{}, errAuthorizationPending
	case database.OAuth2DeviceStatusAuthorized:
		// Continue with token generation - now atomically consume the device code
		//nolint:gocritic // System access needed for atomic device code consumption
		dbDeviceCode, err = db.ConsumeOAuth2ProviderDeviceCodeByPrefix(dbauthz.AsSystemRestricted(ctx), deviceCode.prefix)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Device code was consumed by another request between our check and consumption
				return oauth2.Token{}, errBadDeviceCode
			}
			return oauth2.Token{}, err
		}
	default:
		return oauth2.Token{}, errAuthorizationPending
	}

	// Check that we have a user_id (should be set when authorized)
	if !dbDeviceCode.UserID.Valid {
		return oauth2.Token{}, errAuthorizationPending
	}

	// Verify resource parameter consistency (RFC 8707)
	if dbDeviceCode.ResourceUri.Valid && dbDeviceCode.ResourceUri.String != "" {
		// Resource was specified during device authorization
		if params.resource == "" {
			return oauth2.Token{}, errInvalidResource
		}
		if params.resource != dbDeviceCode.ResourceUri.String {
			return oauth2.Token{}, errInvalidResource
		}
	} else if params.resource != "" {
		// Resource was not specified during device authorization but is now provided
		return oauth2.Token{}, errInvalidResource
	}

	// Generate a refresh token
	refreshToken, err := GenerateSecret()
	if err != nil {
		return oauth2.Token{}, err
	}

	// Generate the API key we will swap for the device code
	tokenName := fmt.Sprintf("%s_%s_oauth_device_token", dbDeviceCode.UserID.UUID, app.ID)
	key, sessionToken, err := apikey.Generate(apikey.CreateParams{
		UserID:          dbDeviceCode.UserID.UUID,
		LoginType:       database.LoginTypeOAuth2ProviderApp,
		DefaultLifetime: lifetimes.DefaultDuration.Value(),
		TokenName:       tokenName,
	})
	if err != nil {
		return oauth2.Token{}, err
	}

	// Get user roles for authorization context
	actor, _, err := httpmw.UserRBACSubject(ctx, db, dbDeviceCode.UserID.UUID, rbac.ScopeAll)
	if err != nil {
		return oauth2.Token{}, xerrors.Errorf("fetch user actor: %w", err)
	}

	// Do the actual token exchange in the database
	err = db.InTx(func(tx database.Store) error {
		ctx := dbauthz.As(ctx, actor)

		// Delete any previous API key for this app/user combination
		prevKey, err := tx.GetAPIKeyByName(ctx, database.GetAPIKeyByNameParams{
			UserID:    dbDeviceCode.UserID.UUID,
			TokenName: tokenName,
		})
		if err == nil {
			err = tx.DeleteAPIKeyByID(ctx, prevKey.ID)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("delete previous API key: %w", err)
		}

		// Insert the new API key
		newKey, err := tx.InsertAPIKey(ctx, key)
		if err != nil {
			return xerrors.Errorf("insert oauth2 access token: %w", err)
		}

		// Find the app secret for token binding
		//nolint:gocritic // System access needed to find app secret
		appSecrets, err := tx.GetOAuth2ProviderAppSecretsByAppID(dbauthz.AsSystemRestricted(ctx), app.ID)
		if err != nil || len(appSecrets) == 0 {
			return xerrors.Errorf("no app secrets found for client")
		}

		// Use the first (most recent) app secret
		appSecret := appSecrets[0]

		// Insert the OAuth2 token record
		_, err = tx.InsertOAuth2ProviderAppToken(ctx, database.InsertOAuth2ProviderAppTokenParams{
			ID:          uuid.New(),
			CreatedAt:   dbtime.Now(),
			ExpiresAt:   key.ExpiresAt,
			HashPrefix:  []byte(refreshToken.Prefix),
			RefreshHash: []byte(refreshToken.Hashed),
			AppSecretID: appSecret.ID,
			APIKeyID:    newKey.ID,
			UserID:      dbDeviceCode.UserID.UUID,
			Audience:    dbDeviceCode.ResourceUri,
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
		AccessToken:  sessionToken,
		TokenType:    "Bearer",
		RefreshToken: refreshToken.Formatted,
		Expiry:       key.ExpiresAt,
		ExpiresIn:    int64(time.Until(key.ExpiresAt).Seconds()),
	}, nil
}

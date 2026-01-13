package oauth2provider

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

var (
	// ErrTokenNotBelongsToClient is returned when a token does not belong to the requesting client
	ErrTokenNotBelongsToClient = xerrors.New("token does not belong to requesting client")
	// ErrInvalidTokenFormat is returned when a token has an invalid format
	ErrInvalidTokenFormat = xerrors.New("invalid token format")
)

func extractRevocationRequest(r *http.Request) (codersdk.OAuth2TokenRevocationRequest, error) {
	if err := r.ParseForm(); err != nil {
		return codersdk.OAuth2TokenRevocationRequest{}, xerrors.Errorf("invalid form data: %w", err)
	}

	req := codersdk.OAuth2TokenRevocationRequest{
		Token:         r.Form.Get("token"),
		TokenTypeHint: codersdk.OAuth2RevocationTokenTypeHint(r.Form.Get("token_type_hint")),
		ClientID:      r.Form.Get("client_id"),
		ClientSecret:  r.Form.Get("client_secret"),
	}

	// RFC 7009 requires 'token' parameter.
	if req.Token == "" {
		return codersdk.OAuth2TokenRevocationRequest{}, xerrors.New("missing token parameter")
	}

	return req, nil
}

// RevokeToken implements RFC 7009 OAuth2 Token Revocation
// Authentication is unique for this endpoint in that it does not use the
// standard token authentication middleware. Instead, it expects the token that
// is being revoked to be valid.
// TODO: Currently the token validation occurs in the revocation logic itself.
// This code should be refactored to share token validation logic with other parts
// of the OAuth2 provider/http middleware.
func RevokeToken(db database.Store, logger slog.Logger) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		app := httpmw.OAuth2ProviderApp(r)

		// RFC 7009 requires POST method with application/x-www-form-urlencoded
		if r.Method != http.MethodPost {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusMethodNotAllowed, codersdk.OAuth2ErrorCodeInvalidRequest, "Method not allowed")
			return
		}

		req, err := extractRevocationRequest(r)
		if err != nil {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, codersdk.OAuth2ErrorCodeInvalidRequest, err.Error())
			return
		}

		// Determine if this is a refresh token (starts with "coder_") or API key
		// APIKeys do not have the SecretIdentifier prefix.
		const coderPrefix = SecretIdentifier + "_"
		isRefreshToken := strings.HasPrefix(req.Token, coderPrefix)

		// Revoke the token with ownership verification
		err = db.InTx(func(tx database.Store) error {
			if isRefreshToken {
				// Handle refresh token revocation
				return revokeRefreshTokenInTx(ctx, tx, req.Token, app.ID)
			}
			// Handle API key revocation
			return revokeAPIKeyInTx(ctx, tx, req.Token, app.ID)
		}, nil)
		if err != nil {
			if errors.Is(err, ErrTokenNotBelongsToClient) {
				// RFC 7009: Return success even if token doesn't belong to client (don't reveal token existence)
				logger.Debug(ctx, "token revocation failed: token does not belong to requesting client",
					slog.F("client_id", app.ID.String()),
					slog.F("app_name", app.Name))
				rw.WriteHeader(http.StatusOK)
				return
			}
			if errors.Is(err, ErrInvalidTokenFormat) {
				// Invalid token format should return 400 bad request
				logger.Debug(ctx, "token revocation failed: invalid token format",
					slog.F("client_id", app.ID.String()),
					slog.F("app_name", app.Name))
				httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, codersdk.OAuth2ErrorCodeInvalidRequest, "Invalid token format")
				return
			}
			logger.Error(ctx, "token revocation failed with internal server error",
				slog.Error(err),
				slog.F("client_id", app.ID.String()),
				slog.F("app_name", app.Name))
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusInternalServerError, codersdk.OAuth2ErrorCodeServerError, "Internal server error")
			return
		}

		// RFC 7009: successful revocation returns HTTP 200
		rw.WriteHeader(http.StatusOK)
	}
}

func revokeRefreshTokenInTx(ctx context.Context, db database.Store, token string, appID uuid.UUID) error {
	// Parse the refresh token using the existing function
	parsedToken, err := ParseFormattedSecret(token)
	if err != nil {
		return ErrInvalidTokenFormat
	}

	// Try to find refresh token by prefix
	//nolint:gocritic // Using AsSystemOAuth2 for OAuth2 public token revocation endpoint
	dbToken, err := db.GetOAuth2ProviderAppTokenByPrefix(dbauthz.AsSystemOAuth2(ctx), []byte(parsedToken.Prefix))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Token not found - return success per RFC 7009 (don't reveal token existence)
			return nil
		}
		return xerrors.Errorf("get oauth2 provider app token by prefix: %w", err)
	}

	equal := apikey.ValidateHash(dbToken.RefreshHash, parsedToken.Secret)
	if !equal {
		return xerrors.Errorf("invalid refresh token")
	}

	// Verify ownership
	//nolint:gocritic // Using AsSystemOAuth2 for OAuth2 public token revocation endpoint
	appSecret, err := db.GetOAuth2ProviderAppSecretByID(dbauthz.AsSystemOAuth2(ctx), dbToken.AppSecretID)
	if err != nil {
		return xerrors.Errorf("get oauth2 provider app secret: %w", err)
	}
	if appSecret.AppID != appID {
		return ErrTokenNotBelongsToClient
	}

	// Delete the associated API key, which should cascade to remove the refresh token
	// According to RFC 7009, when a refresh token is revoked, associated access tokens should be invalidated
	//nolint:gocritic // Using AsSystemOAuth2 for OAuth2 public token revocation endpoint
	err = db.DeleteAPIKeyByID(dbauthz.AsSystemOAuth2(ctx), dbToken.APIKeyID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return xerrors.Errorf("delete api key: %w", err)
	}

	return nil
}

func revokeAPIKeyInTx(ctx context.Context, db database.Store, token string, appID uuid.UUID) error {
	keyID, secret, err := httpmw.SplitAPIToken(token)
	if err != nil {
		return ErrInvalidTokenFormat
	}

	// Get the API key
	//nolint:gocritic // Using AsSystemOAuth2 for OAuth2 public token revocation endpoint
	apiKey, err := db.GetAPIKeyByID(dbauthz.AsSystemOAuth2(ctx), keyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// API key not found - return success per RFC 7009 (don't reveal token existence)
			return nil
		}
		return xerrors.Errorf("get api key by id: %w", err)
	}

	// Checking to see if the provided secret matches the stored hashed secret
	hashedSecret := sha256.Sum256([]byte(secret))
	if subtle.ConstantTimeCompare(apiKey.HashedSecret, hashedSecret[:]) != 1 {
		return xerrors.Errorf("invalid api key")
	}

	// Verify the API key was created by OAuth2
	if apiKey.LoginType != database.LoginTypeOAuth2ProviderApp {
		return xerrors.New("api key is not an oauth2 token")
	}

	// Find the associated OAuth2 token to verify ownership
	//nolint:gocritic // Using AsSystemOAuth2 for OAuth2 public token revocation endpoint
	dbToken, err := db.GetOAuth2ProviderAppTokenByAPIKeyID(dbauthz.AsSystemOAuth2(ctx), apiKey.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// No associated OAuth2 token - return success per RFC 7009
			return nil
		}
		return xerrors.Errorf("get oauth2 provider app token by api key id: %w", err)
	}

	// Verify the token belongs to the requesting app
	//nolint:gocritic // Using AsSystemOAuth2 for OAuth2 public token revocation endpoint
	appSecret, err := db.GetOAuth2ProviderAppSecretByID(dbauthz.AsSystemOAuth2(ctx), dbToken.AppSecretID)
	if err != nil {
		return xerrors.Errorf("get oauth2 provider app secret for api key verification: %w", err)
	}

	if appSecret.AppID != appID {
		return ErrTokenNotBelongsToClient
	}

	// Delete the API key
	//nolint:gocritic // Using AsSystemOAuth2 for OAuth2 public token revocation endpoint
	err = db.DeleteAPIKeyByID(dbauthz.AsSystemOAuth2(ctx), apiKey.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return xerrors.Errorf("delete api key for revocation: %w", err)
	}

	return nil
}

func RevokeApp(db database.Store) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		apiKey := httpmw.APIKey(r)
		app := httpmw.OAuth2ProviderApp(r)

		err := db.InTx(func(tx database.Store) error {
			err := tx.DeleteOAuth2ProviderAppCodesByAppAndUserID(ctx, database.DeleteOAuth2ProviderAppCodesByAppAndUserIDParams{
				AppID:  app.ID,
				UserID: apiKey.UserID,
			})
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}

			err = tx.DeleteOAuth2ProviderAppTokensByAppAndUserID(ctx, database.DeleteOAuth2ProviderAppTokensByAppAndUserIDParams{
				AppID:  app.ID,
				UserID: apiKey.UserID,
			})
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}

			return nil
		}, nil)
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		rw.WriteHeader(http.StatusNoContent)
	}
}

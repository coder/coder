package oauth2provider

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"

	"github.com/google/uuid"
)

var (
	// ErrTokenNotBelongsToClient is returned when a token does not belong to the requesting client
	ErrTokenNotBelongsToClient = xerrors.New("token does not belong to requesting client")
	// ErrInvalidTokenFormat is returned when a token has an invalid format
	ErrInvalidTokenFormat = xerrors.New("invalid token format")
)

// RevokeToken implements RFC 7009 OAuth2 Token Revocation
func RevokeToken(db database.Store, logger slog.Logger) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		app := httpmw.OAuth2ProviderApp(r)

		// RFC 7009 requires POST method with application/x-www-form-urlencoded
		if r.Method != http.MethodPost {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusMethodNotAllowed, "invalid_request", "Method not allowed")
			return
		}

		if err := r.ParseForm(); err != nil {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_request", "Invalid form data")
			return
		}

		// RFC 7009 requires 'token' parameter
		token := r.Form.Get("token")
		if token == "" {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_request", "Missing token parameter")
			return
		}

		// Extract client_id parameter - required for ownership verification
		clientID := r.Form.Get("client_id")
		if clientID == "" {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_request", "Missing client_id parameter")
			return
		}

		// Verify the extracted app matches the client_id parameter
		if app.ID.String() != clientID {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_client", "Invalid client_id")
			return
		}

		// Determine if this is a refresh token (starts with "coder_") or API key
		const coderPrefix = "coder_"
		isRefreshToken := strings.HasPrefix(token, coderPrefix)

		// Revoke the token with ownership verification
		err := db.InTx(func(tx database.Store) error {
			if isRefreshToken {
				// Handle refresh token revocation
				return revokeRefreshTokenInTx(ctx, tx, token, app.ID)
			}
			// Handle API key revocation
			return revokeAPIKeyInTx(ctx, tx, token, app.ID)
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
				httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_request", "Invalid token format")
				return
			}
			logger.Error(ctx, "token revocation failed with internal server error",
				slog.Error(err),
				slog.F("client_id", app.ID.String()),
				slog.F("app_name", app.Name))
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusInternalServerError, "server_error", "Internal server error")
			return
		}

		// RFC 7009: successful revocation returns HTTP 200
		rw.WriteHeader(http.StatusOK)
	}
}

func revokeRefreshTokenInTx(ctx context.Context, db database.Store, token string, appID uuid.UUID) error {
	// Parse the refresh token using the existing function
	parsedToken, err := parseFormattedSecret(token)
	if err != nil {
		return ErrInvalidTokenFormat
	}

	// Try to find refresh token by prefix
	//nolint:gocritic // Using AsSystemOAuth2 for OAuth2 public token revocation endpoint
	dbToken, err := db.GetOAuth2ProviderAppTokenByPrefix(dbauthz.AsSystemOAuth2(ctx), []byte(parsedToken.prefix))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Token not found - return success per RFC 7009 (don't reveal token existence)
			return nil
		}
		return xerrors.Errorf("get oauth2 provider app token by prefix: %w", err)
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

// parsedAPIKey represents the components of an API key token
type parsedAPIKey struct {
	keyID  string // The API key ID for database lookup
	secret string // The secret part for verification
}

// parseAPIKeyToken parses an API key token following the encoder/decoder pattern
func parseAPIKeyToken(token string) (parsedAPIKey, error) {
	parts := strings.SplitN(token, "-", 2)
	if len(parts) != 2 {
		return parsedAPIKey{}, xerrors.Errorf("incorrect number of parts: %d", len(parts))
	}
	if parts[0] == "" || parts[1] == "" {
		return parsedAPIKey{}, xerrors.New("empty key ID or secret")
	}
	return parsedAPIKey{
		keyID:  parts[0],
		secret: parts[1],
	}, nil
}

func revokeAPIKeyInTx(ctx context.Context, db database.Store, token string, appID uuid.UUID) error {
	// Parse the API key using the structured decoder
	parsedKey, err := parseAPIKeyToken(token)
	if err != nil {
		return ErrInvalidTokenFormat
	}

	// Get the API key
	//nolint:gocritic // Using AsSystemOAuth2 for OAuth2 public token revocation endpoint
	apiKey, err := db.GetAPIKeyByID(dbauthz.AsSystemOAuth2(ctx), parsedKey.keyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// API key not found - return success per RFC 7009 (don't reveal token existence)
			// Note: This covers both non-existent keys and invalid key ID formats
			return nil
		}
		return xerrors.Errorf("get api key by id: %w", err)
	}

	// Verify the API key was created by OAuth2
	if apiKey.LoginType != database.LoginTypeOAuth2ProviderApp {
		return xerrors.New("API key is not an OAuth2 token")
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

// RevokeAppTokens implements bulk revocation of all OAuth2 tokens and codes for a specific app and user
func RevokeAppTokens(db database.Store) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		apiKey := httpmw.APIKey(r)
		app := httpmw.OAuth2ProviderApp(r)

		err := db.InTx(func(tx database.Store) error {
			// Delete all authorization codes for this app and user
			err := tx.DeleteOAuth2ProviderAppCodesByAppAndUserID(ctx, database.DeleteOAuth2ProviderAppCodesByAppAndUserIDParams{
				AppID:  app.ID,
				UserID: apiKey.UserID,
			})
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}

			// Delete all tokens for this app and user (handles authorization code flow)
			err = tx.DeleteOAuth2ProviderAppTokensByAppAndUserID(ctx, database.DeleteOAuth2ProviderAppTokensByAppAndUserIDParams{
				AppID:  app.ID,
				UserID: apiKey.UserID,
			})
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}

			// For client credentials flow: if the app has an owner, also delete tokens for the app owner
			// Client credentials tokens are created with UserID = app.UserID.UUID (the app owner)
			if app.UserID.Valid && app.UserID.UUID != apiKey.UserID {
				// Delete client credentials tokens that belong to the app owner
				err = tx.DeleteOAuth2ProviderAppTokensByAppAndUserID(ctx, database.DeleteOAuth2ProviderAppTokensByAppAndUserIDParams{
					AppID:  app.ID,
					UserID: app.UserID.UUID,
				})
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					return err
				}
			}

			return nil
		}, nil)
		if err != nil {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusInternalServerError, "server_error", "Internal server error")
			return
		}

		// Successful revocation returns HTTP 204 No Content
		rw.WriteHeader(http.StatusNoContent)
	}
}

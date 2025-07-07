package oauth2provider

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"

	"github.com/google/uuid"
)

// RevokeToken implements RFC 7009 OAuth2 Token Revocation
func RevokeToken(db database.Store) http.HandlerFunc {
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
				return revokeRefreshToken(ctx, tx, token, app.ID)
			}
			// Handle API key revocation
			return revokeAPIKey(ctx, tx, token, app.ID)
		}, nil)
		if err != nil {
			if strings.Contains(err.Error(), "does not belong to requesting client") {
				// RFC 7009: Return success even if token doesn't belong to client (don't reveal token existence)
				rw.WriteHeader(http.StatusOK)
				return
			}
			if strings.Contains(err.Error(), "invalid") && strings.Contains(err.Error(), "format") {
				// Invalid token format should return 400 bad request
				httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_request", "Invalid token format")
				return
			}
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusInternalServerError, "server_error", "Internal server error")
			return
		}

		// RFC 7009: successful revocation returns HTTP 200
		rw.WriteHeader(http.StatusOK)
	}
}

func revokeRefreshToken(ctx context.Context, db database.Store, token string, appID uuid.UUID) error {
	// Parse the refresh token using the existing function
	parsedToken, err := parseFormattedSecret(token)
	if err != nil {
		return xerrors.New("invalid refresh token format")
	}

	// Try to find refresh token by prefix
	// nolint:gocritic // Using AsSystemRestricted is necessary for OAuth2 public token revocation endpoint
	dbToken, err := db.GetOAuth2ProviderAppTokenByPrefix(dbauthz.AsSystemRestricted(ctx), []byte(parsedToken.prefix))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Token not found - return success per RFC 7009 (don't reveal token existence)
			return nil
		}
		return err
	}

	// Verify ownership
	// nolint:gocritic // Using AsSystemRestricted is necessary for OAuth2 public token revocation endpoint
	appSecret, err := db.GetOAuth2ProviderAppSecretByID(dbauthz.AsSystemRestricted(ctx), dbToken.AppSecretID)
	if err != nil {
		return err
	}
	if appSecret.AppID != appID {
		return xerrors.New("token does not belong to requesting client")
	}

	// Delete the associated API key, which should cascade to remove the refresh token
	// According to RFC 7009, when a refresh token is revoked, associated access tokens should be invalidated
	// nolint:gocritic // Using AsSystemRestricted is necessary for OAuth2 public token revocation endpoint
	err = db.DeleteAPIKeyByID(dbauthz.AsSystemRestricted(ctx), dbToken.APIKeyID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	return nil
}

func revokeAPIKey(ctx context.Context, db database.Store, token string, appID uuid.UUID) error {
	// Parse the API key ID from the token (format: <id>-<secret>)
	parts := strings.SplitN(token, "-", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return xerrors.New("invalid API key format")
	}

	keyID := parts[0]

	// Get the API key
	// nolint:gocritic // Using AsSystemRestricted is necessary for OAuth2 public token revocation endpoint
	apiKey, err := db.GetAPIKeyByID(dbauthz.AsSystemRestricted(ctx), keyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// API key not found - return success per RFC 7009 (don't reveal token existence)
			// Note: This covers both non-existent keys and invalid key ID formats
			return nil
		}
		return err
	}

	// Verify the API key was created by OAuth2
	if apiKey.LoginType != database.LoginTypeOAuth2ProviderApp {
		return xerrors.New("API key is not an OAuth2 token")
	}

	// Find the associated OAuth2 token to verify ownership
	// nolint:gocritic // Using AsSystemRestricted is necessary for OAuth2 public token revocation endpoint
	dbToken, err := db.GetOAuth2ProviderAppTokenByAPIKeyID(dbauthz.AsSystemRestricted(ctx), apiKey.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// No associated OAuth2 token - return success per RFC 7009
			return nil
		}
		return err
	}

	// Verify the token belongs to the requesting app
	// nolint:gocritic // Using AsSystemRestricted is necessary for OAuth2 public token revocation endpoint
	appSecret, err := db.GetOAuth2ProviderAppSecretByID(dbauthz.AsSystemRestricted(ctx), dbToken.AppSecretID)
	if err != nil {
		return err
	}

	if appSecret.AppID != appID {
		return xerrors.New("API key does not belong to requesting client")
	}

	// Delete the API key
	// nolint:gocritic // Using AsSystemRestricted is necessary for OAuth2 public token revocation endpoint
	err = db.DeleteAPIKeyByID(dbauthz.AsSystemRestricted(ctx), apiKey.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	return nil
}

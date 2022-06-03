package httpmw

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
)

// SessionTokenKey represents the name of the cookie or query paramater the API key is stored in.
const SessionTokenKey = "session_token"

type apiKeyContextKey struct{}

// APIKey returns the API key from the ExtractAPIKey handler.
func APIKey(r *http.Request) database.APIKey {
	apiKey, ok := r.Context().Value(apiKeyContextKey{}).(database.APIKey)
	if !ok {
		panic("developer error: apikey middleware not provided")
	}
	return apiKey
}

// User roles are the 'subject' field of Authorize()
type userRolesKey struct{}

// AuthorizationUserRoles returns the roles used for authorization.
// Comes from the ExtractAPIKey handler.
func AuthorizationUserRoles(r *http.Request) database.GetAuthorizationUserRolesRow {
	apiKey, ok := r.Context().Value(userRolesKey{}).(database.GetAuthorizationUserRolesRow)
	if !ok {
		panic("developer error: user roles middleware not provided")
	}
	return apiKey
}

// OAuth2Configs is a collection of configurations for OAuth-based authentication.
// This should be extended to support other authentication types in the future.
type OAuth2Configs struct {
	Github OAuth2Config
}

// ExtractAPIKey requires authentication using a valid API key.
// It handles extending an API key if it comes close to expiry,
// updating the last used time in the database.
func ExtractAPIKey(db database.Store, oauth *OAuth2Configs) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			var cookieValue string
			cookie, err := r.Cookie(SessionTokenKey)
			if err != nil {
				cookieValue = r.URL.Query().Get(SessionTokenKey)
			} else {
				cookieValue = cookie.Value
			}
			if cookieValue == "" {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: fmt.Sprintf("Cookie %q or query parameter must be provided", SessionTokenKey),
				})
				return
			}
			parts := strings.Split(cookieValue, "-")
			// APIKeys are formatted: ID-SECRET
			if len(parts) != 2 {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: fmt.Sprintf("Invalid %q cookie api key format", SessionTokenKey),
				})
				return
			}
			keyID := parts[0]
			keySecret := parts[1]
			// Ensuring key lengths are valid.
			if len(keyID) != 10 {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: fmt.Sprintf("Invalid %q cookie api key id", SessionTokenKey),
				})
				return
			}
			if len(keySecret) != 22 {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: fmt.Sprintf("Invalid %q cookie api key secret", SessionTokenKey),
				})
				return
			}
			key, err := db.GetAPIKeyByID(r.Context(), keyID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
						Message: "Api key is invalid",
					})
					return
				}
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: "Internal error fetching api key by id",
					Detail:  err.Error(),
				})
				return
			}
			hashed := sha256.Sum256([]byte(keySecret))

			// Checking to see if the secret is valid.
			if subtle.ConstantTimeCompare(key.HashedSecret, hashed[:]) != 1 {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: "Api key secret is invalid",
				})
				return
			}
			now := database.Now()
			// Tracks if the API key has properties updated!
			changed := false

			if key.LoginType != database.LoginTypePassword {
				// Check if the OAuth token is expired!
				if key.OAuthExpiry.Before(now) && !key.OAuthExpiry.IsZero() {
					var oauthConfig OAuth2Config
					switch key.LoginType {
					case database.LoginTypeGithub:
						oauthConfig = oauth.Github
					default:
						httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
							Message: fmt.Sprintf("Unexpected authentication type %q", key.LoginType),
						})
						return
					}
					// If it is, let's refresh it from the provided config!
					token, err := oauthConfig.TokenSource(r.Context(), &oauth2.Token{
						AccessToken:  key.OAuthAccessToken,
						RefreshToken: key.OAuthRefreshToken,
						Expiry:       key.OAuthExpiry,
					}).Token()
					if err != nil {
						httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
							Message: "Could not refresh expired oauth token",
							Detail:  err.Error(),
						})
						return
					}
					key.OAuthAccessToken = token.AccessToken
					key.OAuthRefreshToken = token.RefreshToken
					key.OAuthExpiry = token.Expiry
					key.ExpiresAt = token.Expiry
					changed = true
				}
			}

			// Checking if the key is expired.
			if key.ExpiresAt.Before(now) {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: fmt.Sprintf("Api key expired at %q", key.ExpiresAt.String()),
				})
				return
			}

			// Only update LastUsed once an hour to prevent database spam.
			if now.Sub(key.LastUsed) > time.Hour {
				key.LastUsed = now
				changed = true
			}
			// Only update the ExpiresAt once an hour to prevent database spam.
			// We extend the ExpiresAt to reduce re-authentication.
			apiKeyLifetime := time.Duration(key.LifetimeSeconds) * time.Second
			if key.ExpiresAt.Sub(now) <= apiKeyLifetime-time.Hour {
				key.ExpiresAt = now.Add(apiKeyLifetime)
				changed = true
			}
			if changed {
				err := db.UpdateAPIKeyByID(r.Context(), database.UpdateAPIKeyByIDParams{
					ID:                key.ID,
					LastUsed:          key.LastUsed,
					ExpiresAt:         key.ExpiresAt,
					OAuthAccessToken:  key.OAuthAccessToken,
					OAuthRefreshToken: key.OAuthRefreshToken,
					OAuthExpiry:       key.OAuthExpiry,
				})
				if err != nil {
					httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
						Message: fmt.Sprintf("Api key couldn't update: %s", err.Error()),
					})
					return
				}
			}

			// If the key is valid, we also fetch the user roles and status.
			// The roles are used for RBAC authorize checks, and the status
			// is to block 'suspended' users from accessing the platform.
			roles, err := db.GetAuthorizationUserRoles(r.Context(), key.UserID)
			if err != nil {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: "Internal error fetching user's roles",
					Detail:  err.Error(),
				})
				return
			}

			if roles.Status != database.UserStatusActive {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: fmt.Sprintf("User is not active (status = %q), contact an admin to reactivate your account", roles.Status),
				})
				return
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, apiKeyContextKey{}, key)
			ctx = context.WithValue(ctx, userRolesKey{}, roles)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

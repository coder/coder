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
					Message: fmt.Sprintf("%q cookie or query parameter must be provided", SessionTokenKey),
				})
				return
			}
			parts := strings.Split(cookieValue, "-")
			// APIKeys are formatted: ID-SECRET
			if len(parts) != 2 {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: fmt.Sprintf("invalid %q cookie api key format", SessionTokenKey),
				})
				return
			}
			keyID := parts[0]
			keySecret := parts[1]
			// Ensuring key lengths are valid.
			if len(keyID) != 10 {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: fmt.Sprintf("invalid %q cookie api key id", SessionTokenKey),
				})
				return
			}
			if len(keySecret) != 22 {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: fmt.Sprintf("invalid %q cookie api key secret", SessionTokenKey),
				})
				return
			}
			key, err := db.GetAPIKeyByID(r.Context(), keyID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
						Message: "api key is invalid",
					})
					return
				}
				httpapi.Write(rw, http.StatusInternalServerError, httpapi.Response{
					Message: fmt.Sprintf("get api key by id: %s", err.Error()),
				})
				return
			}
			hashed := sha256.Sum256([]byte(keySecret))

			// Checking to see if the secret is valid.
			if subtle.ConstantTimeCompare(key.HashedSecret, hashed[:]) != 1 {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: "api key secret is invalid",
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
							Message: fmt.Sprintf("unexpected authentication type %q", key.LoginType),
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
							Message: fmt.Sprintf("couldn't refresh expired oauth token: %s", err.Error()),
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
					Message: fmt.Sprintf("api key expired at %q", key.ExpiresAt.String()),
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
			apiKeyLifetime := 24 * time.Hour
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
						Message: fmt.Sprintf("api key couldn't update: %s", err.Error()),
					})
					return
				}
			}

			ctx := context.WithValue(r.Context(), apiKeyContextKey{}, key)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

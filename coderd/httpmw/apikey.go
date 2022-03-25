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

	"github.com/coder/coder/database"
	"github.com/coder/coder/httpapi"
)

// AuthCookie represents the name of the cookie the API key is stored in.
const AuthCookie = "session_token"

// OAuth2Config contains a subset of functions exposed from oauth2.Config.
// It is abstracted for simple testing.
type OAuth2Config interface {
	TokenSource(context.Context, *oauth2.Token) oauth2.TokenSource
}

type apiKeyContextKey struct{}

// APIKey returns the API key from the ExtractAPIKey handler.
func APIKey(r *http.Request) database.APIKey {
	apiKey, ok := r.Context().Value(apiKeyContextKey{}).(database.APIKey)
	if !ok {
		panic("developer error: apikey middleware not provided")
	}
	return apiKey
}

// ExtractAPIKey requires authentication using a valid API key.
// It handles extending an API key if it comes close to expiry,
// updating the last used time in the database.
func ExtractAPIKey(db database.Store, oauthConfig OAuth2Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(AuthCookie)
			if err != nil {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: fmt.Sprintf("%q cookie must be provided", AuthCookie),
				})
				return
			}
			parts := strings.Split(cookie.Value, "-")
			// APIKeys are formatted: ID-SECRET
			if len(parts) != 2 {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: fmt.Sprintf("invalid %q cookie api key format", AuthCookie),
				})
				return
			}
			keyID := parts[0]
			keySecret := parts[1]
			// Ensuring key lengths are valid.
			if len(keyID) != 10 {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: fmt.Sprintf("invalid %q cookie api key id", AuthCookie),
				})
				return
			}
			if len(keySecret) != 22 {
				httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
					Message: fmt.Sprintf("invalid %q cookie api key secret", AuthCookie),
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

			if key.LoginType == database.LoginTypeOIDC {
				// Check if the OIDC token is expired!
				if key.OIDCExpiry.Before(now) && !key.OIDCExpiry.IsZero() {
					// If it is, let's refresh it from the provided config!
					token, err := oauthConfig.TokenSource(r.Context(), &oauth2.Token{
						AccessToken:  key.OIDCAccessToken,
						RefreshToken: key.OIDCRefreshToken,
						Expiry:       key.OIDCExpiry,
					}).Token()
					if err != nil {
						httpapi.Write(rw, http.StatusUnauthorized, httpapi.Response{
							Message: fmt.Sprintf("couldn't refresh expired oauth token: %s", err.Error()),
						})
						return
					}
					key.OIDCAccessToken = token.AccessToken
					key.OIDCRefreshToken = token.RefreshToken
					key.OIDCExpiry = token.Expiry
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
			// We extend the ExpiresAt to reduce reauthentication.
			apiKeyLifetime := 24 * time.Hour
			if key.ExpiresAt.Sub(now) <= apiKeyLifetime-time.Hour {
				key.ExpiresAt = now.Add(apiKeyLifetime)
				changed = true
			}

			if changed {
				err := db.UpdateAPIKeyByID(r.Context(), database.UpdateAPIKeyByIDParams{
					ID:               key.ID,
					ExpiresAt:        key.ExpiresAt,
					LastUsed:         key.LastUsed,
					OIDCAccessToken:  key.OIDCAccessToken,
					OIDCRefreshToken: key.OIDCRefreshToken,
					OIDCExpiry:       key.OIDCExpiry,
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

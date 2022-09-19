package httpmw

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/google/uuid"
	"github.com/tabbed/pqtype"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

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
type userAuthKey struct{}

type Authorization struct {
	ID       uuid.UUID
	Username string
	Roles    []string
	Scope    database.APIKeyScope
}

// UserAuthorization returns the roles and scope used for authorization. Depends
// on the ExtractAPIKey handler.
func UserAuthorization(r *http.Request) Authorization {
	auth, ok := r.Context().Value(userAuthKey{}).(Authorization)
	if !ok {
		panic("developer error: ExtractAPIKey middleware not provided")
	}
	return auth
}

// OAuth2Configs is a collection of configurations for OAuth-based authentication.
// This should be extended to support other authentication types in the future.
type OAuth2Configs struct {
	Github OAuth2Config
	OIDC   OAuth2Config
}

const (
	signedOutErrorMessage string = "You are signed out or your session has expired. Please sign in again to continue."
	internalErrorMessage  string = "An internal error occurred. Please try again or contact the system administrator."
)

type loginURLKey struct{}

func getLoginURL(r *http.Request) (*url.URL, bool) {
	val, ok := r.Context().Value(loginURLKey{}).(*url.URL)
	return val, ok
}

// UseLoginURL sets the login URL to use for the request for handlers like
// ExtractAPIKey.
func UseLoginURL(loginURL *url.URL) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), loginURLKey{}, loginURL)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

// ExtractAPIKey requires authentication using a valid API key.
// It handles extending an API key if it comes close to expiry,
// updating the last used time in the database.
// nolint:revive
func ExtractAPIKey(db database.Store, oauth *OAuth2Configs, redirectToLogin bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// Write wraps writing a response to redirect if the handler
			// specified it should. This redirect is used for user-facing
			// pages like workspace applications.
			write := func(code int, response codersdk.Response) {
				if redirectToLogin {
					var (
						u = &url.URL{
							Path: "/login",
						}
						redirectURL = func() string {
							path := r.URL.Path
							if r.URL.RawQuery != "" {
								path += "?" + r.URL.RawQuery
							}
							return path
						}()
					)
					if loginURL, ok := getLoginURL(r); ok {
						u = loginURL
						// Don't redirect to the current page, as it may be on
						// a different domain and we have issues determining the
						// scheme to redirect to.
						redirectURL = ""
					}

					q := r.URL.Query()
					q.Add("message", response.Message)
					if redirectURL != "" {
						q.Add("redirect", redirectURL)
					}
					u.RawQuery = q.Encode()

					http.Redirect(rw, r, u.String(), http.StatusTemporaryRedirect)
					return
				}

				httpapi.Write(rw, code, response)
			}

			cookieValue := apiTokenFromRequest(r)
			if cookieValue == "" {
				write(http.StatusUnauthorized, codersdk.Response{
					Message: signedOutErrorMessage,
					Detail:  fmt.Sprintf("Cookie %q or query parameter must be provided.", codersdk.SessionTokenKey),
				})
				return
			}
			parts := strings.Split(cookieValue, "-")
			// APIKeys are formatted: ID-SECRET
			if len(parts) != 2 {
				write(http.StatusUnauthorized, codersdk.Response{
					Message: signedOutErrorMessage,
					Detail:  fmt.Sprintf("Invalid %q cookie API key format.", codersdk.SessionTokenKey),
				})
				return
			}
			keyID := parts[0]
			keySecret := parts[1]
			// Ensuring key lengths are valid.
			if len(keyID) != 10 {
				write(http.StatusUnauthorized, codersdk.Response{
					Message: signedOutErrorMessage,
					Detail:  fmt.Sprintf("Invalid %q cookie API key id.", codersdk.SessionTokenKey),
				})
				return
			}
			if len(keySecret) != 22 {
				write(http.StatusUnauthorized, codersdk.Response{
					Message: signedOutErrorMessage,
					Detail:  fmt.Sprintf("Invalid %q cookie API key secret.", codersdk.SessionTokenKey),
				})
				return
			}
			key, err := db.GetAPIKeyByID(r.Context(), keyID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					write(http.StatusUnauthorized, codersdk.Response{
						Message: signedOutErrorMessage,
						Detail:  "API key is invalid.",
					})
					return
				}
				write(http.StatusInternalServerError, codersdk.Response{
					Message: internalErrorMessage,
					Detail:  fmt.Sprintf("Internal error fetching API key by id. %s", err.Error()),
				})
				return
			}
			hashed := sha256.Sum256([]byte(keySecret))

			// Checking to see if the secret is valid.
			if subtle.ConstantTimeCompare(key.HashedSecret, hashed[:]) != 1 {
				write(http.StatusUnauthorized, codersdk.Response{
					Message: signedOutErrorMessage,
					Detail:  "API key secret is invalid.",
				})
				return
			}
			now := database.Now()
			// Tracks if the API key has properties updated!
			changed := false

			var link database.UserLink
			if key.LoginType != database.LoginTypePassword {
				link, err = db.GetUserLinkByUserIDLoginType(r.Context(), database.GetUserLinkByUserIDLoginTypeParams{
					UserID:    key.UserID,
					LoginType: key.LoginType,
				})
				if err != nil {
					write(http.StatusInternalServerError, codersdk.Response{
						Message: "A database error occurred",
						Detail:  fmt.Sprintf("get user link by user ID and login type: %s", err.Error()),
					})
					return
				}
				// Check if the OAuth token is expired!
				if link.OAuthExpiry.Before(now) && !link.OAuthExpiry.IsZero() {
					var oauthConfig OAuth2Config
					switch key.LoginType {
					case database.LoginTypeGithub:
						oauthConfig = oauth.Github
					case database.LoginTypeOIDC:
						oauthConfig = oauth.OIDC
					default:
						write(http.StatusInternalServerError, codersdk.Response{
							Message: internalErrorMessage,
							Detail:  fmt.Sprintf("Unexpected authentication type %q.", key.LoginType),
						})
						return
					}
					// If it is, let's refresh it from the provided config!
					token, err := oauthConfig.TokenSource(r.Context(), &oauth2.Token{
						AccessToken:  link.OAuthAccessToken,
						RefreshToken: link.OAuthRefreshToken,
						Expiry:       link.OAuthExpiry,
					}).Token()
					if err != nil {
						write(http.StatusUnauthorized, codersdk.Response{
							Message: "Could not refresh expired Oauth token.",
							Detail:  err.Error(),
						})
						return
					}
					link.OAuthAccessToken = token.AccessToken
					link.OAuthRefreshToken = token.RefreshToken
					link.OAuthExpiry = token.Expiry
					key.ExpiresAt = token.Expiry
					changed = true
				}
			}

			// Checking if the key is expired.
			if key.ExpiresAt.Before(now) {
				write(http.StatusUnauthorized, codersdk.Response{
					Message: signedOutErrorMessage,
					Detail:  fmt.Sprintf("API key expired at %q.", key.ExpiresAt.String()),
				})
				return
			}

			// Only update LastUsed once an hour to prevent database spam.
			if now.Sub(key.LastUsed) > time.Hour {
				key.LastUsed = now
				host, _, _ := net.SplitHostPort(r.RemoteAddr)
				remoteIP := net.ParseIP(host)
				if remoteIP == nil {
					remoteIP = net.IPv4(0, 0, 0, 0)
				}
				bitlen := len(remoteIP) * 8
				key.IPAddress = pqtype.Inet{
					IPNet: net.IPNet{
						IP:   remoteIP,
						Mask: net.CIDRMask(bitlen, bitlen),
					},
					Valid: true,
				}
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
					ID:        key.ID,
					LastUsed:  key.LastUsed,
					ExpiresAt: key.ExpiresAt,
					IPAddress: key.IPAddress,
				})
				if err != nil {
					write(http.StatusInternalServerError, codersdk.Response{
						Message: internalErrorMessage,
						Detail:  fmt.Sprintf("API key couldn't update: %s.", err.Error()),
					})
					return
				}
				// If the API Key is associated with a user_link (e.g. Github/OIDC)
				// then we want to update the relevant oauth fields.
				if link.UserID != uuid.Nil {
					link, err = db.UpdateUserLink(r.Context(), database.UpdateUserLinkParams{
						UserID:            link.UserID,
						LoginType:         link.LoginType,
						OAuthAccessToken:  link.OAuthAccessToken,
						OAuthRefreshToken: link.OAuthRefreshToken,
						OAuthExpiry:       link.OAuthExpiry,
					})
					if err != nil {
						write(http.StatusInternalServerError, codersdk.Response{
							Message: internalErrorMessage,
							Detail:  fmt.Sprintf("update user_link: %s.", err.Error()),
						})
						return
					}
				}
			}

			// If the key is valid, we also fetch the user roles and status.
			// The roles are used for RBAC authorize checks, and the status
			// is to block 'suspended' users from accessing the platform.
			roles, err := db.GetAuthorizationUserRoles(r.Context(), key.UserID)
			if err != nil {
				write(http.StatusUnauthorized, codersdk.Response{
					Message: internalErrorMessage,
					Detail:  fmt.Sprintf("Internal error fetching user's roles. %s", err.Error()),
				})
				return
			}

			if roles.Status != database.UserStatusActive {
				write(http.StatusUnauthorized, codersdk.Response{
					Message: fmt.Sprintf("User is not active (status = %q). Contact an admin to reactivate your account.", roles.Status),
				})
				return
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, apiKeyContextKey{}, key)
			ctx = context.WithValue(ctx, userAuthKey{}, Authorization{
				ID:       key.UserID,
				Username: roles.Username,
				Roles:    roles.Roles,
				Scope:    key.Scope,
			})

			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

// apiTokenFromRequest returns the api token from the request.
// Find the session token from:
// 1: The cookie
// 2: The old cookie
// 3. The coder_session_token query parameter
// 4. The custom auth header
func apiTokenFromRequest(r *http.Request) string {
	cookie, err := r.Cookie(codersdk.SessionTokenKey)
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	// TODO: @emyrk in October 2022, remove this oldCookie check.
	//	This is just to support the old cli for 1 release. Then everyone
	//	must update.
	oldCookie, err := r.Cookie("session_token")
	if err == nil && oldCookie.Value != "" {
		return oldCookie.Value
	}

	urlValue := r.URL.Query().Get(codersdk.SessionTokenKey)
	if urlValue != "" {
		return urlValue
	}

	headerValue := r.Header.Get(codersdk.SessionCustomHeader)
	if headerValue != "" {
		return headerValue
	}

	return ""
}

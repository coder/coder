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

	"github.com/google/uuid"
	"github.com/tabbed/pqtype"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/codersdk"
)

// The special cookie name used for subdomain-based application proxying.
// TODO: this will make dogfooding harder so come up with a more unique
// solution
//
//nolint:gosec
const DevURLSessionTokenCookie = "coder_devurl_session_token"

type apiKeyContextKey struct{}

// APIKeyOptional may return an API key from the ExtractAPIKey handler.
func APIKeyOptional(r *http.Request) (database.APIKey, bool) {
	key, ok := r.Context().Value(apiKeyContextKey{}).(database.APIKey)
	return key, ok
}

// APIKey returns the API key from the ExtractAPIKey handler.
func APIKey(r *http.Request) database.APIKey {
	key, ok := APIKeyOptional(r)
	if !ok {
		panic("developer error: ExtractAPIKey middleware not provided")
	}
	return key
}

// User roles are the 'subject' field of Authorize()
type userAuthKey struct{}

type Authorization struct {
	Actor rbac.Subject
	// Username is required for logging and human friendly related
	// identification.
	Username string
}

// UserAuthorizationOptional may return the roles and scope used for
// authorization. Depends on the ExtractAPIKey handler.
func UserAuthorizationOptional(r *http.Request) (Authorization, bool) {
	auth, ok := r.Context().Value(userAuthKey{}).(Authorization)
	return auth, ok
}

// UserAuthorization returns the roles and scope used for authorization. Depends
// on the ExtractAPIKey handler.
func UserAuthorization(r *http.Request) Authorization {
	auth, ok := UserAuthorizationOptional(r)
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
	SignedOutErrorMessage = "You are signed out or your session has expired. Please sign in again to continue."
	internalErrorMessage  = "An internal error occurred. Please try again or contact the system administrator."
)

type ExtractAPIKeyConfig struct {
	DB                          database.Store
	OAuth2Configs               *OAuth2Configs
	RedirectToLogin             bool
	DisableSessionExpiryRefresh bool

	// Optional governs whether the API key is optional. Use this if you want to
	// allow unauthenticated requests.
	//
	// If true and no session token is provided, nothing will be written to the
	// request context. Use the APIKeyOptional and UserAuthorizationOptional
	// functions to retrieve the API key and authorization instead of the
	// regular ones.
	//
	// If true and the API key is invalid (i.e. deleted, expired), the cookie
	// will be deleted and the request will continue. If the request is not a
	// cookie-based request, the request will be rejected with a 401.
	Optional bool
}

// ExtractAPIKey requires authentication using a valid API key. It handles
// extending an API key if it comes close to expiry, updating the last used time
// in the database.
// nolint:revive
func ExtractAPIKey(cfg ExtractAPIKeyConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			// Write wraps writing a response to redirect if the handler
			// specified it should. This redirect is used for user-facing pages
			// like workspace applications.
			write := func(code int, response codersdk.Response) {
				if cfg.RedirectToLogin {
					RedirectToLogin(rw, r, response.Message)
					return
				}

				httpapi.Write(ctx, rw, code, response)
			}

			// optionalWrite wraps write, but will pass the request on to the
			// next handler if the configuration says the API key is optional.
			//
			// It should be used when the API key is not provided or is invalid,
			// but not when there are other errors.
			optionalWrite := func(code int, response codersdk.Response) {
				if cfg.Optional {
					next.ServeHTTP(rw, r)
					return
				}

				write(code, response)
			}

			token := apiTokenFromRequest(r)
			if token == "" {
				optionalWrite(http.StatusUnauthorized, codersdk.Response{
					Message: SignedOutErrorMessage,
					Detail:  fmt.Sprintf("Cookie %q or query parameter must be provided.", codersdk.SessionTokenCookie),
				})
				return
			}

			keyID, keySecret, err := SplitAPIToken(token)
			if err != nil {
				optionalWrite(http.StatusUnauthorized, codersdk.Response{
					Message: SignedOutErrorMessage,
					Detail:  "Invalid API key format: " + err.Error(),
				})
				return
			}

			//nolint:gocritic // System needs to fetch API key to check if it's valid.
			key, err := cfg.DB.GetAPIKeyByID(dbauthz.AsSystemRestricted(ctx), keyID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					optionalWrite(http.StatusUnauthorized, codersdk.Response{
						Message: SignedOutErrorMessage,
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

			// Checking to see if the secret is valid.
			hashedSecret := sha256.Sum256([]byte(keySecret))
			if subtle.ConstantTimeCompare(key.HashedSecret, hashedSecret[:]) != 1 {
				optionalWrite(http.StatusUnauthorized, codersdk.Response{
					Message: SignedOutErrorMessage,
					Detail:  "API key secret is invalid.",
				})
				return
			}

			var (
				link database.UserLink
				now  = database.Now()
				// Tracks if the API key has properties updated
				changed = false
			)
			if key.LoginType == database.LoginTypeGithub || key.LoginType == database.LoginTypeOIDC {
				//nolint:gocritic // System needs to fetch UserLink to check if it's valid.
				link, err = cfg.DB.GetUserLinkByUserIDLoginType(dbauthz.AsSystemRestricted(ctx), database.GetUserLinkByUserIDLoginTypeParams{
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
				// Check if the OAuth token is expired
				if link.OAuthExpiry.Before(now) && !link.OAuthExpiry.IsZero() && link.OAuthRefreshToken != "" {
					var oauthConfig OAuth2Config
					switch key.LoginType {
					case database.LoginTypeGithub:
						oauthConfig = cfg.OAuth2Configs.Github
					case database.LoginTypeOIDC:
						oauthConfig = cfg.OAuth2Configs.OIDC
					default:
						write(http.StatusInternalServerError, codersdk.Response{
							Message: internalErrorMessage,
							Detail:  fmt.Sprintf("Unexpected authentication type %q.", key.LoginType),
						})
						return
					}
					// If it is, let's refresh it from the provided config
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
				optionalWrite(http.StatusUnauthorized, codersdk.Response{
					Message: SignedOutErrorMessage,
					Detail:  fmt.Sprintf("API key expired at %q.", key.ExpiresAt.String()),
				})
				return
			}

			// Only update LastUsed once an hour to prevent database spam.
			if now.Sub(key.LastUsed) > time.Hour {
				key.LastUsed = now
				remoteIP := net.ParseIP(r.RemoteAddr)
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
			if !cfg.DisableSessionExpiryRefresh {
				apiKeyLifetime := time.Duration(key.LifetimeSeconds) * time.Second
				if key.ExpiresAt.Sub(now) <= apiKeyLifetime-time.Hour {
					key.ExpiresAt = now.Add(apiKeyLifetime)
					changed = true
				}
			}
			if changed {
				//nolint:gocritic // System needs to update API Key LastUsed
				err := cfg.DB.UpdateAPIKeyByID(dbauthz.AsSystemRestricted(ctx), database.UpdateAPIKeyByIDParams{
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
					// nolint:gocritic
					link, err = cfg.DB.UpdateUserLink(dbauthz.AsSystemRestricted(ctx), database.UpdateUserLinkParams{
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

				// We only want to update this occasionally to reduce DB write
				// load. We update alongside the UserLink and APIKey since it's
				// easier on the DB to colocate writes.
				// nolint:gocritic
				_, err = cfg.DB.UpdateUserLastSeenAt(dbauthz.AsSystemRestricted(ctx), database.UpdateUserLastSeenAtParams{
					ID:         key.UserID,
					LastSeenAt: database.Now(),
					UpdatedAt:  database.Now(),
				})
				if err != nil {
					write(http.StatusInternalServerError, codersdk.Response{
						Message: internalErrorMessage,
						Detail:  fmt.Sprintf("update user last_seen_at: %s", err.Error()),
					})
					return
				}
			}

			// If the key is valid, we also fetch the user roles and status.
			// The roles are used for RBAC authorize checks, and the status
			// is to block 'suspended' users from accessing the platform.
			// nolint:gocritic
			roles, err := cfg.DB.GetAuthorizationUserRoles(dbauthz.AsSystemRestricted(ctx), key.UserID)
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

			// Actor is the user's authorization context.
			actor := rbac.Subject{
				ID:     key.UserID.String(),
				Roles:  rbac.RoleNames(roles.Roles),
				Groups: roles.Groups,
				Scope:  rbac.ScopeName(key.Scope),
			}
			ctx = context.WithValue(ctx, apiKeyContextKey{}, key)
			ctx = context.WithValue(ctx, userAuthKey{}, Authorization{
				Username: roles.Username,
				Actor:    actor,
			})
			// Set the auth context for the authzquerier as well.
			ctx = dbauthz.As(ctx, actor)

			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}

// apiTokenFromRequest returns the api token from the request.
// Find the session token from:
// 1: The cookie
// 1: The devurl cookie
// 3: The old cookie
// 4. The coder_session_token query parameter
// 5. The custom auth header
func apiTokenFromRequest(r *http.Request) string {
	cookie, err := r.Cookie(codersdk.SessionTokenCookie)
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	urlValue := r.URL.Query().Get(codersdk.SessionTokenCookie)
	if urlValue != "" {
		return urlValue
	}

	headerValue := r.Header.Get(codersdk.SessionTokenHeader)
	if headerValue != "" {
		return headerValue
	}

	cookie, err = r.Cookie(DevURLSessionTokenCookie)
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}

	return ""
}

// SplitAPIToken verifies the format of an API key and returns the split ID and
// secret.
//
// APIKeys are formatted: ${ID}-${SECRET}
func SplitAPIToken(token string) (id string, secret string, err error) {
	parts := strings.Split(token, "-")
	if len(parts) != 2 {
		return "", "", xerrors.Errorf("incorrect amount of API key parts, expected 2 got %d", len(parts))
	}

	// Ensure key lengths are valid.
	keyID := parts[0]
	keySecret := parts[1]
	if len(keyID) != 10 {
		return "", "", xerrors.Errorf("invalid API key ID length, expected 10 got %d", len(keyID))
	}
	if len(keySecret) != 22 {
		return "", "", xerrors.Errorf("invalid API key secret length, expected 22 got %d", len(keySecret))
	}

	return keyID, keySecret, nil
}

// RedirectToLogin redirects the user to the login page with the `message` and
// `redirect` query parameters set.
func RedirectToLogin(rw http.ResponseWriter, r *http.Request, message string) {
	path := r.URL.Path
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}

	q := url.Values{}
	q.Add("message", message)
	q.Add("redirect", path)

	u := &url.URL{
		Path:     "/login",
		RawQuery: q.Encode(),
	}

	http.Redirect(rw, r, u.String(), http.StatusTemporaryRedirect)
}

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
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tabbed/pqtype"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

// The special cookie name used for subdomain-based application proxying.
//
//nolint:gosec
const AppSessionTokenCookie = "coder_app_session_token"

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
	ID       uuid.UUID
	Username string
	Roles    []string
	Scope    database.APIKeyScope
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
	signedOutErrorMessage string = "You are signed out or your session has expired. Please sign in again to continue."
	internalErrorMessage  string = "An internal error occurred. Please try again or contact the system administrator."
)

type TokenSource interface {
	Get(r *http.Request) string
	// strip removes the token from the request if it was set at that source.
	// This is used to prevent the token from being passed to a workspace app.
	Strip(r *http.Request)
}

type tokenSource struct {
	get   func(r *http.Request) string
	strip func(r *http.Request)
}

// Get implements TokenSource.
func (t tokenSource) Get(r *http.Request) string {
	return t.get(r)
}

// Strip implements TokenSource.
func (t tokenSource) Strip(r *http.Request) {
	t.strip(r)
}

var _ TokenSource = tokenSource{}

func TokenSourceHeader(header string) TokenSource {
	return tokenSource{
		get: func(r *http.Request) string {
			return r.Header.Get(header)
		},
		strip: func(r *http.Request) {
			r.Header.Del(header)
		},
	}
}

func TokenSourceQueryParameter(param string) TokenSource {
	return tokenSource{
		get: func(r *http.Request) string {
			return r.URL.Query().Get(param)
		},
		strip: func(r *http.Request) {
			q := r.URL.Query()
			q.Del(param)
			r.URL.RawQuery = q.Encode()
		},
	}
}

func TokenSourceCookie(cookie string) TokenSource {
	cookie = textproto.TrimString(cookie)

	return tokenSource{
		get: func(r *http.Request) string {
			c, err := r.Cookie(cookie)
			if err != nil {
				return ""
			}
			return c.Value
		},
		strip: func(r *http.Request) {
			for i, v := range r.Header["Cookie"] {
				var (
					cookies = []string{}
					header  = textproto.TrimString(v)
					part    string
				)
				for len(header) > 0 { // continue since we have rest
					part, header, _ = strings.Cut(header, ";")
					part = textproto.TrimString(part)
					if part == "" {
						continue
					}
					name, _, _ := strings.Cut(part, "=")
					if textproto.TrimString(name) == cookie {
						continue
					}

					cookies = append(cookies, part)
				}

				r.Header["Cookie"][i] = strings.Join(cookies, "; ")
			}
		},
	}
}

// MultiTokenSource returns a TokenSource that returns the first non-empty token
// on Get. It strips all tokens on Strip.
func MultiTokenSource(sources ...TokenSource) TokenSource {
	return tokenSource{
		get: func(r *http.Request) string {
			for _, src := range sources {
				if token := src.Get(r); token != "" {
					return token
				}
			}
			return ""
		},
		strip: func(r *http.Request) {
			for _, src := range sources {
				src.Strip(r)
			}
		},
	}
}

// DefaultTokenSource is the token source used by the API.
var DefaultTokenSource = MultiTokenSource(
	TokenSourceCookie(codersdk.SessionTokenKey),
	TokenSourceQueryParameter(codersdk.SessionTokenKey),
	TokenSourceHeader(codersdk.SessionCustomHeader),
)

// SubdomainAppTokenSource is the token source used by the subdomain application
// proxying middleware.
var SubdomainAppTokenSource = MultiTokenSource(
	// We don't include the default session token cookie here as it prevents
	// accessing Coder running inside of Coder over app proxying.
	TokenSourceCookie(AppSessionTokenCookie),
	// We don't include the query parameter because this will break websockets
	// if trying to run Coder inside of Coder, and because it's not used on app
	// requests anyways.
	//
	// We don't include the custom header here because it's not used on app
	// requests anyways.
)

type ExtractAPIKeyConfig struct {
	DB              database.Store
	OAuth2Configs   *OAuth2Configs
	RedirectToLogin bool

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

	// TokenSource provides the token from the request. Defaults to
	// DefaultTokenSource.
	TokenSource TokenSource
}

// ExtractAPIKey requires authentication using a valid API key. It handles
// extending an API key if it comes close to expiry, updating the last used time
// in the database.
// nolint:revive
func ExtractAPIKey(cfg ExtractAPIKeyConfig) func(http.Handler) http.Handler {
	if cfg.TokenSource == nil {
		cfg.TokenSource = DefaultTokenSource
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			// Write wraps writing a response to redirect if the handler
			// specified it should. This redirect is used for user-facing pages
			// like workspace applications.
			write := func(code int, response codersdk.Response) {
				if cfg.RedirectToLogin {
					path := r.URL.Path
					if r.URL.RawQuery != "" {
						path += "?" + r.URL.RawQuery
					}

					q := url.Values{}
					q.Add("message", response.Message)
					q.Add("redirect", path)

					u := &url.URL{
						Path:     "/login",
						RawQuery: q.Encode(),
					}

					http.Redirect(rw, r, u.String(), http.StatusTemporaryRedirect)
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

			token := cfg.TokenSource.Get(r)
			if token == "" {
				optionalWrite(http.StatusUnauthorized, codersdk.Response{
					Message: signedOutErrorMessage,
					Detail:  "Authentication is required.",
				})
				return
			}

			keyID, keySecret, err := SplitAPIToken(token)
			if err != nil {
				optionalWrite(http.StatusUnauthorized, codersdk.Response{
					Message: signedOutErrorMessage,
					Detail:  "Invalid API key format: " + err.Error(),
				})
				return
			}

			key, err := cfg.DB.GetAPIKeyByID(r.Context(), keyID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					optionalWrite(http.StatusUnauthorized, codersdk.Response{
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

			// Checking to see if the secret is valid.
			hashedSecret := sha256.Sum256([]byte(keySecret))
			if subtle.ConstantTimeCompare(key.HashedSecret, hashedSecret[:]) != 1 {
				optionalWrite(http.StatusUnauthorized, codersdk.Response{
					Message: signedOutErrorMessage,
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
			if key.LoginType != database.LoginTypePassword {
				link, err = cfg.DB.GetUserLinkByUserIDLoginType(r.Context(), database.GetUserLinkByUserIDLoginTypeParams{
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
				if link.OAuthExpiry.Before(now) && !link.OAuthExpiry.IsZero() {
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
				err := cfg.DB.UpdateAPIKeyByID(r.Context(), database.UpdateAPIKeyByIDParams{
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
					link, err = cfg.DB.UpdateUserLink(r.Context(), database.UpdateUserLinkParams{
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
				_, err = cfg.DB.UpdateUserLastSeenAt(ctx, database.UpdateUserLastSeenAtParams{
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
			roles, err := cfg.DB.GetAuthorizationUserRoles(r.Context(), key.UserID)
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

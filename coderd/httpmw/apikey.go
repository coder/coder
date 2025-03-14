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
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/oauth2"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/promoauth"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
	"github.com/coder/coder/v2/codersdk"
)
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
// UserAuthorizationOptional may return the roles and scope used for
// authorization. Depends on the ExtractAPIKey handler.
func UserAuthorizationOptional(r *http.Request) (rbac.Subject, bool) {
	return dbauthz.ActorFromContext(r.Context())
}
// UserAuthorization returns the roles and scope used for authorization. Depends
// on the ExtractAPIKey handler.
func UserAuthorization(r *http.Request) rbac.Subject {
	auth, ok := UserAuthorizationOptional(r)
	if !ok {
		panic("developer error: ExtractAPIKey middleware not provided")
	}
	return auth
}
// OAuth2Configs is a collection of configurations for OAuth-based authentication.
// This should be extended to support other authentication types in the future.
type OAuth2Configs struct {
	Github promoauth.OAuth2Config
	OIDC   promoauth.OAuth2Config
}
func (c *OAuth2Configs) IsZero() bool {
	if c == nil {
		return true
	}
	return c.Github == nil && c.OIDC == nil
}
const (
	SignedOutErrorMessage = "You are signed out or your session has expired. Please sign in again to continue."
	internalErrorMessage  = "An internal error occurred. Please try again or contact the system administrator."
)
type ExtractAPIKeyConfig struct {
	DB                          database.Store
	ActivateDormantUser         func(ctx context.Context, u database.User) (database.User, error)
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
	// SessionTokenFunc is a custom function that can be used to extract the API
	// key. If nil, the default behavior is used.
	SessionTokenFunc func(r *http.Request) string
	// PostAuthAdditionalHeadersFunc is a function that can be used to add
	// headers to the response after the user has been authenticated.
	//
	// This is originally implemented to send entitlement warning headers after
	// a user is authenticated to prevent additional CLI invocations.
	PostAuthAdditionalHeadersFunc func(a rbac.Subject, header http.Header)
}
// ExtractAPIKeyMW calls ExtractAPIKey with the given config on each request,
// storing the result in the request context.
func ExtractAPIKeyMW(cfg ExtractAPIKeyConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			keyPtr, authzPtr, ok := ExtractAPIKey(rw, r, cfg)
			if !ok {
				return
			}
			if keyPtr == nil || authzPtr == nil {
				// Auth was optional and not provided.
				next.ServeHTTP(rw, r)
				return
			}
			key, authz := *keyPtr, *authzPtr
			// Actor is the user's authorization context.
			ctx := r.Context()
			ctx = context.WithValue(ctx, apiKeyContextKey{}, key)
			// Set the auth context for the user.
			ctx = dbauthz.As(ctx, authz)
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}
}
func APIKeyFromRequest(ctx context.Context, db database.Store, sessionTokenFunc func(r *http.Request) string, r *http.Request) (*database.APIKey, codersdk.Response, bool) {
	tokenFunc := APITokenFromRequest
	if sessionTokenFunc != nil {
		tokenFunc = sessionTokenFunc
	}
	token := tokenFunc(r)
	if token == "" {
		return nil, codersdk.Response{
			Message: SignedOutErrorMessage,
			Detail:  fmt.Sprintf("Cookie %q or query parameter must be provided.", codersdk.SessionTokenCookie),
		}, false
	}
	keyID, keySecret, err := SplitAPIToken(token)
	if err != nil {
		return nil, codersdk.Response{
			Message: SignedOutErrorMessage,
			Detail:  "Invalid API key format: " + err.Error(),
		}, false
	}
	//nolint:gocritic // System needs to fetch API key to check if it's valid.
	key, err := db.GetAPIKeyByID(dbauthz.AsSystemRestricted(ctx), keyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, codersdk.Response{
				Message: SignedOutErrorMessage,
				Detail:  "API key is invalid.",
			}, false
		}
		return nil, codersdk.Response{
			Message: internalErrorMessage,
			Detail:  fmt.Sprintf("Internal error fetching API key by id. %s", err.Error()),
		}, false
	}
	// Checking to see if the secret is valid.
	hashedSecret := sha256.Sum256([]byte(keySecret))
	if subtle.ConstantTimeCompare(key.HashedSecret, hashedSecret[:]) != 1 {
		return nil, codersdk.Response{
			Message: SignedOutErrorMessage,
			Detail:  "API key secret is invalid.",
		}, false
	}
	return &key, codersdk.Response{}, true
}
// ExtractAPIKey requires authentication using a valid API key. It handles
// extending an API key if it comes close to expiry, updating the last used time
// in the database.
//
// If the configuration specifies that the API key is optional, a nil API key
// and authz object may be returned. False is returned if a response was written
// to the request and the caller should give up.
// nolint:revive
func ExtractAPIKey(rw http.ResponseWriter, r *http.Request, cfg ExtractAPIKeyConfig) (*database.APIKey, *rbac.Subject, bool) {
	ctx := r.Context()
	// Write wraps writing a response to redirect if the handler
	// specified it should. This redirect is used for user-facing pages
	// like workspace applications.
	write := func(code int, response codersdk.Response) (*database.APIKey, *rbac.Subject, bool) {
		if cfg.RedirectToLogin {
			RedirectToLogin(rw, r, nil, response.Message)
			return nil, nil, false
		}
		httpapi.Write(ctx, rw, code, response)
		return nil, nil, false
	}
	// optionalWrite wraps write, but will return nil, true if the API key is
	// optional.
	//
	// It should be used when the API key is not provided or is invalid,
	// but not when there are other errors.
	optionalWrite := func(code int, response codersdk.Response) (*database.APIKey, *rbac.Subject, bool) {
		if cfg.Optional {
			return nil, nil, true
		}
		write(code, response)
		return nil, nil, false
	}
	key, resp, ok := APIKeyFromRequest(ctx, cfg.DB, cfg.SessionTokenFunc, r)
	if !ok {
		return optionalWrite(http.StatusUnauthorized, resp)
	}
	var (
		link database.UserLink
		now  = dbtime.Now()
		// Tracks if the API key has properties updated
		changed = false
	)
	if key.LoginType == database.LoginTypeGithub || key.LoginType == database.LoginTypeOIDC {
		var err error
		//nolint:gocritic // System needs to fetch UserLink to check if it's valid.
		link, err = cfg.DB.GetUserLinkByUserIDLoginType(dbauthz.AsSystemRestricted(ctx), database.GetUserLinkByUserIDLoginTypeParams{
			UserID:    key.UserID,
			LoginType: key.LoginType,
		})
		if errors.Is(err, sql.ErrNoRows) {
			return optionalWrite(http.StatusUnauthorized, codersdk.Response{
				Message: SignedOutErrorMessage,
				Detail:  "You must re-authenticate with the login provider.",
			})
		}
		if err != nil {
			return write(http.StatusInternalServerError, codersdk.Response{
				Message: "A database error occurred",
				Detail:  fmt.Sprintf("get user link by user ID and login type: %s", err.Error()),
			})
		}
		// Check if the OAuth token is expired
		if link.OAuthExpiry.Before(now) && !link.OAuthExpiry.IsZero() && link.OAuthRefreshToken != "" {
			if cfg.OAuth2Configs.IsZero() {
				return write(http.StatusInternalServerError, codersdk.Response{
					Message: internalErrorMessage,
					Detail: fmt.Sprintf("Unable to refresh OAuth token for login type %q. "+
						"No OAuth2Configs provided. Contact an administrator to configure this login type.", key.LoginType),
				})
			}
			var oauthConfig promoauth.OAuth2Config
			switch key.LoginType {
			case database.LoginTypeGithub:
				oauthConfig = cfg.OAuth2Configs.Github
			case database.LoginTypeOIDC:
				oauthConfig = cfg.OAuth2Configs.OIDC
			default:
				return write(http.StatusInternalServerError, codersdk.Response{
					Message: internalErrorMessage,
					Detail:  fmt.Sprintf("Unexpected authentication type %q.", key.LoginType),
				})
			}
			// It's possible for cfg.OAuth2Configs to be non-nil, but still
			// missing this type. For example, if a user logged in with GitHub,
			// but the administrator later removed GitHub and replaced it with
			// OIDC.
			if oauthConfig == nil {
				return write(http.StatusInternalServerError, codersdk.Response{
					Message: internalErrorMessage,
					Detail: fmt.Sprintf("Unable to refresh OAuth token for login type %q. "+
						"OAuth2Config not provided. Contact an administrator to configure this login type.", key.LoginType),
				})
			}
			// If it is, let's refresh it from the provided config
			token, err := oauthConfig.TokenSource(r.Context(), &oauth2.Token{
				AccessToken:  link.OAuthAccessToken,
				RefreshToken: link.OAuthRefreshToken,
				Expiry:       link.OAuthExpiry,
			}).Token()
			if err != nil {
				return write(http.StatusUnauthorized, codersdk.Response{
					Message: "Could not refresh expired Oauth token. Try re-authenticating to resolve this issue.",
					Detail:  err.Error(),
				})
			}
			link.OAuthAccessToken = token.AccessToken
			link.OAuthRefreshToken = token.RefreshToken
			link.OAuthExpiry = token.Expiry
			key.ExpiresAt = token.Expiry
			changed = true
		}
	}
	// Checking if the key is expired.
	// NOTE: The `RequireAuth` React component depends on this `Detail` to detect when
	// the users token has expired. If you change the text here, make sure to update it
	// in site/src/components/RequireAuth/RequireAuth.tsx as well.
	if key.ExpiresAt.Before(now) {
		return optionalWrite(http.StatusUnauthorized, codersdk.Response{
			Message: SignedOutErrorMessage,
			Detail:  fmt.Sprintf("API key expired at %q.", key.ExpiresAt.String()),
		})
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
			return write(http.StatusInternalServerError, codersdk.Response{
				Message: internalErrorMessage,
				Detail:  fmt.Sprintf("API key couldn't update: %s.", err.Error()),
			})
		}
		// If the API Key is associated with a user_link (e.g. Github/OIDC)
		// then we want to update the relevant oauth fields.
		if link.UserID != uuid.Nil {
			//nolint:gocritic // system needs to update user link
			link, err = cfg.DB.UpdateUserLink(dbauthz.AsSystemRestricted(ctx), database.UpdateUserLinkParams{
				UserID:                 link.UserID,
				LoginType:              link.LoginType,
				OAuthAccessToken:       link.OAuthAccessToken,
				OAuthAccessTokenKeyID:  sql.NullString{}, // dbcrypt will update as required
				OAuthRefreshToken:      link.OAuthRefreshToken,
				OAuthRefreshTokenKeyID: sql.NullString{}, // dbcrypt will update as required
				OAuthExpiry:            link.OAuthExpiry,
				// Refresh should keep the same debug context because we use
				// the original claims for the group/role sync.
				Claims: link.Claims,
			})
			if err != nil {
				return write(http.StatusInternalServerError, codersdk.Response{
					Message: internalErrorMessage,
					Detail:  fmt.Sprintf("update user_link: %s.", err.Error()),
				})
			}
		}
		// We only want to update this occasionally to reduce DB write
		// load. We update alongside the UserLink and APIKey since it's
		// easier on the DB to colocate writes.
		//nolint:gocritic // system needs to update user last seen at
		_, err = cfg.DB.UpdateUserLastSeenAt(dbauthz.AsSystemRestricted(ctx), database.UpdateUserLastSeenAtParams{
			ID:         key.UserID,
			LastSeenAt: dbtime.Now(),
			UpdatedAt:  dbtime.Now(),
		})
		if err != nil {
			return write(http.StatusInternalServerError, codersdk.Response{
				Message: internalErrorMessage,
				Detail:  fmt.Sprintf("update user last_seen_at: %s", err.Error()),
			})
		}
	}
	// If the key is valid, we also fetch the user roles and status.
	// The roles are used for RBAC authorize checks, and the status
	// is to block 'suspended' users from accessing the platform.
	actor, userStatus, err := UserRBACSubject(ctx, cfg.DB, key.UserID, rbac.ScopeName(key.Scope))
	if err != nil {
		return write(http.StatusUnauthorized, codersdk.Response{
			Message: internalErrorMessage,
			Detail:  fmt.Sprintf("Internal error fetching user's roles. %s", err.Error()),
		})
	}
	if userStatus == database.UserStatusDormant && cfg.ActivateDormantUser != nil {
		id, _ := uuid.Parse(actor.ID)
		user, err := cfg.ActivateDormantUser(ctx, database.User{
			ID:       id,
			Username: actor.FriendlyName,
			Status:   userStatus,
		})
		if err != nil {
			return write(http.StatusInternalServerError, codersdk.Response{
				Message: internalErrorMessage,
				Detail:  fmt.Sprintf("update user status: %s", err.Error()),
			})
		}
		userStatus = user.Status
	}
	if userStatus != database.UserStatusActive {
		return write(http.StatusUnauthorized, codersdk.Response{
			Message: fmt.Sprintf("User is not active (status = %q). Contact an admin to reactivate your account.", userStatus),
		})
	}
	if cfg.PostAuthAdditionalHeadersFunc != nil {
		cfg.PostAuthAdditionalHeadersFunc(actor, rw.Header())
	}
	return key, &actor, true
}
// UserRBACSubject fetches a user's rbac.Subject from the database. It pulls all roles from both
// site and organization scopes. It also pulls the groups, and the user's status.
func UserRBACSubject(ctx context.Context, db database.Store, userID uuid.UUID, scope rbac.ExpandableScope) (rbac.Subject, database.UserStatus, error) {
	//nolint:gocritic // system needs to update user roles
	roles, err := db.GetAuthorizationUserRoles(dbauthz.AsSystemRestricted(ctx), userID)
	if err != nil {
		return rbac.Subject{}, "", fmt.Errorf("get authorization user roles: %w", err)
	}
	roleNames, err := roles.RoleNames()
	if err != nil {
		return rbac.Subject{}, "", fmt.Errorf("expand role names: %w", err)
	}
	//nolint:gocritic // Permission to lookup custom roles the user has assigned.
	rbacRoles, err := rolestore.Expand(dbauthz.AsSystemRestricted(ctx), db, roleNames)
	if err != nil {
		return rbac.Subject{}, "", fmt.Errorf("expand role names: %w", err)
	}
	actor := rbac.Subject{
		FriendlyName: roles.Username,
		ID:           userID.String(),
		Roles:        rbacRoles,
		Groups:       roles.Groups,
		Scope:        scope,
	}.WithCachedASTValue()
	return actor, roles.Status, nil
}
// APITokenFromRequest returns the api token from the request.
// Find the session token from:
// 1: The cookie
// 2. The coder_session_token query parameter
// 3. The custom auth header
//
// API tokens for apps are read from workspaceapps/cookies.go.
func APITokenFromRequest(r *http.Request) string {
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
	return ""
}
// SplitAPIToken verifies the format of an API key and returns the split ID and
// secret.
//
// APIKeys are formatted: ${ID}-${SECRET}
func SplitAPIToken(token string) (id string, secret string, err error) {
	parts := strings.Split(token, "-")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("incorrect amount of API key parts, expected 2 got %d", len(parts))
	}
	// Ensure key lengths are valid.
	keyID := parts[0]
	keySecret := parts[1]
	if len(keyID) != 10 {
		return "", "", fmt.Errorf("invalid API key ID length, expected 10 got %d", len(keyID))
	}
	if len(keySecret) != 22 {
		return "", "", fmt.Errorf("invalid API key secret length, expected 22 got %d", len(keySecret))
	}
	return keyID, keySecret, nil
}
// RedirectToLogin redirects the user to the login page with the `message` and
// `redirect` query parameters set.
//
// If dashboardURL is nil, the redirect will be relative to the current
// request's host. If it is not nil, the redirect will be absolute with dashboard
// url as the host.
func RedirectToLogin(rw http.ResponseWriter, r *http.Request, dashboardURL *url.URL, message string) {
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
	// If dashboardURL is provided, we want to redirect to the dashboard
	// login page.
	if dashboardURL != nil {
		cpy := *dashboardURL
		cpy.Path = u.Path
		cpy.RawQuery = u.RawQuery
		u = &cpy
	}
	// See other forces a GET request rather than keeping the current method
	// (like temporary redirect does).
	http.Redirect(rw, r, u.String(), http.StatusSeeOther)
}
// CustomRedirectToLogin redirects the user to the login page with the `message` and
// `redirect` query parameters set, with a provided code
func CustomRedirectToLogin(rw http.ResponseWriter, r *http.Request, redirect string, message string, code int) {
	q := url.Values{}
	q.Add("message", message)
	q.Add("redirect", redirect)
	u := &url.URL{
		Path:     "/login",
		RawQuery: q.Encode(),
	}
	http.Redirect(rw, r, u.String(), code)
}

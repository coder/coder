package httpmw

import (
	"context"
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
	"golang.org/x/net/idna"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/coderd/apikey"
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
func UserAuthorizationOptional(ctx context.Context) (rbac.Subject, bool) {
	return dbauthz.ActorFromContext(ctx)
}

// UserAuthorization returns the roles and scope used for authorization. Depends
// on the ExtractAPIKey handler.
func UserAuthorization(ctx context.Context) rbac.Subject {
	auth, ok := UserAuthorizationOptional(ctx)
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

	// AccessURL is the configured access URL for this Coder deployment.
	// Used for generating OAuth2 resource metadata URLs in WWW-Authenticate headers.
	AccessURL *url.URL

	// Logger is used for logging middleware operations.
	Logger slog.Logger
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
	if !apikey.ValidateHash(key.HashedSecret, keySecret) {
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
	write := func(code int, response codersdk.Response) (apiKey *database.APIKey, subject *rbac.Subject, ok bool) {
		if cfg.RedirectToLogin {
			RedirectToLogin(rw, r, nil, response.Message)
			return nil, nil, false
		}

		// Add WWW-Authenticate header for 401/403 responses (RFC 6750 + RFC 9728)
		if code == http.StatusUnauthorized || code == http.StatusForbidden {
			rw.Header().Set("WWW-Authenticate", buildWWWAuthenticateHeader(cfg.AccessURL, r, code, response))
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

	now := dbtime.Now()
	if key.ExpiresAt.Before(now) {
		return optionalWrite(http.StatusUnauthorized, codersdk.Response{
			Message: SignedOutErrorMessage,
			Detail:  fmt.Sprintf("API key expired at %q.", key.ExpiresAt.String()),
		})
	}

	// Validate OAuth2 provider app token audience (RFC 8707) if applicable
	if key.LoginType == database.LoginTypeOAuth2ProviderApp {
		if err := validateOAuth2ProviderAppTokenAudience(ctx, cfg.DB, *key, cfg.AccessURL, r); err != nil {
			// Log the detailed error for debugging but don't expose it to the client
			cfg.Logger.Debug(ctx, "oauth2 token audience validation failed", slog.Error(err))
			return optionalWrite(http.StatusForbidden, codersdk.Response{
				Message: "Token audience validation failed",
			})
		}
	}

	// We only check OIDC stuff if we have a valid APIKey. An expired key means we don't trust the requestor
	// really is the user whose key they have, and so we shouldn't be doing anything on their behalf including possibly
	// refreshing the OIDC token.
	if key.LoginType == database.LoginTypeGithub || key.LoginType == database.LoginTypeOIDC {
		var err error
		//nolint:gocritic // System needs to fetch UserLink to check if it's valid.
		link, err := cfg.DB.GetUserLinkByUserIDLoginType(dbauthz.AsSystemRestricted(ctx), database.GetUserLinkByUserIDLoginTypeParams{
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
		if !link.OAuthExpiry.IsZero() && link.OAuthExpiry.Before(now) {
			if cfg.OAuth2Configs.IsZero() {
				return write(http.StatusInternalServerError, codersdk.Response{
					Message: internalErrorMessage,
					Detail: fmt.Sprintf("Unable to refresh OAuth token for login type %q. "+
						"No OAuth2Configs provided. Contact an administrator to configure this login type.", key.LoginType),
				})
			}

			var friendlyName string
			var oauthConfig promoauth.OAuth2Config
			switch key.LoginType {
			case database.LoginTypeGithub:
				oauthConfig = cfg.OAuth2Configs.Github
				friendlyName = "GitHub"
			case database.LoginTypeOIDC:
				oauthConfig = cfg.OAuth2Configs.OIDC
				friendlyName = "OpenID Connect"
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

			if link.OAuthRefreshToken == "" {
				return optionalWrite(http.StatusUnauthorized, codersdk.Response{
					Message: SignedOutErrorMessage,
					Detail:  fmt.Sprintf("%s session expired at %q. Try signing in again.", friendlyName, link.OAuthExpiry.String()),
				})
			}
			// We have a refresh token, so let's try it
			token, err := oauthConfig.TokenSource(r.Context(), &oauth2.Token{
				AccessToken:  link.OAuthAccessToken,
				RefreshToken: link.OAuthRefreshToken,
				Expiry:       link.OAuthExpiry,
			}).Token()
			if err != nil {
				return write(http.StatusUnauthorized, codersdk.Response{
					Message: fmt.Sprintf(
						"Could not refresh expired %s token. Try re-authenticating to resolve this issue.",
						friendlyName),
					Detail: err.Error(),
				})
			}
			link.OAuthAccessToken = token.AccessToken
			link.OAuthRefreshToken = token.RefreshToken
			link.OAuthExpiry = token.Expiry
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
	}

	// Tracks if the API key has properties updated
	changed := false

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
	actor, userStatus, err := UserRBACSubject(ctx, cfg.DB, key.UserID, key.ScopeSet())
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

// validateOAuth2ProviderAppTokenAudience validates that an OAuth2 provider app token
// is being used with the correct audience/resource server (RFC 8707).
func validateOAuth2ProviderAppTokenAudience(ctx context.Context, db database.Store, key database.APIKey, accessURL *url.URL, r *http.Request) error {
	// Get the OAuth2 provider app token to check its audience
	//nolint:gocritic // System needs to access token for audience validation
	token, err := db.GetOAuth2ProviderAppTokenByAPIKeyID(dbauthz.AsSystemRestricted(ctx), key.ID)
	if err != nil {
		return xerrors.Errorf("failed to get OAuth2 token: %w", err)
	}

	// If no audience is set, allow the request (for backward compatibility)
	if !token.Audience.Valid || token.Audience.String == "" {
		return nil
	}

	// Extract the expected audience from the access URL
	expectedAudience := extractExpectedAudience(accessURL, r)

	// Normalize both audience values for RFC 3986 compliant comparison
	normalizedTokenAudience := normalizeAudienceURI(token.Audience.String)
	normalizedExpectedAudience := normalizeAudienceURI(expectedAudience)

	// Validate that the token's audience matches the expected audience
	if normalizedTokenAudience != normalizedExpectedAudience {
		return xerrors.Errorf("token audience %q does not match expected audience %q",
			token.Audience.String, expectedAudience)
	}

	return nil
}

// normalizeAudienceURI implements RFC 3986 URI normalization for OAuth2 audience comparison.
// This ensures consistent audience matching between authorization and token validation.
func normalizeAudienceURI(audienceURI string) string {
	if audienceURI == "" {
		return ""
	}

	u, err := url.Parse(audienceURI)
	if err != nil {
		// If parsing fails, return as-is to avoid breaking existing functionality
		return audienceURI
	}

	// Apply RFC 3986 syntax-based normalization:

	// 1. Scheme normalization - case-insensitive
	u.Scheme = strings.ToLower(u.Scheme)

	// 2. Host normalization - case-insensitive and IDN (punnycode) normalization
	u.Host = normalizeHost(u.Host)

	// 3. Remove default ports for HTTP/HTTPS
	if (u.Scheme == "http" && strings.HasSuffix(u.Host, ":80")) ||
		(u.Scheme == "https" && strings.HasSuffix(u.Host, ":443")) {
		// Extract host without default port
		if idx := strings.LastIndex(u.Host, ":"); idx > 0 {
			u.Host = u.Host[:idx]
		}
	}

	// 4. Path normalization including dot-segment removal (RFC 3986 Section 6.2.2.3)
	u.Path = normalizePathSegments(u.Path)

	// 5. Remove fragment - should already be empty due to earlier validation,
	// but clear it as a safety measure in case validation was bypassed
	if u.Fragment != "" {
		// This should not happen if validation is working correctly
		u.Fragment = ""
	}

	// 6. Keep query parameters as-is (rarely used in audience URIs but preserved for compatibility)

	return u.String()
}

// normalizeHost performs host normalization including case-insensitive conversion
// and IDN (Internationalized Domain Name) punnycode normalization.
func normalizeHost(host string) string {
	if host == "" {
		return host
	}

	// Handle IPv6 addresses - they are enclosed in brackets
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		// IPv6 addresses should be normalized to lowercase
		return strings.ToLower(host)
	}

	// Extract port if present
	var port string
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		// Check if this is actually a port (not part of IPv6)
		if !strings.Contains(host[idx+1:], ":") {
			port = host[idx:]
			host = host[:idx]
		}
	}

	// Convert to lowercase for case-insensitive comparison
	host = strings.ToLower(host)

	// Apply IDN normalization - convert Unicode domain names to ASCII (punnycode)
	if normalizedHost, err := idna.ToASCII(host); err == nil {
		host = normalizedHost
	}
	// If IDN conversion fails, continue with lowercase version

	return host + port
}

// normalizePathSegments normalizes path segments for consistent OAuth2 audience matching.
// Uses url.URL.ResolveReference() which implements RFC 3986 dot-segment removal.
func normalizePathSegments(path string) string {
	if path == "" {
		// If no path is specified, use "/" for consistency with RFC 8707 examples
		return "/"
	}

	// Use url.URL.ResolveReference() to handle dot-segment removal per RFC 3986
	base := &url.URL{Path: "/"}
	ref := &url.URL{Path: path}
	resolved := base.ResolveReference(ref)

	normalizedPath := resolved.Path

	// Remove trailing slash from paths longer than "/" to normalize
	// This ensures "/api/" and "/api" are treated as equivalent
	if len(normalizedPath) > 1 && strings.HasSuffix(normalizedPath, "/") {
		normalizedPath = strings.TrimSuffix(normalizedPath, "/")
	}

	return normalizedPath
}

// Test export functions for testing package access

// buildWWWAuthenticateHeader constructs RFC 6750 + RFC 9728 compliant WWW-Authenticate header
func buildWWWAuthenticateHeader(accessURL *url.URL, r *http.Request, code int, response codersdk.Response) string {
	// Use the configured access URL for resource metadata
	if accessURL == nil {
		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}

		// Use the Host header to construct the canonical audience URI
		accessURL = &url.URL{
			Scheme: scheme,
			Host:   r.Host,
		}
	}

	resourceMetadata := accessURL.JoinPath("/.well-known/oauth-protected-resource").String()

	switch code {
	case http.StatusUnauthorized:
		switch {
		case strings.Contains(response.Message, "expired") || strings.Contains(response.Detail, "expired"):
			return fmt.Sprintf(`Bearer realm="coder", error="invalid_token", error_description="The access token has expired", resource_metadata=%q`, resourceMetadata)
		case strings.Contains(response.Message, "audience") || strings.Contains(response.Message, "mismatch"):
			return fmt.Sprintf(`Bearer realm="coder", error="invalid_token", error_description="The access token audience does not match this resource", resource_metadata=%q`, resourceMetadata)
		default:
			return fmt.Sprintf(`Bearer realm="coder", error="invalid_token", error_description="The access token is invalid", resource_metadata=%q`, resourceMetadata)
		}
	case http.StatusForbidden:
		return fmt.Sprintf(`Bearer realm="coder", error="insufficient_scope", error_description="The request requires higher privileges than provided by the access token", resource_metadata=%q`, resourceMetadata)
	default:
		return fmt.Sprintf(`Bearer realm="coder", resource_metadata=%q`, resourceMetadata)
	}
}

// extractExpectedAudience determines the expected audience for the current request.
// This should match the resource parameter used during authorization.
func extractExpectedAudience(accessURL *url.URL, r *http.Request) string {
	// For MCP compliance, the audience should be the canonical URI of the resource server
	// This typically matches the access URL of the Coder deployment
	var audience string

	if accessURL != nil {
		audience = accessURL.String()
	} else {
		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}

		// Use the Host header to construct the canonical audience URI
		audience = fmt.Sprintf("%s://%s", scheme, r.Host)
	}

	// Normalize the URI according to RFC 3986 for consistent comparison
	return normalizeAudienceURI(audience)
}

// UserRBACSubject fetches a user's rbac.Subject from the database. It pulls all roles from both
// site and organization scopes. It also pulls the groups, and the user's status.
func UserRBACSubject(ctx context.Context, db database.Store, userID uuid.UUID, scope rbac.ExpandableScope) (rbac.Subject, database.UserStatus, error) {
	//nolint:gocritic // system needs to update user roles
	roles, err := db.GetAuthorizationUserRoles(dbauthz.AsSystemRestricted(ctx), userID)
	if err != nil {
		return rbac.Subject{}, "", xerrors.Errorf("get authorization user roles: %w", err)
	}

	roleNames, err := roles.RoleNames()
	if err != nil {
		return rbac.Subject{}, "", xerrors.Errorf("expand role names: %w", err)
	}

	//nolint:gocritic // Permission to lookup custom roles the user has assigned.
	rbacRoles, err := rolestore.Expand(dbauthz.AsSystemRestricted(ctx), db, roleNames)
	if err != nil {
		return rbac.Subject{}, "", xerrors.Errorf("expand role names: %w", err)
	}

	actor := rbac.Subject{
		Type:         rbac.SubjectTypeUser,
		FriendlyName: roles.Username,
		Email:        roles.Email,
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
// 4. RFC 6750 Authorization: Bearer header
// 5. RFC 6750 access_token query parameter
//
// API tokens for apps are read from workspaceapps/cookies.go.
func APITokenFromRequest(r *http.Request) string {
	// Prioritize existing Coder custom authentication methods first
	// to maintain backward compatibility and existing behavior

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

	// RFC 6750 Bearer Token support (added as fallback methods)
	// Check Authorization: Bearer <token> header (case-insensitive per RFC 6750)
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		// Skip "Bearer " (7 characters) and trim surrounding whitespace
		return strings.TrimSpace(authHeader[7:])
	}

	// Check access_token query parameter
	accessToken := r.URL.Query().Get("access_token")
	if accessToken != "" {
		return strings.TrimSpace(accessToken)
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

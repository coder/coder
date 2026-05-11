package httpmw

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// ExternalAuthHeaderName is the request header used by external
// authentication gateways to assert the authenticated user. See
// ExternalAuthHeaderConfig for the threat model.
const ExternalAuthHeaderName = "Coder-Authorization"

// ExternalAuthHeaderCreateUserParams carries the user identity asserted
// by the Coder-Authorization header to a deployment-supplied callback
// that creates the Coder user on first sight. Auto-creation is gated
// behind ExternalAuthHeaderConfig.AllowAutoCreateUsers.
type ExternalAuthHeaderCreateUserParams struct {
	// Username and Email are read from the header. Email is required;
	// the header parser rejects auto-create attempts without it.
	Username string
	Email    string
	// Name is the user's full name. Optional. Mirrors the OIDC flow,
	// which also accepts a real-name claim.
	Name string
	// Roles is the deployment-effective role list for the new user.
	// It comes from the header's Roles= field if present, otherwise
	// the config's AutoCreateDefaultRoles. The callback validates
	// and assigns these.
	Roles []string
}

// ExternalAuthHeaderConfig configures trust of the
// Coder-Authorization header used by external authentication gateways.
//
// When Enabled is true and the request originates from an address in
// TrustedOrigins, the middleware accepts the gateway's user identity
// assertion in lieu of a Coder session token. The header has the
// format:
//
//	Coder-Authorization: Basic Username=alice
//	Coder-Authorization: Basic UserEmail=alice@example.com
//	Coder-Authorization: Basic Username=alice, UserEmail=alice@example.com, Roles=member
//
// At least one of Username or UserEmail must be supplied. Auto-creation
// additionally requires UserEmail (and Username, derived from the local
// part if absent). Other fields are accepted and ignored for forward
// compatibility.
//
// SECURITY: this header is fully trusted on trusted origins. A
// misconfigured deployment that lists a network containing untrusted
// clients will allow those clients to impersonate any user. Use only
// when Coder is bound to localhost or sits behind an authenticating
// reverse proxy on a network you control. Implements the design
// proposed in https://github.com/coder/coder/issues/8889.
type ExternalAuthHeaderConfig struct {
	// Enabled gates the entire feature. When false, the header is
	// ignored everywhere and callers fall back to normal session-token
	// authentication.
	Enabled bool

	// TrustedOrigins is the list of CIDR ranges whose source addresses
	// may assert user identity via the header. An empty list with
	// Enabled=true is a misconfiguration: the header will never be
	// honored.
	TrustedOrigins []*net.IPNet

	// AllowAutoCreateUsers controls whether a header that names a
	// previously unknown user causes that user to be provisioned on
	// the fly. When false (the default), an unknown user fails the
	// request with a 401. Auto-creation requires a non-empty Email
	// in the header or a deployment that synthesizes one.
	AllowAutoCreateUsers bool

	// AutoCreateDefaultRoles is the role list assigned to a freshly
	// created user when the header does not carry a Roles= field.
	// Nil or empty means a plain member with no extra site roles.
	AutoCreateDefaultRoles []string

	// CreateUser is the deployment-supplied callback that performs the
	// actual user creation. It runs only when AllowAutoCreateUsers is
	// true and the looked-up user is missing. nil disables auto-create
	// even if AllowAutoCreateUsers is true.
	//
	// Wired by coderd.New so the callback has access to the full user
	// creation path (default org, SSH keypair generation, audit, and
	// notifications), without httpmw importing coderd.
	CreateUser func(context.Context, ExternalAuthHeaderCreateUserParams) (database.User, error)
}

// externalAuthHeader holds the parsed contents of a
// Coder-Authorization: Basic header.
type externalAuthHeader struct {
	Username string
	Email    string
	Name     string
	Roles    []string
}

// hasIdentity returns true if the parsed header carries a usable
// user identity field.
func (h externalAuthHeader) hasIdentity() bool {
	return h.Username != "" || h.Email != ""
}

// errNoExternalAuthHeader signals that the Coder-Authorization header
// is absent. Callers fall back to normal session-token authentication
// in that case.
var errNoExternalAuthHeader = xerrors.New("no external auth header")

// parseExternalAuthHeader parses a Coder-Authorization header value.
// Currently only the Basic scheme is supported; future schemes (e.g.
// signed JWTs) can be added without breaking callers.
//
// The Basic scheme uses comma-separated Field=Value pairs:
//
//	Basic Username=alice
//	Basic UserEmail=alice@example.com, Roles=member
//	Basic Username=alice, UserEmail=alice@example.com, Name=Alice Liddell, Roles=member,auditor
//
// Unknown fields are accepted and ignored. The function returns
// errNoExternalAuthHeader when value is empty so callers can
// distinguish "not present" from "malformed".
func parseExternalAuthHeader(value string) (externalAuthHeader, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return externalAuthHeader{}, errNoExternalAuthHeader
	}

	scheme, rest, ok := strings.Cut(value, " ")
	if !ok {
		return externalAuthHeader{}, xerrors.Errorf("missing scheme in %s header", ExternalAuthHeaderName)
	}
	if !strings.EqualFold(scheme, "Basic") {
		return externalAuthHeader{}, xerrors.Errorf("unsupported %s scheme %q", ExternalAuthHeaderName, scheme)
	}

	var parsed externalAuthHeader
	// The Roles field is itself comma-separated, but the outer
	// header is also comma-separated. We parse left-to-right and
	// only treat Roles= specially: once we see it, the rest of the
	// header is the role list. This matches the issue's example
	// "Roles=member,auditor" without forcing the gateway to quote
	// or escape commas.
	remaining := rest
	for remaining != "" {
		var pair string
		if i := strings.IndexByte(remaining, ','); i >= 0 {
			pair = remaining[:i]
			remaining = remaining[i+1:]
		} else {
			pair = remaining
			remaining = ""
		}
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		key, val, ok := strings.Cut(pair, "=")
		if !ok {
			return externalAuthHeader{}, xerrors.Errorf("malformed field %q in %s header (expected Key=Value)", pair, ExternalAuthHeaderName)
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch strings.ToLower(key) {
		case "username":
			parsed.Username = val
		case "useremail":
			parsed.Email = val
		case "name":
			parsed.Name = val
		case "roles":
			// Roles consumes the rest of the header as a single
			// comma-separated list, so a value like
			// "Roles=member,auditor" is preserved.
			full := val
			if remaining != "" {
				full = full + "," + remaining
				remaining = ""
			}
			for _, role := range strings.Split(full, ",") {
				role = strings.TrimSpace(role)
				if role == "" {
					continue
				}
				parsed.Roles = append(parsed.Roles, role)
			}
		default:
			// Ignore unknown fields. The original proposal lists
			// ActiveSession and TokenName, which we accept and
			// ignore so deployments that already send them keep
			// working as we add support over time.
		}
	}

	if !parsed.hasIdentity() {
		return externalAuthHeader{}, xerrors.Errorf("%s header must include Username or UserEmail", ExternalAuthHeaderName)
	}

	return parsed, nil
}

// ParseExternalAuthHeaderConfig builds an ExternalAuthHeaderConfig
// from the deployment values. trustedOrigins is a list of CIDR
// strings; an empty list with enabled=true is allowed but logs a
// warning at construction time (the feature is silently disabled
// because no origin can match).
//
// The CreateUser callback must be set separately by coderd.New so
// that this parser stays free of import cycles with the user-creation
// machinery.
func ParseExternalAuthHeaderConfig(enabled bool, trustedOrigins []string, allowAutoCreate bool, autoCreateDefaultRoles []string) (ExternalAuthHeaderConfig, error) {
	cfg := ExternalAuthHeaderConfig{
		Enabled:                enabled,
		AllowAutoCreateUsers:   allowAutoCreate,
		AutoCreateDefaultRoles: append([]string(nil), autoCreateDefaultRoles...),
	}
	for _, origin := range trustedOrigins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		_, network, err := net.ParseCIDR(origin)
		if err != nil {
			return ExternalAuthHeaderConfig{}, xerrors.Errorf("parse external auth header trusted origin %q: %w", origin, err)
		}
		cfg.TrustedOrigins = append(cfg.TrustedOrigins, network)
	}
	return cfg, nil
}

// originAllowed returns true if the request's source address is
// inside one of the trusted CIDR ranges. An empty range list never
// matches.
func (c ExternalAuthHeaderConfig) originAllowed(remoteAddr string) bool {
	if len(c.TrustedOrigins) == 0 {
		return false
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// RemoteAddr is sometimes a bare IP (e.g. in tests) without
		// a port. Try the value as-is.
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, network := range c.TrustedOrigins {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// validateExternalAuthHeader inspects the Coder-Authorization header
// and, when accepted, returns a successful ValidateAPIKeyResult with
// a synthesized in-memory APIKey. The synthesized key is never
// persisted: it exists only to satisfy downstream middleware that
// expects a database.APIKey shape.
//
// Returns (nil, nil, false) when the feature is disabled, the header
// is absent, or the request did not come from a trusted origin. In
// those cases callers fall through to normal session-token
// authentication.
//
// Returns (nil, *ValidateAPIKeyError, true) when the header is
// present but cannot be honored (malformed, user not found, etc.).
// The error is hard so the caller surfaces it even on optional-auth
// routes; otherwise an attacker on a trusted origin could mask a
// failed impersonation by simply omitting cookies.
func validateExternalAuthHeader(
	ctx context.Context,
	cfg ExternalAuthHeaderConfig,
	db database.Store,
	logger slog.Logger,
	r *http.Request,
) (*ValidateAPIKeyResult, *ValidateAPIKeyError, bool) {
	if !cfg.Enabled {
		return nil, nil, false
	}
	raw := r.Header.Get(ExternalAuthHeaderName)
	if raw == "" {
		return nil, nil, false
	}

	if !cfg.originAllowed(r.RemoteAddr) {
		// Header was set but the origin is untrusted. Log loudly
		// and fall through to normal auth so we don't pretend the
		// header is gospel from an arbitrary network.
		logger.Warn(ctx, "ignoring external auth header from untrusted origin",
			slog.F("remote_addr", r.RemoteAddr),
		)
		return nil, nil, false
	}

	parsed, err := parseExternalAuthHeader(raw)
	if err != nil {
		if errors.Is(err, errNoExternalAuthHeader) {
			return nil, nil, false
		}
		return nil, &ValidateAPIKeyError{
			Code: http.StatusBadRequest,
			Response: codersdk.Response{
				Message: SignedOutErrorMessage,
				Detail:  fmt.Sprintf("Invalid %s header: %s", ExternalAuthHeaderName, err.Error()),
			},
			Hard: true,
		}, true
	}

	// nolint:gocritic // System needs to look up the asserted user
	// regardless of the caller's RBAC.
	user, err := db.GetUserByEmailOrUsername(dbauthz.AsSystemRestricted(ctx), database.GetUserByEmailOrUsernameParams{
		Username: parsed.Username,
		Email:    parsed.Email,
	})
	switch {
	case err == nil:
		// Found, fall through to RBAC resolution below.
	case errors.Is(err, sql.ErrNoRows) && cfg.AllowAutoCreateUsers && cfg.CreateUser != nil:
		// Unknown user, auto-create is enabled, callback wired.
		user, err = autoCreateExternalAuthHeaderUser(ctx, cfg, parsed)
		if err != nil {
			return nil, &ValidateAPIKeyError{
				Code: http.StatusUnauthorized,
				Response: codersdk.Response{
					Message: SignedOutErrorMessage,
					Detail:  fmt.Sprintf("%s header user auto-create failed: %s", ExternalAuthHeaderName, err.Error()),
				},
				Hard: true,
			}, true
		}
	default:
		// We treat all lookup failures (no rows without auto-create,
		// permission errors, real DB errors) as a hard 401 rather
		// than leaking which case occurred. The Detail field carries
		// enough context for an operator to diagnose, while the
		// Message remains the standard signed-out copy.
		return nil, &ValidateAPIKeyError{
			Code: http.StatusUnauthorized,
			Response: codersdk.Response{
				Message: SignedOutErrorMessage,
				Detail:  fmt.Sprintf("%s header user lookup failed: %s", ExternalAuthHeaderName, err.Error()),
			},
			Hard: true,
		}, true
	}

	// The shared GetUserByEmailOrUsername query already filters
	// out deleted=true rows. Status is then enforced by the
	// route-specific check in ExtractAPIKey via the returned
	// UserStatus, matching the cookie-based flow.

	// Build a synthetic in-memory APIKey. The values that matter
	// downstream are UserID (ratelimit, RBAC actor lookup),
	// LoginType (skips OAuth refresh), and Scopes (RBAC scope set).
	// ID/HashedSecret/ExpiresAt are set to non-empty placeholders
	// so accidental DB writes would be obvious.
	key := database.APIKey{
		ID:        "external",
		UserID:    user.ID,
		LoginType: database.LoginTypeNone,
		Scopes:    database.APIKeyScopes{database.ApiKeyScopeCoderAll},
		// AllowList must contain at least the wildcard element so
		// IntersectAllowLists doesn't fail closed for a key with no
		// stored row. The synthesized session has no DB-level
		// resource scoping; it inherits the user's full RBAC.
		AllowList: database.AllowList{rbac.AllowListAll()},
	}

	actor, userStatus, err := UserRBACSubject(ctx, db, user.ID, key.ScopeSet())
	if err != nil {
		return nil, &ValidateAPIKeyError{
			Code: http.StatusInternalServerError,
			Response: codersdk.Response{
				Message: internalErrorMessage,
				Detail:  fmt.Sprintf("fetch %s header user roles: %s", ExternalAuthHeaderName, err.Error()),
			},
			Hard: true,
		}, true
	}

	return &ValidateAPIKeyResult{
		Key:        key,
		Subject:    actor,
		UserStatus: userStatus,
	}, nil, true
}

// autoCreateExternalAuthHeaderUser materializes a previously unknown
// user asserted by the Coder-Authorization header. The header must
// supply an email; auto-create cannot synthesize one because Coder
// users own a real email address. Username defaults to the local
// part of the email when the header omits it.
//
// Role assignment precedence (issue #8889): the header's Roles= field
// wins; otherwise AutoCreateDefaultRoles from the deployment config.
// A nil/empty result hands a plain member to the underlying
// CreateUser callback.
func autoCreateExternalAuthHeaderUser(
	ctx context.Context,
	cfg ExternalAuthHeaderConfig,
	parsed externalAuthHeader,
) (database.User, error) {
	email := parsed.Email
	if email == "" {
		return database.User{}, xerrors.Errorf("UserEmail is required to auto-create a user via the %s header", ExternalAuthHeaderName)
	}
	username := parsed.Username
	if username == "" {
		// Derive a username from the email's local part. The
		// downstream codersdk.NameValid check still applies in
		// the CreateUser callback, so an invalid derivation
		// surfaces as a clear "invalid username" error rather
		// than silently corrupting the row.
		if at := strings.IndexByte(email, '@'); at > 0 {
			username = email[:at]
		} else {
			username = email
		}
	}

	roles := parsed.Roles
	if len(roles) == 0 {
		roles = cfg.AutoCreateDefaultRoles
	}

	return cfg.CreateUser(ctx, ExternalAuthHeaderCreateUserParams{
		Username: username,
		Email:    email,
		Name:     parsed.Name,
		Roles:    roles,
	})
}

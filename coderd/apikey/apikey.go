package apikey

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/namesgenerator"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/cryptorand"
)

type CreateParams struct {
	UserID    uuid.UUID
	LoginType database.LoginType
	// DefaultLifetime is configured in DeploymentValues.
	// It is used if both ExpiresAt and LifetimeSeconds are not set.
	DefaultLifetime time.Duration

	// Optional.
	ExpiresAt       time.Time
	LifetimeSeconds int64

	// Scope is legacy single-scope input kept for backward compatibility.
	//
	// Deprecated: use Scopes instead.
	Scope database.APIKeyScope
	// Scopes is the full list of scopes to attach to the key.
	Scopes     database.APIKeyScopes
	TokenName  string
	RemoteAddr string
	// AllowList is an optional, normalized allow-list
	// of resource type and uuid entries. If empty, defaults to wildcard.
	AllowList database.AllowList
}

// Generate generates an API key, returning the key as a string as well as the
// database representation. It is the responsibility of the caller to insert it
// into the database.
func Generate(params CreateParams) (database.InsertAPIKeyParams, string, error) {
	// Length of an API Key ID.
	keyID, err := cryptorand.String(10)
	if err != nil {
		return database.InsertAPIKeyParams{}, "", xerrors.Errorf("generate API key ID: %w", err)
	}

	// Length of an API Key secret.
	keySecret, hashedSecret, err := GenerateSecret(22)
	if err != nil {
		return database.InsertAPIKeyParams{}, "", xerrors.Errorf("generate API key secret: %w", err)
	}

	params.ExpiresAt, params.LifetimeSeconds = params.expiry()

	if len(params.AllowList) == 0 {
		params.AllowList = database.AllowList{{Type: policy.WildcardSymbol, ID: policy.WildcardSymbol}}
	}

	ip := net.ParseIP(params.RemoteAddr)
	if ip == nil {
		ip = net.IPv4(0, 0, 0, 0)
	}

	bitlen := len(ip) * 8

	var requested database.APIKeyScopes
	switch {
	case len(params.Scopes) > 0:
		requested = params.Scopes
	case params.Scope != "":
		requested = database.APIKeyScopes{params.Scope}
	default:
		// Default to coder:all scope for backward compatibility.
		requested = database.APIKeyScopes{database.ApiKeyScopeCoderAll}
	}

	// Canonicalize scope names before validating them against the set of known
	// scopes.
	scopes := make(database.APIKeyScopes, 0, len(requested))
	for _, s := range requested {
		canonical := database.APIKeyScope(rbac.CanonicalScopeName(rbac.ScopeName(s)))
		if !canonical.Valid() {
			return database.InsertAPIKeyParams{}, "", xerrors.Errorf("invalid API key scope: %q", s)
		}
		scopes = append(scopes, canonical)
	}
	// Ensure scopes are still unique after canonicalizing.
	scopes = slice.Unique(scopes)

	token := fmt.Sprintf("%s-%s", keyID, keySecret)

	return database.InsertAPIKeyParams{
		ID:              keyID,
		UserID:          params.UserID,
		LastUsed:        time.Unix(0, 0).UTC(),
		LifetimeSeconds: params.LifetimeSeconds,
		IPAddress: pqtype.Inet{
			IPNet: net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(bitlen, bitlen),
			},
			Valid: true,
		},
		// Make sure in UTC time for common time zone
		ExpiresAt:    params.ExpiresAt.UTC(),
		CreatedAt:    dbtime.Now(),
		UpdatedAt:    dbtime.Now(),
		HashedSecret: hashedSecret,
		LoginType:    params.LoginType,
		Scopes:       scopes,
		AllowList:    params.AllowList,
		TokenName:    params.TokenName,
	}, token, nil
}

// expiry resolves ExpiresAt and LifetimeSeconds, falling back to DefaultLifetime.
func (p CreateParams) expiry() (time.Time, int64) {
	expiresAt, lifetime := p.ExpiresAt, p.LifetimeSeconds
	if expiresAt.IsZero() {
		if lifetime != 0 {
			expiresAt = dbtime.Now().Add(time.Duration(lifetime) * time.Second)
		} else {
			expiresAt = dbtime.Now().Add(p.DefaultLifetime)
			lifetime = int64(p.DefaultLifetime.Seconds())
		}
	}
	if lifetime == 0 {
		lifetime = int64(time.Until(expiresAt).Seconds())
	}
	return expiresAt, lifetime
}

func GenerateSecret(length int) (secret string, hashed []byte, err error) {
	secret, err = cryptorand.String(length)
	if err != nil {
		return "", nil, err
	}
	hash := HashSecret(secret)
	return secret, hash, nil
}

// ValidateHash compares a secret against an expected hashed secret.
func ValidateHash(hashedSecret []byte, secret string) bool {
	hash := HashSecret(secret)
	return subtle.ConstantTimeCompare(hashedSecret, hash) == 1
}

// HashSecret is the single function used to hash API key secrets.
// Use this to ensure a consistent hashing algorithm.
func HashSecret(secret string) []byte {
	hash := sha256.Sum256([]byte(secret))
	return hash[:]
}

var (
	// The request names a scope or allow list entry the caller lacks.
	ErrExceedsCaller = xerrors.New("exceeds the creating API key")
	// The scope comparison failed: a server fault, not a refusal.
	ErrCoverageUndecidable = xerrors.New("scope coverage could not be determined")
)

// InheritWithinCaller fills omitted scopes and allow list from the caller, and
// refuses rather than narrows anything the caller lacks. A caller short of
// coder:all with `*:*` also caps the expiry at its own and gets a token, which
// never slides past it. Token names are unique per user, so an unnamed token
// gets a generated name.
func InheritWithinCaller(caller database.APIKey, params CreateParams) (CreateParams, error) {
	if len(params.Scopes) == 0 && params.Scope != "" {
		params.Scopes = database.APIKeyScopes{params.Scope}
	}
	if len(params.Scopes) == 0 {
		params.Scopes = caller.Scopes
	}
	if len(params.AllowList) == 0 {
		params.AllowList = caller.AllowList
	}

	canonical := func(s database.APIKeyScope) rbac.ScopeName { return rbac.CanonicalScopeName(rbac.ScopeName(s)) }
	outside, err := rbac.FirstScopeNotCovered(slice.List(caller.Scopes, canonical), slice.List(params.Scopes, canonical))
	if err != nil {
		return CreateParams{}, xerrors.Errorf("compare scope %q: %w", outside, errors.Join(ErrCoverageUndecidable, err))
	}
	if outside != "" {
		return CreateParams{}, xerrors.Errorf("%w: scope %q is not among its scopes %v; request a subset, or authenticate with a key that holds it", ErrExceedsCaller, outside, caller.Scopes)
	}
	if entry, ok := rbac.FirstAllowListEntryNotCovered(caller.AllowList, params.AllowList); ok {
		return CreateParams{}, xerrors.Errorf("%w: allow list entry %q is not covered by its allow list %v; request a subset, or authenticate with a key that allows it", ErrExceedsCaller, entry, caller.AllowList)
	}

	if !caller.Scopes.Has(database.ApiKeyScopeCoderAll) || !slices.Contains(caller.AllowList, rbac.AllowListAll()) {
		params.ExpiresAt, params.LifetimeSeconds = params.expiry()
		if params.ExpiresAt.After(caller.ExpiresAt) {
			params.ExpiresAt = caller.ExpiresAt
			params.LifetimeSeconds = int64(time.Until(caller.ExpiresAt).Seconds())
		}
		params.LoginType = database.LoginTypeToken
		if params.TokenName == "" {
			params.TokenName = namesgenerator.NameDigitWith("_")
		}
	}
	return params, nil
}

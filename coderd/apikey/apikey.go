package apikey

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/rbac/policy"
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

	// Default expires at to now+lifetime, or use the configured value if not
	// set.
	if params.ExpiresAt.IsZero() {
		if params.LifetimeSeconds != 0 {
			params.ExpiresAt = dbtime.Now().Add(time.Duration(params.LifetimeSeconds) * time.Second)
		} else {
			params.ExpiresAt = dbtime.Now().Add(params.DefaultLifetime)
			params.LifetimeSeconds = int64(params.DefaultLifetime.Seconds())
		}
	}
	if params.LifetimeSeconds == 0 {
		params.LifetimeSeconds = int64(time.Until(params.ExpiresAt).Seconds())
	}

	if len(params.AllowList) == 0 {
		params.AllowList = database.AllowList{{Type: policy.WildcardSymbol, ID: policy.WildcardSymbol}}
	}

	ip := net.ParseIP(params.RemoteAddr)
	if ip == nil {
		ip = net.IPv4(0, 0, 0, 0)
	}

	bitlen := len(ip) * 8

	var scopes database.APIKeyScopes
	switch {
	case len(params.Scopes) > 0:
		scopes = params.Scopes
	case params.Scope != "":
		var scope database.APIKeyScope
		switch params.Scope {
		case "all":
			scope = database.ApiKeyScopeCoderAll
		case "application_connect":
			scope = database.ApiKeyScopeCoderApplicationConnect
		default:
			scope = params.Scope
		}
		scopes = database.APIKeyScopes{scope}
	default:
		// Default to coder:all scope for backward compatibility.
		scopes = database.APIKeyScopes{database.ApiKeyScopeCoderAll}
	}

	for _, s := range scopes {
		if !s.Valid() {
			return database.InsertAPIKeyParams{}, "", xerrors.Errorf("invalid API key scope: %q", s)
		}
	}

	token := fmt.Sprintf("%s-%s", keyID, keySecret)

	return database.InsertAPIKeyParams{
		ID:              keyID,
		UserID:          params.UserID,
		LastUsed:        time.Time{},
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

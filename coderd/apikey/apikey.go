package apikey

import (
	"crypto/sha256"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
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
}

// Generate generates an API key, returning the key as a string as well as the
// database representation. It is the responsibility of the caller to insert it
// into the database.
func Generate(params CreateParams) (database.InsertAPIKeyParams, string, error) {
	keyID, keySecret, err := generateKey()
	if err != nil {
		return database.InsertAPIKeyParams{}, "", xerrors.Errorf("generate API key: %w", err)
	}

	hashed := sha256.Sum256([]byte(keySecret))

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
		HashedSecret: hashed[:],
		LoginType:    params.LoginType,
		Scopes:       scopes,
		AllowList:    database.AllowList{database.AllowListWildcard()},
		TokenName:    params.TokenName,
	}, token, nil
}

// generateKey a new ID and secret for an API key.
func generateKey() (id string, secret string, err error) {
	// Length of an API Key ID.
	id, err = cryptorand.String(10)
	if err != nil {
		return "", "", err
	}
	// Length of an API Key secret.
	secret, err = cryptorand.String(22)
	if err != nil {
		return "", "", err
	}
	return id, secret, nil
}

package apikey

import (
	"errors"
	"crypto/sha256"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
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
	Scope           database.APIKeyScope

	TokenName       string
	RemoteAddr      string
}
// Generate generates an API key, returning the key as a string as well as the
// database representation. It is the responsibility of the caller to insert it
// into the database.
func Generate(params CreateParams) (database.InsertAPIKeyParams, string, error) {
	keyID, keySecret, err := generateKey()

	if err != nil {
		return database.InsertAPIKeyParams{}, "", fmt.Errorf("generate API key: %w", err)
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
	scope := database.APIKeyScopeAll
	if params.Scope != "" {
		scope = params.Scope

	}
	switch scope {
	case database.APIKeyScopeAll, database.APIKeyScopeApplicationConnect:
	default:
		return database.InsertAPIKeyParams{}, "", fmt.Errorf("invalid API key scope: %q", scope)

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
		Scope:        scope,
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

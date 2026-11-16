// Package workspacerelay ties the session token of a workspace to that
// workspace's daemon.
//
// A build gives the workspace owner a session token, which templates hand
// to what runs in the workspace. On its own, that token works for whoever
// holds it, from anywhere. With the relay, the control plane accepts it only
// together with a lease, and only the workspace's daemon can obtain one. The
// daemon serves a local address inside the workspace, forwards requests from
// there to the control plane, and attaches its lease. A session token that
// leaves the workspace is then useless.
//
// A lease is short-lived. The daemon renews it for as long as it runs. One
// instance of a daemon holds the relay at a time: it proves a key it made at
// start and keeps in memory, so a copy of the daemon's own token cannot renew.
// Another instance takes over once the holder stops renewing.
package workspacerelay

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/jwtutils"
)

// Mode says what the control plane does with a workspace session token that
// arrives without a lease.
type Mode string

const (
	// ModeOff accepts it, and gives daemons no lease.
	ModeOff Mode = "off"
	// ModeLog accepts it and records that it would have been refused, so
	// that an operator can see what enforcing would break.
	ModeLog Mode = "log"
	// ModeEnforce refuses it.
	ModeEnforce Mode = "enforce"
)

// ParseMode reads a mode. Empty means off.
func ParseMode(raw string) (Mode, error) {
	switch Mode(raw) {
	case "", ModeOff:
		return ModeOff, nil
	case ModeLog:
		return ModeLog, nil
	case ModeEnforce:
		return ModeEnforce, nil
	default:
		return ModeOff, xerrors.Errorf("unknown workspace relay mode %q: use %q, %q or %q", raw, ModeOff, ModeLog, ModeEnforce)
	}
}

const (
	// LeaseHeader carries the lease on a relayed request.
	LeaseHeader = "Coder-Workspace-Relay-Lease"

	// DefaultLeaseTTL is how long a lease lasts. It is also how long a
	// daemon instance keeps the relay after it last renewed, and so how
	// long a restarted daemon may have to wait.
	DefaultLeaseTTL = time.Minute

	// MaxClockSkew is how far the time a daemon signed may be from the
	// control plane's.
	MaxClockSkew = time.Minute

	leaseIssuer   = "coderd"
	leaseAudience = "workspace-relay-lease"
)

// LeaseClaims is what a lease says.
type LeaseClaims struct {
	jwtutils.RegisteredClaims
	WorkspaceID uuid.UUID `json:"workspace_id"`
	AgentID     uuid.UUID `json:"agent_id"`
	// Generation is which holder of the relay the lease was issued to.
	Generation int64 `json:"generation"`
}

// IssueLease signs a lease for a workspace's daemon.
func IssueLease(ctx context.Context, keys jwtutils.SigningKeyProvider, now time.Time, ttl time.Duration, workspaceID, agentID uuid.UUID, generation int64) (string, time.Time, error) {
	expiresAt := now.Add(ttl)
	lease, err := jwtutils.Sign(ctx, keys, LeaseClaims{
		RegisteredClaims: jwtutils.RegisteredClaims{
			Issuer:    leaseIssuer,
			Audience:  jwt.Audience{leaseAudience},
			Subject:   agentID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-MaxClockSkew)),
			Expiry:    jwt.NewNumericDate(expiresAt),
		},
		WorkspaceID: workspaceID,
		AgentID:     agentID,
		Generation:  generation,
	})
	if err != nil {
		return "", time.Time{}, xerrors.Errorf("sign lease: %w", err)
	}
	return lease, expiresAt, nil
}

// VerifyLease checks that a lease was issued by the control plane and has
// not run out, and returns what it says.
func VerifyLease(ctx context.Context, keys jwtutils.VerifyKeyProvider, lease string, now time.Time) (LeaseClaims, error) {
	var claims LeaseClaims
	err := jwtutils.Verify(ctx, keys, lease, &claims, jwtutils.WithVerifyExpected(jwt.Expected{
		Issuer:      leaseIssuer,
		AnyAudience: jwt.Audience{leaseAudience},
		Time:        now,
	}))
	if err != nil {
		return LeaseClaims{}, xerrors.Errorf("verify lease: %w", err)
	}
	if claims.WorkspaceID == uuid.Nil || claims.AgentID == uuid.Nil {
		return LeaseClaims{}, xerrors.New("lease names no workspace")
	}
	return claims, nil
}

// SigningInput is what a daemon instance signs, with the key it holds, to
// obtain a lease.
func SigningInput(timestamp time.Time) []byte {
	return []byte(fmt.Sprintf("coder-workspace-relay-lease\n%d", timestamp.UnixNano()))
}

// VerifyHolder checks that a request for a lease was signed, now, by the
// holder of the private half of publicKey.
func VerifyHolder(publicKey, signature []byte, timestamp, now time.Time) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return xerrors.Errorf("public key must be %d bytes", ed25519.PublicKeySize)
	}
	skew := now.Sub(timestamp)
	if skew < 0 {
		skew = -skew
	}
	if skew > MaxClockSkew {
		return xerrors.Errorf("signed %s away from the control plane's time", skew.Round(time.Second))
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), SigningInput(timestamp), signature) {
		return xerrors.New("signature does not match the public key")
	}
	return nil
}

// Package dlppolicy resolves an agent's data loss prevention policy for
// coderd enforcement gates.
package dlppolicy

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

// ForAgent returns the DLP policy attached to the given workspace agent, or
// nil if the agent has no policy configured. A nil result means no policy is
// in effect (default-permissive).
//
// Callers must have already authorized the request against the workspace.
// The DLP policy is metadata about the agent and is not itself a directly
// authorized resource, so we read it under a system-restricted context.
//
// already authorized access to the agent and we only need the policy
// attached to that authorized agent.
//
//nolint:gocritic // System-restricted ctx is intentional: callers have
func ForAgent(ctx context.Context, db database.Store, agentID uuid.UUID) (*database.TemplateVersionDlpPolicy, error) {
	p, err := db.GetTemplateVersionDLPPolicyByAgentID(dbauthz.AsSystemRestricted(ctx), agentID)
	if errors.Is(err, sql.ErrNoRows) {
		// No policy attached. Use nil to signal default-permissive so
		// callers don't need a separate sentinel; an unset policy is
		// not an error condition.
		return nil, nil //nolint:nilnil // nil policy = default permissive; intentional.
	}
	if err != nil {
		return nil, xerrors.Errorf("get dlp policy for agent %q: %w", agentID, err)
	}
	return &p, nil
}

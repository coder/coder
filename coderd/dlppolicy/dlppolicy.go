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
// This function reads the policy under a system-restricted context because
// the policy lookup is system-internal post-authz.
func ForAgent(ctx context.Context, db database.Store, agentID uuid.UUID) (*database.TemplateVersionDlpPolicy, error) {
	p, err := db.GetTemplateVersionDLPPolicyByAgentID(dbauthz.AsSystemRestricted(ctx), agentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, xerrors.Errorf("get dlp policy for agent %q: %w", agentID, err)
	}
	return &p, nil
}

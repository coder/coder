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
// the policy lookup is system-internal post-authz. Policies exist to
// constrain the actor, so the lookup must not depend on the actor's
// permissions.
func ForAgent(ctx context.Context, db database.Store, agentID uuid.UUID) (*database.TemplateVersionDlpPolicy, error) {
	// The escalation is intentional. See the doc comment above.
	//nolint:gocritic // post-authz system-internal lookup, see doc comment.
	p, err := db.GetTemplateVersionDLPPolicyByAgentID(dbauthz.AsSystemRestricted(ctx), agentID)
	if errors.Is(err, sql.ErrNoRows) {
		// A nil policy with a nil error is the documented "no policy" signal;
		// callers check dlp != nil. A sentinel error would force every gate
		// to branch on errors.Is, which is noisier than the existing
		// nil-policy check.
		//nolint:nilnil // nil policy is the no-policy signal; see doc comment.
		return nil, nil
	}
	if err != nil {
		return nil, xerrors.Errorf("get dlp policy for agent %q: %w", agentID, err)
	}
	return &p, nil
}

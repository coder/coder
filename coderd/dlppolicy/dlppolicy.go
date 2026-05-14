// Package dlppolicy resolves a workspace's data loss prevention policy
// for coderd enforcement gates.
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

// ForWorkspace returns the DLP policy attached to the given workspace, or nil
// if the workspace's template version has no policy configured. A nil result
// means no policy is in effect (default-permissive).
//
// Callers must have already authorized the request against the workspace.
// The DLP policy is metadata about the workspace's current build and is not
// itself a directly authorized resource, so we read it under a
// system-restricted context.
//
// already authorized access to the workspace and we only need the policy
// attached to that authorized workspace.
//
//nolint:gocritic // System-restricted ctx is intentional: callers have
func ForWorkspace(ctx context.Context, db database.Store, workspaceID uuid.UUID) (*database.TemplateVersionDlpPolicy, error) {
	p, err := db.GetTemplateVersionDLPPolicyByWorkspaceID(dbauthz.AsSystemRestricted(ctx), workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		// No policy attached. Use nil to signal default-permissive so
		// callers don't need a separate sentinel; an unset policy is
		// not an error condition.
		return nil, nil //nolint:nilnil // nil policy = default permissive; intentional.
	}
	if err != nil {
		return nil, xerrors.Errorf("get dlp policy for workspace %q: %w", workspaceID, err)
	}
	return &p, nil
}

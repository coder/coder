// Package workspaceexec coordinates durable workspace execution sessions.
package workspaceexec

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

// ErrAdmissionClosed means preservation or deletion already owns the workspace.
var ErrAdmissionClosed = xerrors.New("workspace execution admission is closed")

// CheckAdmission must run on the same transaction as admission or binding.
// Callers retain their usual workspace authorization. This fence applies even
// when new session creation has been disabled by an administrator.
func CheckAdmission(ctx context.Context, tx database.Store, workspaceID uuid.UUID) error {
	if err := tx.AcquireLock(ctx, database.WorkspaceLifecycleLockID(workspaceID)); err != nil {
		return xerrors.Errorf("lock workspace admission: %w", err)
	}
	//nolint:gocritic // The lifecycle fence is internal state, not user-visible data.
	closed, err := tx.HasClosedWorkspaceExecutionAdmission(dbauthz.AsSystemRestricted(ctx), uuid.NullUUID{UUID: workspaceID, Valid: true})
	if err != nil {
		return xerrors.Errorf("check workspace admission: %w", err)
	}
	if closed {
		return ErrAdmissionClosed
	}
	if err := tx.LockWorkspaceExecutionWorkspace(ctx, workspaceID); err != nil {
		return xerrors.Errorf("lock workspace identity: %w", err)
	}
	workspace, err := tx.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return xerrors.Errorf("fetch workspace for admission: %w", err)
	}
	if workspace.Deleted {
		return ErrAdmissionClosed
	}
	//nolint:gocritic // Build state is an internal fence after workspace authorization.
	committed, err := HasCommittedDeletion(dbauthz.AsSystemRestricted(ctx), tx, workspaceID)
	if err != nil {
		return xerrors.Errorf("check committed workspace deletion: %w", err)
	}
	if committed {
		return ErrAdmissionClosed
	}
	return nil
}

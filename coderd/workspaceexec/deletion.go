package workspaceexec

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// HasCommittedDeletion observes the latest build while the caller holds the
// workspace lifecycle and row locks. Failed or acknowledged canceled jobs no
// longer own deletion; an unacknowledged cancellation still does.
func HasCommittedDeletion(ctx context.Context, tx database.Store, workspaceID uuid.UUID) (bool, error) {
	build, err := tx.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspaceID)
	if xerrors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if build.Transition != database.WorkspaceTransitionDelete {
		return false, nil
	}
	job, err := tx.GetProvisionerJobByID(ctx, build.JobID)
	if err != nil {
		return false, err
	}
	switch job.JobStatus {
	case database.ProvisionerJobStatusFailed, database.ProvisionerJobStatusCanceled:
		return false, nil
	default:
		return true, nil
	}
}

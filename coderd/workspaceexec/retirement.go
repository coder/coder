package workspaceexec

import (
	"context"
	"database/sql"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

// retireDeletedWorkspace acknowledges an independently completed deletion. The
// caller holds the workspace lifecycle and row locks and revalidated ownership.
func (c *Controller) retireDeletedWorkspace(ctx context.Context, tx database.Store, current database.WorkspaceExecutionSession, workspace database.Workspace) error {
	if !workspace.Deleted || !current.WorkspaceID.Valid || current.WorkspaceID.UUID != workspace.ID ||
		!current.WorkspaceOwnerID.Valid || current.WorkspaceOwnerID.UUID != workspace.OwnerID ||
		current.OrganizationID != workspace.OrganizationID {
		return ErrSessionChanged
	}
	pending, err := tx.HasPendingWorkspaceExecutionReceiptsByWorkspaceID(ctx, current.WorkspaceID)
	if err != nil {
		return err
	}
	busy, err := tx.HasBusyWorkspaceExecutionChats(ctx, current.WorkspaceID)
	if err != nil {
		return err
	}
	if pending || busy {
		return c.deferSession(ctx, tx, current, current.State, "source workspace was deleted; pending or uncertain work still requires its actual outcome")
	}
	build, err := tx.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
	if err != nil {
		return err
	}
	if build.WorkspaceID != workspace.ID || build.Transition != database.WorkspaceTransitionDelete {
		return c.deferFailure(ctx, tx, current, "preservation_failed", "source workspace deletion has no matching delete build")
	}
	job, err := tx.GetProvisionerJobByID(ctx, build.JobID)
	if err != nil {
		return err
	}
	if job.Type != database.ProvisionerJobTypeWorkspaceBuild || job.OrganizationID != current.OrganizationID {
		return ErrSessionChanged
	}
	if job.JobStatus != database.ProvisionerJobStatusSucceeded {
		return c.deferSession(ctx, tx, current, current.State, "source workspace deletion has no successful delete job acknowledgment")
	}
	if err := requiredResultsAvailable(ctx, tx, current, c.Clock.Now()); err != nil {
		return c.deferFailure(ctx, tx, current, "preservation_failed", "source workspace was deleted before required results were available: "+err.Error())
	}
	next := current
	next.State, next.Error, next.NextRetryAt = "completed", "", sql.NullTime{}
	next.DeleteBuildID = uuid.NullUUID{UUID: build.ID, Valid: true}
	_, err = saveSession(ctx, tx, current, next, c.Clock.Now())
	return err
}

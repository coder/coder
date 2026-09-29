package workspaceexec

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/provisionerjobs"
	"github.com/coder/coder/v2/coderd/wsbuilder"
)

func (c *Controller) deleteWorkspace(ctx context.Context, expected database.WorkspaceExecutionSession) error {
	var job *database.ProvisionerJob
	err := c.Database.InTx(func(tx database.Store) error {
		current, workspace, err := lockSession(ctx, tx, expected)
		if err != nil {
			return err
		}
		if current.State != "preserved" || current.Revision != expected.Revision || current.Retained || !current.Disposable ||
			workspace.Deleted || !c.AllowDeletion {
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
			return xerrors.New("new work appeared after preservation admission closed")
		}

		others, protection, err := otherSessionProtection(ctx, tx, current, c.Clock.Now())
		if err != nil {
			return err
		}
		if protection != "" {
			return c.deferSession(ctx, tx, current, "preserved", protection)
		}
		for _, other := range others {
			if err := requiredResultsAvailable(ctx, tx, other, c.Clock.Now()); err != nil {
				return c.deferSession(ctx, tx, current, "preserved", err.Error())
			}
		}
		if err := requiredResultsAvailable(ctx, tx, current, c.Clock.Now()); err != nil {
			return err
		}
		if c.UsageChecker == nil {
			return xerrors.New("workspace build usage checker is unavailable")
		}
		builder := wsbuilder.New(workspace, database.WorkspaceTransitionDelete, c.UsageChecker()).
			Reason(database.BuildReasonAutodelete).DeploymentValues(c.DeploymentValues).Experiments(c.Experiments)
		build, createdJob, _, err := builder.Build(ctx, tx, c.FileCache, nil, audit.WorkspaceBuildBaggage{IP: "127.0.0.1"})
		if err != nil {
			return err
		}
		next := current
		next.State = "deleting"
		next.DeleteBuildID = uuid.NullUUID{UUID: build.ID, Valid: true}
		next.Error = ""
		next.NextRetryAt = sql.NullTime{}
		if _, err := saveSession(ctx, tx, current, next, c.Clock.Now()); err != nil {
			return err
		}
		job = createdJob
		return nil
	}, nil)
	if err != nil {
		return err
	}
	// The build and session pointer are committed before notifying provisioners.
	// A failed/lost notification is retried using that same persisted job.
	if job == nil {
		return nil
	}
	if err := provisionerjobs.PostJob(c.Pubsub, *job); err != nil {
		c.Logger.Warn(ctx, "publish workspace execution delete job", slog.F("job_id", job.ID), slog.Error(err))
	}
	return nil
}

func (c *Controller) observeDeletion(ctx context.Context, expected database.WorkspaceExecutionSession) error {
	var pendingJob *database.ProvisionerJob
	err := c.Database.InTx(func(tx database.Store) error {
		current, workspace, err := lockSession(ctx, tx, expected)
		if err != nil {
			return err
		}
		if current.State != "deleting" || current.Revision != expected.Revision {
			return ErrSessionChanged
		}
		if !current.DeleteBuildID.Valid {
			return xerrors.New("deletion session has no durable build identity")
		}
		build, err := tx.GetWorkspaceBuildByID(ctx, current.DeleteBuildID.UUID)
		if err != nil {
			return err
		}
		if build.WorkspaceID != workspace.ID || build.Transition != database.WorkspaceTransitionDelete {
			return xerrors.New("delete build does not match session workspace")
		}
		job, err := tx.GetProvisionerJobByID(ctx, build.JobID)
		if err != nil {
			return err
		}
		switch job.JobStatus {
		case database.ProvisionerJobStatusSucceeded:
			if !workspace.Deleted {
				return c.deferFailure(ctx, tx, current, "deletion_failed", "delete job succeeded without deleting workspace")
			}
			next := current
			next.State = "completed"
			next.Error = ""
			next.NextRetryAt = sql.NullTime{}
			_, err = saveSession(ctx, tx, current, next, c.Clock.Now())
			return err
		case database.ProvisionerJobStatusFailed, database.ProvisionerJobStatusCanceled:
			return c.deferFailure(ctx, tx, current, "deletion_failed", "delete job did not complete successfully: "+job.Error.String)
		default:
			pendingJob = &job
			return c.deferSession(ctx, tx, current, "deleting", "")
		}
	}, nil)
	if err != nil {
		return err
	}
	if pendingJob != nil {
		return provisionerjobs.PostJob(c.Pubsub, *pendingJob)
	}
	return nil
}

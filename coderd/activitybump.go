package coderd

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

// activityBumpWorkspace automatically bumps the workspace's auto-off timer
// if it is set to expire soon.
func activityBumpWorkspace(ctx context.Context, log slog.Logger, db database.Store, workspaceID uuid.UUID) {
	// We set a short timeout so if the app is under load, these
	// low priority operations fail first.
	ctx, cancel := context.WithTimeout(ctx, time.Second*15)
	defer cancel()

	err := db.InTx(func(s database.Store) error {
		build, err := s.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspaceID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return xerrors.Errorf("get latest workspace build: %w", err)
		}

		job, err := s.GetProvisionerJobByID(ctx, build.JobID)
		if err != nil {
			return xerrors.Errorf("get provisioner job: %w", err)
		}

		if build.Transition != database.WorkspaceTransitionStart || !job.CompletedAt.Valid {
			return nil
		}

		if build.Deadline.IsZero() {
			// Workspace shutdown is manual
			return nil
		}

		workspace, err := s.GetWorkspaceByID(ctx, workspaceID)
		if err != nil {
			return xerrors.Errorf("get workspace: %w", err)
		}

		var (
			// We bump by the original TTL to prevent counter-intuitive behavior
			// as the TTL wraps. For example, if I set the TTL to 12 hours, sign off
			// work at midnight, come back at 10am, I would want another full day
			// of uptime. In the prior implementation, the workspace would enter
			// a state of always expiring 1 hour in the future
			bumpAmount = time.Duration(workspace.Ttl.Int64)
			// DB writes are expensive so we only bump when 5% of the deadline
			// has elapsed.
			bumpEvery         = bumpAmount / 20
			timeSinceLastBump = bumpAmount - time.Until(build.Deadline)
		)

		if timeSinceLastBump < bumpEvery {
			return nil
		}

		if bumpAmount == 0 {
			return nil
		}

		newDeadline := dbtime.Now().Add(bumpAmount)
		if !build.MaxDeadline.IsZero() && newDeadline.After(build.MaxDeadline) {
			newDeadline = build.MaxDeadline
		}

		if err := s.UpdateWorkspaceBuildByID(ctx, database.UpdateWorkspaceBuildByIDParams{
			ID:               build.ID,
			UpdatedAt:        dbtime.Now(),
			ProvisionerState: build.ProvisionerState,
			Deadline:         newDeadline,
			MaxDeadline:      build.MaxDeadline,
		}); err != nil {
			return xerrors.Errorf("update workspace build: %w", err)
		}
		return nil
	}, nil)
	if err != nil {
		if !xerrors.Is(err, context.Canceled) && !database.IsQueryCanceledError(err) {
			// Bump will fail if the context is canceled, but this is ok.
			log.Error(ctx, "bump failed", slog.Error(err),
				slog.F("workspace_id", workspaceID),
			)
		}
		return
	}

	log.Debug(ctx, "bumped deadline from activity",
		slog.F("workspace_id", workspaceID),
	)
}

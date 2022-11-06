package coderd

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
)

// activityBumpWorkspace automatically bumps the workspace's auto-off timer
// if it is set to expire soon.
func activityBumpWorkspace(log slog.Logger, db database.Store, workspace database.Workspace) {
	// We set a short timeout so if the app is under load, these
	// low priority operations fail first.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	err := db.InTx(func(s database.Store) error {
		build, err := s.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
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

		// We sent bumpThreshold slightly under bumpAmount to minimize DB writes.
		const (
			bumpAmount    = time.Hour
			bumpThreshold = time.Hour - (time.Minute * 10)
		)

		if !build.Deadline.Before(time.Now().Add(bumpThreshold)) {
			return nil
		}

		newDeadline := database.Now().Add(bumpAmount)

		if _, err := s.UpdateWorkspaceBuildByID(ctx, database.UpdateWorkspaceBuildByIDParams{
			ID:               build.ID,
			UpdatedAt:        database.Now(),
			ProvisionerState: build.ProvisionerState,
			Deadline:         newDeadline,
		}); err != nil {
			return xerrors.Errorf("update workspace build: %w", err)
		}
		return nil
	})
	if err != nil {
		log.Error(
			ctx, "bump failed",
			slog.Error(err),
			slog.F("workspace_id", workspace.ID),
		)
	} else {
		log.Debug(
			ctx, "bumped deadline from activity",
			slog.F("workspace_id", workspace.ID),
		)
	}
}

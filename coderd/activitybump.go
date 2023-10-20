package coderd

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
)

// activityBumpWorkspace automatically bumps the workspace's auto-off timer
// if it is set to expire soon.
func activityBumpWorkspace(ctx context.Context, log slog.Logger, db database.Store, workspaceID uuid.UUID) {
	// We set a short timeout so if the app is under load, these
	// low priority operations fail first.
	ctx, cancel := context.WithTimeout(ctx, time.Second*15)
	defer cancel()
	if err := db.ActivityBumpWorkspace(ctx, workspaceID); err != nil {
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

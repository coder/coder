package agentapi

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
)

// ActivityBumpWorkspace automatically bumps the workspace's auto-off timer
// if it is set to expire soon. The deadline will be bumped by 1 hour*.
// If the bump crosses over an autostart time, the workspace will be
// bumped by the workspace ttl instead.
//
// If nextAutostart is the zero value or in the past, the workspace
// will be bumped by 1 hour.
// It handles the edge case in the example:
//  1. Autostart is set to 9am.
//  2. User works all day, and leaves a terminal open to the workspace overnight.
//  3. The open terminal continually bumps the workspace deadline.
//  4. 9am the next day, the activity bump pushes to 10am.
//  5. If the user goes inactive for 1 hour during the day, the workspace will
//     now stop, because it has been extended by 1 hour durations. Despite the TTL
//     being set to 8hrs from the autostart time.
//
// So the issue is that when the workspace is bumped across an autostart
// deadline, we should treat the workspace as being "started" again and
// extend the deadline by the autostart time + workspace ttl instead.
//
// The issue still remains with build_max_deadline. We need to respect the original
// maximum deadline, so that will need to be handled separately.
// A way to avoid this is to configure the max deadline to something that will not
// span more than 1 day. This will force the workspace to restart and reset the deadline
// each morning when it autostarts.
func ActivityBumpWorkspace(ctx context.Context, log slog.Logger, db database.Store, workspaceID uuid.UUID, nextAutostart time.Time) {
	// We set a short timeout so if the app is under load, these
	// low priority operations fail first.
	ctx, cancel := context.WithTimeout(ctx, time.Second*15)
	defer cancel()
	if err := db.ActivityBumpWorkspace(ctx, database.ActivityBumpWorkspaceParams{
		NextAutostart: nextAutostart.UTC(),
		WorkspaceID:   workspaceID,
	}); err != nil {
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

package workspacestats

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// ActivityBumpReason represents the reason for an activity bump.
type ActivityBumpReason string

const (
	// ActivityBumpReasonWorkspaceStats indicates the bump was triggered
	// by SSH or terminal activity reported via workspace stats.
	ActivityBumpReasonWorkspaceStats ActivityBumpReason = "workspace_stats"
	// ActivityBumpReasonChatHeartbeat indicates the bump was triggered
	// by an AI chat heartbeat. Its deadline source type has the same value.
	ActivityBumpReasonChatHeartbeat = ActivityBumpReason(codersdk.WorkspaceBuildDeadlineSourceTypeChatHeartbeat)
	// ActivityBumpReasonAppActivity indicates the bump was triggered
	// by app or port-forward activity.
	ActivityBumpReasonAppActivity ActivityBumpReason = "app_activity"
	// ActivityBumpReasonChatWorkspaceUpload indicates the bump was
	// triggered by a chat file upload streaming into the workspace. Its
	// deadline source type has the same value.
	ActivityBumpReasonChatWorkspaceUpload = ActivityBumpReason(codersdk.WorkspaceBuildDeadlineSourceTypeChatWorkspaceUpload)
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
//
// sources describe the activity and are recorded when the bump changes the
// deadline. Invalid sources are logged and dropped so the bump still happens.
func ActivityBumpWorkspace(ctx context.Context, log slog.Logger, db database.Store, workspaceID uuid.UUID, nextAutostart time.Time, reason ActivityBumpReason, sources ...database.WorkspaceBuildDeadlineSource) {
	// We set a short timeout so if the app is under load, these
	// low priority operations fail first.
	ctx, cancel := context.WithTimeout(ctx, time.Second*15)
	defer cancel()
	unique := make(database.WorkspaceBuildDeadlineSources, 0, len(sources))
	seen := make(map[database.WorkspaceBuildDeadlineSource]bool, len(sources))
	for _, source := range sources {
		if err := source.Validate(); err != nil {
			log.Error(ctx, "dropped invalid deadline source", slog.Error(err),
				slog.F("workspace_id", workspaceID),
				slog.F("reason", reason),
				slog.F("source", source),
			)
			continue
		}
		// The display name is presentation, not identity.
		identity := source
		identity.DisplayName = ""
		if !seen[identity] {
			seen[identity] = true
			unique = append(unique, source)
		}
	}
	err := db.ActivityBumpWorkspace(ctx, database.ActivityBumpWorkspaceParams{
		DeadlineReason:  database.WorkspaceBuildDeadlineWriterReasonActivity,
		DeadlineSources: unique,
		NextAutostart:   nextAutostart.UTC(),
		WorkspaceID:     workspaceID,
	})
	if err != nil {
		if !xerrors.Is(err, context.Canceled) && !database.IsQueryCanceledError(err) {
			// Bump will fail if the context is canceled, but this is ok.
			log.Error(ctx, "activity bump failed", slog.Error(err),
				slog.F("workspace_id", workspaceID),
				slog.F("reason", reason),
				slog.F("sources", unique),
			)
		}
		return
	}

	log.Debug(ctx, "bumped deadline from activity",
		slog.F("workspace_id", workspaceID),
		slog.F("reason", reason),
		slog.F("sources", unique),
	)
}

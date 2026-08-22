package workspacestats

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
)

// ActivityBumpReason represents the source of activity that triggered a
// workspace deadline bump. It is persisted to workspaces.last_activity_source
// so operators can see why a workspace's autostop deadline keeps extending.
type ActivityBumpReason string

const (
	// ActivityBumpReasonSSH indicates the bump was triggered by an SSH session.
	ActivityBumpReasonSSH ActivityBumpReason = "ssh"
	// ActivityBumpReasonVSCode indicates the bump was triggered by a VS Code session.
	ActivityBumpReasonVSCode ActivityBumpReason = "vscode"
	// ActivityBumpReasonJetBrains indicates the bump was triggered by a JetBrains session.
	ActivityBumpReasonJetBrains ActivityBumpReason = "jetbrains"
	// ActivityBumpReasonReconnectingPTY indicates the bump was triggered by
	// a web terminal (reconnecting PTY) session.
	ActivityBumpReasonReconnectingPTY ActivityBumpReason = "reconnecting_pty"
	// ActivityBumpReasonChatHeartbeat indicates the bump was triggered
	// by an AI chat heartbeat.
	ActivityBumpReasonChatHeartbeat ActivityBumpReason = "chat_heartbeat"
	// ActivityBumpReasonAppActivity indicates the bump was triggered by
	// app activity, when the specific app slug is unavailable.
	ActivityBumpReasonAppActivity ActivityBumpReason = "app_activity"
)

// ActivityBumpReasonApp returns the source recorded for activity from a
// specific workspace app, identified by its slug.
func ActivityBumpReasonApp(slug string) ActivityBumpReason {
	return ActivityBumpReason("app:" + slug)
}

// ActivityBumpReasonFromStats derives the source to record when a bump is
// triggered by agent-reported session stats. Priority order when multiple
// session types are simultaneously active: SSH > VS Code > JetBrains > web
// terminal. Only one source is recorded per bump.
func ActivityBumpReasonFromStats(stats *agentproto.Stats) ActivityBumpReason {
	switch {
	case stats.SessionCountSsh > 0:
		return ActivityBumpReasonSSH
	case stats.SessionCountVscode > 0:
		return ActivityBumpReasonVSCode
	case stats.SessionCountJetbrains > 0:
		return ActivityBumpReasonJetBrains
	case stats.SessionCountReconnectingPty > 0:
		return ActivityBumpReasonReconnectingPTY
	default:
		// Legacy stats (ConnectionCount > 0) with no per-session
		// breakdown available.
		return ActivityBumpReasonSSH
	}
}

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
func ActivityBumpWorkspace(ctx context.Context, log slog.Logger, db database.Store, workspaceID uuid.UUID, nextAutostart time.Time, reason ActivityBumpReason) {
	// We set a short timeout so if the app is under load, these
	// low priority operations fail first.
	ctx, cancel := context.WithTimeout(ctx, time.Second*15)
	defer cancel()
	err := db.ActivityBumpWorkspace(ctx, database.ActivityBumpWorkspaceParams{
		NextAutostart: nextAutostart.UTC(),
		WorkspaceID:   workspaceID,
		Source:        string(reason),
	})
	if err != nil {
		if !xerrors.Is(err, context.Canceled) && !database.IsQueryCanceledError(err) {
			// Bump will fail if the context is canceled, but this is ok.
			log.Error(ctx, "activity bump failed", slog.Error(err),
				slog.F("workspace_id", workspaceID),
				slog.F("reason", reason),
			)
		}
		return
	}

	log.Debug(ctx, "bumped deadline from activity",
		slog.F("workspace_id", workspaceID),
		slog.F("reason", reason),
	)
}

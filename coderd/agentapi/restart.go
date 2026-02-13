package agentapi

import (
	"context"
	"database/sql"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/wspubsub"
)

// RestartAPI handles the ReportRestart RPC, which is called by the
// agent when it has been restarted by the reaper after an OOM kill
// or other SIGKILL event.
type RestartAPI struct {
	AgentFn                  func(context.Context) (database.WorkspaceAgent, error)
	WorkspaceID              uuid.UUID
	Database                 database.Store
	Log                      slog.Logger
	NotificationsEnqueuer    notifications.Enqueuer
	PublishWorkspaceUpdateFn func(context.Context, *database.WorkspaceAgent, wspubsub.WorkspaceEventKind) error
	Metrics                  *LifecycleMetrics
	TemplateName             string
	TemplateVersionName      string
}

func (a *RestartAPI) ReportRestart(ctx context.Context, req *agentproto.ReportRestartRequest) (*agentproto.ReportRestartResponse, error) {
	workspaceAgent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, err
	}

	now := dbtime.Now()
	err = a.Database.UpdateWorkspaceAgentRestartCount(ctx, database.UpdateWorkspaceAgentRestartCountParams{
		ID:              workspaceAgent.ID,
		RestartCount:    req.RestartCount,
		LastRestartedAt: sql.NullTime{Time: now, Valid: true},
	})
	if err != nil {
		return nil, xerrors.Errorf("update workspace agent restart count: %w", err)
	}

	a.Log.Info(ctx, "agent reported restart",
		slog.F("agent_id", workspaceAgent.ID),
		slog.F("restart_count", req.RestartCount),
		slog.F("reason", req.Reason),
		slog.F("kill_signal", req.KillSignal),
	)

	if a.Metrics != nil {
		a.Metrics.AgentRestarts.WithLabelValues(
			a.TemplateName,
			a.TemplateVersionName,
			req.Reason,
			req.KillSignal,
		).Add(float64(req.RestartCount))
	}

	if a.PublishWorkspaceUpdateFn != nil {
		if err := a.PublishWorkspaceUpdateFn(ctx, &workspaceAgent, wspubsub.WorkspaceEventKindAgentLifecycleUpdate); err != nil {
			a.Log.Error(ctx, "failed to publish workspace update after restart report", slog.Error(err))
		}
	}

	// Notify the workspace owner that the agent has been restarted.
	if a.NotificationsEnqueuer != nil {
		workspace, err := a.Database.GetWorkspaceByID(ctx, a.WorkspaceID)
		if err != nil {
			a.Log.Error(ctx, "failed to get workspace for restart notification", slog.Error(err))
		} else {
			if _, err := a.NotificationsEnqueuer.EnqueueWithData(
				// nolint:gocritic // Notifier context required to enqueue.
				dbauthz.AsNotifier(ctx),
				workspace.OwnerID,
				notifications.TemplateWorkspaceAgentRestarted,
				map[string]string{
					"workspace":     workspace.Name,
					"agent":         workspaceAgent.Name,
					"restart_count": fmt.Sprintf("%d", req.RestartCount),
					"reason":        req.Reason,
					"kill_signal":   req.KillSignal,
				},
				map[string]any{
					// Include a timestamp to prevent deduplication
					// of repeated restart notifications within the
					// same day.
					"timestamp": now,
				},
				"agent-restart",
				workspace.ID,
				workspace.OwnerID,
				workspace.OrganizationID,
			); err != nil {
				a.Log.Error(ctx, "failed to send restart notification", slog.Error(err))
			}
		}
	}

	return &agentproto.ReportRestartResponse{}, nil
}

package agentapi

import (
	"context"
	"database/sql"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/wspubsub"
)

// RestartAPI handles the ReportRestart RPC, which is called by the
// agent when it has been restarted by the reaper after an OOM kill
// or other SIGKILL event.
type RestartAPI struct {
	AgentFn                  func(context.Context) (database.WorkspaceAgent, error)
	Database                 database.Store
	Log                      slog.Logger
	PublishWorkspaceUpdateFn func(context.Context, *database.WorkspaceAgent, wspubsub.WorkspaceEventKind) error
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
		slog.F("restart_reason", req.RestartReason),
	)

	if a.PublishWorkspaceUpdateFn != nil {
		if err := a.PublishWorkspaceUpdateFn(ctx, &workspaceAgent, wspubsub.WorkspaceEventKindAgentLifecycleUpdate); err != nil {
			a.Log.Error(ctx, "failed to publish workspace update after restart report", slog.Error(err))
		}
	}

	return &agentproto.ReportRestartResponse{}, nil
}

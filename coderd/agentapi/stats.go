package agentapi

import (
	"context"
	"time"

	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/google/uuid"

	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/workspacestats"
)

type StatsBatcher interface {
	Add(now time.Time, agentID uuid.UUID, templateID uuid.UUID, userID uuid.UUID, workspaceID uuid.UUID, st *agentproto.Stats) error
}

type StatsAPI struct {
	AgentFn                   func(context.Context) (database.WorkspaceAgent, error)
	Database                  database.Store
	Log                       slog.Logger
	StatsReporter             *workspacestats.Reporter
	AgentStatsRefreshInterval time.Duration

	TimeNowFn func() time.Time // defaults to dbtime.Now()
}

func (a *StatsAPI) now() time.Time {
	if a.TimeNowFn != nil {
		return a.TimeNowFn()
	}
	return dbtime.Now()
}

func (a *StatsAPI) UpdateStats(ctx context.Context, req *agentproto.UpdateStatsRequest) (*agentproto.UpdateStatsResponse, error) {
	res := &agentproto.UpdateStatsResponse{
		ReportInterval: durationpb.New(a.AgentStatsRefreshInterval),
	}
	// An empty stat means it's just looking for the report interval.
	if req.Stats == nil {
		return res, nil
	}

	workspaceAgent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, err
	}
	getWorkspaceAgentByIDRow, err := a.Database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace by agent ID %q: %w", workspaceAgent.ID, err)
	}
	workspace := getWorkspaceAgentByIDRow.Workspace
	a.Log.Debug(ctx, "read stats report",
		slog.F("interval", a.AgentStatsRefreshInterval),
		slog.F("workspace_id", workspace.ID),
		slog.F("payload", req),
	)

	err = a.StatsReporter.ReportAgentStats(
		ctx,
		a.now(),
		workspace,
		workspaceAgent,
		getWorkspaceAgentByIDRow.TemplateName,
		req.Stats,
	)
	if err != nil {
		return nil, xerrors.Errorf("report agent stats: %w", err)
	}

	return res, nil
}

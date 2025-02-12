package agentapi

import (
	"context"
	"time"

	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/durationpb"

	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/codersdk"
)

type StatsAPI struct {
	AgentFn                   func(context.Context) (database.WorkspaceAgent, error)
	Database                  database.Store
	Log                       slog.Logger
	StatsReporter             *workspacestats.Reporter
	AgentStatsRefreshInterval time.Duration
	Experiments               codersdk.Experiments

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
	workspace := getWorkspaceAgentByIDRow
	a.Log.Debug(ctx, "read stats report",
		slog.F("interval", a.AgentStatsRefreshInterval),
		slog.F("workspace_id", workspace.ID),
		slog.F("payload", req),
	)

	if a.Experiments.Enabled(codersdk.ExperimentWorkspaceUsage) {
		// while the experiment is enabled we will not report
		// session stats from the agent. This is because it is
		// being handled by the CLI and the postWorkspaceUsage route.
		req.Stats.SessionCountSsh = 0
		req.Stats.SessionCountJetbrains = 0
		req.Stats.SessionCountVscode = 0
		req.Stats.SessionCountReconnectingPty = 0
	}

	err = a.StatsReporter.ReportAgentStats(
		ctx,
		a.now(),
		workspace,
		workspaceAgent,
		getWorkspaceAgentByIDRow.TemplateName,
		req.Stats,
		false,
	)
	if err != nil {
		return nil, xerrors.Errorf("report agent stats: %w", err)
	}

	return res, nil
}

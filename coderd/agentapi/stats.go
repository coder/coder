package agentapi

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/durationpb"

	"cdr.dev/slog/v3"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/workspacestats"
	"github.com/coder/coder/v2/codersdk"
)

type StatsAPI struct {
	AgentID                   uuid.UUID
	AgentName                 string
	Workspace                 *CachedWorkspaceFields
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

	// If cache is empty (prebuild or invalid), fall back to DB
	var ws database.WorkspaceIdentity
	var ok bool
	if ws, ok = a.Workspace.AsWorkspaceIdentity(); !ok {
		w, err := a.Database.GetWorkspaceByAgentID(ctx, a.AgentID)
		if err != nil {
			return nil, xerrors.Errorf("get workspace by agent ID %q: %w", a.AgentID, err)
		}
		ws = database.WorkspaceIdentityFromWorkspace(w)
	}

	a.Log.Debug(ctx, "read stats report",
		slog.F("interval", a.AgentStatsRefreshInterval),
		slog.F("workspace_id", ws.ID),
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

	err := a.StatsReporter.ReportAgentStats(
		ctx,
		a.now(),
		ws,
		a.AgentID,
		a.AgentName,
		req.Stats,
		false,
	)
	if err != nil {
		return nil, xerrors.Errorf("report agent stats: %w", err)
	}

	return res, nil
}

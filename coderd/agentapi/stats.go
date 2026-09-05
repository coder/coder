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

	a.boundSessionCounts(ctx, req.Stats)

	// The report itself is agent controlled, so log bounded scalars about it
	// rather than the payload.
	a.Log.Debug(ctx, "read stats report",
		slog.F("interval", a.AgentStatsRefreshInterval),
		slog.F("workspace_id", ws.ID),
		slog.F("connection_count", req.Stats.GetConnectionCount()),
		slog.F("session_count_keys", len(req.Stats.GetSessionCounts())),
		slog.F("connections_by_proto_keys", len(req.Stats.GetConnectionsByProto())),
	)

	if a.Experiments.Enabled(codersdk.ExperimentWorkspaceUsage) {
		// while the experiment is enabled we will not report
		// session stats from the agent. This is because it is
		// being handled by the CLI and the postWorkspaceUsage route.
		workspacestats.ClearSessionCounts(req.Stats)
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

// boundSessionCounts truncates an oversized session_counts map in place and
// sums the counts it drops into AppFamilyUnknown. Agents choose the keys, so
// this bounds the work the stats pipeline does per report before
// normalization allocates for it. The rest of the report is still accepted,
// as with oversized agent metadata.
//
// Which names survive is arbitrary because map iteration order is random.
// The batcher's own cap is what makes the stored result deterministic. The
// truncated map holds one name past the bound when the unknown bucket was
// not already among the entries that fit.
func (a *StatsAPI) boundSessionCounts(ctx context.Context, st *agentproto.Stats) {
	reported := st.GetSessionCounts()
	if len(reported) <= workspacestats.MaxReportedSessionCountEntries {
		return
	}

	bounded := make(map[string]int64, workspacestats.MaxReportedSessionCountEntries+1)
	var folded int64
	for app, count := range reported {
		if len(bounded) < workspacestats.MaxReportedSessionCountEntries {
			bounded[app] = count
			continue
		}
		folded += count
	}
	if folded > 0 {
		bounded[string(codersdk.AppFamilyUnknown)] += folded
	}
	st.SessionCounts = bounded

	// The batcher warns about the fold on a throttle, so this stays at debug
	// to avoid one log line per report from the same agent.
	a.Log.Debug(ctx, "too many session counts reported, overflow counted under unknown",
		slog.F("reported", len(reported)),
		slog.F("max", workspacestats.MaxReportedSessionCountEntries),
	)
}

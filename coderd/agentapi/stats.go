package agentapi

import (
	"context"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/google/uuid"

	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/prometheusmetrics"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/workspaceapps"
)

type StatsBatcher interface {
	Add(now time.Time, agentID uuid.UUID, templateID uuid.UUID, userID uuid.UUID, workspaceID uuid.UUID, st *agentproto.Stats) error
}

type StatsAPI struct {
	AgentFn                   func(context.Context) (database.WorkspaceAgent, error)
	Database                  database.Store
	Pubsub                    pubsub.Pubsub
	Log                       slog.Logger
	StatsBatcher              StatsBatcher
	TemplateScheduleStore     *atomic.Pointer[schedule.TemplateScheduleStore]
	AgentStatsRefreshInterval time.Duration
	UpdateAgentMetricsFn      func(ctx context.Context, labels prometheusmetrics.AgentMetricLabels, metrics []*agentproto.Stats_Metric)
	StatsCollector            workspaceapps.StatsCollector

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

	now := a.now()
	var errGroup errgroup.Group
	errGroup.Go(func() error {
		err := a.StatsBatcher.Add(now, workspaceAgent.ID, workspace.TemplateID, workspace.OwnerID, workspace.ID, req.Stats)
		if err != nil {
			a.Log.Error(ctx, "add agent stats to batcher", slog.Error(err))
			return xerrors.Errorf("insert workspace agent stats batch: %w", err)
		}
		return nil
	})
	if a.UpdateAgentMetricsFn != nil {
		errGroup.Go(func() error {
			user, err := a.Database.GetUserByID(ctx, workspace.OwnerID)
			if err != nil {
				return xerrors.Errorf("get user: %w", err)
			}

			a.UpdateAgentMetricsFn(ctx, prometheusmetrics.AgentMetricLabels{
				Username:      user.Username,
				WorkspaceName: workspace.Name,
				AgentName:     workspaceAgent.Name,
				TemplateName:  getWorkspaceAgentByIDRow.TemplateName,
			}, req.Stats.Metrics)
			return nil
		})
	}
	err = errGroup.Wait()
	if err != nil {
		return nil, xerrors.Errorf("update stats in database: %w", err)
	}

	// Flushing the stats collector will update last_used_at,
	// dealine for the workspace, and will publish a workspace update event.
	a.StatsCollector.CollectAndFlush(ctx, workspaceapps.StatsReport{
		WorkspaceID: workspace.ID,
		// TODO: fill out
	})

	return res, nil
}

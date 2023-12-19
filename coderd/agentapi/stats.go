package agentapi

import (
	"context"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/types/known/durationpb"

	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/autobuild"
	"github.com/coder/coder/v2/coderd/batchstats"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/prometheusmetrics"
	"github.com/coder/coder/v2/coderd/schedule"
)

type StatsAPI struct {
	AgentFn                   func(context.Context) (database.WorkspaceAgent, error)
	Database                  database.Store
	Log                       slog.Logger
	StatsBatcher              *batchstats.Batcher
	TemplateScheduleStore     *atomic.Pointer[schedule.TemplateScheduleStore]
	AgentStatsRefreshInterval time.Duration
	UpdateAgentMetricsFn      func(ctx context.Context, labels prometheusmetrics.AgentMetricLabels, metrics []*agentproto.Stats_Metric)
}

func (a *StatsAPI) UpdateStats(ctx context.Context, req *agentproto.UpdateStatsRequest) (*agentproto.UpdateStatsResponse, error) {
	workspaceAgent, err := a.AgentFn(ctx)
	if err != nil {
		return nil, err
	}
	row, err := a.Database.GetWorkspaceByAgentID(ctx, workspaceAgent.ID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace by agent ID %q: %w", workspaceAgent.ID, err)
	}
	workspace := row.Workspace

	res := &agentproto.UpdateStatsResponse{
		ReportInterval: durationpb.New(a.AgentStatsRefreshInterval),
	}

	// An empty stat means it's just looking for the report interval.
	if len(req.Stats.ConnectionsByProto) == 0 {
		return res, nil
	}

	a.Log.Debug(ctx, "read stats report",
		slog.F("interval", a.AgentStatsRefreshInterval),
		slog.F("workspace_id", workspace.ID),
		slog.F("payload", req),
	)

	if req.Stats.ConnectionCount > 0 {
		var nextAutostart time.Time
		if workspace.AutostartSchedule.String != "" {
			templateSchedule, err := (*(a.TemplateScheduleStore.Load())).Get(ctx, a.Database, workspace.TemplateID)
			// If the template schedule fails to load, just default to bumping without the next trasition and log it.
			if err != nil {
				a.Log.Error(ctx, "failed to load template schedule bumping activity, defaulting to bumping by 60min",
					slog.F("workspace_id", workspace.ID),
					slog.F("template_id", workspace.TemplateID),
					slog.Error(err),
				)
			} else {
				next, allowed := autobuild.NextAutostartSchedule(time.Now(), workspace.AutostartSchedule.String, templateSchedule)
				if allowed {
					nextAutostart = next
				}
			}
		}
		ActivityBumpWorkspace(ctx, a.Log.Named("activity_bump"), a.Database, workspace.ID, nextAutostart)
	}

	now := dbtime.Now()

	var errGroup errgroup.Group
	errGroup.Go(func() error {
		if err := a.StatsBatcher.Add(time.Now(), workspaceAgent.ID, workspace.TemplateID, workspace.OwnerID, workspace.ID, req.Stats); err != nil {
			a.Log.Error(ctx, "failed to add stats to batcher", slog.Error(err))
			return xerrors.Errorf("can't insert workspace agent stat: %w", err)
		}
		return nil
	})
	errGroup.Go(func() error {
		err := a.Database.UpdateWorkspaceLastUsedAt(ctx, database.UpdateWorkspaceLastUsedAtParams{
			ID:         workspace.ID,
			LastUsedAt: now,
		})
		if err != nil {
			return xerrors.Errorf("can't update workspace LastUsedAt: %w", err)
		}
		return nil
	})
	if a.UpdateAgentMetricsFn != nil {
		errGroup.Go(func() error {
			user, err := a.Database.GetUserByID(ctx, workspace.OwnerID)
			if err != nil {
				return xerrors.Errorf("can't get user: %w", err)
			}

			a.UpdateAgentMetricsFn(ctx, prometheusmetrics.AgentMetricLabels{
				Username:      user.Username,
				WorkspaceName: workspace.Name,
				AgentName:     workspaceAgent.Name,
				TemplateName:  row.TemplateName,
			}, req.Stats.Metrics)
			return nil
		})
	}
	err = errGroup.Wait()
	if err != nil {
		return nil, xerrors.Errorf("update stats in database: %w", err)
	}

	return res, nil
}

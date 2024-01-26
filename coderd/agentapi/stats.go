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
	"github.com/coder/coder/v2/coderd/autobuild"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/prometheusmetrics"
	"github.com/coder/coder/v2/coderd/schedule"
)

type StatsBatcher interface {
	Add(now time.Time, agentID uuid.UUID, templateID uuid.UUID, userID uuid.UUID, workspaceID uuid.UUID, st *agentproto.Stats) error
}

type StatsAPI struct {
	AgentFn                   func(context.Context) (database.WorkspaceAgent, error)
	Database                  database.Store
	Log                       slog.Logger
	StatsBatcher              StatsBatcher
	TemplateScheduleStore     *atomic.Pointer[schedule.TemplateScheduleStore]
	AgentStatsRefreshInterval time.Duration
	UpdateAgentMetricsFn      func(ctx context.Context, labels prometheusmetrics.AgentMetricLabels, metrics []*agentproto.Stats_Metric)

	TimeNowFn func() time.Time // defaults to dbtime.Now()
}

func (a *StatsAPI) now() time.Time {
	if a.TimeNowFn != nil {
		return a.TimeNowFn()
	}
	return dbtime.Now()
}

func (a *StatsAPI) UpdateStats(ctx context.Context, req *agentproto.UpdateStatsRequest) (*agentproto.UpdateStatsResponse, error) {
	// An empty stat means it's just looking for the report interval.
	res := &agentproto.UpdateStatsResponse{
		ReportInterval: durationpb.New(a.AgentStatsRefreshInterval),
	}
	if req.Stats == nil || len(req.Stats.ConnectionsByProto) == 0 {
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
	if req.Stats.ConnectionCount > 0 {
		var nextAutostart time.Time
		if workspace.AutostartSchedule.String != "" {
			templateSchedule, err := (*(a.TemplateScheduleStore.Load())).Get(ctx, a.Database, workspace.TemplateID)
			// If the template schedule fails to load, just default to bumping
			// without the next transition and log it.
			if err != nil {
				a.Log.Error(ctx, "failed to load template schedule bumping activity, defaulting to bumping by 60min",
					slog.F("workspace_id", workspace.ID),
					slog.F("template_id", workspace.TemplateID),
					slog.Error(err),
				)
			} else {
				next, allowed := autobuild.NextAutostartSchedule(now, workspace.AutostartSchedule.String, templateSchedule)
				if allowed {
					nextAutostart = next
				}
			}
		}
		ActivityBumpWorkspace(ctx, a.Log.Named("activity_bump"), a.Database, workspace.ID, nextAutostart)
	}

	var errGroup errgroup.Group
	errGroup.Go(func() error {
		err := a.StatsBatcher.Add(now, workspaceAgent.ID, workspace.TemplateID, workspace.OwnerID, workspace.ID, req.Stats)
		if err != nil {
			a.Log.Error(ctx, "add agent stats to batcher", slog.Error(err))
			return xerrors.Errorf("insert workspace agent stats batch: %w", err)
		}
		return nil
	})
	errGroup.Go(func() error {
		err := a.Database.UpdateWorkspaceLastUsedAt(ctx, database.UpdateWorkspaceLastUsedAtParams{
			ID:         workspace.ID,
			LastUsedAt: now,
		})
		if err != nil {
			return xerrors.Errorf("update workspace LastUsedAt: %w", err)
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

	return res, nil
}

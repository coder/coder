package workspacestats

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	agentproto "github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/prometheusmetrics"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/codersdk"
)

type StatsBatcher interface {
	Add(now time.Time, agentID uuid.UUID, templateID uuid.UUID, userID uuid.UUID, workspaceID uuid.UUID, st *agentproto.Stats) error
}

type ReporterOptions struct {
	Database              database.Store
	Logger                slog.Logger
	Pubsub                pubsub.Pubsub
	TemplateScheduleStore *atomic.Pointer[schedule.TemplateScheduleStore]
	StatsBatcher          StatsBatcher
	UpdateAgentMetricsFn  func(ctx context.Context, labels prometheusmetrics.AgentMetricLabels, metrics []*agentproto.Stats_Metric)

	AppStatBatchSize int
}

type Reporter struct {
	opts ReporterOptions
}

func NewReporter(opts ReporterOptions) *Reporter {
	return &Reporter{opts: opts}
}

func (r *Reporter) ReportAppStats(ctx context.Context, stats []workspaceapps.StatsReport) error {
	err := r.opts.Database.InTx(func(tx database.Store) error {
		maxBatchSize := r.opts.AppStatBatchSize
		if len(stats) < maxBatchSize {
			maxBatchSize = len(stats)
		}
		batch := database.InsertWorkspaceAppStatsParams{
			UserID:           make([]uuid.UUID, 0, maxBatchSize),
			WorkspaceID:      make([]uuid.UUID, 0, maxBatchSize),
			AgentID:          make([]uuid.UUID, 0, maxBatchSize),
			AccessMethod:     make([]string, 0, maxBatchSize),
			SlugOrPort:       make([]string, 0, maxBatchSize),
			SessionID:        make([]uuid.UUID, 0, maxBatchSize),
			SessionStartedAt: make([]time.Time, 0, maxBatchSize),
			SessionEndedAt:   make([]time.Time, 0, maxBatchSize),
			Requests:         make([]int32, 0, maxBatchSize),
		}
		for _, stat := range stats {
			batch.UserID = append(batch.UserID, stat.UserID)
			batch.WorkspaceID = append(batch.WorkspaceID, stat.WorkspaceID)
			batch.AgentID = append(batch.AgentID, stat.AgentID)
			batch.AccessMethod = append(batch.AccessMethod, string(stat.AccessMethod))
			batch.SlugOrPort = append(batch.SlugOrPort, stat.SlugOrPort)
			batch.SessionID = append(batch.SessionID, stat.SessionID)
			batch.SessionStartedAt = append(batch.SessionStartedAt, stat.SessionStartedAt)
			batch.SessionEndedAt = append(batch.SessionEndedAt, stat.SessionEndedAt)
			batch.Requests = append(batch.Requests, int32(stat.Requests))

			if len(batch.UserID) >= r.opts.AppStatBatchSize {
				err := tx.InsertWorkspaceAppStats(ctx, batch)
				if err != nil {
					return err
				}

				// Reset batch.
				batch.UserID = batch.UserID[:0]
				batch.WorkspaceID = batch.WorkspaceID[:0]
				batch.AgentID = batch.AgentID[:0]
				batch.AccessMethod = batch.AccessMethod[:0]
				batch.SlugOrPort = batch.SlugOrPort[:0]
				batch.SessionID = batch.SessionID[:0]
				batch.SessionStartedAt = batch.SessionStartedAt[:0]
				batch.SessionEndedAt = batch.SessionEndedAt[:0]
				batch.Requests = batch.Requests[:0]
			}
		}
		if len(batch.UserID) == 0 {
			return nil
		}

		if err := tx.InsertWorkspaceAppStats(ctx, batch); err != nil {
			return err
		}

		// TODO: We currently measure workspace usage based on when we get stats from it.
		// There are currently two paths for this:
		// 1) From SSH -> workspace agent stats POSTed from agent
		// 2) From workspace apps / rpty -> workspace app stats (from coderd / wsproxy)
		// Ideally we would have a single code path for this.
		uniqueIDs := slice.Unique(batch.WorkspaceID)
		if err := tx.BatchUpdateWorkspaceLastUsedAt(ctx, database.BatchUpdateWorkspaceLastUsedAtParams{
			IDs:        uniqueIDs,
			LastUsedAt: dbtime.Now(), // This isn't 100% accurate, but it's good enough.
		}); err != nil {
			return err
		}

		return nil
	}, nil)
	if err != nil {
		return xerrors.Errorf("insert workspace app stats failed: %w", err)
	}

	return nil
}

func (r *Reporter) ReportAgentStats(ctx context.Context, now time.Time, workspace database.Workspace, workspaceAgent database.WorkspaceAgent, templateName string, stats *agentproto.Stats) error {
	if stats.ConnectionCount > 0 {
		var nextAutostart time.Time
		if workspace.AutostartSchedule.String != "" {
			templateSchedule, err := (*(r.opts.TemplateScheduleStore.Load())).Get(ctx, r.opts.Database, workspace.TemplateID)
			// If the template schedule fails to load, just default to bumping
			// without the next transition and log it.
			if err != nil {
				r.opts.Logger.Error(ctx, "failed to load template schedule bumping activity, defaulting to bumping by 60min",
					slog.F("workspace_id", workspace.ID),
					slog.F("template_id", workspace.TemplateID),
					slog.Error(err),
				)
			} else {
				next, allowed := schedule.NextAutostart(now, workspace.AutostartSchedule.String, templateSchedule)
				if allowed {
					nextAutostart = next
				}
			}
		}
		ActivityBumpWorkspace(ctx, r.opts.Logger.Named("activity_bump"), r.opts.Database, workspace.ID, nextAutostart)
	}

	var errGroup errgroup.Group
	errGroup.Go(func() error {
		err := r.opts.StatsBatcher.Add(now, workspaceAgent.ID, workspace.TemplateID, workspace.OwnerID, workspace.ID, stats)
		if err != nil {
			r.opts.Logger.Error(ctx, "add agent stats to batcher", slog.Error(err))
			return xerrors.Errorf("insert workspace agent stats batch: %w", err)
		}
		return nil
	})
	errGroup.Go(func() error {
		err := r.opts.Database.UpdateWorkspaceLastUsedAt(ctx, database.UpdateWorkspaceLastUsedAtParams{
			ID:         workspace.ID,
			LastUsedAt: now,
		})
		if err != nil {
			return xerrors.Errorf("update workspace LastUsedAt: %w", err)
		}
		return nil
	})
	if r.opts.UpdateAgentMetricsFn != nil {
		errGroup.Go(func() error {
			user, err := r.opts.Database.GetUserByID(ctx, workspace.OwnerID)
			if err != nil {
				return xerrors.Errorf("get user: %w", err)
			}

			r.opts.UpdateAgentMetricsFn(ctx, prometheusmetrics.AgentMetricLabels{
				Username:      user.Username,
				WorkspaceName: workspace.Name,
				AgentName:     workspaceAgent.Name,
				TemplateName:  templateName,
			}, stats.Metrics)
			return nil
		})
	}
	err := errGroup.Wait()
	if err != nil {
		return xerrors.Errorf("update stats in database: %w", err)
	}

	err = r.opts.Pubsub.Publish(codersdk.WorkspaceNotifyChannel(workspace.ID), []byte{})
	if err != nil {
		r.opts.Logger.Warn(ctx, "failed to publish workspace agent stats",
			slog.F("workspace_id", workspace.ID), slog.Error(err))
	}

	return nil
}

type UpdateTemplateWorkspacesLastUsedAtFunc func(ctx context.Context, db database.Store, templateID uuid.UUID, lastUsedAt time.Time) error

func UpdateTemplateWorkspacesLastUsedAt(ctx context.Context, db database.Store, templateID uuid.UUID, lastUsedAt time.Time) error {
	err := db.UpdateTemplateWorkspacesLastUsedAt(ctx, database.UpdateTemplateWorkspacesLastUsedAtParams{
		TemplateID: templateID,
		LastUsedAt: lastUsedAt,
	})
	if err != nil {
		return xerrors.Errorf("update template workspaces last used at: %w", err)
	}
	return nil
}

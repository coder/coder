package executor

import (
	"context"
	"encoding/json"
	"time"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd/autobuild/schedule"
	"github.com/coder/coder/coderd/database"

	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"
)

// Executor automatically starts or stops workspaces.
type Executor struct {
	ctx  context.Context
	db   database.Store
	log  slog.Logger
	tick <-chan time.Time
}

// New returns a new autobuild executor.
func New(ctx context.Context, db database.Store, log slog.Logger, tick <-chan time.Time) *Executor {
	le := &Executor{
		ctx:  ctx,
		db:   db,
		tick: tick,
		log:  log,
	}
	return le
}

// Run will cause executor to start or stop workspaces on every
// tick from its channel. It will stop when its context is Done, or when
// its channel is closed.
func (e *Executor) Run() {
	go func() {
		for t := range e.tick {
			if err := e.runOnce(t); err != nil {
				e.log.Error(e.ctx, "error running once", slog.Error(err))
			}
		}
	}()
}

func (e *Executor) runOnce(t time.Time) error {
	currentTick := t.Truncate(time.Minute)
	return e.db.InTx(func(db database.Store) error {
		eligibleWorkspaces, err := db.GetWorkspacesAutostart(e.ctx)
		if err != nil {
			return xerrors.Errorf("get eligible workspaces for autostart or autostop: %w", err)
		}

		for _, ws := range eligibleWorkspaces {
			// Determine the workspace state based on its latest build.
			priorHistory, err := db.GetLatestWorkspaceBuildByWorkspaceID(e.ctx, ws.ID)
			if err != nil {
				e.log.Warn(e.ctx, "get latest workspace build",
					slog.F("workspace_id", ws.ID),
					slog.Error(err),
				)
				continue
			}

			priorJob, err := db.GetProvisionerJobByID(e.ctx, priorHistory.JobID)
			if err != nil {
				e.log.Warn(e.ctx, "get last provisioner job for workspace %q: %w",
					slog.F("workspace_id", ws.ID),
					slog.Error(err),
				)
				continue
			}

			if !priorJob.CompletedAt.Valid || priorJob.Error.String != "" {
				e.log.Warn(e.ctx, "last workspace build did not complete successfully, skipping",
					slog.F("workspace_id", ws.ID),
					slog.F("error", priorJob.Error.String),
				)
				continue
			}

			var validTransition database.WorkspaceTransition
			var nextTransition time.Time
			switch priorHistory.Transition {
			case database.WorkspaceTransitionStart:
				validTransition = database.WorkspaceTransitionStop
				if !ws.Ttl.Valid || ws.Ttl.Int64 == 0 {
					e.log.Debug(e.ctx, "invalid or zero ws ttl, skipping",
						slog.F("workspace_id", ws.ID),
						slog.F("ttl", time.Duration(ws.Ttl.Int64)),
					)
					continue
				}
				ttl := time.Duration(ws.Ttl.Int64)
				// Measure TTL from the time the workspace finished building.
				// Truncate to nearest minute for consistency with autostart
				// behavior, and add one minute for padding.
				nextTransition = priorHistory.UpdatedAt.Truncate(time.Minute).Add(ttl + time.Minute)
			case database.WorkspaceTransitionStop:
				validTransition = database.WorkspaceTransitionStart
				sched, err := schedule.Weekly(ws.AutostartSchedule.String)
				if err != nil {
					e.log.Warn(e.ctx, "workspace has invalid autostart schedule, skipping",
						slog.F("workspace_id", ws.ID),
						slog.F("autostart_schedule", ws.AutostartSchedule.String),
					)
					continue
				}
				// Round down to the nearest minute, as this is the finest granularity cron supports.
				// Truncate is probably not necessary here, but doing it anyway to be sure.
				nextTransition = sched.Next(priorHistory.CreatedAt).Truncate(time.Minute)
			default:
				e.log.Debug(e.ctx, "last transition not valid for autostart or autostop",
					slog.F("workspace_id", ws.ID),
					slog.F("latest_build_transition", priorHistory.Transition),
				)
				continue
			}

			if currentTick.Before(nextTransition) {
				e.log.Debug(e.ctx, "skipping workspace: too early",
					slog.F("workspace_id", ws.ID),
					slog.F("next_transition_at", nextTransition),
					slog.F("transition", validTransition),
					slog.F("current_tick", currentTick),
				)
				continue
			}

			e.log.Info(e.ctx, "scheduling workspace transition",
				slog.F("workspace_id", ws.ID),
				slog.F("transition", validTransition),
			)

			if err := build(e.ctx, db, ws, validTransition, priorHistory, priorJob); err != nil {
				e.log.Error(e.ctx, "unable to transition workspace",
					slog.F("workspace_id", ws.ID),
					slog.F("transition", validTransition),
					slog.Error(err),
				)
			}
		}
		return nil
	})
}

// TODO(cian): this function duplicates most of api.postWorkspaceBuilds. Refactor.
// See: https://github.com/coder/coder/issues/1401
func build(ctx context.Context, store database.Store, workspace database.Workspace, trans database.WorkspaceTransition, priorHistory database.WorkspaceBuild, priorJob database.ProvisionerJob) error {
	template, err := store.GetTemplateByID(ctx, workspace.TemplateID)
	if err != nil {
		return xerrors.Errorf("get workspace template: %w", err)
	}

	priorBuildNumber := priorHistory.BuildNumber

	// This must happen in a transaction to ensure history can be inserted, and
	// the prior history can update it's "after" column to point at the new.
	workspaceBuildID := uuid.New()
	input, err := json.Marshal(struct {
		WorkspaceBuildID string `json:"workspace_build_id"`
	}{
		WorkspaceBuildID: workspaceBuildID.String(),
	})
	if err != nil {
		return xerrors.Errorf("marshal provision job: %w", err)
	}
	provisionerJobID := uuid.New()
	now := database.Now()
	newProvisionerJob, err := store.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
		ID:             provisionerJobID,
		CreatedAt:      now,
		UpdatedAt:      now,
		InitiatorID:    workspace.OwnerID,
		OrganizationID: template.OrganizationID,
		Provisioner:    template.Provisioner,
		Type:           database.ProvisionerJobTypeWorkspaceBuild,
		StorageMethod:  priorJob.StorageMethod,
		StorageSource:  priorJob.StorageSource,
		Input:          input,
	})
	if err != nil {
		return xerrors.Errorf("insert provisioner job: %w", err)
	}
	_, err = store.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
		ID:                workspaceBuildID,
		CreatedAt:         now,
		UpdatedAt:         now,
		WorkspaceID:       workspace.ID,
		TemplateVersionID: priorHistory.TemplateVersionID,
		BuildNumber:       priorBuildNumber + 1,
		Name:              namesgenerator.GetRandomName(1),
		ProvisionerState:  priorHistory.ProvisionerState,
		InitiatorID:       workspace.OwnerID,
		Transition:        trans,
		JobID:             newProvisionerJob.ID,
	})
	if err != nil {
		return xerrors.Errorf("insert workspace build: %w", err)
	}
	return nil
}

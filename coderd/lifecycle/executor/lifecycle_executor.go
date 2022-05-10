package executor

import (
	"context"
	"encoding/json"
	"time"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/lifecycle/schedule"

	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"
)

// Executor executes automated workspace lifecycle operations.
type Executor struct {
	ctx  context.Context
	db   database.Store
	log  slog.Logger
	tick <-chan time.Time
}

// New returns a new lifecycle executor.
func New(ctx context.Context, db database.Store, log slog.Logger, tick <-chan time.Time) *Executor {
	le := &Executor{
		ctx:  ctx,
		db:   db,
		tick: tick,
		log:  log,
	}
	return le
}

// Run will cause executor to run workspace lifecycle operations on every
// tick from its channel. It will stop when its context is Done, or when
// its channel is closed.
func (e *Executor) Run() {
	for t := range e.tick {
		if err := e.runOnce(t); err != nil {
			e.log.Error(e.ctx, "error running once", slog.Error(err))
		}
	}
}

func (e *Executor) runOnce(t time.Time) error {
	currentTick := t.Round(time.Minute)
	return e.db.InTx(func(db database.Store) error {
		eligibleWorkspaces, err := db.GetWorkspacesAutostartAutostop(e.ctx)
		if err != nil {
			return xerrors.Errorf("get eligible workspaces for autostart or autostop: %w", err)
		}

		for _, ws := range eligibleWorkspaces {
			// Determine the workspace state based on its latest build.
			latestBuild, err := db.GetWorkspaceBuildByWorkspaceIDWithoutAfter(e.ctx, ws.ID)
			if err != nil {
				return xerrors.Errorf("get latest build for workspace %q: %w", ws.ID, err)
			}

			var validTransition database.WorkspaceTransition
			var sched *schedule.Schedule
			switch latestBuild.Transition {
			case database.WorkspaceTransitionStart:
				validTransition = database.WorkspaceTransitionStop
				sched, err = schedule.Weekly(ws.AutostopSchedule.String)
				if err != nil {
					e.log.Warn(e.ctx, "workspace has invalid autostop schedule, skipping",
						slog.F("workspace_id", ws.ID),
						slog.F("autostart_schedule", ws.AutostopSchedule.String),
					)
					continue
				}
			case database.WorkspaceTransitionStop:
				validTransition = database.WorkspaceTransitionStart
				sched, err = schedule.Weekly(ws.AutostartSchedule.String)
				if err != nil {
					e.log.Warn(e.ctx, "workspace has invalid autostart schedule, skipping",
						slog.F("workspace_id", ws.ID),
						slog.F("autostart_schedule", ws.AutostartSchedule.String),
					)
					continue
				}
			default:
				e.log.Debug(e.ctx, "last transition not valid for autostart or autostop",
					slog.F("workspace_id", ws.ID),
					slog.F("latest_build_transition", latestBuild.Transition),
				)
				continue
			}

			// Round time to the nearest minute, as this is the finest granularity cron supports.
			nextTransitionAt := sched.Next(latestBuild.CreatedAt).Round(time.Minute)
			if nextTransitionAt.After(currentTick) {
				e.log.Debug(e.ctx, "skipping workspace: too early",
					slog.F("workspace_id", ws.ID),
					slog.F("next_transition_at", nextTransitionAt),
					slog.F("transition", validTransition),
					slog.F("current_tick", currentTick),
				)
				continue
			}

			e.log.Info(e.ctx, "scheduling workspace transition",
				slog.F("workspace_id", ws.ID),
				slog.F("transition", validTransition),
			)

			if err := doBuild(e.ctx, db, ws, validTransition); err != nil {
				e.log.Error(e.ctx, "transition workspace",
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
func doBuild(ctx context.Context, store database.Store, workspace database.Workspace, trans database.WorkspaceTransition) error {
	template, err := store.GetTemplateByID(ctx, workspace.TemplateID)
	if err != nil {
		return xerrors.Errorf("get template: %w", err)
	}

	priorHistory, err := store.GetWorkspaceBuildByWorkspaceIDWithoutAfter(ctx, workspace.ID)
	if err != nil {
		return xerrors.Errorf("get prior history: %w", err)
	}

	priorJob, err := store.GetProvisionerJobByID(ctx, priorHistory.JobID)
	if err == nil && !priorJob.CompletedAt.Valid {
		return xerrors.Errorf("workspace build already active")
	}

	priorHistoryID := uuid.NullUUID{
		UUID:  priorHistory.ID,
		Valid: true,
	}

	var newWorkspaceBuild database.WorkspaceBuild
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
	newProvisionerJob, err := store.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
		ID:             provisionerJobID,
		CreatedAt:      database.Now(),
		UpdatedAt:      database.Now(),
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
	newWorkspaceBuild, err = store.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
		ID:                workspaceBuildID,
		CreatedAt:         database.Now(),
		UpdatedAt:         database.Now(),
		WorkspaceID:       workspace.ID,
		TemplateVersionID: priorHistory.TemplateVersionID,
		BeforeID:          priorHistoryID,
		Name:              namesgenerator.GetRandomName(1),
		ProvisionerState:  priorHistory.ProvisionerState,
		InitiatorID:       workspace.OwnerID,
		Transition:        trans,
		JobID:             newProvisionerJob.ID,
	})
	if err != nil {
		return xerrors.Errorf("insert workspace build: %w", err)
	}

	if priorHistoryID.Valid {
		// Update the prior history entries "after" column.
		err = store.UpdateWorkspaceBuildByID(ctx, database.UpdateWorkspaceBuildByIDParams{
			ID:               priorHistory.ID,
			ProvisionerState: priorHistory.ProvisionerState,
			UpdatedAt:        database.Now(),
			AfterID: uuid.NullUUID{
				UUID:  newWorkspaceBuild.ID,
				Valid: true,
			},
		})
		if err != nil {
			return xerrors.Errorf("update prior workspace build: %w", err)
		}
	}
	return nil
}

package lifecycle

import (
	"context"
	"encoding/json"
	"time"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd/autostart/schedule"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"

	"github.com/google/uuid"
	"github.com/moby/moby/pkg/namesgenerator"
	"golang.org/x/xerrors"
)

//var ExecutorUUID = uuid.MustParse("00000000-0000-0000-0000-000000000000")

// Executor executes automated workspace lifecycle operations.
type Executor struct {
	ctx  context.Context
	db   database.Store
	log  slog.Logger
	tick <-chan time.Time
}

func NewExecutor(ctx context.Context, db database.Store, log slog.Logger, tick <-chan time.Time) *Executor {
	le := &Executor{
		ctx:  ctx,
		db:   db,
		tick: tick,
		log:  log,
	}
	return le
}

func (e *Executor) Run() error {
	for {
		select {
		case t := <-e.tick:
			if err := e.runOnce(t); err != nil {
				e.log.Error(e.ctx, "error running once", slog.Error(err))
			}
		case <-e.ctx.Done():
			return nil
		default:
		}
	}
}

func (e *Executor) runOnce(t time.Time) error {
	currentTick := t.Round(time.Minute)
	return e.db.InTx(func(db database.Store) error {
		allWorkspaces, err := db.GetWorkspaces(e.ctx)
		if err != nil {
			return xerrors.Errorf("get all workspaces: %w", err)
		}

		for _, ws := range allWorkspaces {
			// We only care about workspaces with autostart enabled.
			if ws.AutostartSchedule.String == "" {
				continue
			}
			sched, err := schedule.Weekly(ws.AutostartSchedule.String)
			if err != nil {
				e.log.Warn(e.ctx, "workspace has invalid autostart schedule",
					slog.F("workspace_id", ws.ID),
					slog.F("autostart_schedule", ws.AutostartSchedule.String),
				)
				continue
			}

			// Determine the workspace state based on its latest build. We expect it to be stopped.
			// TODO(cian): is this **guaranteed** to be the latest build???
			latestBuild, err := db.GetWorkspaceBuildByWorkspaceIDWithoutAfter(e.ctx, ws.ID)
			if err != nil {
				return xerrors.Errorf("get latest build for workspace %q: %w", ws.ID, err)
			}
			if latestBuild.Transition != database.WorkspaceTransitionStop {
				e.log.Debug(e.ctx, "autostart: skipping workspace: wrong transition",
					slog.F("transition", latestBuild.Transition),
					slog.F("workspace_id", ws.ID),
				)
				continue
			}

			// Round time to the nearest minute, as this is the finest granularity cron supports.
			earliestAutostart := sched.Next(latestBuild.CreatedAt).Round(time.Minute)
			if earliestAutostart.After(currentTick) {
				e.log.Debug(e.ctx, "autostart: skipping workspace: too early",
					slog.F("workspace_id", ws.ID),
					slog.F("earliest_autostart", earliestAutostart),
					slog.F("current_tick", currentTick),
				)
				continue
			}

			e.log.Info(e.ctx, "autostart: scheduling workspace start",
				slog.F("workspace_id", ws.ID),
			)

			if err := doBuild(e.ctx, db, ws, currentTick); err != nil {
				e.log.Error(e.ctx, "autostart workspace", slog.F("workspace_id", ws.ID), slog.Error(err))
			}
		}
		return nil
	})
}

// XXX: cian: this function shouldn't really exist. Refactor.
func doBuild(ctx context.Context, store database.Store, workspace database.Workspace, now time.Time) error {
	template, err := store.GetTemplateByID(ctx, workspace.TemplateID)
	if err != nil {
		return xerrors.Errorf("get template: %w", err)
	}

	priorHistory, err := store.GetWorkspaceBuildByWorkspaceIDWithoutAfter(ctx, workspace.ID)
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
		Transition:        database.WorkspaceTransitionStart,
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

func provisionerJobStatus(j database.ProvisionerJob, now time.Time) codersdk.ProvisionerJobStatus {
	switch {
	case j.CanceledAt.Valid:
		if j.CompletedAt.Valid {
			return codersdk.ProvisionerJobCanceled
		}
		return codersdk.ProvisionerJobCanceling
	case !j.StartedAt.Valid:
		return codersdk.ProvisionerJobPending
	case j.CompletedAt.Valid:
		if j.Error.String == "" {
			return codersdk.ProvisionerJobSucceeded
		}
		return codersdk.ProvisionerJobFailed
	case now.Sub(j.UpdatedAt) > 30*time.Second:
		return codersdk.ProvisionerJobFailed
	default:
		return codersdk.ProvisionerJobRunning
	}
}

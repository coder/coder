package executor

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/autobuild/schedule"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/provisionerdserver"
)

// Executor automatically starts or stops workspaces.
type Executor struct {
	ctx     context.Context
	db      database.Store
	log     slog.Logger
	tick    <-chan time.Time
	statsCh chan<- Stats
}

// Stats contains information about one run of Executor.
type Stats struct {
	Transitions map[uuid.UUID]database.WorkspaceTransition
	Elapsed     time.Duration
	Error       error
}

// New returns a new autobuild executor.
func New(ctx context.Context, db database.Store, log slog.Logger, tick <-chan time.Time) *Executor {
	le := &Executor{
		//nolint:gocritic // Autostart has a limited set of permissions.
		ctx:  dbauthz.AsAutostart(ctx),
		db:   db,
		tick: tick,
		log:  log,
	}
	return le
}

// WithStatsChannel will cause Executor to push a RunStats to ch after
// every tick.
func (e *Executor) WithStatsChannel(ch chan<- Stats) *Executor {
	e.statsCh = ch
	return e
}

// Run will cause executor to start or stop workspaces on every
// tick from its channel. It will stop when its context is Done, or when
// its channel is closed.
func (e *Executor) Run() {
	go func() {
		for {
			select {
			case <-e.ctx.Done():
				return
			case t, ok := <-e.tick:
				if !ok {
					return
				}
				stats := e.runOnce(t)
				if stats.Error != nil {
					e.log.Error(e.ctx, "error running once", slog.Error(stats.Error))
				}
				if e.statsCh != nil {
					select {
					case <-e.ctx.Done():
						return
					case e.statsCh <- stats:
					}
				}
				e.log.Debug(e.ctx, "run stats", slog.F("elapsed", stats.Elapsed), slog.F("transitions", stats.Transitions))
			}
		}
	}()
}

func (e *Executor) runOnce(t time.Time) Stats {
	var err error
	stats := Stats{
		Transitions: make(map[uuid.UUID]database.WorkspaceTransition),
	}
	defer func() {
		stats.Elapsed = time.Since(t)
		stats.Error = err
	}()
	currentTick := t.Truncate(time.Minute)

	// TTL is set at the workspace level, and deadline at the workspace build level.
	// When a workspace build is created, its deadline initially starts at zero.
	// When provisionerd successfully completes a provision job, the deadline is
	// set to now + TTL if the associated workspace has a TTL set. This deadline
	// is what we compare against when performing autostop operations, rounded down
	// to the minute.
	//
	// NOTE: If a workspace build is created with a given TTL and then the user either
	//       changes or unsets the TTL, the deadline for the workspace build will not
	//       have changed. This behavior is as expected per #2229.
	workspaceRows, err := e.db.GetWorkspaces(e.ctx, database.GetWorkspacesParams{
		Deleted: false,
	})
	if err != nil {
		e.log.Error(e.ctx, "get workspaces for autostart or autostop", slog.Error(err))
		return stats
	}
	workspaces := database.ConvertWorkspaceRows(workspaceRows)

	var eligibleWorkspaceIDs []uuid.UUID
	for _, ws := range workspaces {
		if isEligibleForAutoStartStop(ws) {
			eligibleWorkspaceIDs = append(eligibleWorkspaceIDs, ws.ID)
		}
	}

	// We only use errgroup here for convenience of API, not for early
	// cancellation. This means we only return nil errors in th eg.Go.
	eg := errgroup.Group{}
	// Limit the concurrency to avoid overloading the database.
	eg.SetLimit(10)

	for _, wsID := range eligibleWorkspaceIDs {
		wsID := wsID
		log := e.log.With(slog.F("workspace_id", wsID))

		eg.Go(func() error {
			err := e.db.InTx(func(db database.Store) error {
				// Re-check eligibility since the first check was outside the
				// transaction and the workspace settings may have changed.
				ws, err := db.GetWorkspaceByID(e.ctx, wsID)
				if err != nil {
					log.Error(e.ctx, "get workspace autostart failed", slog.Error(err))
					return nil
				}
				if !isEligibleForAutoStartStop(ws) {
					return nil
				}

				// Determine the workspace state based on its latest build.
				priorHistory, err := db.GetLatestWorkspaceBuildByWorkspaceID(e.ctx, ws.ID)
				if err != nil {
					log.Warn(e.ctx, "get latest workspace build", slog.Error(err))
					return nil
				}

				priorJob, err := db.GetProvisionerJobByID(e.ctx, priorHistory.JobID)
				if err != nil {
					log.Warn(e.ctx, "get last provisioner job for workspace %q: %w", slog.Error(err))
					return nil
				}

				validTransition, nextTransition, err := getNextTransition(ws, priorHistory, priorJob)
				if err != nil {
					log.Debug(e.ctx, "skipping workspace", slog.Error(err))
					return nil
				}

				if currentTick.Before(nextTransition) {
					log.Debug(e.ctx, "skipping workspace: too early",
						slog.F("next_transition_at", nextTransition),
						slog.F("transition", validTransition),
						slog.F("current_tick", currentTick),
					)
					return nil
				}

				log.Info(e.ctx, "scheduling workspace transition", slog.F("transition", validTransition))

				stats.Transitions[ws.ID] = validTransition
				if err := build(e.ctx, db, ws, validTransition, priorHistory, priorJob); err != nil {
					log.Error(e.ctx, "unable to transition workspace",
						slog.F("transition", validTransition),
						slog.Error(err),
					)
					return nil
				}

				return nil
			}, nil)
			if err != nil {
				log.Error(e.ctx, "workspace scheduling failed", slog.Error(err))
			}
			return nil
		})
	}

	// This should not happen since we don't want early cancellation.
	err = eg.Wait()
	if err != nil {
		e.log.Error(e.ctx, "workspace scheduling errgroup failed", slog.Error(err))
	}

	return stats
}

func isEligibleForAutoStartStop(ws database.Workspace) bool {
	return !ws.Deleted && (ws.AutostartSchedule.String != "" || ws.Ttl.Int64 > 0)
}

func getNextTransition(
	ws database.Workspace,
	priorHistory database.WorkspaceBuild,
	priorJob database.ProvisionerJob,
) (
	validTransition database.WorkspaceTransition,
	nextTransition time.Time,
	err error,
) {
	if !priorJob.CompletedAt.Valid || priorJob.Error.String != "" {
		return "", time.Time{}, xerrors.Errorf("last workspace build did not complete successfully")
	}

	switch priorHistory.Transition {
	case database.WorkspaceTransitionStart:
		if priorHistory.Deadline.IsZero() {
			return "", time.Time{}, xerrors.Errorf("latest workspace build has zero deadline")
		}
		// For stopping, do not truncate. This is inconsistent with autostart, but
		// it ensures we will not stop too early.
		return database.WorkspaceTransitionStop, priorHistory.Deadline, nil
	case database.WorkspaceTransitionStop:
		sched, err := schedule.Weekly(ws.AutostartSchedule.String)
		if err != nil {
			return "", time.Time{}, xerrors.Errorf("workspace has invalid autostart schedule: %w", err)
		}
		// Round down to the nearest minute, as this is the finest granularity cron supports.
		// Truncate is probably not necessary here, but doing it anyway to be sure.
		nextTransition = sched.Next(priorHistory.CreatedAt).Truncate(time.Minute)
		return database.WorkspaceTransitionStart, nextTransition, nil
	default:
		return "", time.Time{}, xerrors.Errorf("last transition not valid for autostart or autostop")
	}
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
	input, err := json.Marshal(provisionerdserver.WorkspaceProvisionJob{
		WorkspaceBuildID: workspaceBuildID,
	})
	if err != nil {
		return xerrors.Errorf("marshal provision job: %w", err)
	}
	provisionerJobID := uuid.New()
	now := database.Now()

	var buildReason database.BuildReason
	switch trans {
	case database.WorkspaceTransitionStart:
		buildReason = database.BuildReasonAutostart
	case database.WorkspaceTransitionStop:
		buildReason = database.BuildReasonAutostop
	default:
		return xerrors.Errorf("Unsupported transition: %q", trans)
	}

	lastBuildParameters, err := store.GetWorkspaceBuildParameters(ctx, priorHistory.ID)
	if err != nil {
		return xerrors.Errorf("fetch prior workspace build parameters: %w", err)
	}

	return store.InTx(func(db database.Store) error {
		newProvisionerJob, err := store.InsertProvisionerJob(ctx, database.InsertProvisionerJobParams{
			ID:             provisionerJobID,
			CreatedAt:      now,
			UpdatedAt:      now,
			InitiatorID:    workspace.OwnerID,
			OrganizationID: template.OrganizationID,
			Provisioner:    template.Provisioner,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			StorageMethod:  priorJob.StorageMethod,
			FileID:         priorJob.FileID,
			Tags:           priorJob.Tags,
			Input:          input,
		})
		if err != nil {
			return xerrors.Errorf("insert provisioner job: %w", err)
		}
		workspaceBuild, err := store.InsertWorkspaceBuild(ctx, database.InsertWorkspaceBuildParams{
			ID:                workspaceBuildID,
			CreatedAt:         now,
			UpdatedAt:         now,
			WorkspaceID:       workspace.ID,
			TemplateVersionID: template.ActiveVersionID,
			BuildNumber:       priorBuildNumber + 1,
			ProvisionerState:  priorHistory.ProvisionerState,
			InitiatorID:       workspace.OwnerID,
			Transition:        trans,
			JobID:             newProvisionerJob.ID,
			Reason:            buildReason,
		})
		if err != nil {
			return xerrors.Errorf("insert workspace build: %w", err)
		}

		names := make([]string, 0, len(lastBuildParameters))
		values := make([]string, 0, len(lastBuildParameters))
		for _, param := range lastBuildParameters {
			names = append(names, param.Name)
			values = append(values, param.Value)
		}
		err = db.InsertWorkspaceBuildParameters(ctx, database.InsertWorkspaceBuildParametersParams{
			WorkspaceBuildID: workspaceBuild.ID,
			Name:             names,
			Value:            values,
		})
		if err != nil {
			return xerrors.Errorf("insert workspace build parameters: %w", err)
		}
		return nil
	}, nil)
}

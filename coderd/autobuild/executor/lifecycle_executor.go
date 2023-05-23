package executor

import (
	"context"
	"database/sql"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/schedule"
	"github.com/coder/coder/coderd/wsbuilder"
)

// Executor automatically starts or stops workspaces.
type Executor struct {
	ctx                   context.Context
	db                    database.Store
	templateScheduleStore *atomic.Pointer[schedule.TemplateScheduleStore]
	log                   slog.Logger
	tick                  <-chan time.Time
	statsCh               chan<- Stats
}

// Stats contains information about one run of Executor.
type Stats struct {
	Transitions map[uuid.UUID]database.WorkspaceTransition
	Elapsed     time.Duration
	Error       error
}

// New returns a new autobuild executor.
func New(ctx context.Context, db database.Store, tss *atomic.Pointer[schedule.TemplateScheduleStore], log slog.Logger, tick <-chan time.Time) *Executor {
	le := &Executor{
		//nolint:gocritic // Autostart has a limited set of permissions.
		ctx:                   dbauthz.AsAutostart(ctx),
		db:                    db,
		templateScheduleStore: tss,
		tick:                  tick,
		log:                   log,
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
	workspaces, err := e.db.GetWorkspacesEligibleForAutoStartStop(e.ctx, t)
	if err != nil {
		e.log.Error(e.ctx, "get workspaces for autostart or autostop", slog.Error(err))
		return stats
	}

	// We only use errgroup here for convenience of API, not for early
	// cancellation. This means we only return nil errors in th eg.Go.
	eg := errgroup.Group{}
	// Limit the concurrency to avoid overloading the database.
	eg.SetLimit(10)

	for _, ws := range workspaces {
		wsID := ws.ID
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

				// Determine the workspace state based on its latest build.
				priorHistory, err := db.GetLatestWorkspaceBuildByWorkspaceID(e.ctx, ws.ID)
				if err != nil {
					log.Warn(e.ctx, "get latest workspace build", slog.Error(err))
					return nil
				}

				templateSchedule, err := (*(e.templateScheduleStore.Load())).GetTemplateScheduleOptions(e.ctx, db, ws.TemplateID)
				if err != nil {
					log.Warn(e.ctx, "get template schedule options", slog.Error(err))
					return nil
				}

				if !isEligibleForAutoStartStop(ws, priorHistory, templateSchedule) {
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
				builder := wsbuilder.New(ws, validTransition).
					SetLastWorkspaceBuildInTx(&priorHistory).
					SetLastWorkspaceBuildJobInTx(&priorJob)

				switch validTransition {
				case database.WorkspaceTransitionStart:
					builder = builder.Reason(database.BuildReasonAutostart)
				case database.WorkspaceTransitionStop:
					builder = builder.Reason(database.BuildReasonAutostop)
				default:
					log.Error(e.ctx, "unsupported transition", slog.F("transition", validTransition))
					return nil
				}
				if _, _, err := builder.Build(e.ctx, db, nil); err != nil {
					log.Error(e.ctx, "unable to transition workspace",
						slog.F("transition", validTransition),
						slog.Error(err),
					)
					return nil
				}
				stats.Transitions[ws.ID] = validTransition

				log.Info(e.ctx, "scheduling workspace transition", slog.F("transition", validTransition))

				return nil

				// Run with RepeatableRead isolation so that the build process sees the same data
				// as our calculation that determines whether an autobuild is necessary.
			}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
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

func isEligibleForAutoStartStop(ws database.Workspace, priorHistory database.WorkspaceBuild, templateSchedule schedule.TemplateScheduleOptions) bool {
	if ws.Deleted {
		return false
	}
	if templateSchedule.UserAutostartEnabled && ws.AutostartSchedule.Valid && ws.AutostartSchedule.String != "" {
		return true
	}
	// Don't check the template schedule to see whether it allows autostop, this
	// is done during the build when determining the deadline.
	if priorHistory.Transition == database.WorkspaceTransitionStart && !priorHistory.Deadline.IsZero() {
		return true
	}

	return false
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

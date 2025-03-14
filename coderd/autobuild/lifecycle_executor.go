package autobuild
import (
	"errors"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
	"github.com/dustin/go-humanize"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/sync/errgroup"
	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/provisionerjobs"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/wsbuilder"
)
// Executor automatically starts or stops workspaces.
type Executor struct {
	ctx                   context.Context
	db                    database.Store
	ps                    pubsub.Pubsub
	templateScheduleStore *atomic.Pointer[schedule.TemplateScheduleStore]
	accessControlStore    *atomic.Pointer[dbauthz.AccessControlStore]
	auditor               *atomic.Pointer[audit.Auditor]
	log                   slog.Logger
	tick                  <-chan time.Time
	statsCh               chan<- Stats
	// NotificationsEnqueuer handles enqueueing notifications for delivery by SMTP, webhook, etc.
	notificationsEnqueuer notifications.Enqueuer
	reg                   prometheus.Registerer
	metrics executorMetrics
}
type executorMetrics struct {
	autobuildExecutionDuration prometheus.Histogram
}
// Stats contains information about one run of Executor.
type Stats struct {
	Transitions map[uuid.UUID]database.WorkspaceTransition
	Elapsed     time.Duration
	Errors      map[uuid.UUID]error
}
// New returns a new wsactions executor.
func NewExecutor(ctx context.Context, db database.Store, ps pubsub.Pubsub, reg prometheus.Registerer, tss *atomic.Pointer[schedule.TemplateScheduleStore], auditor *atomic.Pointer[audit.Auditor], acs *atomic.Pointer[dbauthz.AccessControlStore], log slog.Logger, tick <-chan time.Time, enqueuer notifications.Enqueuer) *Executor {
	factory := promauto.With(reg)
	le := &Executor{
		//nolint:gocritic // Autostart has a limited set of permissions.
		ctx:                   dbauthz.AsAutostart(ctx),
		db:                    db,
		ps:                    ps,
		templateScheduleStore: tss,
		tick:                  tick,
		log:                   log.Named("autobuild"),
		auditor:               auditor,
		accessControlStore:    acs,
		notificationsEnqueuer: enqueuer,
		reg:                   reg,
		metrics: executorMetrics{
			autobuildExecutionDuration: factory.NewHistogram(prometheus.HistogramOpts{
				Namespace: "coderd",
				Subsystem: "lifecycle",
				Name:      "autobuild_execution_duration_seconds",
				Help:      "Duration of each autobuild execution.",
				Buckets:   prometheus.DefBuckets,
			}),
		},
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
				e.metrics.autobuildExecutionDuration.Observe(stats.Elapsed.Seconds())
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
	stats := Stats{
		Transitions: make(map[uuid.UUID]database.WorkspaceTransition),
		Errors:      make(map[uuid.UUID]error),
	}
	// we build the map of transitions concurrently, so need a mutex to serialize writes to the map
	statsMu := sync.Mutex{}
	defer func() {
		stats.Elapsed = time.Since(t)
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
	workspaces, err := e.db.GetWorkspacesEligibleForTransition(e.ctx, currentTick)
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
		wsName := ws.Name
		log := e.log.With(
			slog.F("workspace_id", wsID),
			slog.F("workspace_name", wsName),
		)
		eg.Go(func() error {
			err := func() error {
				var (
					job                   *database.ProvisionerJob
					auditLog              *auditParams
					shouldNotifyDormancy  bool
					nextBuild             *database.WorkspaceBuild
					activeTemplateVersion database.TemplateVersion
					ws                    database.Workspace
					tmpl                  database.Template
					didAutoUpdate         bool
				)
				err := e.db.InTx(func(tx database.Store) error {
					var err error
					ok, err := tx.TryAcquireLock(e.ctx, database.GenLockID(fmt.Sprintf("lifecycle-executor:%s", wsID)))
					if err != nil {
						return fmt.Errorf("try acquire lifecycle executor lock: %w", err)
					}
					if !ok {
						log.Debug(e.ctx, "unable to acquire lock for workspace, skipping")
						return nil
					}
					// Re-check eligibility since the first check was outside the
					// transaction and the workspace settings may have changed.
					ws, err = tx.GetWorkspaceByID(e.ctx, wsID)
					if err != nil {
						return fmt.Errorf("get workspace by id: %w", err)
					}
					user, err := tx.GetUserByID(e.ctx, ws.OwnerID)
					if err != nil {
						return fmt.Errorf("get user by id: %w", err)
					}
					// Determine the workspace state based on its latest build.
					latestBuild, err := tx.GetLatestWorkspaceBuildByWorkspaceID(e.ctx, ws.ID)
					if err != nil {
						return fmt.Errorf("get latest workspace build: %w", err)
					}
					latestJob, err := tx.GetProvisionerJobByID(e.ctx, latestBuild.JobID)
					if err != nil {
						return fmt.Errorf("get latest provisioner job: %w", err)
					}
					templateSchedule, err := (*(e.templateScheduleStore.Load())).Get(e.ctx, tx, ws.TemplateID)
					if err != nil {
						return fmt.Errorf("get template scheduling options: %w", err)
					}
					// If next start at is not valid we need to re-compute it
					if !ws.NextStartAt.Valid && ws.AutostartSchedule.Valid {
						next, err := schedule.NextAllowedAutostart(currentTick, ws.AutostartSchedule.String, templateSchedule)
						if err == nil {
							nextStartAt := sql.NullTime{Valid: true, Time: dbtime.Time(next.UTC())}
							if err = tx.UpdateWorkspaceNextStartAt(e.ctx, database.UpdateWorkspaceNextStartAtParams{
								ID:          wsID,
								NextStartAt: nextStartAt,
							}); err != nil {
								return fmt.Errorf("update workspace next start at: %w", err)
							}
							// Save re-fetching the workspace
							ws.NextStartAt = nextStartAt
						}
					}
					tmpl, err = tx.GetTemplateByID(e.ctx, ws.TemplateID)
					if err != nil {
						return fmt.Errorf("get template by ID: %w", err)
					}
					activeTemplateVersion, err = tx.GetTemplateVersionByID(e.ctx, tmpl.ActiveVersionID)
					if err != nil {
						return fmt.Errorf("get active template version by ID: %w", err)
					}
					accessControl := (*(e.accessControlStore.Load())).GetTemplateAccessControl(tmpl)
					nextTransition, reason, err := getNextTransition(user, ws, latestBuild, latestJob, templateSchedule, currentTick)
					if err != nil {
						log.Debug(e.ctx, "skipping workspace", slog.Error(err))
						// err is used to indicate that a workspace is not eligible
						// so returning nil here is ok although ultimately the distinction
						// doesn't matter since the transaction is  read-only up to
						// this point.
						return nil
					}
					if nextTransition != "" {
						builder := wsbuilder.New(ws, nextTransition).
							SetLastWorkspaceBuildInTx(&latestBuild).
							SetLastWorkspaceBuildJobInTx(&latestJob).
							Reason(reason)
						log.Debug(e.ctx, "auto building workspace", slog.F("transition", nextTransition))
						if nextTransition == database.WorkspaceTransitionStart &&
							useActiveVersion(accessControl, ws) {
							log.Debug(e.ctx, "autostarting with active version")
							builder = builder.ActiveVersion()
							if latestBuild.TemplateVersionID != tmpl.ActiveVersionID {
								// control flag to know if the workspace was auto-updated,
								// so the lifecycle executor can notify the user
								didAutoUpdate = true
							}
						}
						nextBuild, job, _, err = builder.Build(e.ctx, tx, nil, audit.WorkspaceBuildBaggage{IP: "127.0.0.1"})
						if err != nil {
							return fmt.Errorf("build workspace with transition %q: %w", nextTransition, err)
						}
					}
					// Transition the workspace to dormant if it has breached the template's
					// threshold for inactivity.
					if reason == database.BuildReasonDormancy {
						wsOld := ws
						wsNew, err := tx.UpdateWorkspaceDormantDeletingAt(e.ctx, database.UpdateWorkspaceDormantDeletingAtParams{
							ID: ws.ID,
							DormantAt: sql.NullTime{
								Time:  dbtime.Now(),
								Valid: true,
							},
						})
						if err != nil {
							return fmt.Errorf("update workspace dormant deleting at: %w", err)
						}
						auditLog = &auditParams{
							Old: wsOld.WorkspaceTable(),
							New: wsNew,
						}
						// To keep the `ws` accurate without doing a sql fetch
						ws.DormantAt = wsNew.DormantAt
						shouldNotifyDormancy = true
						log.Info(e.ctx, "dormant workspace",
							slog.F("last_used_at", ws.LastUsedAt),
							slog.F("time_til_dormant", templateSchedule.TimeTilDormant),
							slog.F("since_last_used_at", time.Since(ws.LastUsedAt)),
						)
					}
					if reason == database.BuildReasonAutodelete {
						log.Info(e.ctx, "deleted workspace",
							slog.F("dormant_at", ws.DormantAt.Time),
							slog.F("time_til_dormant_autodelete", templateSchedule.TimeTilDormantAutoDelete),
						)
					}
					if nextTransition == "" {
						return nil
					}
					statsMu.Lock()
					stats.Transitions[ws.ID] = nextTransition
					statsMu.Unlock()
					log.Info(e.ctx, "scheduling workspace transition",
						slog.F("transition", nextTransition),
						slog.F("reason", reason),
					)
					return nil
					// Run with RepeatableRead isolation so that the build process sees the same data
					// as our calculation that determines whether an autobuild is necessary.
				}, &database.TxOptions{
					Isolation:    sql.LevelRepeatableRead,
					TxIdentifier: "lifecycle",
				})
				if auditLog != nil {
					// If the transition didn't succeed then updating the workspace
					// to indicate dormant didn't either.
					auditLog.Success = err == nil
					auditBuild(e.ctx, log, *e.auditor.Load(), *auditLog)
				}
				if didAutoUpdate && err == nil {
					nextBuildReason := ""
					if nextBuild != nil {
						nextBuildReason = string(nextBuild.Reason)
					}
					if _, err := e.notificationsEnqueuer.Enqueue(e.ctx, ws.OwnerID, notifications.TemplateWorkspaceAutoUpdated,
						map[string]string{
							"name":                     ws.Name,
							"initiator":                "autobuild",
							"reason":                   nextBuildReason,
							"template_version_name":    activeTemplateVersion.Name,
							"template_version_message": activeTemplateVersion.Message,
						}, "autobuild",
						// Associate this notification with all the related entities.
						ws.ID, ws.OwnerID, ws.TemplateID, ws.OrganizationID,
					); err != nil {
						log.Warn(e.ctx, "failed to notify of autoupdated workspace", slog.Error(err))
					}
				}
				if err != nil {
					return fmt.Errorf("transition workspace: %w", err)
				}
				if job != nil {
					// Note that we can't refactor such that posting the job happens inside wsbuilder because it's called
					// with an outer transaction like this, and we need to make sure the outer transaction commits before
					// posting the job.  If we post before the transaction commits, provisionerd might try to acquire the
					// job, fail, and then sit idle instead of picking up the job.
					err = provisionerjobs.PostJob(e.ps, *job)
					if err != nil {
						return fmt.Errorf("post provisioner job to pubsub: %w", err)
					}
				}
				if shouldNotifyDormancy {
					dormantTime := dbtime.Now().Add(time.Duration(tmpl.TimeTilDormant))
					_, err = e.notificationsEnqueuer.Enqueue(
						e.ctx,
						ws.OwnerID,
						notifications.TemplateWorkspaceDormant,
						map[string]string{
							"name":           ws.Name,
							"reason":         "inactivity exceeded the dormancy threshold",
							"timeTilDormant": humanize.Time(dormantTime),
						},
						"lifecycle_executor",
						ws.ID,
						ws.OwnerID,
						ws.TemplateID,
						ws.OrganizationID,
					)
					if err != nil {
						log.Warn(e.ctx, "failed to notify of workspace marked as dormant", slog.Error(err), slog.F("workspace_id", ws.ID))
					}
				}
				return nil
			}()
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Error(e.ctx, "failed to transition workspace", slog.Error(err))
				statsMu.Lock()
				stats.Errors[wsID] = err
				statsMu.Unlock()
			}
			// Even though we got an error we still return nil  to avoid
			// short-circuiting the evaluation loop.
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
// getNextTransition returns the next eligible transition for the workspace
// as well as the reason for why it is transitioning. It is possible
// for this function to return a nil error as well as an empty transition.
// In such cases it means no provisioning should occur but the workspace
// may be "transitioning" to a new state (such as an inactive, stopped
// workspace transitioning to the dormant state).
func getNextTransition(
	user database.User,
	ws database.Workspace,
	latestBuild database.WorkspaceBuild,
	latestJob database.ProvisionerJob,
	templateSchedule schedule.TemplateScheduleOptions,
	currentTick time.Time,
) (
	database.WorkspaceTransition,
	database.BuildReason,
	error,
) {
	switch {
	case isEligibleForAutostop(user, ws, latestBuild, latestJob, currentTick):
		return database.WorkspaceTransitionStop, database.BuildReasonAutostop, nil
	case isEligibleForAutostart(user, ws, latestBuild, latestJob, templateSchedule, currentTick):
		return database.WorkspaceTransitionStart, database.BuildReasonAutostart, nil
	case isEligibleForFailedStop(latestBuild, latestJob, templateSchedule, currentTick):
		return database.WorkspaceTransitionStop, database.BuildReasonAutostop, nil
	case isEligibleForDormantStop(ws, templateSchedule, currentTick):
		// Only stop started workspaces.
		if latestBuild.Transition == database.WorkspaceTransitionStart {
			return database.WorkspaceTransitionStop, database.BuildReasonDormancy, nil
		}
		// We shouldn't transition the workspace but we should still
		// make it dormant.
		return "", database.BuildReasonDormancy, nil
	case isEligibleForDelete(ws, templateSchedule, latestBuild, latestJob, currentTick):
		return database.WorkspaceTransitionDelete, database.BuildReasonAutodelete, nil
	default:
		return "", "", fmt.Errorf("last transition not valid for autostart or autostop")
	}
}
// isEligibleForAutostart returns true if the workspace should be autostarted.
func isEligibleForAutostart(user database.User, ws database.Workspace, build database.WorkspaceBuild, job database.ProvisionerJob, templateSchedule schedule.TemplateScheduleOptions, currentTick time.Time) bool {
	// Don't attempt to autostart workspaces for suspended users.
	if user.Status != database.UserStatusActive {
		return false
	}
	// Don't attempt to autostart failed workspaces.
	if job.JobStatus == database.ProvisionerJobStatusFailed {
		return false
	}
	// If the workspace is dormant we should not autostart it.
	if ws.DormantAt.Valid {
		return false
	}
	// If the last transition for the workspace was not 'stop' then the workspace
	// cannot be started.
	if build.Transition != database.WorkspaceTransitionStop {
		return false
	}
	// If autostart isn't enabled, or the schedule isn't valid/populated we can't
	// autostart the workspace.
	if !templateSchedule.UserAutostartEnabled || !ws.AutostartSchedule.Valid || ws.AutostartSchedule.String == "" {
		return false
	}
	nextTransition, err := schedule.NextAllowedAutostart(build.CreatedAt, ws.AutostartSchedule.String, templateSchedule)
	if err != nil {
		return false
	}
	// Must use '.Before' vs '.After' so equal times are considered "valid for autostart".
	return !currentTick.Before(nextTransition)
}
// isEligibleForAutostop returns true if the workspace should be autostopped.
func isEligibleForAutostop(user database.User, ws database.Workspace, build database.WorkspaceBuild, job database.ProvisionerJob, currentTick time.Time) bool {
	if job.JobStatus == database.ProvisionerJobStatusFailed {
		return false
	}
	// If the workspace is dormant we should not autostop it.
	if ws.DormantAt.Valid {
		return false
	}
	if build.Transition == database.WorkspaceTransitionStart && user.Status == database.UserStatusSuspended {
		return true
	}
	// A workspace must be started in order for it to be auto-stopped.
	return build.Transition == database.WorkspaceTransitionStart &&
		!build.Deadline.IsZero() &&
		// We do not want to stop a workspace prior to it breaching its deadline.
		!currentTick.Before(build.Deadline)
}
// isEligibleForDormantStop returns true if the workspace should be dormant
// for breaching the inactivity threshold of the template.
func isEligibleForDormantStop(ws database.Workspace, templateSchedule schedule.TemplateScheduleOptions, currentTick time.Time) bool {
	// Only attempt against workspaces not already dormant.
	return !ws.DormantAt.Valid &&
		// The template must specify an time_til_dormant value.
		templateSchedule.TimeTilDormant > 0 &&
		// The workspace must breach the time_til_dormant value.
		currentTick.Sub(ws.LastUsedAt) > templateSchedule.TimeTilDormant
}
func isEligibleForDelete(ws database.Workspace, templateSchedule schedule.TemplateScheduleOptions, lastBuild database.WorkspaceBuild, lastJob database.ProvisionerJob, currentTick time.Time) bool {
	eligible := ws.DormantAt.Valid && ws.DeletingAt.Valid &&
		// Dormant workspaces should only be deleted if a time_til_dormant_autodelete value is specified.
		templateSchedule.TimeTilDormantAutoDelete > 0 &&
		// The workspace must breach the time_til_dormant_autodelete value.
		currentTick.After(ws.DeletingAt.Time)
	// If the last delete job failed we should wait 24 hours before trying again.
	// Builds are resource-intensive so retrying every minute is not productive
	// and will hold compute hostage.
	if lastBuild.Transition == database.WorkspaceTransitionDelete && lastJob.JobStatus == database.ProvisionerJobStatusFailed {
		return eligible && lastJob.Finished() && currentTick.Sub(lastJob.FinishedAt()) > time.Hour*24
	}
	return eligible
}
// isEligibleForFailedStop returns true if the workspace is eligible to be stopped
// due to a failed build.
func isEligibleForFailedStop(build database.WorkspaceBuild, job database.ProvisionerJob, templateSchedule schedule.TemplateScheduleOptions, currentTick time.Time) bool {
	// If the template has specified a failure TLL.
	return templateSchedule.FailureTTL > 0 &&
		// And the job resulted in failure.
		job.JobStatus == database.ProvisionerJobStatusFailed &&
		build.Transition == database.WorkspaceTransitionStart &&
		// And sufficient time has elapsed since the job has completed.
		job.CompletedAt.Valid &&
		currentTick.Sub(job.CompletedAt.Time) > templateSchedule.FailureTTL
}
type auditParams struct {
	Old     database.WorkspaceTable
	New     database.WorkspaceTable
	Success bool
}
func auditBuild(ctx context.Context, log slog.Logger, auditor audit.Auditor, params auditParams) {
	status := http.StatusInternalServerError
	if params.Success {
		status = http.StatusOK
	}
	audit.BackgroundAudit(ctx, &audit.BackgroundAuditParams[database.WorkspaceTable]{
		Audit:          auditor,
		Log:            log,
		UserID:         params.New.OwnerID,
		OrganizationID: params.New.OrganizationID,
		// Right now there's no request associated with an autobuild
		// operation.
		RequestID: uuid.Nil,
		Action:    database.AuditActionWrite,
		Old:       params.Old,
		New:       params.New,
		Status:    status,
	})
}
func useActiveVersion(opts dbauthz.TemplateAccessControl, ws database.Workspace) bool {
	return opts.RequireActiveVersion || ws.AutomaticUpdates == database.AutomaticUpdatesAlways
}

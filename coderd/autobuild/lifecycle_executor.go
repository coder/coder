package autobuild

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/provisionerjobs"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/pproflabel"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/wsbuilder"
	"github.com/coder/coder/v2/codersdk"
)

// Executor automatically starts or stops workspaces.
type Executor struct {
	ctx                   context.Context
	db                    database.Store
	ps                    pubsub.Pubsub
	fileCache             *files.Cache
	templateScheduleStore *atomic.Pointer[schedule.TemplateScheduleStore]
	accessControlStore    *atomic.Pointer[dbauthz.AccessControlStore]
	auditor               *atomic.Pointer[audit.Auditor]
	buildUsageChecker     *atomic.Pointer[wsbuilder.UsageChecker]
	log                   slog.Logger
	tick                  <-chan time.Time
	statsCh               chan<- Stats
	// NotificationsEnqueuer handles enqueueing notifications for delivery by SMTP, webhook, etc.
	notificationsEnqueuer   notifications.Enqueuer
	reg                     prometheus.Registerer
	experiments             codersdk.Experiments
	workspaceBuilderMetrics *wsbuilder.Metrics

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
func NewExecutor(ctx context.Context, db database.Store, ps pubsub.Pubsub, fc *files.Cache, reg prometheus.Registerer, tss *atomic.Pointer[schedule.TemplateScheduleStore], auditor *atomic.Pointer[audit.Auditor], acs *atomic.Pointer[dbauthz.AccessControlStore], buildUsageChecker *atomic.Pointer[wsbuilder.UsageChecker], log slog.Logger, tick <-chan time.Time, enqueuer notifications.Enqueuer, exp codersdk.Experiments, workspaceBuilderMetrics *wsbuilder.Metrics) *Executor {
	factory := promauto.With(reg)
	le := &Executor{
		//nolint:gocritic // Autostart has a limited set of permissions.
		ctx:                     dbauthz.AsAutostart(ctx),
		db:                      db,
		ps:                      ps,
		fileCache:               fc,
		templateScheduleStore:   tss,
		tick:                    tick,
		log:                     log.Named("autobuild"),
		auditor:                 auditor,
		accessControlStore:      acs,
		buildUsageChecker:       buildUsageChecker,
		notificationsEnqueuer:   enqueuer,
		reg:                     reg,
		experiments:             exp,
		workspaceBuilderMetrics: workspaceBuilderMetrics,
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
	pproflabel.Go(e.ctx, pproflabel.Service(pproflabel.ServiceLifecycles), func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case t, ok := <-e.tick:
				if !ok {
					return
				}
				stats := e.runOnce(t)
				e.metrics.autobuildExecutionDuration.Observe(stats.Elapsed.Seconds())
				if e.statsCh != nil {
					select {
					case <-ctx.Done():
						return
					case e.statsCh <- stats:
					}
				}
				e.log.Debug(ctx, "run stats", slog.F("elapsed", stats.Elapsed), slog.F("transitions", stats.Transitions))
			}
		}
	})
}

// hasValidProvisioner checks whether there is at least one valid (non-stale, correct tags) provisioner
// based on time t and the tags maps (such as from a templateVersionJob).
func (e *Executor) hasValidProvisioner(ctx context.Context, tx database.Store, t time.Time, ws database.Workspace, tags map[string]string) (bool, error) {
	queryParams := database.GetProvisionerDaemonsByOrganizationParams{
		OrganizationID: ws.OrganizationID,
		WantTags:       tags,
	}

	// nolint: gocritic // The user (in this case, the user/context for autostart builds) may not have the full
	// permissions to read provisioner daemons, but we need to check if there's any for the job prior to the
	// execution of the job via autostart to fix: https://github.com/coder/coder/issues/17941
	provisionerDaemons, err := tx.GetProvisionerDaemonsByOrganization(dbauthz.AsSystemReadProvisionerDaemons(ctx), queryParams)
	if err != nil {
		return false, xerrors.Errorf("get provisioner daemons: %w", err)
	}

	logger := e.log.With(slog.F("tags", tags))
	// Check if any provisioners are active (not stale)
	for _, pd := range provisionerDaemons {
		if pd.LastSeenAt.Valid {
			age := t.Sub(pd.LastSeenAt.Time)
			if age <= provisionerdserver.StaleInterval {
				logger.Debug(ctx, "hasValidProvisioner: found active provisioner",
					slog.F("daemon_id", pd.ID),
				)
				return true, nil
			}
		}
	}
	logger.Debug(ctx, "hasValidProvisioner: no active provisioners found")
	return false, nil
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
	workspaces, err := e.db.GetWorkspacesEligibleForLifecycleAction(e.ctx, currentTick)
	if err != nil {
		e.log.Error(e.ctx, "get workspaces for autostart or autostop", slog.Error(err))
		return stats
	}

	// Sort the workspaces by build template version ID so that we can group
	// identical template versions together. This is a slight (and imperfect)
	// optimization.
	//
	// `wsbuilder` needs to load the terraform files for a given template version
	// into memory. If 2 workspaces are using the same template version, they will
	// share the same files in the FileCache. This only happens if the builds happen
	// in parallel.
	// TODO: Actually make sure the cache has the files in the cache for the full
	//  set of identical template versions. Then unload the files when the builds
	//  are done. Right now, this relies on luck for the 10 goroutine workers to
	//  overlap and keep the file reference in the cache alive.
	slices.SortFunc(workspaces, func(a, b database.GetWorkspacesEligibleForLifecycleActionRow) int {
		return strings.Compare(a.BuildTemplateVersionID.UUID.String(), b.BuildTemplateVersionID.UUID.String())
	})

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
					shouldNotifyTaskPause bool
					shouldRemind          bool
					reminderDeadline      time.Time
					reminderBuildID       uuid.UUID
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
						return xerrors.Errorf("try acquire lifecycle executor lock: %w", err)
					}
					if !ok {
						log.Debug(e.ctx, "unable to acquire lock for workspace, skipping")
						return nil
					}

					// Re-check eligibility since the first check was outside the
					// transaction and the workspace settings may have changed.
					ws, err = tx.GetWorkspaceByID(e.ctx, wsID)
					if err != nil {
						return xerrors.Errorf("get workspace by id: %w", err)
					}

					user, err := tx.GetUserByID(e.ctx, ws.OwnerID)
					if err != nil {
						return xerrors.Errorf("get user by id: %w", err)
					}

					// Determine the workspace state based on its latest build.
					latestBuild, err := tx.GetLatestWorkspaceBuildByWorkspaceID(e.ctx, ws.ID)
					if err != nil {
						return xerrors.Errorf("get latest workspace build: %w", err)
					}

					latestJob, err := tx.GetProvisionerJobByID(e.ctx, latestBuild.JobID)
					if err != nil {
						return xerrors.Errorf("get latest provisioner job: %w", err)
					}

					templateSchedule, err := (*(e.templateScheduleStore.Load())).Get(e.ctx, tx, ws.TemplateID)
					if err != nil {
						return xerrors.Errorf("get template scheduling options: %w", err)
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
								return xerrors.Errorf("update workspace next start at: %w", err)
							}

							// Save re-fetching the workspace
							ws.NextStartAt = nextStartAt
						}
					}

					tmpl, err = tx.GetTemplateByID(e.ctx, ws.TemplateID)
					if err != nil {
						return xerrors.Errorf("get template by ID: %w", err)
					}

					activeTemplateVersion, err = tx.GetTemplateVersionByID(e.ctx, tmpl.ActiveVersionID)
					if err != nil {
						return xerrors.Errorf("get active template version by ID: %w", err)
					}

					accessControl := (*(e.accessControlStore.Load())).GetTemplateAccessControl(tmpl)

					nextTransition, reason, err := getNextTransition(user, ws, latestBuild, latestJob, templateSchedule, currentTick)
					if err != nil {
						return xerrors.Errorf("get next transition: %w", err)
					}

					// No transition is due. The workspace may still need a one-time
					// autostop reminder; reuse the lock and transaction we already
					// hold to stamp the marker.
					if reason == "" {
						log.Debug(e.ctx, "skipping workspace, no transition due")
						// A deadline change (e.g. activity bump) re-arms the reminder; users near
						// the boundary may receive one reminder per bump. Intentional: one-per-build
						// would leave stale reminders after a bump.
						if shouldRemindAutostop(latestBuild, ws.LastUsedAt, templateSchedule, currentTick) {
							if err := tx.UpdateWorkspaceBuildNotifiedAutostopDeadline(e.ctx, database.UpdateWorkspaceBuildNotifiedAutostopDeadlineParams{
								ID:                       latestBuild.ID,
								NotifiedAutostopDeadline: latestBuild.Deadline,
								UpdatedAt:                dbtime.Now(),
							}); err != nil {
								return xerrors.Errorf("stamp autostop reminder marker: %w", err)
							}
							reminderDeadline = latestBuild.Deadline
							reminderBuildID = latestBuild.ID
							shouldRemind = true
						}
						return nil
					}

					if reason == database.BuildReasonTaskAutoPause {
						shouldNotifyTaskPause = true
					}

					// Get the template version job to access tags
					templateVersionJob, err := tx.GetProvisionerJobByID(e.ctx, activeTemplateVersion.JobID)
					if err != nil {
						return xerrors.Errorf("get template version job: %w", err)
					}

					// Before creating the workspace build, check for available provisioners
					hasProvisioners, err := e.hasValidProvisioner(e.ctx, tx, t, ws, templateVersionJob.Tags)
					if err != nil {
						return xerrors.Errorf("check provisioner availability: %w", err)
					}
					if !hasProvisioners {
						log.Warn(e.ctx, "skipping autostart - no available provisioners")
						return nil // Skip this workspace
					}

					if nextTransition != "" {
						builder := wsbuilder.New(ws, nextTransition, *e.buildUsageChecker.Load()).
							SetLastWorkspaceBuildInTx(&latestBuild).
							SetLastWorkspaceBuildJobInTx(&latestJob).
							Experiments(e.experiments).
							Reason(reason).
							BuildMetrics(e.workspaceBuilderMetrics)
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

						nextBuild, job, _, err = builder.Build(e.ctx, tx, e.fileCache, nil, audit.WorkspaceBuildBaggage{IP: "127.0.0.1"})
						if err != nil {
							return xerrors.Errorf("build workspace with transition %q: %w", nextTransition, err)
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
							return xerrors.Errorf("update workspace dormant deleting at: %w", err)
						}

						auditLog = &auditParams{
							Old: wsOld.WorkspaceTable(),
							New: wsNew,
						}
						// To keep the `ws` accurate without doing a sql fetch.
						// deleting_at is computed atomically inside the UPDATE from
						// the workspace's template_id, so it reflects the auto-delete
						// deadline the database persisted.
						ws.DormantAt = wsNew.DormantAt
						ws.DeletingAt = wsNew.DeletingAt

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
				// A concurrent build (e.g. from the API or another lifecycle
				// executor) may have already inserted a build with the same
				// number. This is a benign race; the other actor's build
				// will take effect. Clear the error so downstream checks
				// (audit, notification, stats) treat this as a no-op.
				if database.IsUniqueViolation(err, database.UniqueWorkspaceBuildsWorkspaceIDBuildNumberKey) {
					log.Info(e.ctx, "skipping workspace: concurrent build already inserted", slog.Error(err))
					err = nil
					// Reset notification flags set before builder.Build.
					// The build was rolled back, so this executor did not
					// perform the transition. The concurrent actor handles
					// both the build and any notifications. Without these
					// resets, downstream code would send duplicate or
					// incorrect notifications.
					didAutoUpdate = false
					shouldNotifyTaskPause = false
				}
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

					templateVersionMessage := activeTemplateVersion.Message
					if templateVersionMessage == "" {
						templateVersionMessage = "None provided"
					}

					if _, err := e.notificationsEnqueuer.Enqueue(e.ctx, ws.OwnerID, notifications.TemplateWorkspaceAutoUpdated,
						map[string]string{
							"name":                     ws.Name,
							"initiator":                "autobuild",
							"reason":                   nextBuildReason,
							"template_version_name":    activeTemplateVersion.Name,
							"template_version_message": templateVersionMessage,
						}, "autobuild",
						// Associate this notification with all the related entities.
						ws.ID, ws.OwnerID, ws.TemplateID, ws.OrganizationID,
					); err != nil {
						log.Warn(e.ctx, "failed to notify of autoupdated workspace", slog.Error(err))
					}
				}
				if err != nil {
					return xerrors.Errorf("transition workspace: %w", err)
				}
				if job != nil {
					// Note that we can't refactor such that posting the job happens inside wsbuilder because it's called
					// with an outer transaction like this, and we need to make sure the outer transaction commits before
					// posting the job.  If we post before the transaction commits, provisionerd might try to acquire the
					// job, fail, and then sit idle instead of picking up the job.
					err = provisionerjobs.PostJob(e.ps, *job)
					if err != nil {
						return xerrors.Errorf("post provisioner job to pubsub: %w", err)
					}
				}
				if shouldNotifyDormancy {
					labels := map[string]string{
						"name":   ws.Name,
						"reason": "inactivity exceeded the dormancy threshold",
					}
					// DeletingAt is set by the UPDATE only when the template's
					// time_til_dormant_autodelete is non-zero, so skip the label when
					// auto-delete is disabled so the body omits the deletion
					// timeline.
					if ws.DeletingAt.Valid {
						labels["timeTilDelete"] = humanize.Time(ws.DeletingAt.Time)
					}
					_, err = e.notificationsEnqueuer.Enqueue(
						e.ctx,
						ws.OwnerID,
						notifications.TemplateWorkspaceDormant,
						labels,
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
				if shouldNotifyTaskPause {
					task, err := e.db.GetTaskByID(e.ctx, ws.TaskID.UUID)
					if err != nil {
						log.Warn(e.ctx, "failed to get task for pause notification", slog.Error(err), slog.F("task_id", ws.TaskID.UUID), slog.F("workspace_id", ws.ID))
					} else {
						if _, err := e.notificationsEnqueuer.Enqueue(
							e.ctx,
							ws.OwnerID,
							notifications.TemplateTaskPaused,
							map[string]string{
								"task":         task.Name,
								"task_id":      task.ID.String(),
								"workspace":    ws.Name,
								"pause_reason": "idle timeout",
							},
							"lifecycle_executor",
							ws.ID, ws.OwnerID, ws.OrganizationID,
						); err != nil {
							log.Warn(e.ctx, "failed to notify of task paused", slog.Error(err), slog.F("task_id", ws.TaskID.UUID), slog.F("workspace_id", ws.ID))
						}
					}
				}
				if shouldRemind {
					// At-most-once: the marker is already committed, so a failed
					// enqueue only logs (no retry).
					if _, err := e.notificationsEnqueuer.Enqueue(
						e.ctx,
						ws.OwnerID,
						notifications.TemplateWorkspaceAutostopReminder,
						map[string]string{
							"workspace":       ws.Name,
							"timeTilShutdown": humanize.Time(reminderDeadline),
						},
						"lifecycle_executor",
						// Associate this notification with all the related entities.
						ws.ID, ws.OwnerID, ws.TemplateID, ws.OrganizationID,
					); err != nil {
						log.Warn(e.ctx, "failed to notify of upcoming workspace autostop", slog.F("build_id", reminderBuildID), slog.Error(err))
					}
				}
				return nil
			}()
			if err != nil && !xerrors.Is(err, context.Canceled) {
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

// autostopReminderActiveThreshold is how recently a workspace must have been
// used to count as "active" and suppress the autostop reminder. A default
// deployment refreshes last_used_at within ~90s, but the agent stats interval
// is configurable up to a few minutes, so we stay conservative. It must remain
// well below activity_bump (default 1h): an idle user's now-last_used_at is
// bounded by activity_bump, so a larger threshold would make idle users look
// active forever and never be reminded. Keep in sync with the INTERVAL '15
// minutes' literal in GetWorkspacesEligibleForLifecycleAction (workspaces.sql).
const autostopReminderActiveThreshold = 15 * time.Minute

// shouldRemindAutostop reports whether an autostop reminder should be sent for
// the build at currentTick. It skips genuinely-active workspaces only when
// activity can still move the deadline out of the lead window.
//
// time_til_autostop_notify has no upper bound, so the lead window can already
// cover "now" at build creation. The result is still exactly one reminder per
// deadline (never one per tick): we require deadline > now, and the marker
// (NotifiedAutostopDeadline == Deadline, stamped before the send attempt)
// filters every subsequent tick.
//
// The skip-guard below is the exact complement of the keep-condition in the
// reminder arm of GetWorkspacesEligibleForLifecycleAction, so a row that passes
// the SQL pre-filter also passes this re-check (and vice versa).
func shouldRemindAutostop(build database.WorkspaceBuild, lastUsedAt time.Time, templateSchedule schedule.TemplateScheduleOptions, currentTick time.Time) bool {
	if templateSchedule.TimeTilAutostopNotify <= 0 {
		return false
	}

	if build.Transition != database.WorkspaceTransitionStart || build.Deadline.IsZero() {
		return false
	}

	if !build.Deadline.After(currentTick) {
		return false
	}

	// "now" must be within the lead window before the deadline, i.e.
	// deadline <= now + time_til_autostop_notify.
	if build.Deadline.After(currentTick.Add(templateSchedule.TimeTilAutostopNotify)) {
		return false
	}

	// Skip the reminder only for an active user whose deadline can still be
	// bumped out of the lead window.
	userActive := currentTick.Sub(lastUsedAt) < autostopReminderActiveThreshold
	bumpEnabled := templateSchedule.ActivityBump > 0
	// The hard ceiling traps the workspace inside the window: a non-zero
	// max_deadline at or before now+ttl means no bump can push the stop out of
	// the lead window, so it WILL stop regardless of activity.
	maxDeadlineTraps := !build.MaxDeadline.IsZero() &&
		!build.MaxDeadline.After(currentTick.Add(templateSchedule.TimeTilAutostopNotify))

	if userActive && bumpEnabled && !maxDeadlineTraps {
		return false
	}

	// Idempotence: a reminder has not yet been sent for THIS deadline. The
	// marker re-arms automatically when the deadline changes (e.g. an activity
	// bump), so a new reminder fires once the new deadline re-enters the window.
	return !build.NotifiedAutostopDeadline.Equal(build.Deadline)
}

// getNextTransition returns the next eligible transition for the workspace
// as well as the reason for why it is transitioning. It is possible for this
// function to return a nil error as well as an empty transition with a
// non-empty reason. In such cases it means no provisioning should occur but
// the workspace may be "transitioning" to a new state (such as an inactive,
// stopped workspace transitioning to the dormant state).
//
// When nothing is due, it returns an empty transition, an empty reason, and a
// nil error. Callers gate on reason == "" for the "nothing to do" case.
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
		// Use task-specific reason for AI task workspaces.
		if ws.TaskID.Valid {
			return database.WorkspaceTransitionStop, database.BuildReasonTaskAutoPause, nil
		}
		return database.WorkspaceTransitionStop, database.BuildReasonAutostop, nil
	case isEligibleForAutostart(user, ws, latestBuild, latestJob, templateSchedule, currentTick):
		return database.WorkspaceTransitionStart, database.BuildReasonAutostart, nil
	case isEligibleForFailedCleanup(latestBuild, latestJob, templateSchedule, currentTick):
		// Use task-specific reason for AI task workspaces.
		if ws.TaskID.Valid {
			return database.WorkspaceTransitionStop, database.BuildReasonTaskAutoPause, nil
		}
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
		// No autostart, autostop, dormancy, or deletion transition is due.
		return "", "", nil
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

	// Get the next allowed autostart time after the build's creation time,
	// based on the workspace's schedule and the template's allowed days.
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

// isEligibleForFailedCleanup returns true if the workspace is eligible to be
// stopped due to a failed build. A failed start is cleaned up by stopping it,
// and a failed stop is retried by issuing another stop. In both cases the
// remediation is a stop build.
func isEligibleForFailedCleanup(build database.WorkspaceBuild, job database.ProvisionerJob, templateSchedule schedule.TemplateScheduleOptions, currentTick time.Time) bool {
	// If the template has specified a failure TTL.
	return templateSchedule.FailureTTL > 0 &&
		// And the job resulted in failure.
		job.JobStatus == database.ProvisionerJobStatusFailed &&
		(build.Transition == database.WorkspaceTransitionStart ||
			build.Transition == database.WorkspaceTransitionStop) &&
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

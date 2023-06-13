package unhanger

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/db2sdk"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisionersdk"
)

const (
	// HungJobDuration is the duration of time since the last update to a job
	// before it is considered hung.
	HungJobDuration = 5 * time.Minute

	// HungJobExitTimeout is the duration of time that provisioners should allow
	// for a graceful exit upon cancellation due to failing to send an update to
	// a job.
	//
	// Provisioners should avoid keeping a job "running" for longer than this
	// time after failing to send an update to the job.
	HungJobExitTimeout = 3 * time.Minute
)

// HungJobLogMessages are written to provisioner job logs when a job is hung and
// terminated.
var HungJobLogMessages = []string{
	"",
	"====================",
	"Coder: Build has been detected as hung for 5 minutes and will be terminated.",
	"====================",
	"",
}

// acquireLockError is returned when the detector fails to acquire a lock and
// cancels the current run.
type acquireLockError struct {
	err error
}

// Error implements error.
func (e *acquireLockError) Error() string {
	return "acquire lock: " + e.err.Error()
}

// Unwrap implements xerrors.Wrapper.
func (e *acquireLockError) Unwrap() error {
	return e.err
}

// Detector automatically detects hung provisioner jobs, sends messages into the
// build log and terminates them as failed.
type Detector struct {
	ctx    context.Context
	db     database.Store
	pubsub database.Pubsub
	log    slog.Logger
	tick   <-chan time.Time
	stats  chan<- Stats
	done   chan struct{}
}

// Stats contains statistics about the last run of the detector.
type Stats struct {
	// HungJobIDs contains the IDs of all jobs that were detected as hung and
	// terminated.
	HungJobIDs []uuid.UUID
	// Error is the fatal error that occurred during the last run of the
	// detector, if any. Error may be set to AcquireLockError if the detector
	// failed to acquire a lock.
	Error error
}

// New returns a new hang detector.
func New(ctx context.Context, db database.Store, pubsub database.Pubsub, log slog.Logger, tick <-chan time.Time) *Detector {
	le := &Detector{
		//nolint:gocritic // Hang detector has a limited set of permissions.
		ctx:    dbauthz.AsHangDetector(ctx),
		db:     db,
		pubsub: pubsub,
		tick:   tick,
		log:    log,
		done:   make(chan struct{}),
	}
	return le
}

// WithStatsChannel will cause Executor to push a RunStats to ch after
// every tick. This push is blocking, so if ch is not read, the detector will
// hang. This should only be used in tests.
func (d *Detector) WithStatsChannel(ch chan<- Stats) *Detector {
	d.stats = ch
	return d
}

// Start will cause the detector to detect and unhang provisioner jobs on every
// tick from its channel. It will stop when its context is Done, or when its
// channel is closed.
//
// Start should only be called once.
func (d *Detector) Start() {
	go func() {
		defer close(d.done)
		for {
			select {
			case <-d.ctx.Done():
				return
			case t, ok := <-d.tick:
				if !ok {
					return
				}
				stats := d.run(t)
				if stats.Error != nil && !xerrors.As(stats.Error, &acquireLockError{}) {
					d.log.Warn(d.ctx, "error running workspace build hang detector once", slog.Error(stats.Error))
				}
				if len(stats.HungJobIDs) != 0 {
					d.log.Warn(d.ctx, "detected (and terminated) hung provisioner jobs", slog.F("job_ids", stats.HungJobIDs))
				}
				if d.stats != nil {
					select {
					case <-d.ctx.Done():
						return
					case d.stats <- stats:
					}
				}
			}
		}
	}()
}

// Wait will block until the detector is stopped.
func (d *Detector) Wait() {
	<-d.done
}

func (d *Detector) run(t time.Time) Stats {
	ctx, cancel := context.WithTimeout(d.ctx, 5*time.Minute)
	defer cancel()

	stats := Stats{
		HungJobIDs: []uuid.UUID{},
		Error:      nil,
	}

	err := d.db.InTx(func(db database.Store) error {
		err := db.AcquireLock(ctx, database.LockIDHangDetector)
		if err != nil {
			// If we can't acquire the lock, it means another instance of the
			// hang detector is already running in another coder replica.
			// There's no point in waiting to run it again, so we'll just retry
			// on the next tick.
			d.log.Info(ctx, "skipping workspace build hang detector run due to lock", slog.Error(err))
			// This error is ignored.
			return &acquireLockError{err: err}
		}
		d.log.Info(ctx, "running workspace build hang detector")

		// Find all provisioner jobs that are currently running but have not
		// received an update in the last 5 minutes.
		jobs, err := db.GetHungProvisionerJobs(ctx, t.Add(-HungJobDuration))
		if err != nil {
			return xerrors.Errorf("get hung provisioner jobs: %w", err)
		}

		// Send a message into the build log for each hung job saying that it
		// has been detected and will be terminated, then mark the job as
		// failed.
		for _, job := range jobs {
			log := d.log.With(slog.F("job_id", job.ID))

			jobStatus := db2sdk.ProvisionerJobStatus(job)
			if jobStatus != codersdk.ProvisionerJobRunning {
				log.Error(ctx, "hang detector query discovered non-running job, this is a bug", slog.F("status", jobStatus))
				continue
			}

			log.Info(ctx, "detected hung (>5m) provisioner job, forcefully terminating")

			// First, get the latest logs from the build so we can make sure
			// our messages are in the latest stage.
			logs, err := db.GetProvisionerLogsAfterID(ctx, database.GetProvisionerLogsAfterIDParams{
				JobID:        job.ID,
				CreatedAfter: 0,
			})
			if err != nil {
				log.Warn(ctx, "get logs for hung job", slog.Error(err))
				continue
			}
			logStage := ""
			if len(logs) != 0 {
				logStage = logs[len(logs)-1].Stage
			}
			if logStage == "" {
				logStage = "Unknown"
			}

			// Insert the messages into the build log.
			insertParams := database.InsertProvisionerJobLogsParams{
				JobID: job.ID,
			}
			now := database.Now()
			for i, msg := range HungJobLogMessages {
				// Set the created at in a way that ensures each message has
				// a unique timestamp so they will be sorted correctly.
				insertParams.CreatedAt = append(insertParams.CreatedAt, now.Add(time.Millisecond*time.Duration(i)))
				insertParams.Level = append(insertParams.Level, database.LogLevelError)
				insertParams.Stage = append(insertParams.Stage, logStage)
				insertParams.Source = append(insertParams.Source, database.LogSourceProvisionerDaemon)
				insertParams.Output = append(insertParams.Output, msg)
			}
			newLogs, err := db.InsertProvisionerJobLogs(ctx, insertParams)
			if err != nil {
				log.Warn(ctx, "insert logs for hung job", slog.Error(err))
				continue
			}

			// Publish the new log notification to pubsub. Use the lowest
			// log ID inserted so the log stream will fetch everything after
			// that point.
			lowestID := newLogs[0].ID
			data, err := json.Marshal(provisionersdk.ProvisionerJobLogsNotifyMessage{
				CreatedAfter: lowestID - 1,
			})
			if err != nil {
				log.Warn(ctx, "marshal log notification", slog.Error(err))
				continue
			}
			err = d.pubsub.Publish(provisionersdk.ProvisionerJobLogsNotifyChannel(job.ID), data)
			if err != nil {
				log.Warn(ctx, "publish log notification", slog.Error(err))
				continue
			}

			// Mark the job as failed.
			now = database.Now()
			err = db.UpdateProvisionerJobWithCompleteByID(ctx, database.UpdateProvisionerJobWithCompleteByIDParams{
				ID:        job.ID,
				UpdatedAt: now,
				CompletedAt: sql.NullTime{
					Time:  now,
					Valid: true,
				},
				Error: sql.NullString{
					String: "Coder: Build has been detected as hung for 5 minutes and has been terminated.",
					Valid:  true,
				},
				ErrorCode: sql.NullString{
					Valid: false,
				},
			})
			if err != nil {
				log.Warn(ctx, "mark job as failed", slog.Error(err))
				continue
			}

			// If the provisioner job is a workspace build, copy the
			// provisioner state from the previous build to this workspace
			// build.
			if job.Type == database.ProvisionerJobTypeWorkspaceBuild {
				build, err := db.GetWorkspaceBuildByJobID(ctx, job.ID)
				if err != nil {
					log.Warn(ctx, "get workspace build for workspace build job by job id", slog.Error(err))
					continue
				}

				// Only copy the provisioner state if there's no state in
				// the current build.
				if len(build.ProvisionerState) == 0 {
					// Get the previous build if it exists.
					prevBuild, err := db.GetWorkspaceBuildByWorkspaceIDAndBuildNumber(ctx, database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams{
						WorkspaceID: build.WorkspaceID,
						BuildNumber: build.BuildNumber - 1,
					})
					if err == nil {
						_, err = db.UpdateWorkspaceBuildByID(ctx, database.UpdateWorkspaceBuildByIDParams{
							ID:               build.ID,
							UpdatedAt:        database.Now(),
							ProvisionerState: prevBuild.ProvisionerState,
							Deadline:         time.Time{},
							MaxDeadline:      time.Time{},
						})
						if err != nil {
							log.Warn(ctx, "update hung workspace build provisioner state to match previous build", slog.Error(err))
							continue
						}
					} else if !xerrors.Is(err, sql.ErrNoRows) {
						log.Warn(ctx, "get previous workspace build", slog.Error(err))
						continue
					}
				}
			}

			stats.HungJobIDs = append(stats.HungJobIDs, job.ID)
		}

		return nil
	}, nil)
	if err != nil {
		stats.Error = err
		return stats
	}

	return stats
}

package unhanger

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand" //#nosec // this is only used for shuffling an array to pick random jobs to unhang
	"time"

	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/db2sdk"
	"github.com/coder/coder/coderd/database/dbauthz"
	"github.com/coder/coder/coderd/database/pubsub"
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

	// MaxJobsPerRun is the maximum number of hung jobs that the detector will
	// terminate in a single run.
	MaxJobsPerRun = 10
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
type acquireLockError struct{}

// Error implements error.
func (acquireLockError) Error() string {
	return "lock is held by another client"
}

// jobInelligibleError is returned when a job is not eligible to be terminated
// anymore.
type jobInelligibleError struct {
	Err error
}

// Error implements error.
func (e jobInelligibleError) Error() string {
	return fmt.Sprintf("job is no longer eligible to be terminated: %s", e.Err)
}

// Detector automatically detects hung provisioner jobs, sends messages into the
// build log and terminates them as failed.
type Detector struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	db     database.Store
	pubsub pubsub.Pubsub
	log    slog.Logger
	tick   <-chan time.Time
	stats  chan<- Stats
}

// Stats contains statistics about the last run of the detector.
type Stats struct {
	// TerminatedJobIDs contains the IDs of all jobs that were detected as hung and
	// terminated.
	TerminatedJobIDs []uuid.UUID
	// Error is the fatal error that occurred during the last run of the
	// detector, if any. Error may be set to AcquireLockError if the detector
	// failed to acquire a lock.
	Error error
}

// New returns a new hang detector.
func New(ctx context.Context, db database.Store, pub pubsub.Pubsub, log slog.Logger, tick <-chan time.Time) *Detector {
	//nolint:gocritic // Hang detector has a limited set of permissions.
	ctx, cancel := context.WithCancel(dbauthz.AsHangDetector(ctx))
	d := &Detector{
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
		db:     db,
		pubsub: pub,
		log:    log,
		tick:   tick,
		stats:  nil,
	}
	return d
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
		defer d.cancel()

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
				if len(stats.TerminatedJobIDs) != 0 {
					d.log.Warn(d.ctx, "detected (and terminated) hung provisioner jobs", slog.F("job_ids", stats.TerminatedJobIDs))
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

// Close will stop the detector.
func (d *Detector) Close() {
	d.cancel()
	<-d.done
}

func (d *Detector) run(t time.Time) Stats {
	ctx, cancel := context.WithTimeout(d.ctx, 5*time.Minute)
	defer cancel()

	stats := Stats{
		TerminatedJobIDs: []uuid.UUID{},
		Error:            nil,
	}

	// Find all provisioner jobs that are currently running but have not
	// received an update in the last 5 minutes.
	jobs, err := d.db.GetHungProvisionerJobs(ctx, t.Add(-HungJobDuration))
	if err != nil {
		stats.Error = xerrors.Errorf("get hung provisioner jobs: %w", err)
		return stats
	}

	// Limit the number of jobs we'll unhang in a single run to avoid
	// timing out.
	if len(jobs) > MaxJobsPerRun {
		// Pick a random subset of the jobs to unhang.
		rand.Shuffle(len(jobs), func(i, j int) {
			jobs[i], jobs[j] = jobs[j], jobs[i]
		})
		jobs = jobs[:MaxJobsPerRun]
	}

	// Send a message into the build log for each hung job saying that it
	// has been detected and will be terminated, then mark the job as
	// failed.
	for _, job := range jobs {
		log := d.log.With(slog.F("job_id", job.ID))

		err := unhangJob(ctx, log, d.db, d.pubsub, job.ID)
		if err != nil && !(xerrors.As(err, &acquireLockError{}) || xerrors.As(err, &jobInelligibleError{})) {
			log.Error(ctx, "error forcefully terminating hung provisioner job", slog.Error(err))
			continue
		}

		stats.TerminatedJobIDs = append(stats.TerminatedJobIDs, job.ID)
	}

	return stats
}

func unhangJob(ctx context.Context, log slog.Logger, db database.Store, pub pubsub.Pubsub, jobID uuid.UUID) error {
	var lowestLogID int64

	err := db.InTx(func(db database.Store) error {
		locked, err := db.TryAcquireLock(ctx, database.GenLockID(fmt.Sprintf("hang-detector:%s", jobID)))
		if err != nil {
			return xerrors.Errorf("acquire lock: %w", err)
		}
		if !locked {
			// This error is ignored.
			return acquireLockError{}
		}

		// Refetch the job while we hold the lock.
		job, err := db.GetProvisionerJobByID(ctx, jobID)
		if err != nil {
			return xerrors.Errorf("get provisioner job: %w", err)
		}

		// Check if we should still unhang it.
		jobStatus := db2sdk.ProvisionerJobStatus(job)
		if jobStatus != codersdk.ProvisionerJobRunning {
			return jobInelligibleError{
				Err: xerrors.Errorf("job is not running (status %s)", jobStatus),
			}
		}
		if job.UpdatedAt.After(time.Now().Add(-HungJobDuration)) {
			return jobInelligibleError{
				Err: xerrors.New("job has been updated recently"),
			}
		}

		log.Info(ctx, "detected hung (>5m) provisioner job, forcefully terminating")

		// First, get the latest logs from the build so we can make sure
		// our messages are in the latest stage.
		logs, err := db.GetProvisionerLogsAfterID(ctx, database.GetProvisionerLogsAfterIDParams{
			JobID:        job.ID,
			CreatedAfter: 0,
		})
		if err != nil {
			return xerrors.Errorf("get logs for hung job: %w", err)
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
			return xerrors.Errorf("insert logs for hung job: %w", err)
		}
		lowestLogID = newLogs[0].ID

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
				String: "Coder: Build has been detected as hung for 5 minutes and has been terminated by hang detector.",
				Valid:  true,
			},
			ErrorCode: sql.NullString{
				Valid: false,
			},
		})
		if err != nil {
			return xerrors.Errorf("mark job as failed: %w", err)
		}

		// If the provisioner job is a workspace build, copy the
		// provisioner state from the previous build to this workspace
		// build.
		if job.Type == database.ProvisionerJobTypeWorkspaceBuild {
			build, err := db.GetWorkspaceBuildByJobID(ctx, job.ID)
			if err != nil {
				return xerrors.Errorf("get workspace build for workspace build job by job id: %w", err)
			}

			// Only copy the provisioner state if there's no state in
			// the current build.
			if len(build.ProvisionerState) == 0 {
				// Get the previous build if it exists.
				prevBuild, err := db.GetWorkspaceBuildByWorkspaceIDAndBuildNumber(ctx, database.GetWorkspaceBuildByWorkspaceIDAndBuildNumberParams{
					WorkspaceID: build.WorkspaceID,
					BuildNumber: build.BuildNumber - 1,
				})
				if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
					return xerrors.Errorf("get previous workspace build: %w", err)
				}
				if err == nil {
					_, err = db.UpdateWorkspaceBuildByID(ctx, database.UpdateWorkspaceBuildByIDParams{
						ID:               build.ID,
						UpdatedAt:        database.Now(),
						ProvisionerState: prevBuild.ProvisionerState,
						Deadline:         time.Time{},
						MaxDeadline:      time.Time{},
					})
					if err != nil {
						return xerrors.Errorf("update workspace build by id: %w", err)
					}
				}
			}
		}

		return nil
	}, nil)
	if err != nil {
		return xerrors.Errorf("in tx: %w", err)
	}

	// Publish the new log notification to pubsub. Use the lowest log ID
	// inserted so the log stream will fetch everything after that point.
	data, err := json.Marshal(provisionersdk.ProvisionerJobLogsNotifyMessage{
		CreatedAfter: lowestLogID - 1,
		EndOfLogs:    true,
	})
	if err != nil {
		return xerrors.Errorf("marshal log notification: %w", err)
	}
	err = pub.Publish(provisionersdk.ProvisionerJobLogsNotifyChannel(jobID), data)
	if err != nil {
		return xerrors.Errorf("publish log notification: %w", err)
	}

	return nil
}

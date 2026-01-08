package jobreaper

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt" //#nosec // this is only used for shuffling an array to pick random jobs to unhang
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/provisionersdk"
)

const (
	// HungJobDuration is the duration of time since the last update
	// to a RUNNING job before it is considered hung.
	HungJobDuration = 5 * time.Minute

	// PendingJobDuration is the duration of time since last update
	// to a PENDING job before it is considered dead.
	PendingJobDuration = 30 * time.Minute

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

// jobLogMessages are written to provisioner job logs when a job is reaped
func JobLogMessages(reapType ReapType, threshold time.Duration) []string {
	return []string{
		"",
		"====================",
		fmt.Sprintf("Coder: Build has been detected as %s for %.0f minutes and will be terminated.", reapType, threshold.Minutes()),
		"====================",
		"",
	}
}

type jobToReap struct {
	ID        uuid.UUID
	Threshold time.Duration
	Type      ReapType
}

type ReapType string

const (
	Pending ReapType = "pending"
	Hung    ReapType = "hung"
)

// acquireLockError is returned when the detector fails to acquire a lock and
// cancels the current run.
type acquireLockError struct{}

// Error implements error.
func (acquireLockError) Error() string {
	return "lock is held by another client"
}

// jobIneligibleError is returned when a job is not eligible to be terminated
// anymore.
type jobIneligibleError struct {
	Err error
}

// Error implements error.
func (e jobIneligibleError) Error() string {
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

// New returns a new job reaper.
func New(ctx context.Context, db database.Store, pub pubsub.Pubsub, log slog.Logger, tick <-chan time.Time) *Detector {
	//nolint:gocritic // Job reaper has a limited set of permissions.
	ctx, cancel := context.WithCancel(dbauthz.AsJobReaper(ctx))
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

	// Find all provisioner jobs to be reaped
	jobs, err := d.db.GetProvisionerJobsToBeReaped(ctx, database.GetProvisionerJobsToBeReapedParams{
		PendingSince: t.Add(-PendingJobDuration),
		HungSince:    t.Add(-HungJobDuration),
		MaxJobs:      MaxJobsPerRun,
	})
	if err != nil {
		stats.Error = xerrors.Errorf("get provisioner jobs to be reaped: %w", err)
		return stats
	}

	jobsToReap := make([]*jobToReap, 0, len(jobs))

	for _, job := range jobs {
		j := &jobToReap{
			ID: job.ID,
		}
		if job.JobStatus == database.ProvisionerJobStatusPending {
			j.Threshold = PendingJobDuration
			j.Type = Pending
		} else {
			j.Threshold = HungJobDuration
			j.Type = Hung
		}
		jobsToReap = append(jobsToReap, j)
	}

	// Send a message into the build log for each hung or pending job saying that it
	// has been detected and will be terminated, then mark the job as failed.
	for _, job := range jobsToReap {
		log := d.log.With(slog.F("job_id", job.ID))

		err := reapJob(ctx, log, d.db, d.pubsub, job)
		if err != nil {
			if !(xerrors.As(err, &acquireLockError{}) || xerrors.As(err, &jobIneligibleError{})) {
				log.Error(ctx, "error forcefully terminating provisioner job", slog.F("type", job.Type), slog.Error(err))
			}
			continue
		}

		stats.TerminatedJobIDs = append(stats.TerminatedJobIDs, job.ID)
	}

	return stats
}

func reapJob(ctx context.Context, log slog.Logger, db database.Store, pub pubsub.Pubsub, jobToReap *jobToReap) error {
	var lowestLogID int64

	err := db.InTx(func(db database.Store) error {
		// Refetch the job while we hold the lock.
		job, err := db.GetProvisionerJobByIDForUpdate(ctx, jobToReap.ID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				return acquireLockError{}
			}
			return xerrors.Errorf("get provisioner job: %w", err)
		}

		if job.CompletedAt.Valid {
			return jobIneligibleError{
				Err: xerrors.Errorf("job is completed (status %s)", job.JobStatus),
			}
		}
		if job.UpdatedAt.After(time.Now().Add(-jobToReap.Threshold)) {
			return jobIneligibleError{
				Err: xerrors.New("job has been updated recently"),
			}
		}

		log.Warn(
			ctx, "forcefully terminating provisioner job",
			slog.F("type", jobToReap.Type),
			slog.F("threshold", jobToReap.Threshold),
		)

		// First, get the latest logs from the build so we can make sure
		// our messages are in the latest stage.
		logs, err := db.GetProvisionerLogsAfterID(ctx, database.GetProvisionerLogsAfterIDParams{
			JobID:        job.ID,
			CreatedAfter: 0,
		})
		if err != nil {
			return xerrors.Errorf("get logs for %s job: %w", jobToReap.Type, err)
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
			JobID:     job.ID,
			CreatedAt: nil,
			Source:    nil,
			Level:     nil,
			Stage:     nil,
			Output:    nil,
		}
		now := dbtime.Now()
		for i, msg := range JobLogMessages(jobToReap.Type, jobToReap.Threshold) {
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
			return xerrors.Errorf("insert logs for %s job: %w", job.JobStatus, err)
		}
		lowestLogID = newLogs[0].ID

		// Mark the job as failed.
		now = dbtime.Now()

		// If the job was never started (pending), set the StartedAt time to the current
		// time so that the build duration is correct.
		if job.JobStatus == database.ProvisionerJobStatusPending {
			job.StartedAt = sql.NullTime{
				Time:  now,
				Valid: true,
			}
		}
		err = db.UpdateProvisionerJobWithCompleteWithStartedAtByID(ctx, database.UpdateProvisionerJobWithCompleteWithStartedAtByIDParams{
			ID:        job.ID,
			UpdatedAt: now,
			CompletedAt: sql.NullTime{
				Time:  now,
				Valid: true,
			},
			Error: sql.NullString{
				String: fmt.Sprintf("Coder: Build has been detected as %s for %.0f minutes and has been terminated by the reaper.", jobToReap.Type, jobToReap.Threshold.Minutes()),
				Valid:  true,
			},
			ErrorCode: sql.NullString{
				Valid: false,
			},
			StartedAt: job.StartedAt,
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
					err = db.UpdateWorkspaceBuildProvisionerStateByID(ctx, database.UpdateWorkspaceBuildProvisionerStateByIDParams{
						ID:               build.ID,
						UpdatedAt:        dbtime.Now(),
						ProvisionerState: prevBuild.ProvisionerState,
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
	err = pub.Publish(provisionersdk.ProvisionerJobLogsNotifyChannel(jobToReap.ID), data)
	if err != nil {
		return xerrors.Errorf("publish log notification: %w", err)
	}

	return nil
}

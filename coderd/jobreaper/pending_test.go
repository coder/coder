package jobreaper_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/jobreaper"
	"github.com/coder/coder/v2/testutil"
)

// newTemplateJob creates the template version required to authorize job reads.
func newTemplateJob(
	t *testing.T, db database.Store, seed database.ProvisionerJob,
) database.ProvisionerJob {
	t.Helper()

	job := dbgen.ProvisionerJob(t, db, nil, seed)
	dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: job.OrganizationID,
		CreatedBy:      job.InitiatorID,
		JobID:          job.ID,
	})
	return job
}

// newDaemonJob creates a job acquired by the daemon. dbgen gives a started
// job a tag unique to it, so acquisition cannot pick up a pending job under
// test instead.
func newDaemonJob(
	t *testing.T, db database.Store, daemon database.ProvisionerDaemon,
	startedAt time.Time, completedAt sql.NullTime,
) {
	t.Helper()

	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: daemon.OrganizationID,
		WorkerID:       uuid.NullUUID{UUID: daemon.ID, Valid: true},
		StartedAt:      sql.NullTime{Time: startedAt, Valid: true},
		CompletedAt:    completedAt,
	})
	require.Equal(t, daemon.ID, job.WorkerID.UUID)
}

// daemonProgress is how a test daemon is, or is not, making progress.
type daemonProgress int

const (
	// completedRecently: the daemon completed a job inside the window.
	completedRecently daemonProgress = iota
	// runningJob: the daemon completed nothing inside the window but is
	// running a job that is not hung, like a long build.
	runningJob
	// noProgress: the daemon's last job completed before the window.
	noProgress
)

// newDaemonWithProgress creates a provisioner daemon and the job history that
// gives it the requested progress at now.
func newDaemonWithProgress(
	t *testing.T, db database.Store, seed database.ProvisionerDaemon,
	progress daemonProgress, now time.Time,
) {
	t.Helper()

	daemon := dbgen.ProvisionerDaemon(t, db, seed)
	switch progress {
	case completedRecently:
		newDaemonJob(t, db, daemon, now.Add(-10*time.Minute),
			sql.NullTime{Time: now.Add(-5 * time.Minute), Valid: true})
	case runningJob:
		newDaemonJob(t, db, daemon, now.Add(-time.Minute), sql.NullTime{})
	case noProgress:
		windowStart := now.Add(-jobreaper.ProvisionerProgressWindow)
		newDaemonJob(t, db, daemon, windowStart.Add(-10*time.Minute),
			sql.NullTime{Time: windowStart.Add(-5 * time.Minute), Valid: true})
	}
}

// TestDetectorPendingProvisionerAvailability verifies that pending jobs past
// the ordinary timeout are reaped only when no eligible provisioner is online
// and making progress.
//
//nolint:tparallel,paralleltest // Subtests share detector tick/stats channels.
func TestDetectorPendingProvisionerAvailability(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	env := newDetectorTestEnv(ctx, t)
	defer env.close()
	user := dbgen.User(t, env.DB, database.User{})
	file := dbgen.File(t, env.DB, database.File{})

	for _, tc := range []struct {
		name                 string
		wrongOrg             bool
		wrongProvisionerType bool
		stale                bool
		canceled             bool
		daemonTags           database.StringMap
		progress             daemonProgress
		jobAge               time.Duration
		wantReaped           bool
	}{
		{name: "Online"},
		// A moving queue protects jobs past the former 2 hour limit.
		{name: "OnlinePastFormerMaximumAge", jobAge: 3 * time.Hour},
		{name: "OnlineRunningLongBuild", progress: runningJob},
		{name: "OnlineNoProgress", progress: noProgress, wantReaped: true},
		{name: "WrongOrganization", wrongOrg: true, wantReaped: true},
		{name: "WrongProvisionerType", wrongProvisionerType: true, wantReaped: true},
		{name: "WrongTags", daemonTags: database.StringMap{"pool": "other"}, wantReaped: true},
		{name: "StaleHeartbeat", stale: true, wantReaped: true},
		// Canceled jobs that were never acquired are not pending, so an
		// online provisioner does not protect them.
		{name: "CanceledBeforeAcquisition", canceled: true, wantReaped: true},
	} {
		// Subtests run sequentially because they share a detector and
		// its tick/stats channels.
		t.Run(tc.name, func(t *testing.T) {
			now := dbtime.Now()
			org := dbgen.Organization(t, env.DB, database.Organization{})
			tags := database.StringMap{"pool": "test"}
			queuedAt := now.Add(-35 * time.Minute)
			if tc.jobAge != 0 {
				queuedAt = now.Add(-tc.jobAge)
			}
			var canceledAt sql.NullTime
			if tc.canceled {
				canceledAt = sql.NullTime{Time: queuedAt, Valid: true}
			}
			job := newTemplateJob(t, env.DB, database.ProvisionerJob{
				OrganizationID: org.ID,
				InitiatorID:    user.ID,
				FileID:         file.ID,
				Type:           database.ProvisionerJobTypeTemplateVersionImport,
				CreatedAt:      queuedAt,
				UpdatedAt:      queuedAt,
				Tags:           tags,
				CanceledAt:     canceledAt,
			})
			daemonSeed := database.ProvisionerDaemon{
				OrganizationID: org.ID,
				Tags:           tags,
				LastSeenAt:     sql.NullTime{Time: now.Add(-2 * time.Minute), Valid: true},
			}
			if tc.wrongOrg {
				daemonSeed.OrganizationID = dbgen.Organization(t, env.DB, database.Organization{}).ID
			}
			if tc.wrongProvisionerType {
				daemonSeed.Provisioners = []database.ProvisionerType{database.ProvisionerTypeTerraform}
			}
			if tc.daemonTags != nil {
				daemonSeed.Tags = tc.daemonTags
			}
			if tc.stale {
				daemonSeed.LastSeenAt.Time = now.Add(-jobreaper.ProvisionerStaleInterval - time.Minute)
			}
			newDaemonWithProgress(t, env.DB, daemonSeed, tc.progress, now)

			stats := env.tick(ctx, now)
			require.NoError(t, stats.Error)
			if tc.wantReaped {
				require.Contains(t, stats.TerminatedJobIDs, job.ID)
				reapType := jobreaper.Pending
				if tc.canceled {
					reapType = jobreaper.Hung
				}
				requireTerminatedJob(ctx, t, env.DB, job.ID, now, reapType)
			} else {
				require.NotContains(t, stats.TerminatedJobIDs, job.ID)
				current, err := env.DB.GetProvisionerJobByID(ctx, job.ID)
				require.NoError(t, err)
				require.Equal(t, database.ProvisionerJobStatusPending, current.JobStatus)
				require.Equal(t, job.UpdatedAt, current.UpdatedAt)
			}
		})
	}
}

// TestDetectorPendingMaximumAge verifies that a job pending past the absolute
// age limit is reaped even when an eligible provisioner is online and making
// progress.
func TestDetectorPendingMaximumAge(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	env := newDetectorTestEnv(ctx, t)
	defer env.close()

	// GIVEN: A pending job past the absolute age limit and an eligible online
	// provisioner that is making progress.
	now := dbtime.Now()
	queuedAt := now.Add(-jobreaper.MaxPendingJobDuration - time.Hour)
	org := dbgen.Organization(t, env.DB, database.Organization{})
	user := dbgen.User(t, env.DB, database.User{})
	file := dbgen.File(t, env.DB, database.File{})
	tags := database.StringMap{"pool": "test"}
	newDaemonWithProgress(t, env.DB, database.ProvisionerDaemon{
		OrganizationID: org.ID, Tags: tags,
		LastSeenAt: sql.NullTime{Time: now, Valid: true},
	}, completedRecently, now)
	job := newTemplateJob(t, env.DB, database.ProvisionerJob{
		OrganizationID: org.ID,
		InitiatorID:    user.ID,
		FileID:         file.ID,
		Type:           database.ProvisionerJobTypeTemplateVersionImport,
		CreatedAt:      queuedAt,
		UpdatedAt:      queuedAt,
		Tags:           tags,
	})

	// WHEN: The detector runs.
	stats := env.tick(ctx, now)

	// THEN: The job is reaped for exceeding the maximum pending age.
	require.NoError(t, stats.Error)
	require.Contains(t, stats.TerminatedJobIDs, job.ID)
	requireTerminatedJob(ctx, t, env.DB, job.ID, now, jobreaper.PendingMaxAge)

	// Verify the build log explains why the job is being terminated.
	logs, err := env.DB.GetProvisionerLogsAfterID(ctx, database.GetProvisionerLogsAfterIDParams{
		JobID:        job.ID,
		CreatedAfter: 0,
	})
	require.NoError(t, err)
	var logMessages []string
	for _, log := range logs {
		logMessages = append(logMessages, log.Output)
	}
	require.Contains(t, logMessages, "Coder: Build has been pending for more than 24 hours, exceeding the maximum pending age. It will be terminated. Contact your administrator.")
}

// TestDetectorExcludesProtectedPendingJobsFromCandidates verifies that
// protected pending jobs do not consume the MaxJobsPerRun candidate limit, so
// jobs eligible for reaping are not starved by a large protected backlog.
func TestDetectorExcludesProtectedPendingJobsFromCandidates(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	env := newDetectorTestEnv(ctx, t)
	defer env.close()

	// GIVEN: Pending jobs past the ordinary timeout, including a large protected
	// backlog and MaxJobsPerRun jobs with no eligible online provisioner.
	now := dbtime.Now()
	org := dbgen.Organization(t, env.DB, database.Organization{})
	user := dbgen.User(t, env.DB, database.User{})
	file := dbgen.File(t, env.DB, database.File{})
	tags := database.StringMap{"pool": "available"}
	newDaemonWithProgress(t, env.DB, database.ProvisionerDaemon{
		OrganizationID: org.ID, Tags: tags,
		LastSeenAt: sql.NullTime{Time: now.Add(-2 * time.Minute), Valid: true},
	}, completedRecently, now)
	for range 3 * jobreaper.MaxJobsPerRun {
		newTemplateJob(t, env.DB, database.ProvisionerJob{
			OrganizationID: org.ID, InitiatorID: user.ID, FileID: file.ID,
			Type:      database.ProvisionerJobTypeTemplateVersionImport,
			CreatedAt: now.Add(-35 * time.Minute), UpdatedAt: now.Add(-35 * time.Minute),
			Tags: tags,
		})
	}
	var expected []uuid.UUID
	for range jobreaper.MaxJobsPerRun {
		job := newTemplateJob(t, env.DB, database.ProvisionerJob{
			OrganizationID: org.ID, InitiatorID: user.ID, FileID: file.ID,
			Type:      database.ProvisionerJobTypeTemplateVersionImport,
			CreatedAt: now.Add(-35 * time.Minute), UpdatedAt: now.Add(-35 * time.Minute),
			Tags: database.StringMap{"pool": "missing"},
		})
		expected = append(expected, job.ID)
	}

	// WHEN: The detector runs.
	stats := env.tick(ctx, now)

	// THEN: All jobs eligible for reaping are terminated in this run,
	// despite the larger backlog of protected pending jobs.
	require.NoError(t, stats.Error)
	require.ElementsMatch(t, expected, stats.TerminatedJobIDs)
}

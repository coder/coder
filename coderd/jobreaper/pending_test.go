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
) database.ProvisionerJob {
	t.Helper()

	return dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		OrganizationID: daemon.OrganizationID,
		WorkerID:       uuid.NullUUID{UUID: daemon.ID, Valid: true},
		StartedAt:      sql.NullTime{Time: startedAt, Valid: true},
		CompletedAt:    completedAt,
	})
}

// daemonProgress is how a test daemon is, or is not, making progress. The
// zero value is completedRecently, so table rows that omit it get a daemon
// that is making progress.
type daemonProgress int

const (
	// completedRecently: the daemon completed a job inside the window.
	completedRecently daemonProgress = iota
	// runningLongBuild: the daemon completed nothing inside the window but is
	// running a job that started before the window and is still being
	// updated.
	runningLongBuild
	// noProgress: the daemon's last job completed before the window.
	noProgress
)

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
	case runningLongBuild:
		windowStart := now.Add(-jobreaper.ProvisionerProgressWindow)
		job := newDaemonJob(t, db, daemon, windowStart.Add(-10*time.Minute), sql.NullTime{})
		// Acquisition sets updated_at to started_at, so refresh it as the
		// provisioner does while the build runs.
		err := db.UpdateProvisionerJobByID(t.Context(), database.UpdateProvisionerJobByIDParams{
			ID: job.ID, UpdatedAt: now.Add(-time.Minute),
		})
		require.NoError(t, err)
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
		// A daemon making progress protects jobs for as long as it keeps
		// making progress, with no absolute age limit.
		{name: "OnlineLongQueue", jobAge: 48 * time.Hour},
		{name: "OnlineRunningLongBuild", progress: runningLongBuild},
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
			queuedAt := now.Add(-jobreaper.PendingJobDuration - 5*time.Minute)
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
			// A pending job with the same requirements makes the requirement
			// set protected, so the canceled job is reaped only because it is
			// not pending.
			var pendingSibling database.ProvisionerJob
			if tc.canceled {
				pendingSibling = newTemplateJob(t, env.DB, database.ProvisionerJob{
					OrganizationID: org.ID,
					InitiatorID:    user.ID,
					FileID:         file.ID,
					Type:           database.ProvisionerJobTypeTemplateVersionImport,
					CreatedAt:      queuedAt,
					UpdatedAt:      queuedAt,
					Tags:           tags,
				})
			}
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
				if tc.canceled {
					require.NotContains(t, stats.TerminatedJobIDs, pendingSibling.ID)
				}
			} else {
				require.NotContains(t, stats.TerminatedJobIDs, job.ID)
				// The tick did not reap the job, so run the locked recheck
				// directly to cover its progress query.
				err := jobreaper.ReapPendingJob(ctx, testutil.Logger(t), env.DB, env.Pubsub, job.ID)
				require.ErrorContains(t, err, "an eligible provisioner is online and making progress")
				current, err := env.DB.GetProvisionerJobByID(ctx, job.ID)
				require.NoError(t, err)
				require.Equal(t, database.ProvisionerJobStatusPending, current.JobStatus)
				require.Equal(t, job.UpdatedAt, current.UpdatedAt)
			}
		})
	}
}

// TestDetectorExcludesProtectedPendingJobsFromCandidates verifies that
// protected pending jobs do not consume the MaxJobsPerRun candidate limit, so
// jobs eligible for reaping are not starved by a large protected backlog.
func TestDetectorExcludesProtectedPendingJobsFromCandidates(t *testing.T) {
	t.Parallel()

	// Each kind of progress must protect the backlog in the candidate query
	// itself, not only in the locked recheck.
	for _, tc := range []struct {
		name     string
		progress daemonProgress
	}{
		{name: "CompletedRecently", progress: completedRecently},
		{name: "RunningLongBuild", progress: runningLongBuild},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			queuedAt := now.Add(-jobreaper.PendingJobDuration - 5*time.Minute)
			newDaemonWithProgress(t, env.DB, database.ProvisionerDaemon{
				OrganizationID: org.ID, Tags: tags,
				LastSeenAt: sql.NullTime{Time: now.Add(-2 * time.Minute), Valid: true},
			}, tc.progress, now)
			for range 3 * jobreaper.MaxJobsPerRun {
				newTemplateJob(t, env.DB, database.ProvisionerJob{
					OrganizationID: org.ID, InitiatorID: user.ID, FileID: file.ID,
					Type:      database.ProvisionerJobTypeTemplateVersionImport,
					CreatedAt: queuedAt, UpdatedAt: queuedAt,
					Tags: tags,
				})
			}
			var expected []uuid.UUID
			for range jobreaper.MaxJobsPerRun {
				job := newTemplateJob(t, env.DB, database.ProvisionerJob{
					OrganizationID: org.ID, InitiatorID: user.ID, FileID: file.ID,
					Type:      database.ProvisionerJobTypeTemplateVersionImport,
					CreatedAt: queuedAt, UpdatedAt: queuedAt,
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
		})
	}
}

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

// TestDetectorPendingProvisionerAvailability verifies that pending jobs past
// the ordinary timeout are reaped only when no eligible provisioner is online.
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
		wantReaped           bool
	}{
		{name: "Online"},
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
			var canceledAt sql.NullTime
			if tc.canceled {
				canceledAt = sql.NullTime{Time: now.Add(-35 * time.Minute), Valid: true}
			}
			job := newTemplateJob(t, env.DB, database.ProvisionerJob{
				OrganizationID: org.ID,
				InitiatorID:    user.ID,
				FileID:         file.ID,
				Type:           database.ProvisionerJobTypeTemplateVersionImport,
				CreatedAt:      now.Add(-35 * time.Minute),
				UpdatedAt:      now.Add(-35 * time.Minute),
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
			dbgen.ProvisionerDaemon(t, env.DB, daemonSeed)

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
// age limit is reaped even when an eligible provisioner is online.
func TestDetectorPendingMaximumAge(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	env := newDetectorTestEnv(ctx, t)
	defer env.close()

	// GIVEN: A pending job past the absolute age limit and an eligible online
	// provisioner.
	now := dbtime.Now()
	queuedAt := now.Add(-jobreaper.MaxPendingJobDuration - time.Hour)
	org := dbgen.Organization(t, env.DB, database.Organization{})
	user := dbgen.User(t, env.DB, database.User{})
	file := dbgen.File(t, env.DB, database.File{})
	tags := database.StringMap{"pool": "test"}
	dbgen.ProvisionerDaemon(t, env.DB, database.ProvisionerDaemon{
		OrganizationID: org.ID, Tags: tags,
		LastSeenAt: sql.NullTime{Time: now, Valid: true},
	})
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
	require.Contains(t, logMessages, "Coder: Build has been pending for more than 120 minutes, exceeding the maximum pending age. It will be terminated. Contact your administrator.")
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
	dbgen.ProvisionerDaemon(t, env.DB, database.ProvisionerDaemon{
		OrganizationID: org.ID, Tags: tags,
		LastSeenAt: sql.NullTime{Time: now.Add(-2 * time.Minute), Valid: true},
	})
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

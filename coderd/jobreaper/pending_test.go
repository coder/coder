package jobreaper_test

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/jobreaper"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// newTemplateJob creates the template version required to authorize job reads.
func newTemplateJob(
	t *testing.T, db database.Store, seed database.ProvisionerJob,
) database.ProvisionerJob {
	t.Helper()

	if seed.Type == database.ProvisionerJobTypeTemplateVersionDryRun {
		version := dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: seed.OrganizationID,
			CreatedBy:      seed.InitiatorID,
		})
		input, err := json.Marshal(provisionerdserver.TemplateVersionDryRunJob{
			TemplateVersionID: version.ID,
		})
		require.NoError(t, err)
		seed.Input = input
	}
	job := dbgen.ProvisionerJob(t, db, nil, seed)
	if job.Type == database.ProvisionerJobTypeTemplateVersionImport {
		dbgen.TemplateVersion(t, db, database.TemplateVersion{
			OrganizationID: job.OrganizationID,
			CreatedBy:      job.InitiatorID,
			JobID:          job.ID,
		})
	}
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
		name         string
		wrongOrg     bool
		wrongBackend bool
		stale        bool
		noHeartbeat  bool
		deletedKey   bool
		reservedKey  bool
		canceled     bool
		jobTags      database.StringMap
		daemonTags   database.StringMap
		wantReaped   bool
	}{
		{name: "Online"},
		{name: "WrongOrganization", wrongOrg: true, wantReaped: true},
		{name: "WrongBackend", wrongBackend: true, wantReaped: true},
		{name: "WrongTags", daemonTags: database.StringMap{"pool": "other"}, wantReaped: true},
		{name: "StaleHeartbeat", stale: true, wantReaped: true},
		{name: "NoHeartbeat", noHeartbeat: true, wantReaped: true},
		{name: "DeletedKey", deletedKey: true, wantReaped: true},
		{name: "ReservedKey", reservedKey: true},
		{name: "CanceledBeforeAcquisition", canceled: true, wantReaped: true},
		{
			name:    "OrganizationScopeExactMatch",
			jobTags: database.StringMap{"scope": "organization", "owner": ""},
		},
		{
			name:       "OrganizationScopeExtraTag",
			jobTags:    database.StringMap{"scope": "organization", "owner": ""},
			daemonTags: database.StringMap{"scope": "organization", "owner": "", "pool": "extra"},
			wantReaped: true,
		},
		{
			name:       "TagSubset",
			daemonTags: database.StringMap{"pool": "test", "region": "east"},
		},
	} {
		// Subtests run sequentially because they share a detector and
		// its tick/stats channels.
		t.Run(tc.name, func(t *testing.T) {
			now := dbtime.Now()
			org := dbgen.Organization(t, env.DB, database.Organization{})
			tags := tc.jobTags
			if tags == nil {
				tags = database.StringMap{"pool": "test"}
			}
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
				LastSeenAt:     sql.NullTime{Time: now, Valid: true},
			}
			if tc.wrongOrg {
				daemonSeed.OrganizationID = dbgen.Organization(t, env.DB, database.Organization{}).ID
			}
			if tc.wrongBackend {
				daemonSeed.Provisioners = []database.ProvisionerType{database.ProvisionerTypeTerraform}
			}
			if tc.daemonTags != nil {
				daemonSeed.Tags = tc.daemonTags
			}
			if tc.stale {
				daemonSeed.LastSeenAt.Time = now.Add(-5 * time.Minute)
			}
			if tc.reservedKey {
				daemonSeed.KeyID = codersdk.ProvisionerKeyUUIDBuiltIn
			}
			if tc.noHeartbeat {
				// Keep Time nonzero so dbgen does not supply its default heartbeat.
				daemonSeed.LastSeenAt.Valid = false
			}
			daemon := dbgen.ProvisionerDaemon(t, env.DB, daemonSeed)
			require.Equal(t, !tc.noHeartbeat, daemon.LastSeenAt.Valid)
			if tc.deletedKey {
				require.NoError(t, env.DB.DeleteProvisionerKey(ctx, daemon.KeyID))
			}

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

// TestDetectorPendingBusyProvisioner verifies that an eligible online provisioner
// protects pending jobs from the ordinary timeout even while busy with another job.
func TestDetectorPendingBusyProvisioner(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	env := newDetectorTestEnv(ctx, t)
	defer env.close()

	// GIVEN: Jobs past the ordinary pending timeout and an eligible online
	// provisioner already processing another job.
	now := dbtime.Now()
	queuedAt := now.Add(-35 * time.Minute)
	org := dbgen.Organization(t, env.DB, database.Organization{})
	user := dbgen.User(t, env.DB, database.User{})
	build := dbfake.WorkspaceBuild(t, env.DB, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
	}).Pending(dbfake.WithJobCreatedAt(queuedAt), dbfake.WithJobUpdatedAt(queuedAt)).Do()
	buildJob, err := env.DB.GetProvisionerJobByID(ctx, build.Build.JobID)
	require.NoError(t, err)
	daemon := dbgen.ProvisionerDaemon(t, env.DB, database.ProvisionerDaemon{
		OrganizationID: buildJob.OrganizationID,
		Provisioners:   []database.ProvisionerType{buildJob.Provisioner},
		Tags:           buildJob.Tags,
		LastSeenAt:     sql.NullTime{Time: now, Valid: true},
	})
	jobs := []database.ProvisionerJob{buildJob}
	for _, jobType := range []database.ProvisionerJobType{
		database.ProvisionerJobTypeTemplateVersionImport,
		database.ProvisionerJobTypeTemplateVersionDryRun,
	} {
		jobs = append(jobs, newTemplateJob(t, env.DB, database.ProvisionerJob{
			OrganizationID: buildJob.OrganizationID,
			InitiatorID:    buildJob.InitiatorID,
			FileID:         buildJob.FileID,
			Provisioner:    buildJob.Provisioner,
			Type:           jobType,
			Tags:           buildJob.Tags,
			CreatedAt:      queuedAt,
			UpdatedAt:      queuedAt,
		}))
	}
	newTemplateJob(t, env.DB, database.ProvisionerJob{
		OrganizationID: buildJob.OrganizationID,
		InitiatorID:    buildJob.InitiatorID,
		FileID:         buildJob.FileID,
		Provisioner:    buildJob.Provisioner,
		Type:           database.ProvisionerJobTypeTemplateVersionImport,
		StartedAt:      sql.NullTime{Time: now, Valid: true},
		WorkerID:       uuid.NullUUID{UUID: daemon.ID, Valid: true},
	})

	// WHEN: The detector runs.
	stats := env.tick(ctx, now)

	// THEN: Queued jobs remain pending with unchanged updated_at.
	// Protecting a job must not reset its ordinary pending timeout.
	require.NoError(t, stats.Error)
	require.Empty(t, stats.TerminatedJobIDs)
	for _, job := range jobs {
		current, err := env.DB.GetProvisionerJobByID(ctx, job.ID)
		require.NoError(t, err)
		require.Equal(t, database.ProvisionerJobStatusPending, current.JobStatus)
		require.Equal(t, job.UpdatedAt, current.UpdatedAt)
	}
}

// TestDetectorPendingMaximumAge verifies that jobs past the absolute pending
// age limit are reaped regardless of provisioner availability or recent updates.
func TestDetectorPendingMaximumAge(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	env := newDetectorTestEnv(ctx, t)
	defer env.close()

	// GIVEN: Pending jobs exceed the absolute age limit, with an eligible online
	// provisioner and either an old or a recent update.
	now := dbtime.Now()
	org := dbgen.Organization(t, env.DB, database.Organization{})
	user := dbgen.User(t, env.DB, database.User{})
	file := dbgen.File(t, env.DB, database.File{})
	tags := database.StringMap{"pool": "test"}
	dbgen.ProvisionerDaemon(t, env.DB, database.ProvisionerDaemon{
		OrganizationID: org.ID, Tags: tags,
		LastSeenAt: sql.NullTime{Time: now, Valid: true},
	})
	for _, updatedAt := range []time.Time{now.Add(-3 * time.Hour), now} {
		job := newTemplateJob(t, env.DB, database.ProvisionerJob{
			OrganizationID: org.ID,
			InitiatorID:    user.ID,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			CreatedAt:      now.Add(-3 * time.Hour),
			UpdatedAt:      updatedAt,
			Tags:           tags,
		})

		// WHEN: The detector runs.
		stats := env.tick(ctx, now)

		// THEN: The job is reaped for exceeding the maximum pending age.
		require.NoError(t, stats.Error)
		require.Contains(t, stats.TerminatedJobIDs, job.ID)
		current, err := env.DB.GetProvisionerJobByID(ctx, job.ID)
		require.NoError(t, err)
		require.True(t, current.CompletedAt.Valid)
		require.Contains(t, current.Error.String, "pending beyond the maximum age of 120 minutes")
	}
}

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
		LastSeenAt: sql.NullTime{Time: now, Valid: true},
	})
	for range 100 {
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

// TestPendingReaperCandidatesWithSharedProvisionerRequirements verifies that
// deduplicating provisioner requirements preserves each job's eligibility
// for reaping.
func TestPendingReaperCandidatesWithSharedProvisionerRequirements(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)

	// GIVEN: Jobs with shared and differing provisioner requirements,
	// timestamps, and statuses.
	now := dbtime.Now()
	queuedAt := now.Add(-35 * time.Minute)
	org := dbgen.Organization(t, db, database.Organization{})
	otherOrg := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	file := dbgen.File(t, db, database.File{})
	tags := database.StringMap{"pool": "available"}
	unmatchedTags := database.StringMap{"pool": "missing"}
	for range 2 {
		dbgen.ProvisionerDaemon(t, db, database.ProvisionerDaemon{
			OrganizationID: org.ID,
			Tags:           tags,
			LastSeenAt:     sql.NullTime{Time: now, Valid: true},
		})
	}
	params := database.GetProvisionerJobsToBeReapedParams{
		PendingSince:         now.Add(-jobreaper.PendingJobDuration),
		MaxPendingSince:      now.Add(-2 * time.Hour),
		ProvisionerSeenSince: now.Add(-provisionerdserver.StaleInterval),
		HungSince:            now.Add(-jobreaper.HungJobDuration),
		MaxJobs:              100,
	}
	var expected []uuid.UUID
	for _, jobSpec := range []struct {
		name          string
		jobOverrides  database.ProvisionerJob
		wantCandidate bool
	}{
		{name: "Protected"},
		{name: "ProtectedDuplicateRequirements"},
		{name: "OtherOrganization", jobOverrides: database.ProvisionerJob{OrganizationID: otherOrg.ID}, wantCandidate: true},
		{name: "OtherBackend", jobOverrides: database.ProvisionerJob{Provisioner: database.ProvisionerTypeTerraform}, wantCandidate: true},
		{name: "OtherTags", jobOverrides: database.ProvisionerJob{Tags: unmatchedTags}, wantCandidate: true},
		{name: "OtherTagsDuplicateRequirements", jobOverrides: database.ProvisionerJob{Tags: unmatchedTags}, wantCandidate: true},
		{
			name: "MaximumAgeWithRecentUpdate",
			jobOverrides: database.ProvisionerJob{
				CreatedAt: params.MaxPendingSince.Add(-time.Microsecond), UpdatedAt: now,
			},
			wantCandidate: true,
		},
		{name: "AtMaximumAge", jobOverrides: database.ProvisionerJob{CreatedAt: params.MaxPendingSince}},
		{name: "AtOrdinaryTimeout", jobOverrides: database.ProvisionerJob{Tags: unmatchedTags, UpdatedAt: params.PendingSince}},
		{name: "RecentUpdate", jobOverrides: database.ProvisionerJob{Tags: unmatchedTags, UpdatedAt: now}},
		{
			name:          "CanceledWithSharedRequirements",
			jobOverrides:  database.ProvisionerJob{CanceledAt: sql.NullTime{Time: queuedAt, Valid: true}},
			wantCandidate: true,
		},
		{
			name:         "CompletedWithSharedRequirements",
			jobOverrides: database.ProvisionerJob{CompletedAt: sql.NullTime{Time: now, Valid: true}},
		},
	} {
		job := jobSpec.jobOverrides
		if job.OrganizationID == uuid.Nil {
			job.OrganizationID = org.ID
		}
		if job.Tags == nil {
			job.Tags = tags
		}
		if job.CreatedAt.IsZero() {
			job.CreatedAt = queuedAt
		}
		if job.UpdatedAt.IsZero() {
			job.UpdatedAt = queuedAt
		}
		job.InitiatorID = user.ID
		job.FileID = file.ID
		job = dbgen.ProvisionerJob(t, db, nil, job)
		t.Logf("%s: %s", jobSpec.name, job.ID)
		if jobSpec.wantCandidate {
			expected = append(expected, job.ID)
		}
	}

	// WHEN: Query candidates directly.
	jobs, err := db.GetProvisionerJobsToBeReaped(ctx, params)
	require.NoError(t, err)
	var actual []uuid.UUID
	for _, job := range jobs {
		actual = append(actual, job.ID)
	}

	// THEN: Exactly the expected jobs are returned, with no duplicates.
	require.ElementsMatch(t, expected, actual)
}

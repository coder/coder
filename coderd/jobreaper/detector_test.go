package jobreaper_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/jobreaper"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

// detectorTestEnv provides common infrastructure for jobreaper detector tests,
// reducing the repeated setup/teardown boilerplate across every test function.
type detectorTestEnv struct {
	t        *testing.T
	DB       database.Store
	Pubsub   pubsub.Pubsub
	detector *jobreaper.Detector
	tickCh   chan time.Time
	statsCh  chan jobreaper.Stats
}

// newDetectorTestEnv creates a new test environment with a started detector.
func newDetectorTestEnv(ctx context.Context, t *testing.T) *detectorTestEnv {
	t.Helper()
	db, ps := dbtestutil.NewDB(t)
	log := testutil.Logger(t)
	tickCh := make(chan time.Time)
	statsCh := make(chan jobreaper.Stats)

	detector := jobreaper.New(ctx, wrapDBAuthz(db, log), ps, log, tickCh).WithStatsChannel(statsCh)
	detector.Start()

	return &detectorTestEnv{
		t:        t,
		DB:       db,
		Pubsub:   ps,
		detector: detector,
		tickCh:   tickCh,
		statsCh:  statsCh,
	}
}

// tick sends a tick with the given time and returns the stats from the
// detector run. It respects context cancellation to avoid blocking forever
// if the detector exits unexpectedly.
//
// tick must not be called from a separate goroutine, as it calls
// require.FailNow which uses runtime.Goexit under the hood.
func (e *detectorTestEnv) tick(ctx context.Context, now time.Time) jobreaper.Stats {
	e.t.Helper()
	testutil.RequireSend(ctx, e.t, e.tickCh, now)
	return testutil.RequireReceive(ctx, e.t, e.statsCh)
}

// close stops the detector and waits for it to finish.
func (e *detectorTestEnv) close() {
	e.detector.Close()
	e.detector.Wait()
}

// requireTerminatedJob asserts that a provisioner job was properly terminated
// by the job reaper with the expected reap type (hung or pending).
func requireTerminatedJob(ctx context.Context, t *testing.T, db database.Store, jobID uuid.UUID, now time.Time, reapType jobreaper.ReapType) {
	t.Helper()
	job, err := db.GetProvisionerJobByID(ctx, jobID)
	require.NoError(t, err)
	require.WithinDuration(t, now, job.UpdatedAt, 30*time.Second)
	require.True(t, job.CompletedAt.Valid)
	require.WithinDuration(t, now, job.CompletedAt.Time, 30*time.Second)
	if reapType == jobreaper.Pending {
		require.True(t, job.StartedAt.Valid)
		require.WithinDuration(t, now, job.StartedAt.Time, 30*time.Second)
	}
	require.True(t, job.Error.Valid)
	require.Contains(t, job.Error.String, fmt.Sprintf("Build has been detected as %s", reapType))
	require.False(t, job.ErrorCode.Valid)
}

func TestDetectorNoJobs(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	env := newDetectorTestEnv(ctx, t)
	defer env.close()

	stats := env.tick(ctx, time.Now())
	require.NoError(t, stats.Error)
	require.Empty(t, stats.TerminatedJobIDs)
}

func TestDetectorNoHungJobs(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	env := newDetectorTestEnv(ctx, t)
	defer env.close()

	// Insert some jobs that are running and haven't been updated in a while,
	// but not enough to be considered hung.
	now := time.Now()
	org := dbgen.Organization(t, env.DB, database.Organization{})
	user := dbgen.User(t, env.DB, database.User{})
	file := dbgen.File(t, env.DB, database.File{})
	for i := 0; i < 5; i++ {
		dbgen.ProvisionerJob(t, env.DB, env.Pubsub, database.ProvisionerJob{
			CreatedAt: now.Add(-time.Minute * 5),
			UpdatedAt: now.Add(-time.Minute * time.Duration(i)),
			StartedAt: sql.NullTime{
				Time:  now.Add(-time.Minute * 5),
				Valid: true,
			},
			OrganizationID: org.ID,
			InitiatorID:    user.ID,
			Provisioner:    database.ProvisionerTypeEcho,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeWorkspaceBuild,
			Input:          []byte("{}"),
		})
	}

	stats := env.tick(ctx, now)
	require.NoError(t, stats.Error)
	require.Empty(t, stats.TerminatedJobIDs)
}

func TestDetectorHungWorkspaceBuild(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	env := newDetectorTestEnv(ctx, t)
	defer env.close()

	var (
		now                         = time.Now()
		twentyMinAgo                = now.Add(-time.Minute * 20)
		tenMinAgo                   = now.Add(-time.Minute * 10)
		sixMinAgo                   = now.Add(-time.Minute * 6)
		org                         = dbgen.Organization(t, env.DB, database.Organization{})
		user                        = dbgen.User(t, env.DB, database.User{})
		expectedWorkspaceBuildState = []byte(`{"dean":"cool","colin":"also cool"}`)
	)

	// Previous build (completed successfully).
	previousBuild := dbfake.WorkspaceBuild(t, env.DB, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
	}).Pubsub(env.Pubsub).Seed(database.WorkspaceBuild{}).
		ProvisionerState(expectedWorkspaceBuildState).
		Succeeded(dbfake.WithJobCompletedAt(twentyMinAgo)).
		Do()

	// Current build (hung - running job with UpdatedAt > 5 min ago).
	currentBuild := dbfake.WorkspaceBuild(t, env.DB, previousBuild.Workspace).
		Pubsub(env.Pubsub).
		Seed(database.WorkspaceBuild{BuildNumber: 2}).
		Starting(dbfake.WithJobStartedAt(tenMinAgo), dbfake.WithJobUpdatedAt(sixMinAgo)).
		Do()

	t.Log("previous job ID: ", previousBuild.Build.JobID)
	t.Log("current job ID: ", currentBuild.Build.JobID)

	stats := env.tick(ctx, now)
	require.NoError(t, stats.Error)
	require.Len(t, stats.TerminatedJobIDs, 1)
	require.Equal(t, currentBuild.Build.JobID, stats.TerminatedJobIDs[0])

	// Check that the current provisioner job was updated.
	requireTerminatedJob(ctx, t, env.DB, currentBuild.Build.JobID, now, jobreaper.Hung)

	// Check that the provisioner state was copied.
	build, err := env.DB.GetWorkspaceBuildByID(ctx, currentBuild.Build.ID)
	require.NoError(t, err)
	provisionerStateRow, err := env.DB.GetWorkspaceBuildProvisionerStateByID(ctx, build.ID)
	require.NoError(t, err)
	require.Equal(t, expectedWorkspaceBuildState, provisionerStateRow.ProvisionerState)
}

func TestDetectorHungWorkspaceBuildNoOverrideState(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	env := newDetectorTestEnv(ctx, t)
	defer env.close()

	var (
		now                         = time.Now()
		twentyMinAgo                = now.Add(-time.Minute * 20)
		tenMinAgo                   = now.Add(-time.Minute * 10)
		sixMinAgo                   = now.Add(-time.Minute * 6)
		org                         = dbgen.Organization(t, env.DB, database.Organization{})
		user                        = dbgen.User(t, env.DB, database.User{})
		expectedWorkspaceBuildState = []byte(`{"dean":"cool","colin":"also cool"}`)
	)

	// Previous build (completed successfully).
	previousBuild := dbfake.WorkspaceBuild(t, env.DB, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
	}).Pubsub(env.Pubsub).Seed(database.WorkspaceBuild{}).
		ProvisionerState([]byte(`{"dean":"NOT cool","colin":"also NOT cool"}`)).
		Succeeded(dbfake.WithJobCompletedAt(twentyMinAgo)).
		Do()

	// Current build (hung - running job with UpdatedAt > 5 min ago).
	// This build already has provisioner state, which should NOT be overridden.
	currentBuild := dbfake.WorkspaceBuild(t, env.DB, previousBuild.Workspace).
		Pubsub(env.Pubsub).
		Seed(database.WorkspaceBuild{
			BuildNumber: 2,
		}).ProvisionerState(expectedWorkspaceBuildState).
		Starting(dbfake.WithJobStartedAt(tenMinAgo), dbfake.WithJobUpdatedAt(sixMinAgo)).
		Do()

	t.Log("previous job ID: ", previousBuild.Build.JobID)
	t.Log("current job ID: ", currentBuild.Build.JobID)

	stats := env.tick(ctx, now)
	require.NoError(t, stats.Error)
	require.Len(t, stats.TerminatedJobIDs, 1)
	require.Equal(t, currentBuild.Build.JobID, stats.TerminatedJobIDs[0])

	// Check that the current provisioner job was updated.
	requireTerminatedJob(ctx, t, env.DB, currentBuild.Build.JobID, now, jobreaper.Hung)

	// Check that the provisioner state was NOT copied.
	build, err := env.DB.GetWorkspaceBuildByID(ctx, currentBuild.Build.ID)
	require.NoError(t, err)
	provisionerStateRow, err := env.DB.GetWorkspaceBuildProvisionerStateByID(ctx, build.ID)
	require.NoError(t, err)
	require.Equal(t, expectedWorkspaceBuildState, provisionerStateRow.ProvisionerState)
}

func TestDetectorHungWorkspaceBuildNoOverrideStateIfNoExistingBuild(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	env := newDetectorTestEnv(ctx, t)
	defer env.close()

	var (
		now                         = time.Now()
		tenMinAgo                   = now.Add(-time.Minute * 10)
		sixMinAgo                   = now.Add(-time.Minute * 6)
		org                         = dbgen.Organization(t, env.DB, database.Organization{})
		user                        = dbgen.User(t, env.DB, database.User{})
		expectedWorkspaceBuildState = []byte(`{"dean":"cool","colin":"also cool"}`)
	)

	// First build (hung - no previous build exists).
	// This build has provisioner state, which should NOT be overridden.
	currentBuild := dbfake.WorkspaceBuild(t, env.DB, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
	}).Pubsub(env.Pubsub).Seed(database.WorkspaceBuild{}).
		ProvisionerState(expectedWorkspaceBuildState).
		Starting(dbfake.WithJobStartedAt(tenMinAgo), dbfake.WithJobUpdatedAt(sixMinAgo)).
		Do()

	t.Log("current job ID: ", currentBuild.Build.JobID)

	stats := env.tick(ctx, now)
	require.NoError(t, stats.Error)
	require.Len(t, stats.TerminatedJobIDs, 1)
	require.Equal(t, currentBuild.Build.JobID, stats.TerminatedJobIDs[0])

	// Check that the current provisioner job was updated.
	requireTerminatedJob(ctx, t, env.DB, currentBuild.Build.JobID, now, jobreaper.Hung)

	// Check that the provisioner state was NOT updated.
	build, err := env.DB.GetWorkspaceBuildByID(ctx, currentBuild.Build.ID)
	require.NoError(t, err)
	provisionerStateRow, err := env.DB.GetWorkspaceBuildProvisionerStateByID(ctx, build.ID)
	require.NoError(t, err)
	require.Equal(t, expectedWorkspaceBuildState, provisionerStateRow.ProvisionerState)
}

func TestDetectorPendingWorkspaceBuildNoOverrideStateIfNoExistingBuild(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	env := newDetectorTestEnv(ctx, t)
	defer env.close()

	var (
		now                         = time.Now()
		thirtyFiveMinAgo            = now.Add(-time.Minute * 35)
		org                         = dbgen.Organization(t, env.DB, database.Organization{})
		user                        = dbgen.User(t, env.DB, database.User{})
		expectedWorkspaceBuildState = []byte(`{"dean":"cool","colin":"also cool"}`)
	)

	// First build (hung pending - no previous build exists).
	// This build has provisioner state, which should NOT be overridden.
	currentBuild := dbfake.WorkspaceBuild(t, env.DB, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
	}).Pubsub(env.Pubsub).Seed(database.WorkspaceBuild{}).
		ProvisionerState(expectedWorkspaceBuildState).
		Pending(dbfake.WithJobCreatedAt(thirtyFiveMinAgo), dbfake.WithJobUpdatedAt(thirtyFiveMinAgo)).
		Do()

	t.Log("current job ID: ", currentBuild.Build.JobID)

	stats := env.tick(ctx, now)
	require.NoError(t, stats.Error)
	require.Len(t, stats.TerminatedJobIDs, 1)
	require.Equal(t, currentBuild.Build.JobID, stats.TerminatedJobIDs[0])

	// Check that the current provisioner job was updated.
	requireTerminatedJob(ctx, t, env.DB, currentBuild.Build.JobID, now, jobreaper.Pending)

	// Check that the provisioner state was NOT updated.
	build, err := env.DB.GetWorkspaceBuildByID(ctx, currentBuild.Build.ID)
	require.NoError(t, err)
	provisionerStateRow, err := env.DB.GetWorkspaceBuildProvisionerStateByID(ctx, build.ID)
	require.NoError(t, err)
	require.Equal(t, expectedWorkspaceBuildState, provisionerStateRow.ProvisionerState)
}

// TestDetectorWorkspaceBuildForDormantWorkspace ensures that the jobreaper has
// enough permissions to fix dormant workspaces.
//
// Dormant workspaces are treated as rbac.ResourceWorkspaceDormant rather than
// rbac.ResourceWorkspace, which resulted in a bug where the jobreaper would
// be able to see but not fix dormant workspaces.
func TestDetectorWorkspaceBuildForDormantWorkspace(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	env := newDetectorTestEnv(ctx, t)
	defer env.close()

	var (
		now                         = time.Now()
		tenMinAgo                   = now.Add(-time.Minute * 10)
		sixMinAgo                   = now.Add(-time.Minute * 6)
		org                         = dbgen.Organization(t, env.DB, database.Organization{})
		user                        = dbgen.User(t, env.DB, database.User{})
		expectedWorkspaceBuildState = []byte(`{"dean":"cool","colin":"also cool"}`)
	)

	// First build (hung - running job with UpdatedAt > 5 min ago).
	// This build has provisioner state, which should NOT be overridden.
	// The workspace is dormant from the start.
	currentBuild := dbfake.WorkspaceBuild(t, env.DB, database.WorkspaceTable{
		OrganizationID: org.ID,
		OwnerID:        user.ID,
		DormantAt: sql.NullTime{
			Time:  now.Add(-time.Hour),
			Valid: true,
		},
	}).Pubsub(env.Pubsub).Seed(database.WorkspaceBuild{}).
		ProvisionerState(expectedWorkspaceBuildState).
		Starting(dbfake.WithJobStartedAt(tenMinAgo), dbfake.WithJobUpdatedAt(sixMinAgo)).
		Do()

	t.Log("current job ID: ", currentBuild.Build.JobID)

	// Ensure the RBAC is the dormant type to ensure we're testing the right
	// thing.
	require.Equal(t, rbac.ResourceWorkspaceDormant.Type, currentBuild.Workspace.RBACObject().Type)

	stats := env.tick(ctx, now)
	require.NoError(t, stats.Error)
	require.Len(t, stats.TerminatedJobIDs, 1)
	require.Equal(t, currentBuild.Build.JobID, stats.TerminatedJobIDs[0])

	// Check that the current provisioner job was updated.
	requireTerminatedJob(ctx, t, env.DB, currentBuild.Build.JobID, now, jobreaper.Hung)
}

func TestDetectorHungOtherJobTypes(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	env := newDetectorTestEnv(ctx, t)
	defer env.close()

	var (
		now       = time.Now()
		tenMinAgo = now.Add(-time.Minute * 10)
		sixMinAgo = now.Add(-time.Minute * 6)
		org       = dbgen.Organization(t, env.DB, database.Organization{})
		user      = dbgen.User(t, env.DB, database.User{})
		file      = dbgen.File(t, env.DB, database.File{})

		// Template import job.
		templateImportJob = dbgen.ProvisionerJob(t, env.DB, env.Pubsub, database.ProvisionerJob{
			CreatedAt: tenMinAgo,
			UpdatedAt: sixMinAgo,
			StartedAt: sql.NullTime{
				Time:  tenMinAgo,
				Valid: true,
			},
			OrganizationID: org.ID,
			InitiatorID:    user.ID,
			Provisioner:    database.ProvisionerTypeEcho,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			Input:          []byte("{}"),
		})
		_ = dbgen.TemplateVersion(t, env.DB, database.TemplateVersion{
			OrganizationID: org.ID,
			JobID:          templateImportJob.ID,
			CreatedBy:      user.ID,
		})
	)

	// Template dry-run job.
	dryRunVersion := dbgen.TemplateVersion(t, env.DB, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	input, err := json.Marshal(provisionerdserver.TemplateVersionDryRunJob{
		TemplateVersionID: dryRunVersion.ID,
	})
	require.NoError(t, err)
	templateDryRunJob := dbgen.ProvisionerJob(t, env.DB, env.Pubsub, database.ProvisionerJob{
		CreatedAt: tenMinAgo,
		UpdatedAt: sixMinAgo,
		StartedAt: sql.NullTime{
			Time:  tenMinAgo,
			Valid: true,
		},
		OrganizationID: org.ID,
		InitiatorID:    user.ID,
		Provisioner:    database.ProvisionerTypeEcho,
		StorageMethod:  database.ProvisionerStorageMethodFile,
		FileID:         file.ID,
		Type:           database.ProvisionerJobTypeTemplateVersionDryRun,
		Input:          input,
	})

	t.Log("template import job ID: ", templateImportJob.ID)
	t.Log("template dry-run job ID: ", templateDryRunJob.ID)

	stats := env.tick(ctx, now)
	require.NoError(t, stats.Error)
	require.Len(t, stats.TerminatedJobIDs, 2)
	require.Contains(t, stats.TerminatedJobIDs, templateImportJob.ID)
	require.Contains(t, stats.TerminatedJobIDs, templateDryRunJob.ID)

	// Check that both jobs were terminated as hung.
	requireTerminatedJob(ctx, t, env.DB, templateImportJob.ID, now, jobreaper.Hung)
	requireTerminatedJob(ctx, t, env.DB, templateDryRunJob.ID, now, jobreaper.Hung)
}

func TestDetectorPendingOtherJobTypes(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	env := newDetectorTestEnv(ctx, t)
	defer env.close()

	var (
		now              = time.Now()
		thirtyFiveMinAgo = now.Add(-time.Minute * 35)
		org              = dbgen.Organization(t, env.DB, database.Organization{})
		user             = dbgen.User(t, env.DB, database.User{})
		file             = dbgen.File(t, env.DB, database.File{})

		// Template import job.
		templateImportJob = dbgen.ProvisionerJob(t, env.DB, env.Pubsub, database.ProvisionerJob{
			CreatedAt: thirtyFiveMinAgo,
			UpdatedAt: thirtyFiveMinAgo,
			StartedAt: sql.NullTime{
				Time:  time.Time{},
				Valid: false,
			},
			OrganizationID: org.ID,
			InitiatorID:    user.ID,
			Provisioner:    database.ProvisionerTypeEcho,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			Input:          []byte("{}"),
		})
		_ = dbgen.TemplateVersion(t, env.DB, database.TemplateVersion{
			OrganizationID: org.ID,
			JobID:          templateImportJob.ID,
			CreatedBy:      user.ID,
		})
	)

	// Template dry-run job.
	dryRunVersion := dbgen.TemplateVersion(t, env.DB, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	input, err := json.Marshal(provisionerdserver.TemplateVersionDryRunJob{
		TemplateVersionID: dryRunVersion.ID,
	})
	require.NoError(t, err)
	templateDryRunJob := dbgen.ProvisionerJob(t, env.DB, env.Pubsub, database.ProvisionerJob{
		CreatedAt: thirtyFiveMinAgo,
		UpdatedAt: thirtyFiveMinAgo,
		StartedAt: sql.NullTime{
			Time:  time.Time{},
			Valid: false,
		},
		OrganizationID: org.ID,
		InitiatorID:    user.ID,
		Provisioner:    database.ProvisionerTypeEcho,
		StorageMethod:  database.ProvisionerStorageMethodFile,
		FileID:         file.ID,
		Type:           database.ProvisionerJobTypeTemplateVersionDryRun,
		Input:          input,
	})

	t.Log("template import job ID: ", templateImportJob.ID)
	t.Log("template dry-run job ID: ", templateDryRunJob.ID)

	stats := env.tick(ctx, now)
	require.NoError(t, stats.Error)
	require.Len(t, stats.TerminatedJobIDs, 2)
	require.Contains(t, stats.TerminatedJobIDs, templateImportJob.ID)
	require.Contains(t, stats.TerminatedJobIDs, templateDryRunJob.ID)

	// Check that both jobs were terminated as pending.
	requireTerminatedJob(ctx, t, env.DB, templateImportJob.ID, now, jobreaper.Pending)
	requireTerminatedJob(ctx, t, env.DB, templateDryRunJob.ID, now, jobreaper.Pending)
}

func TestDetectorHungCanceledJob(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	env := newDetectorTestEnv(ctx, t)
	defer env.close()

	var (
		now       = time.Now()
		tenMinAgo = now.Add(-time.Minute * 10)
		sixMinAgo = now.Add(-time.Minute * 6)
		org       = dbgen.Organization(t, env.DB, database.Organization{})
		user      = dbgen.User(t, env.DB, database.User{})
		file      = dbgen.File(t, env.DB, database.File{})

		// Template import job.
		templateImportJob = dbgen.ProvisionerJob(t, env.DB, env.Pubsub, database.ProvisionerJob{
			CreatedAt: tenMinAgo,
			CanceledAt: sql.NullTime{
				Time:  tenMinAgo,
				Valid: true,
			},
			UpdatedAt: sixMinAgo,
			StartedAt: sql.NullTime{
				Time:  tenMinAgo,
				Valid: true,
			},
			OrganizationID: org.ID,
			InitiatorID:    user.ID,
			Provisioner:    database.ProvisionerTypeEcho,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			Input:          []byte("{}"),
		})
		_ = dbgen.TemplateVersion(t, env.DB, database.TemplateVersion{
			OrganizationID: org.ID,
			JobID:          templateImportJob.ID,
			CreatedBy:      user.ID,
		})
	)

	t.Log("template import job ID: ", templateImportJob.ID)

	stats := env.tick(ctx, now)
	require.NoError(t, stats.Error)
	require.Len(t, stats.TerminatedJobIDs, 1)
	require.Contains(t, stats.TerminatedJobIDs, templateImportJob.ID)

	// Check that the job was updated.
	requireTerminatedJob(ctx, t, env.DB, templateImportJob.ID, now, jobreaper.Hung)
}

func TestDetectorPushesLogs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		preLogCount int
		preLogStage string
		expectStage string
	}{
		{
			name:        "WithExistingLogs",
			preLogCount: 10,
			preLogStage: "Stage Name",
			expectStage: "Stage Name",
		},
		{
			name:        "WithExistingLogsNoStage",
			preLogCount: 10,
			preLogStage: "",
			expectStage: "Unknown",
		},
		{
			name:        "WithoutExistingLogs",
			preLogCount: 0,
			expectStage: "Unknown",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			env := newDetectorTestEnv(ctx, t)
			defer env.close()

			var (
				now       = time.Now()
				tenMinAgo = now.Add(-time.Minute * 10)
				sixMinAgo = now.Add(-time.Minute * 6)
				org       = dbgen.Organization(t, env.DB, database.Organization{})
				user      = dbgen.User(t, env.DB, database.User{})
				file      = dbgen.File(t, env.DB, database.File{})

				// Template import job.
				templateImportJob = dbgen.ProvisionerJob(t, env.DB, env.Pubsub, database.ProvisionerJob{
					CreatedAt: tenMinAgo,
					UpdatedAt: sixMinAgo,
					StartedAt: sql.NullTime{
						Time:  tenMinAgo,
						Valid: true,
					},
					OrganizationID: org.ID,
					InitiatorID:    user.ID,
					Provisioner:    database.ProvisionerTypeEcho,
					StorageMethod:  database.ProvisionerStorageMethodFile,
					FileID:         file.ID,
					Type:           database.ProvisionerJobTypeTemplateVersionImport,
					Input:          []byte("{}"),
				})
				_ = dbgen.TemplateVersion(t, env.DB, database.TemplateVersion{
					OrganizationID: org.ID,
					JobID:          templateImportJob.ID,
					CreatedBy:      user.ID,
				})
			)

			t.Log("template import job ID: ", templateImportJob.ID)

			// Insert some logs at the start of the job.
			if c.preLogCount > 0 {
				insertParams := database.InsertProvisionerJobLogsParams{
					JobID: templateImportJob.ID,
				}
				for i := 0; i < c.preLogCount; i++ {
					insertParams.CreatedAt = append(insertParams.CreatedAt, tenMinAgo.Add(time.Millisecond*time.Duration(i)))
					insertParams.Level = append(insertParams.Level, database.LogLevelInfo)
					insertParams.Stage = append(insertParams.Stage, c.preLogStage)
					insertParams.Source = append(insertParams.Source, database.LogSourceProvisioner)
					insertParams.Output = append(insertParams.Output, fmt.Sprintf("Output %d", i))
				}
				logs, err := env.DB.InsertProvisionerJobLogs(ctx, insertParams)
				require.NoError(t, err)
				require.Len(t, logs, 10)
			}

			// Create pubsub subscription to listen for new log events.
			pubsubCalled := make(chan int64, 1)
			pubsubCancel, err := env.Pubsub.Subscribe(provisionersdk.ProvisionerJobLogsNotifyChannel(templateImportJob.ID), func(ctx context.Context, message []byte) {
				defer close(pubsubCalled)
				var event provisionersdk.ProvisionerJobLogsNotifyMessage
				err := json.Unmarshal(message, &event)
				if !assert.NoError(t, err) {
					return
				}

				assert.True(t, event.EndOfLogs)
				pubsubCalled <- event.CreatedAfter
			})
			require.NoError(t, err)
			defer pubsubCancel()

			stats := env.tick(ctx, now)
			require.NoError(t, stats.Error)
			require.Len(t, stats.TerminatedJobIDs, 1)
			require.Contains(t, stats.TerminatedJobIDs, templateImportJob.ID)

			after := <-pubsubCalled

			// Get the jobs after the given time and check that they are what we
			// expect.
			logs, err := env.DB.GetProvisionerLogsAfterID(ctx, database.GetProvisionerLogsAfterIDParams{
				JobID:        templateImportJob.ID,
				CreatedAfter: after,
			})
			require.NoError(t, err)
			threshold := jobreaper.HungJobDuration
			jobType := jobreaper.Hung
			if templateImportJob.JobStatus == database.ProvisionerJobStatusPending {
				threshold = jobreaper.PendingJobDuration
				jobType = jobreaper.Pending
			}
			expectedLogs := jobreaper.JobLogMessages(jobType, threshold)
			require.Len(t, logs, len(expectedLogs))
			for i, log := range logs {
				assert.Equal(t, database.LogLevelError, log.Level)
				assert.Equal(t, c.expectStage, log.Stage)
				assert.Equal(t, database.LogSourceProvisionerDaemon, log.Source)
				assert.Equal(t, expectedLogs[i], log.Output)
			}

			// Double check the full log count.
			logs, err = env.DB.GetProvisionerLogsAfterID(ctx, database.GetProvisionerLogsAfterIDParams{
				JobID:        templateImportJob.ID,
				CreatedAfter: 0,
			})
			require.NoError(t, err)
			require.Len(t, logs, c.preLogCount+len(expectedLogs))
		})
	}
}

func TestDetectorMaxJobsPerRun(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	env := newDetectorTestEnv(ctx, t)
	defer env.close()

	org := dbgen.Organization(t, env.DB, database.Organization{})
	user := dbgen.User(t, env.DB, database.User{})
	file := dbgen.File(t, env.DB, database.File{})

	// Create MaxJobsPerRun + 1 hung jobs.
	now := time.Now()
	for i := 0; i < jobreaper.MaxJobsPerRun+1; i++ {
		pj := dbgen.ProvisionerJob(t, env.DB, env.Pubsub, database.ProvisionerJob{
			CreatedAt: now.Add(-time.Hour),
			UpdatedAt: now.Add(-time.Hour),
			StartedAt: sql.NullTime{
				Time:  now.Add(-time.Hour),
				Valid: true,
			},
			OrganizationID: org.ID,
			InitiatorID:    user.ID,
			Provisioner:    database.ProvisionerTypeEcho,
			StorageMethod:  database.ProvisionerStorageMethodFile,
			FileID:         file.ID,
			Type:           database.ProvisionerJobTypeTemplateVersionImport,
			Input:          []byte("{}"),
		})
		_ = dbgen.TemplateVersion(t, env.DB, database.TemplateVersion{
			OrganizationID: org.ID,
			JobID:          pj.ID,
			CreatedBy:      user.ID,
		})
	}

	// Make sure that only MaxJobsPerRun jobs are terminated.
	stats := env.tick(ctx, now)
	require.NoError(t, stats.Error)
	require.Len(t, stats.TerminatedJobIDs, jobreaper.MaxJobsPerRun)

	// Run the detector again and make sure that only the remaining job is
	// terminated.
	stats = env.tick(ctx, now)
	require.NoError(t, stats.Error)
	require.Len(t, stats.TerminatedJobIDs, 1)
}

// wrapDBAuthz adds our Authorization/RBAC around the given database store, to
// ensure the reaper has the right permissions to do its work.
func wrapDBAuthz(db database.Store, logger slog.Logger) database.Store {
	return dbauthz.New(
		db,
		rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()),
		logger,
		coderdtest.AccessControlStorePointer(),
	)
}
